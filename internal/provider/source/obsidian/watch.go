package obsidian

import (
	"context"
	"time"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

// Watch 使用共享 Folder Source 监听 Markdown 文件变化。
func (p *Provider) Watch(ctx context.Context, changes chan<- contracts.SourceChange) error {
	// Folder Source 只监听 Markdown；这里额外轮询 app.json，保证附件设置变化触发全量重扫。
	folderChanges := make(chan contracts.SourceChange)
	folderErrors := make(chan error, 1)
	go func() { folderErrors <- p.folder.Watch(ctx, folderChanges) }()
	settings, _ := readObsidianSettings(p.config.Root)
	previousFingerprint := settings.Fingerprint
	ticker := time.NewTicker(p.config.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-folderErrors:
			return err
		case change := <-folderChanges:
			select {
			case changes <- change:
			case <-ctx.Done():
				return ctx.Err()
			}
		case <-ticker.C:
			current, _ := readObsidianSettings(p.config.Root)
			if current.Fingerprint == previousFingerprint {
				continue
			}
			previousFingerprint = current.Fingerprint
			select {
			case changes <- contracts.SourceChange{Kind: contracts.SourceRescanRequired, Ref: contracts.SourceRef{SourceID: p.config.SourceID}}:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}
