package contracts

import "context"

// AssetUploadRequest 描述经过渠道校验、可安全上传的本地资源。
type AssetUploadRequest struct {
	LocalPath string
	Digest    string
	MediaType string
	Extension string
}

// AssetUploadResult 是资源上传或复用后的公开地址。
type AssetUploadResult struct {
	URL    string
	Reused bool
}

// AssetPlanItem 是可安全展示的文章资源准备项。
type AssetPlanItem struct {
	Reference string
	MediaType string
	Size      int64
	State     string
}

// AssetPlanningProvider 在外部写入前只读检查渠道资源。
type AssetPlanningProvider interface {
	InspectAssets(ctx context.Context, input PublishInput) ([]AssetPlanItem, []Diagnostic, error)
}
