// Package disposition 编排文章批量处置命令。
package disposition

import (
	"context"
	"errors"
	"sort"
	"strings"
)

// Operation 是批量处置操作。
type Operation string

const (
	// OperationPublished 将当前文章版本标记为外部已发表。
	OperationPublished Operation = "published"
	// OperationIgnored 将文章长期排除在日常管理之外。
	OperationIgnored Operation = "ignored"
	// OperationRestore 恢复被忽略文章的日常管理。
	OperationRestore Operation = "restore"
)

var (
	// ErrInvalidCommand 表示批量处置输入无效。
	ErrInvalidCommand = errors.New("文章处置请求无效")
	// ErrContentChanged 表示文章已经不是用户选择时看到的版本。
	ErrContentChanged = errors.New("文章内容已变化")
	// ErrArticleNotFound 表示文章不属于当前工作区或已删除。
	ErrArticleNotFound = errors.New("文章不存在")
	// ErrChannelUnavailable 表示所选发布渠道未配置或未启用。
	ErrChannelUnavailable = errors.New("发布渠道未配置")
)

// ArticleVersion 是用户选择文章时看到的稳定身份与内容版本。
type ArticleVersion struct {
	ID             string
	ContentVersion string
}

// Command 描述一次文章批量处置请求。
type Command struct {
	Operation Operation
	Articles  []ArticleVersion
	Channels  []string
}

// Result 汇总一次批量处置的实际结果。
type Result struct {
	Processed int
	Changed   int
	Unchanged int
}

// Store 在单个持久化事务中执行已经规范化的处置命令。
type Store interface {
	Apply(ctx context.Context, command Command) (Result, error)
}

// Service 校验并规范化文章批量处置命令。
type Service struct {
	store Store
}

// NewService 创建文章批量处置服务。
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Apply 校验并规范化批量命令后交给持久化事务执行。
func (s *Service) Apply(ctx context.Context, command Command) (Result, error) {
	if s == nil || s.store == nil {
		return Result{}, ErrInvalidCommand
	}
	normalized, err := normalize(command)
	if err != nil {
		return Result{}, err
	}
	return s.store.Apply(ctx, normalized)
}

func normalize(command Command) (Command, error) {
	if len(command.Articles) == 0 || len(command.Articles) > 100 {
		return Command{}, ErrInvalidCommand
	}
	versions := make(map[string]string, len(command.Articles))
	for _, value := range command.Articles {
		id, version := strings.TrimSpace(value.ID), strings.TrimSpace(value.ContentVersion)
		if id == "" || version == "" {
			return Command{}, ErrInvalidCommand
		}
		if existing, found := versions[id]; found && existing != version {
			return Command{}, ErrInvalidCommand
		}
		versions[id] = version
	}
	articles := make([]ArticleVersion, 0, len(versions))
	for id, version := range versions {
		articles = append(articles, ArticleVersion{ID: id, ContentVersion: version})
	}
	sort.Slice(articles, func(i, j int) bool { return articles[i].ID < articles[j].ID })

	channels, err := normalizeChannels(command.Operation, command.Channels)
	if err != nil {
		return Command{}, err
	}
	return Command{Operation: command.Operation, Articles: articles, Channels: channels}, nil
}

func normalizeChannels(operation Operation, values []string) ([]string, error) {
	if operation != OperationPublished && operation != OperationIgnored && operation != OperationRestore {
		return nil, ErrInvalidCommand
	}
	if operation != OperationPublished {
		if len(values) != 0 {
			return nil, ErrInvalidCommand
		}
		return nil, nil
	}
	channels := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "hugo" && value != "wechat" {
			return nil, ErrInvalidCommand
		}
		channels[value] = true
	}
	if len(channels) == 0 {
		return nil, ErrInvalidCommand
	}
	result := make([]string, 0, len(channels))
	for value := range channels {
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}
