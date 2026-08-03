package editorial

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gkmz/InkHub/internal/domain/article"
	domaineditorial "github.com/gkmz/InkHub/internal/domain/editorial"
	"github.com/gkmz/InkHub/internal/provider/contracts"
)

var (
	// ErrSuggestionAlreadyAccepted 表示同一字段建议不能重复应用。
	ErrSuggestionAlreadyAccepted = errors.New("AI 建议已经采纳")
)

// MetadataWriter 以乐观并发方式写回源文章元数据。
type MetadataWriter interface {
	WriteMetadata(ctx context.Context, command contracts.MetadataWriteCommand) (contracts.SourceDocument, error)
}

// AcceptSuggestion 校验文章版本并只应用指定的一条字段建议。
func AcceptSuggestion(
	ctx context.Context,
	writer MetadataWriter,
	store SuggestionStore,
	current article.Article,
	record SuggestionSet,
	itemID string,
) (SuggestionSet, error) {
	if writer == nil || store == nil {
		return SuggestionSet{}, fmt.Errorf("采纳 AI 建议所需依赖不完整")
	}
	if record.ArticleID != current.ID || record.InputContentHash != current.ContentHash {
		return SuggestionSet{}, ErrStaleSuggestion
	}
	index := -1
	for candidateIndex := range record.Items {
		if record.Items[candidateIndex].ID == itemID {
			index = candidateIndex
			break
		}
	}
	if index < 0 {
		return SuggestionSet{}, fmt.Errorf("找不到 AI 建议项: %s", itemID)
	}
	item := record.Items[index]
	if item.Accepted {
		return SuggestionSet{}, ErrSuggestionAlreadyAccepted
	}
	patch, err := buildMetadataPatch(current, item)
	if err != nil {
		return SuggestionSet{}, err
	}
	_, err = writer.WriteMetadata(ctx, contracts.MetadataWriteCommand{
		Ref: contracts.SourceRef{
			SourceID: current.SourceID, RelativePath: current.RelativePath, StableID: string(current.StableID),
		},
		ExpectedFingerprint: current.SourceFingerprint,
		Patch:               patch,
	})
	if err != nil {
		return SuggestionSet{}, fmt.Errorf("写回 AI 建议: %w", err)
	}

	// 只有源文件原子写回成功后才推进建议状态，避免数据库显示已采纳但文件未变化。
	record.Items = append([]SuggestionItem(nil), record.Items...)
	record.Items[index].Accepted = true
	record.State = deriveSuggestionState(record.Items)
	if err := store.Save(ctx, record); err != nil {
		return SuggestionSet{}, fmt.Errorf("保存 AI 建议状态: %w", err)
	}
	return record, nil
}

func buildMetadataPatch(current article.Article, item SuggestionItem) (contracts.MetadataPatch, error) {
	var patch contracts.MetadataPatch
	switch item.Field {
	case "description":
		value, err := decodeString(item)
		patch.Description = &value
		return patch, err
	case "category":
		value, err := decodeString(item)
		patch.Category = &value
		return patch, err
	case "series":
		value, err := decodeString(item)
		patch.Series = &value
		return patch, err
	case "slug":
		value, err := decodeString(item)
		patch.Slug = &value
		return patch, err
	case "keywords":
		var value []string
		if err := json.Unmarshal(item.Value, &value); err != nil {
			return patch, fmt.Errorf("%w: keywords 必须是字符串数组", ErrInvalidSuggestion)
		}
		value = cleanStrings(value)
		patch.Keywords = &value
		return patch, nil
	case "tags":
		var value string
		if err := json.Unmarshal(item.Value, &value); err != nil || value == "" {
			return patch, fmt.Errorf("%w: tag 必须是非空字符串", ErrInvalidSuggestion)
		}
		valueList := cleanStrings(append(append([]string(nil), current.Tags...), value))
		patch.Tags = &valueList
		return patch, nil
	default:
		return patch, fmt.Errorf("%w: 不支持字段 %s", ErrInvalidSuggestion, item.Field)
	}
}

func decodeString(item SuggestionItem) (string, error) {
	var value string
	if err := json.Unmarshal(item.Value, &value); err != nil {
		return "", fmt.Errorf("%w: 字段 %s 必须是字符串", ErrInvalidSuggestion, item.Field)
	}
	return value, nil
}

func deriveSuggestionState(items []SuggestionItem) SuggestionState {
	return domaineditorial.DeriveSuggestionState(items)
}
