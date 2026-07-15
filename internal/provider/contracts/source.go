// Package contracts 定义 Provider 之间共享的稳定输入输出类型。
package contracts

import (
	"context"

	"github.com/gkmz/InkHub/internal/domain/article"
)

// SourceProvider 负责发现、读取、监听和受控写回内容源。
type SourceProvider interface {
	Descriptor() SourceDescriptor
	Validate(ctx context.Context) error
	Scan(ctx context.Context, cursor ScanCursor) (ScanResult, error)
	Read(ctx context.Context, ref SourceRef) (SourceDocument, error)
	WriteMetadata(ctx context.Context, command MetadataWriteCommand) (SourceDocument, error)
	Watch(ctx context.Context, changes chan<- SourceChange) error
}

// SourceDescriptor 描述内容源支持的格式与通用能力。
type SourceDescriptor struct {
	Descriptor
	Formats []string
}

// SourceProviderFactory 构建一个类型安全的 Source Provider 实例。
type SourceProviderFactory interface {
	Type() ProviderType
	Descriptor() SourceDescriptor
	Build(ctx context.Context, ref ProviderRef, config ConfigView, secrets SecretResolver) (SourceProvider, error)
}

// SourceRef 定位内容源中的一篇文章。
type SourceRef struct {
	SourceID     string
	RelativePath string
	StableID     string
}

// SourceDocument 是 Source Provider 解析后的文章文档。
type SourceDocument struct {
	Ref            SourceRef
	Article        article.Article
	Body           string
	RawFrontmatter string
	Fingerprint    string
	ResourceRefs   []ResourceRef
	Diagnostics    []Diagnostic
}

// ResourceRef 描述正文引用的本地或远程资源。
type ResourceRef struct {
	Original string
	Resolved string
	Kind     string
}

// ResourceResolver 是 Source Provider 可选实现的本地/远程资源解析能力。
type ResourceResolver interface {
	ResolveResource(ctx context.Context, ref SourceRef, raw string, kind ResourceKind) (ResolvedResource, error)
}

// ResourceKind 描述 Source 方言中的资源引用语义。
type ResourceKind string

const (
	ResourceMarkdownImage ResourceKind = "markdown"
	ResourceWikiEmbed     ResourceKind = "wiki"
)

// ResolvedResource 是经过 Source 授权边界校验的资源定位结果。
type ResolvedResource struct {
	RelativePath string
	AbsolutePath string
	RemoteURL    string
}

// Diagnostic 描述解析或扫描发现的问题。
type Diagnostic struct {
	Code     string
	Message  string
	Blocking bool
}

// MetadataPatch 使用指针区分不修改和写入空值。
type MetadataPatch struct {
	StableID    *string
	Title       *string
	Description *string
	Tags        *[]string
	Keywords    *[]string
	Category    *string
	Series      *string
	Slug        *string
	Cover       *string
}

// MetadataWriteCommand 描述一次带乐观并发校验的元数据写回。
type MetadataWriteCommand struct {
	Ref                 SourceRef
	ExpectedFingerprint string
	Patch               MetadataPatch
}

// SourceDocumentRef 是扫描阶段返回的轻量文章引用。
type SourceDocumentRef struct {
	Ref         SourceRef
	Fingerprint string
	Diagnostics []Diagnostic
}

// ScanCursor 标识上一次完整扫描修订。
type ScanCursor struct {
	Revision string
}

// ScanResult 保存一次完整或增量扫描结果。
type ScanResult struct {
	Documents []SourceDocumentRef
	Next      ScanCursor
	Complete  bool
}

// SourceChangeKind 是内容源文件变化类型。
type SourceChangeKind string

const (
	SourceCreated        SourceChangeKind = "created"
	SourceModified       SourceChangeKind = "modified"
	SourceDeleted        SourceChangeKind = "deleted"
	SourceRescanRequired SourceChangeKind = "rescan_required"
)

// SourceChange 描述一次需要重新扫描的源文件变化。
type SourceChange struct {
	Kind SourceChangeKind
	Ref  SourceRef
}
