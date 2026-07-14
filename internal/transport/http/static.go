package httptransport

import (
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

// NewApplicationHandler 组合版本化 API 与 React SPA，API 永远不会回退静态入口。
func NewApplicationHandler(api http.Handler, assets fs.FS) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if strings.HasPrefix(request.URL.Path, "/api/") {
			api.ServeHTTP(response, request)
			return
		}
		serveAsset(response, request, assets)
	})
}

func serveAsset(response http.ResponseWriter, request *http.Request, assets fs.FS) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		http.Error(response, "方法不允许", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(path.Clean("/"+request.URL.Path), "/")
	if name == "" || name == "." {
		name = "index.html"
	}
	content, err := fs.ReadFile(assets, name)
	if err != nil {
		if path.Ext(name) != "" {
			http.NotFound(response, request)
			return
		}
		name = "index.html"
		content, err = fs.ReadFile(assets, name)
	}
	if err != nil {
		http.Error(response, "UI 资源不可用", http.StatusServiceUnavailable)
		return
	}
	if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
		response.Header().Set("Content-Type", contentType)
	}
	response.Header().Set("X-Content-Type-Options", "nosniff")
	response.WriteHeader(http.StatusOK)
	if request.Method != http.MethodHead {
		_, _ = response.Write(content)
	}
}
