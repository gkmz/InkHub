package obsidian

import (
	"context"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

// Watch 使用共享 Folder Source 监听 Markdown 文件变化。
func (p *Provider) Watch(ctx context.Context, changes chan<- contracts.SourceChange) error {
	return p.folder.Watch(ctx, changes)
}
