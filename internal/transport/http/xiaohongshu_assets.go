package httptransport

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/gkmz/InkHub/internal/domain/xiaohongshu"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// refreshXiaohongshuDraftAssets 使用当前文章中的有效签名刷新草稿里持久化的本地图片地址。
func refreshXiaohongshuDraftAssets(draft xiaohongshu.Draft, currentHTML string) xiaohongshu.Draft {
	assets := collectCurrentAssetURLs(currentHTML)
	if len(assets) == 0 {
		return draft
	}
	draft.BodyHTML = replaceDraftAssetURLs(draft.BodyHTML, assets)
	for pageIndex := range draft.Pages {
		for blockIndex := range draft.Pages[pageIndex].Blocks {
			block := &draft.Pages[pageIndex].Blocks[blockIndex]
			block.HTML = replaceDraftAssetURLs(block.HTML, assets)
		}
	}
	return draft
}

func collectCurrentAssetURLs(value string) map[string]string {
	assets := make(map[string]string)
	for _, node := range parseXiaohongshuFragment(value) {
		walkXiaohongshuNodes(node, func(current *html.Node) {
			if current.Type != html.ElementNode || current.Data != "img" {
				return
			}
			source := htmlAttribute(current, "src")
			if payload, ok := decodeUnsignedAssetPayload(source); ok {
				assets[assetPayloadKey(payload)] = source
			}
		})
	}
	return assets
}

func replaceDraftAssetURLs(value string, assets map[string]string) string {
	if strings.TrimSpace(value) == "" {
		return value
	}
	nodes := parseXiaohongshuFragment(value)
	if len(nodes) == 0 {
		return value
	}
	for _, node := range nodes {
		walkXiaohongshuNodes(node, func(current *html.Node) {
			if current.Type != html.ElementNode || current.Data != "img" {
				return
			}
			payload, ok := decodeUnsignedAssetPayload(htmlAttribute(current, "src"))
			if !ok {
				return
			}
			if source := assets[assetPayloadKey(payload)]; source != "" {
				setHTMLAttribute(current, "src", source)
			}
		})
	}
	var output bytes.Buffer
	for _, node := range nodes {
		if err := html.Render(&output, node); err != nil {
			return value
		}
	}
	return output.String()
}

func parseXiaohongshuFragment(value string) []*html.Node {
	context := &html.Node{Type: html.ElementNode, DataAtom: atom.Div, Data: "div"}
	nodes, err := html.ParseFragment(strings.NewReader(value), context)
	if err != nil {
		return nil
	}
	return nodes
}

func walkXiaohongshuNodes(node *html.Node, visit func(*html.Node)) {
	visit(node)
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		walkXiaohongshuNodes(child, visit)
	}
}

func decodeUnsignedAssetPayload(source string) (assetTokenPayload, bool) {
	const marker = "/assets/"
	index := strings.Index(source, marker)
	if index < 0 {
		return assetTokenPayload{}, false
	}
	token := strings.SplitN(source[index+len(marker):], "?", 2)[0]
	encoded, _, found := strings.Cut(token, ".")
	if !found {
		return assetTokenPayload{}, false
	}
	payloadJSON, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return assetTokenPayload{}, false
	}
	var payload assetTokenPayload
	if json.Unmarshal(payloadJSON, &payload) != nil || payload.ArticleID == "" || payload.Relative == "" {
		return assetTokenPayload{}, false
	}
	// 旧签名只用于定位资源，实际返回地址始终取自当前已签名的文章 HTML。
	return payload, true
}

func assetPayloadKey(payload assetTokenPayload) string {
	return payload.ArticleID + "\x00" + payload.Relative
}

func htmlAttribute(node *html.Node, key string) string {
	for _, attribute := range node.Attr {
		if attribute.Key == key {
			return attribute.Val
		}
	}
	return ""
}

func setHTMLAttribute(node *html.Node, key, value string) {
	for index := range node.Attr {
		if node.Attr[index].Key == key {
			node.Attr[index].Val = value
			return
		}
	}
	node.Attr = append(node.Attr, html.Attribute{Key: key, Val: value})
}
