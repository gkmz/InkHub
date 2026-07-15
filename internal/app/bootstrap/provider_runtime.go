package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	domaintemplate "github.com/gkmz/InkHub/internal/domain/template"
	"github.com/gkmz/InkHub/internal/provider/contracts"
	"github.com/gkmz/InkHub/internal/provider/publish/hugo"
	"github.com/gkmz/InkHub/internal/provider/publish/wechat"
	"github.com/gkmz/InkHub/internal/provider/registry"
	"github.com/gkmz/InkHub/internal/provider/source/obsidian"
)

// newProviderRuntime 注册所有内置 Provider；具体实现只在装配层出现。
func newProviderRuntime() (*registry.Registry, error) {
	runtime := registry.New(nil)
	if err := runtime.RegisterSource(obsidian.NewFactory()); err != nil {
		return nil, err
	}
	if err := runtime.RegisterPublish(hugo.NewFactory(hugo.CLIBuilder{})); err != nil {
		return nil, err
	}
	if err := runtime.RegisterPublish(wechat.NewFactory(builtinTemplateLoader{}, nil, unusedClipboard{})); err != nil {
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
	return &contracts.TemplateRef{ID: validated.Manifest.ID, Version: validated.Manifest.Version, Digest: validated.Digest}, nil
}

func templateID(value string) string {
	if value == "minimal" {
		return domaintemplate.BuiltinMinimalID
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
