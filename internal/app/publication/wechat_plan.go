package publication

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/gkmz/InkHub/internal/domain/article"
	"github.com/gkmz/InkHub/internal/provider/contracts"
)

var (
	// ErrWeChatPlanInvalid 表示准备计划已过期、被篡改或不再对应当前文章。
	ErrWeChatPlanInvalid = errors.New("微信准备计划无效")
)

// WeChatPlanArticle 是 Resolver 提供的当前微信发布快照。
type WeChatPlanArticle struct {
	WorkspaceID      string
	ArticleID        string
	ProviderID       string
	ContentHash      string
	ContentStage     article.ContentStage
	TemplateID       string
	TemplateRevision string
	Input            contracts.PublishInput
	Provider         contracts.PublishProvider
}

// WeChatPlanResolver 只解析当前工作区可访问的微信发布快照。
type WeChatPlanResolver interface {
	ResolveWeChatPlan(ctx context.Context, articleID, templateID string) (WeChatPlanArticle, error)
}

// WeChatPlanView 是确认前可安全展示的只读计划。
type WeChatPlanView struct {
	Token       string
	TemplateID  string
	Images      []contracts.AssetPlanItem
	Diagnostics []contracts.Diagnostic
	Ready       bool
	ExpiresAt   time.Time
}

// WeChatPlanService 生成短期签名计划，并在确认后创建确定性任务。
type WeChatPlanService struct {
	resolver WeChatPlanResolver
	queue    JobQueue
	key      []byte
	now      func() time.Time
}

type wechatPlanToken struct {
	WorkspaceID      string                    `json:"workspace_id"`
	ArticleID        string                    `json:"article_id"`
	ProviderID       string                    `json:"provider_id"`
	ContentHash      string                    `json:"content_hash"`
	TemplateID       string                    `json:"template_id"`
	TemplateRevision string                    `json:"template_revision"`
	Images           []contracts.AssetPlanItem `json:"images"`
	ExpiresAt        time.Time                 `json:"expires_at"`
}

// NewWeChatPlanService 创建微信准备计划服务。
func NewWeChatPlanService(resolver WeChatPlanResolver, queue JobQueue, key []byte, now func() time.Time) (*WeChatPlanService, error) {
	if resolver == nil || queue == nil || len(key) < 32 {
		return nil, fmt.Errorf("微信准备计划依赖不完整")
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &WeChatPlanService{resolver: resolver, queue: queue, key: append([]byte(nil), key...), now: now}, nil
}

// Plan 只读检查模板和图片，不调用 Prepare 或 Upload。
func (s *WeChatPlanService) Plan(ctx context.Context, articleID, templateID string) (WeChatPlanView, error) {
	value, err := s.resolver.ResolveWeChatPlan(ctx, articleID, templateID)
	if err != nil {
		return WeChatPlanView{}, err
	}
	if value.ContentStage != article.ContentStageReady {
		return WeChatPlanView{}, ErrArticleNotReady
	}
	planner, ok := value.Provider.(contracts.AssetPlanningProvider)
	if !ok {
		return WeChatPlanView{}, fmt.Errorf("微信 Provider 不支持图片规划")
	}
	images, diagnostics, err := planner.InspectAssets(ctx, value.Input)
	if err != nil {
		return WeChatPlanView{}, err
	}
	ready := true
	for _, diagnostic := range diagnostics {
		if diagnostic.Blocking {
			ready = false
		}
	}
	expiresAt := s.now().UTC().Add(10 * time.Minute)
	payload := wechatPlanToken{
		WorkspaceID: value.WorkspaceID, ArticleID: value.ArticleID, ProviderID: value.ProviderID,
		ContentHash: value.ContentHash, TemplateID: value.TemplateID, TemplateRevision: value.TemplateRevision,
		Images: images, ExpiresAt: expiresAt,
	}
	token, err := s.sign(payload)
	if err != nil {
		return WeChatPlanView{}, err
	}
	return WeChatPlanView{Token: token, TemplateID: value.TemplateID, Images: images, Diagnostics: diagnostics, Ready: ready, ExpiresAt: expiresAt}, nil
}

// Confirm 重新解析当前快照，校验计划未变化后创建微信准备任务。
func (s *WeChatPlanService) Confirm(ctx context.Context, articleID, token string) (string, error) {
	payload, err := s.verify(token)
	if err != nil || payload.ArticleID != articleID || !s.now().UTC().Before(payload.ExpiresAt) {
		return "", ErrWeChatPlanInvalid
	}
	current, err := s.resolver.ResolveWeChatPlan(ctx, articleID, payload.TemplateID)
	if err != nil {
		return "", err
	}
	if current.ContentStage != article.ContentStageReady {
		return "", ErrArticleNotReady
	}
	planner, ok := current.Provider.(contracts.AssetPlanningProvider)
	if !ok {
		return "", ErrWeChatPlanInvalid
	}
	images, diagnostics, err := planner.InspectAssets(ctx, current.Input)
	if err != nil {
		return "", err
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.Blocking {
			return "", ErrWeChatPlanInvalid
		}
	}
	if current.WorkspaceID != payload.WorkspaceID || current.ProviderID != payload.ProviderID || current.ContentHash != payload.ContentHash || current.TemplateRevision != payload.TemplateRevision || !equalPlanImages(images, payload.Images) {
		return "", ErrWeChatPlanInvalid
	}
	sum := sha256.Sum256([]byte("wechat_prepare\x00" + current.WorkspaceID + "\x00" + current.ArticleID + "\x00" + current.ProviderID + "\x00" + current.ContentHash + "\x00" + current.TemplateRevision))
	jobID := "wechat_" + hex.EncodeToString(sum[:12])
	return s.queue.Enqueue(ctx, JobIntent{ID: jobID, WorkspaceID: current.WorkspaceID, Kind: "wechat_prepare", ArticleID: current.ArticleID, ProviderInstanceID: current.ProviderID, ContentHash: current.ContentHash})
}

func (s *WeChatPlanService) sign(payload wechatPlanToken) (string, error) {
	content, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	aead, err := s.aead()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := aead.Seal(nonce, nonce, content, nil)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (s *WeChatPlanService) verify(token string) (wechatPlanToken, error) {
	var payload wechatPlanToken
	sealed, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return payload, ErrWeChatPlanInvalid
	}
	aead, err := s.aead()
	if err != nil {
		return payload, ErrWeChatPlanInvalid
	}
	if len(sealed) < aead.NonceSize() {
		return payload, ErrWeChatPlanInvalid
	}
	content, err := aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], nil)
	if err != nil || json.Unmarshal(content, &payload) != nil {
		return wechatPlanToken{}, ErrWeChatPlanInvalid
	}
	return payload, nil
}

func (s *WeChatPlanService) aead() (cipher.AEAD, error) {
	block, err := aes.NewCipher(s.key[:32])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func equalPlanImages(left, right []contracts.AssetPlanItem) bool {
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	return hmac.Equal(leftJSON, rightJSON)
}
