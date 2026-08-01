package obsidian

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

// Scan 扫描 Vault 中全部 Markdown 文件并返回稳定排序结果。
func (p *Provider) Scan(ctx context.Context, cursor contracts.ScanCursor) (contracts.ScanResult, error) {
	paths, err := p.folder.MarkdownPaths(ctx)
	if err != nil {
		return contracts.ScanResult{}, fmt.Errorf("扫描 Vault: %w", err)
	}

	result := contracts.ScanResult{Documents: make([]contracts.SourceDocumentRef, 0, len(paths)), Complete: true}
	revision := sha256.New()
	// app.json 会影响无路径附件的解释，必须参与扫描版本计算。
	if settings, _ := readObsidianSettings(p.config.Root); settings.Fingerprint != "" {
		_, _ = revision.Write([]byte(settings.Fingerprint))
	}
	stableIndexes := make(map[string][]int)
	for _, relative := range paths {
		document, err := p.Read(ctx, contracts.SourceRef{SourceID: p.config.SourceID, RelativePath: relative})
		if err != nil {
			fingerprint := p.rawFingerprint(relative)
			result.Documents = append(result.Documents, contracts.SourceDocumentRef{
				Ref: contracts.SourceRef{SourceID: p.config.SourceID, RelativePath: relative}, Fingerprint: fingerprint,
				Diagnostics: []contracts.Diagnostic{{Code: "obsidian.read_failed", Message: err.Error(), Blocking: true}},
			})
			_, _ = revision.Write([]byte(relative))
			_, _ = revision.Write([]byte(fingerprint))
			continue
		}
		index := len(result.Documents)
		result.Documents = append(result.Documents, contracts.SourceDocumentRef{
			Ref: document.Ref, Fingerprint: document.Fingerprint, Diagnostics: document.Diagnostics,
		})
		if document.Ref.StableID != "" {
			stableIndexes[document.Ref.StableID] = append(stableIndexes[document.Ref.StableID], index)
		}
		_, _ = revision.Write([]byte(relative))
		_, _ = revision.Write([]byte(document.Fingerprint))
	}

	for stableID, indexes := range stableIndexes {
		if len(indexes) < 2 {
			continue
		}
		for _, index := range indexes {
			result.Documents[index].Diagnostics = append(result.Documents[index].Diagnostics, contracts.Diagnostic{
				Code: "obsidian.duplicate_stable_id", Message: "稳定文章 ID 重复: " + stableID, Blocking: true,
			})
		}
	}
	result.Next.Revision = hex.EncodeToString(revision.Sum(nil))
	if cursor.Revision != "" && cursor.Revision == result.Next.Revision {
		result.Documents = nil
	}
	return result, nil
}

func (p *Provider) rawFingerprint(relative string) string {
	path, err := p.resolve(relative)
	if err != nil {
		return ""
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
