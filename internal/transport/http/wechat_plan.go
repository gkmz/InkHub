package httptransport

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gkmz/InkHub/internal/app/publication"
)

// WeChatPlanAPI 提供文章级只读图片计划和安全确认。
type WeChatPlanAPI interface {
	Plan(ctx context.Context, articleID, templateID, mermaidTheme string) (publication.WeChatPlanView, error)
	Confirm(ctx context.Context, articleID, token string) (string, error)
}

func (h *runtimeHandler) createWeChatPlan(response http.ResponseWriter, request *http.Request) {
	if h.wechatPlans == nil {
		writeError(response, http.StatusNotImplemented, "wechat.plan_unavailable", "微信准备计划暂不可用")
		return
	}
	articleID := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, "/api/v1/articles/"), "/wechat-plans")
	var input struct {
		TemplateID   string `json:"template_id"`
		MermaidTheme string `json:"mermaid_theme"`
	}
	if articleID == "" || decodeJSON(request, &input) != nil {
		writeError(response, http.StatusBadRequest, "request.invalid", "微信准备请求无效")
		return
	}
	input.MermaidTheme = strings.ToLower(strings.TrimSpace(input.MermaidTheme))
	if input.MermaidTheme == "" {
		input.MermaidTheme = "handdrawn"
	}
	if input.MermaidTheme != "handdrawn" && input.MermaidTheme != "modern" {
		writeError(response, http.StatusBadRequest, "wechat.mermaid_theme_invalid", "Mermaid 样式无效")
		return
	}
	plan, err := h.wechatPlans.Plan(request.Context(), articleID, input.TemplateID, input.MermaidTheme)
	if err != nil {
		if errors.Is(err, publication.ErrArticleNotReady) {
			mapError(response, ErrArticleNotReady)
			return
		}
		mapError(response, err)
		return
	}
	images := make([]map[string]any, 0, len(plan.Images))
	for _, item := range plan.Images {
		images = append(images, map[string]any{"reference": item.Reference, "media_type": item.MediaType, "size": item.Size, "state": item.State})
	}
	diagnostics := make([]map[string]any, 0, len(plan.Diagnostics))
	for _, item := range plan.Diagnostics {
		diagnostics = append(diagnostics, map[string]any{"code": item.Code, "message": item.Message, "blocking": item.Blocking})
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"plan_token": plan.Token, "template_id": plan.TemplateID, "mermaid_theme": plan.MermaidTheme, "images": images,
		"diagnostics": diagnostics, "ready": plan.Ready, "expires_at": plan.ExpiresAt,
	})
}

func (h *runtimeHandler) confirmWeChatPlan(response http.ResponseWriter, request *http.Request) {
	if h.wechatPlans == nil {
		writeError(response, http.StatusNotImplemented, "wechat.plan_unavailable", "微信准备计划暂不可用")
		return
	}
	articleID := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, "/api/v1/articles/"), "/wechat-plans/confirm")
	var input struct {
		Token string `json:"plan_token"`
	}
	if articleID == "" || decodeJSON(request, &input) != nil || input.Token == "" {
		writeError(response, http.StatusBadRequest, "request.plan_invalid", "微信准备计划无效，请重新生成")
		return
	}
	if _, err := h.wechatPlans.Confirm(request.Context(), articleID, input.Token); err != nil {
		if errors.Is(err, publication.ErrWeChatPlanInvalid) {
			writeError(response, http.StatusBadRequest, "request.plan_invalid", "微信准备计划已失效，请重新生成")
			return
		}
		if errors.Is(err, publication.ErrArticleNotReady) {
			mapError(response, ErrArticleNotReady)
			return
		}
		mapError(response, err)
		return
	}
	writeJSON(response, http.StatusAccepted, map[string]string{"state": "queued"})
}
