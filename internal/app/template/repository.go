package template

import (
	"context"
	"fmt"
	"path/filepath"

	domaintemplate "github.com/gkmz/InkHub/internal/domain/template"
	"github.com/gkmz/InkHub/internal/provider/contracts"
)

// Loader 从内置模板或不可变安装目录读取指定版本。
type Loader struct {
	InstallRoot string
}

// Load 实现微信 Provider 的 TemplateLoader 端口。
func (l Loader) Load(ctx context.Context, ref contracts.TemplateRef) (domaintemplate.Validated, error) {
	if err := ctx.Err(); err != nil {
		return domaintemplate.Validated{}, err
	}
	if ref.ID == domaintemplate.BuiltinDefaultID || ref.ID == domaintemplate.BuiltinMinimalID {
		value, err := domaintemplate.Builtin(ref.ID)
		if err != nil {
			return domaintemplate.Validated{}, err
		}
		if ref.Version != "" && ref.Version != value.Manifest.Version {
			return domaintemplate.Validated{}, fmt.Errorf("内置模板版本不存在: %s", ref.Version)
		}
		return value, nil
	}
	if ref.ID == "" || ref.Version == "" {
		return domaintemplate.Validated{}, fmt.Errorf("模板 ID 或版本为空")
	}
	value, err := domaintemplate.ValidateDirectory(filepath.Join(l.InstallRoot, ref.ID, ref.Version))
	if err != nil {
		return domaintemplate.Validated{}, fmt.Errorf("加载已安装模板: %w", err)
	}
	if ref.Digest != "" && value.Digest != ref.Digest {
		return domaintemplate.Validated{}, fmt.Errorf("已安装模板摘要不匹配")
	}
	return value, nil
}
