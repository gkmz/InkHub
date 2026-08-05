package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	domaintemplate "github.com/gkmz/InkHub/internal/domain/template"
	"github.com/gkmz/InkHub/internal/platform/githubassets"
	openai "github.com/gkmz/InkHub/internal/provider/ai/openai"
	"github.com/gkmz/InkHub/internal/provider/contracts"
	"github.com/gkmz/InkHub/internal/provider/publish/hugo"
	"github.com/gkmz/InkHub/internal/provider/publish/wechat"
	"github.com/gkmz/InkHub/internal/provider/registry"
	"github.com/gkmz/InkHub/internal/provider/source/obsidian"
	taxonomyhugo "github.com/gkmz/InkHub/internal/provider/taxonomy/hugo"
	"go.uber.org/zap"
)

type secretGetter interface {
	Get(context.Context, string) (string, error)
}

// secretStoreResolver 将系统 Secret Store 适配为 Provider 只读解析器。
type secretStoreResolver struct{ store secretGetter }

func (r secretStoreResolver) Resolve(ctx context.Context, ref string) (contracts.SecretValue, error) {
	value, err := r.store.Get(ctx, ref)
	if err != nil {
		return contracts.SecretValue{}, err
	}
	return contracts.SecretValue{Bytes: []byte(value)}, nil
}

// newProviderRuntime 注册所有内置 Provider；具体实现只在装配层出现。
func newProviderRuntime(resolvers ...contracts.SecretResolver) (*registry.Registry, error) {
	return newProviderRuntimeWithLogger(zap.NewNop(), resolvers...)
}

// newProviderRuntimeWithLogger 注册内置 Provider，并把运行期日志器传给需要记录外部边界的实现。
func newProviderRuntimeWithLogger(logger *zap.Logger, resolvers ...contracts.SecretResolver) (*registry.Registry, error) {
	if logger == nil {
		logger = zap.NewNop()
	}
	var resolver contracts.SecretResolver
	if len(resolvers) > 0 {
		resolver = resolvers[0]
	}
	runtime := registry.New(resolver)
	if err := runtime.RegisterAI(openai.NewFactory(nil)); err != nil {
		return nil, err
	}
	if err := runtime.RegisterSource(obsidian.NewFactory()); err != nil {
		return nil, err
	}
	if err := runtime.RegisterPublish(hugo.NewFactory(hugo.CLIBuilder{})); err != nil {
		return nil, err
	}
	if err := runtime.RegisterTaxonomy(taxonomyhugo.NewFactory()); err != nil {
		return nil, err
	}
	githubClient := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if len(via) > 0 && request.URL.Host != via[0].URL.Host {
			return fmt.Errorf("GitHub 请求禁止跨域跳转")
		}
		return nil
	}}
	uploaderBuilder := func(_ context.Context, config wechat.AssetUploaderConfig, token []byte) (wechat.AssetUploader, error) {
		return githubassets.New(githubassets.Config{
			Owner: config.Owner, Repository: config.Repository, Branch: config.Branch, Prefix: config.Prefix, Token: string(token),
		}, githubClient, logger)
	}
	if err := runtime.RegisterPublish(wechat.NewFactoryWithUploaderBuilder(builtinTemplateLoader{}, unusedClipboard{}, uploaderBuilder, wechat.NewMermaidInkRenderer())); err != nil {
		return nil, err
	}
	return runtime, nil
}

// providerConfigView 将持久化 JSON 转为 Provider 可校验的配置与授权路径。
func providerConfigView(data []byte, fixedRoots ...string) (contracts.ConfigView, error) {
	var raw map[string]any
	if len(data) == 0 || strings.TrimSpace(string(data)) == "" {
		raw = map[string]any{}
	} else if err := json.Unmarshal(data, &raw); err != nil {
		return contracts.ConfigView{}, fmt.Errorf("解析 Provider 配置: %w", err)
	}
	roots := append([]string{}, fixedRoots...)
	for key, value := range raw {
		if key != "root" && !strings.HasSuffix(key, "_root") {
			continue
		}
		if path, ok := value.(string); ok && path != "" {
			roots = append(roots, path)
		}
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return contracts.ConfigView{}, fmt.Errorf("编码 Provider 配置: %w", err)
	}
	return contracts.ConfigView{Data: encoded, AllowedRoots: roots}, nil
}

// sourceConfigView 将 sources.root_path 合并进 Provider 自有配置。
func sourceConfigView(root string, data []byte) (contracts.ConfigView, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return contracts.ConfigView{}, fmt.Errorf("解析 Source 配置: %w", err)
	}
	raw["root"] = root
	encoded, err := json.Marshal(raw)
	if err != nil {
		return contracts.ConfigView{}, fmt.Errorf("编码 Source 配置: %w", err)
	}
	return contracts.ConfigView{Data: encoded, AllowedRoots: []string{root}}, nil
}

func configuredTemplate(data []byte) (*contracts.TemplateRef, error) {
	var raw struct {
		Template string `json:"template"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("解析模板配置: %w", err)
	}
	if raw.Template == "" {
		return nil, nil
	}
	validated, err := domaintemplate.Builtin(templateID(raw.Template))
	if err != nil {
		return nil, err
	}
	return &contracts.TemplateRef{ID: validated.Manifest.ID, Version: validated.Manifest.Version, Digest: validated.Digest, Target: validated.Manifest.Target}, nil
}

func templateID(value string) string {
	if value == "minimal" {
		return domaintemplate.BuiltinMinimalID
	}
	if value == "classic" {
		return domaintemplate.BuiltinClassicID
	}
	return domaintemplate.BuiltinDefaultID
}

type builtinTemplateLoader struct{}

func (builtinTemplateLoader) Load(_ context.Context, ref contracts.TemplateRef) (domaintemplate.Validated, error) {
	return domaintemplate.Builtin(ref.ID)
}

type unusedClipboard struct{}

func (unusedClipboard) CopyHTML(context.Context, string) error {
	return fmt.Errorf("后台任务不允许写入剪贴板")
}
