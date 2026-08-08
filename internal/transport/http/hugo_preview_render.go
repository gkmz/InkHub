package httptransport

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/gkmz/InkHub/internal/app/publication"
	"golang.org/x/net/html"
)

const hugoPreviewRoutePrefix = "/api/v1/hugo-previews/"

var cssRootReferencePattern = regexp.MustCompile(`(?i)(url\(\s*["']?)(/[^/"')][^"')]*)(["']?\s*\))`)

func isHugoPreviewRenderPath(requestPath string) bool {
	_, _, ok := parseHugoPreviewRenderPath(requestPath)
	return ok
}

func parseHugoPreviewRenderPath(requestPath string) (string, string, bool) {
	if !strings.HasPrefix(requestPath, hugoPreviewRoutePrefix) {
		return "", "", false
	}
	remainder := strings.TrimPrefix(requestPath, hugoPreviewRoutePrefix)
	marker := "/render"
	index := strings.Index(remainder, marker)
	if index <= 0 || (len(remainder) > index+len(marker) && remainder[index+len(marker)] != '/') {
		return "", "", false
	}
	previewID := remainder[:index]
	resource := strings.TrimPrefix(remainder[index+len(marker):], "/")
	if previewID == "" || strings.Contains(previewID, "/") {
		return "", "", false
	}
	return previewID, resource, true
}

func hugoPreviewRenderRoot(previewID string) string {
	return hugoPreviewRoutePrefix + url.PathEscape(previewID) + "/render"
}

func hugoPreviewRenderURL(previewID, renderPath string) string {
	if previewID == "" || renderPath == "" {
		return ""
	}
	value := strings.TrimSuffix(filepathURL(renderPath), "index.html")
	return hugoPreviewRenderRoot(previewID) + "/" + value
}

func filepathURL(value string) string {
	parts := strings.Split(strings.TrimPrefix(path.Clean("/"+value), "/"), "/")
	for index := range parts {
		parts[index] = url.PathEscape(parts[index])
	}
	return strings.Join(parts, "/")
}

func serveHugoPreviewFile(response http.ResponseWriter, request *http.Request, file publication.PreviewRenderFile, routeRoot, publicURL string) {
	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("X-Content-Type-Options", "nosniff")
	response.Header().Set("Content-Security-Policy", "default-src 'self' data: blob: https:; script-src 'none'; connect-src 'none'; frame-ancestors 'self'; form-action 'none'; style-src 'self' 'unsafe-inline' https:; img-src 'self' data: blob: https:; font-src 'self' data: https:")
	opened, err := os.Open(file.AbsolutePath)
	if err != nil {
		writeHugoPreviewError(response, publication.ErrPreviewRenderNotFound)
		return
	}
	defer opened.Close()
	info, err := opened.Stat()
	if err != nil {
		writeHugoPreviewError(response, publication.ErrPreviewRenderNotFound)
		return
	}
	response.Header().Set("Content-Type", file.MediaType)
	if strings.HasPrefix(file.MediaType, "text/html") || strings.HasPrefix(file.MediaType, "text/css") {
		content, readErr := io.ReadAll(opened)
		if readErr != nil {
			writeHugoPreviewError(response, publication.ErrPreviewRenderNotFound)
			return
		}
		if strings.HasPrefix(file.MediaType, "text/html") {
			content, err = rewriteHugoPreviewHTML(content, routeRoot, publicURL)
		} else {
			content = rewriteHugoPreviewCSS(content, routeRoot)
		}
		if err != nil {
			writeHugoPreviewError(response, publication.ErrPreviewInvalid)
			return
		}
		http.ServeContent(response, request, info.Name(), info.ModTime(), bytes.NewReader(content))
		return
	}
	http.ServeContent(response, request, info.Name(), info.ModTime(), opened)
}

func rewriteHugoPreviewHTML(content []byte, routeRoot, publicURL string) ([]byte, error) {
	document, err := html.Parse(bytes.NewReader(content))
	if err != nil {
		return nil, err
	}
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			for index := range node.Attr {
				attribute := &node.Attr[index]
				switch strings.ToLower(attribute.Key) {
				case "href", "src", "poster", "action":
					attribute.Val = rewriteHugoPreviewReference(attribute.Val, routeRoot, publicURL)
				case "srcset":
					attribute.Val = rewriteHugoPreviewSrcset(attribute.Val, routeRoot, publicURL)
				case "style":
					attribute.Val = string(rewriteHugoPreviewCSS([]byte(attribute.Val), routeRoot))
				}
			}
			if node.Data == "style" && node.FirstChild != nil && node.FirstChild.Type == html.TextNode {
				node.FirstChild.Data = string(rewriteHugoPreviewCSS([]byte(node.FirstChild.Data), routeRoot))
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(document)
	var output bytes.Buffer
	if err := html.Render(&output, document); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func rewriteHugoPreviewReference(value, routeRoot, publicURL string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "data:") || strings.HasPrefix(trimmed, "blob:") || strings.HasPrefix(trimmed, "mailto:") || strings.HasPrefix(trimmed, "tel:") || strings.HasPrefix(trimmed, "javascript:") || strings.HasPrefix(trimmed, "//") {
		return value
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return value
	}
	if parsed.IsAbs() {
		base, baseErr := url.Parse(publicURL)
		if baseErr != nil || base.Host == "" || !strings.EqualFold(base.Host, parsed.Host) || !strings.EqualFold(base.Scheme, parsed.Scheme) {
			return value
		}
	} else if !strings.HasPrefix(parsed.Path, "/") {
		return value
	}
	parsed.Scheme, parsed.Host = "", ""
	parsed.Path = strings.TrimPrefix(parsed.Path, "/")
	return routeRoot + "/" + parsed.String()
}

func rewriteHugoPreviewSrcset(value, routeRoot, publicURL string) string {
	items := strings.Split(value, ",")
	for index, item := range items {
		fields := strings.Fields(strings.TrimSpace(item))
		if len(fields) > 0 {
			fields[0] = rewriteHugoPreviewReference(fields[0], routeRoot, publicURL)
		}
		items[index] = strings.Join(fields, " ")
	}
	return strings.Join(items, ", ")
}

func rewriteHugoPreviewCSS(content []byte, routeRoot string) []byte {
	return cssRootReferencePattern.ReplaceAllFunc(content, func(match []byte) []byte {
		parts := cssRootReferencePattern.FindSubmatch(match)
		if len(parts) != 4 {
			return match
		}
		return []byte(fmt.Sprintf("%s%s/%s%s", parts[1], routeRoot, strings.TrimPrefix(string(parts[2]), "/"), parts[3]))
	})
}
