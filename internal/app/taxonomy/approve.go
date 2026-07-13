package taxonomy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/gkmz/InkHub/internal/platform/filesystem"
	"gopkg.in/yaml.v3"
)

// TermChange 是需要用户确认的 taxonomy 文件变更。
type TermChange struct {
	Before       string
	After        string
	ExpectedHash string
}

// BuildTermChange 生成新增规范 Tag 的可预览变更，不写文件。
func BuildTermChange(ctx context.Context, path string, term Term) (TermChange, error) {
	if err := ctx.Err(); err != nil {
		return TermChange{}, err
	}
	// Build 阶段只生成用户可审阅的前后内容，不产生文件系统副作用。
	before, err := os.ReadFile(path)
	if err != nil {
		return TermChange{}, fmt.Errorf("读取 taxonomy: %w", err)
	}
	current, err := LoadAuthoritative(ctx, path)
	if err != nil {
		return TermChange{}, err
	}
	name := strings.ToLower(strings.TrimSpace(term.Name))
	if name == "" || current.Tags[name].Name != "" || current.Aliases[name] != "" {
		return TermChange{}, fmt.Errorf("Tag 名称不可用: %s", name)
	}
	for _, rawAlias := range term.Aliases {
		alias := strings.ToLower(strings.TrimSpace(rawAlias))
		if alias == "" || current.Tags[alias].Name != "" || current.Aliases[alias] != "" {
			return TermChange{}, fmt.Errorf("Alias 不可用: %s", alias)
		}
	}
	term.Name = name
	current.Tags[name] = term
	after, err := marshalAuthoritative(current)
	if err != nil {
		return TermChange{}, err
	}
	sum := sha256.Sum256(before)
	return TermChange{Before: string(before), After: string(after), ExpectedHash: hex.EncodeToString(sum[:])}, nil
}

// ApplyTermChange 校验源文件未变化后原子应用 taxonomy 变更。
func ApplyTermChange(ctx context.Context, path string, change TermChange) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Apply 阶段重新验证摘要，防止覆盖 Build 后发生的外部修改。
	current, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取 taxonomy: %w", err)
	}
	sum := sha256.Sum256(current)
	if hex.EncodeToString(sum[:]) != change.ExpectedHash {
		return fmt.Errorf("taxonomy 已在外部修改")
	}
	return filesystem.AtomicWrite(path, []byte(change.After), func(temp string) error {
		_, err := LoadAuthoritative(ctx, temp)
		return err
	})
}

func marshalAuthoritative(value Authoritative) ([]byte, error) {
	raw := taxonomyFile{Version: value.Version, Categories: value.Categories, Series: value.Series}
	names := make([]string, 0, len(value.Tags))
	for name := range value.Tags {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		term := value.Tags[name]
		raw.Tags = append(raw.Tags, termFile{Name: term.Name, Aliases: term.Aliases, Core: term.Core, AllowLowFrequency: term.AllowLowFrequency})
	}
	content, err := yaml.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("编码 taxonomy: %w", err)
	}
	return content, nil
}
