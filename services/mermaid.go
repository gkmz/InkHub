package services

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var mermaidBlockRE = regexp.MustCompile("(?s)```mermaid\\s*\\n(.*?)\\n```")
var mermaidInitRE = regexp.MustCompile(`(?m)^\s*%%\{init:`)

const mermaidModernInit = "%%{init: {'theme': 'base', 'themeVariables': {'fontFamily': 'Arial, PingFang SC, Microsoft YaHei, sans-serif', 'fontSize': '16px', 'primaryColor': '#F8FAFC', 'primaryTextColor': '#0F2742', 'primaryBorderColor': '#2E6FD8', 'lineColor': '#4A6079', 'secondaryColor': '#ECF2FF', 'tertiaryColor': '#F4F7FB', 'clusterBkg': '#F1F5FA', 'clusterBorder': '#A7B6C7', 'edgeLabelBackground': '#FFFFFF', 'nodeBorder': '#2E6FD8', 'mainBkg': '#FFFFFF', 'textColor': '#102A43'}, 'flowchart': {'curve': 'catmullRom', 'htmlLabels': false, 'nodeSpacing': 46, 'rankSpacing': 58, 'padding': 18, 'wrappingWidth': 180, 'diagramPadding': 8}}}%%"
const mermaidHandDrawnInit = "%%{init: {'theme': 'base', 'look': 'handDrawn', 'themeVariables': {'fontFamily': 'Comic Sans MS, Bradley Hand, PingFang SC, Microsoft YaHei, sans-serif', 'fontSize': '18px', 'primaryColor': '#FFF8E8', 'primaryTextColor': '#3A2A10', 'primaryBorderColor': '#C77700', 'lineColor': '#8A4B00', 'secondaryColor': '#FFF3D8', 'tertiaryColor': '#FFF9EE', 'clusterBkg': '#FFF6E3', 'clusterBorder': '#D3912D', 'edgeLabelBackground': '#FFFDF7', 'nodeBorder': '#C77700', 'mainBkg': '#FFFFFF', 'textColor': '#3A2A10'}, 'flowchart': {'curve': 'basis', 'htmlLabels': false, 'nodeSpacing': 64, 'rankSpacing': 84, 'padding': 44}}}%%"

// PreprocessMermaidBlocks 发布前将 mermaid 代码块渲染成图片，并替换为 Markdown 图片引用。
// theme 支持: "handdrawn" | "modern"（其他值默认 handdrawn）。
func PreprocessMermaidBlocks(content, projectRoot, mdDir, theme string) (string, error) {
	if !strings.Contains(content, "```mermaid") {
		return content, nil
	}

	generatedDir := filepath.Join(projectRoot, "assets", "generated-mermaid")
	if err := os.MkdirAll(generatedDir, 0o755); err != nil {
		return "", fmt.Errorf("create mermaid assets directory failed: %w", err)
	}

	matches := mermaidBlockRE.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return content, nil
	}

	var out strings.Builder
	last := 0

	for _, m := range matches {
		fullStart, fullEnd := m[0], m[1]
		bodyStart, bodyEnd := m[2], m[3]
		rawBody := strings.TrimSpace(content[bodyStart:bodyEnd])
		if rawBody == "" {
			return "", fmt.Errorf("empty mermaid block detected")
		}

		themed := ensureMermaidTheme(rawBody, theme)
		hash := shortHash(themed)
		filename := fmt.Sprintf("mermaid-%s.png", hash)
		absPath := filepath.Join(generatedDir, filename)

		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			if err := renderMermaidImage(themed, absPath); err != nil {
				return "", fmt.Errorf("render mermaid failed (%s): %w", filename, err)
			}
		}

		relToMD, err := filepath.Rel(mdDir, absPath)
		if err != nil {
			return "", fmt.Errorf("build mermaid relative path failed: %w", err)
		}

		if !strings.HasPrefix(relToMD, ".") {
			relToMD = "./" + relToMD
		}
		relToMD = filepath.ToSlash(relToMD)

		out.WriteString(content[last:fullStart])
		out.WriteString(fmt.Sprintf("![Mermaid 图表](%s)", relToMD))
		last = fullEnd
	}

	out.WriteString(content[last:])
	return out.String(), nil
}

func ensureMermaidTheme(body, theme string) string {
	if mermaidInitRE.MatchString(body) {
		return body
	}
	if normalizeMermaidTheme(theme) == "modern" {
		return mermaidModernInit + "\n" + body
	}
	return mermaidHandDrawnInit + "\n" + body
}

func normalizeMermaidTheme(theme string) string {
	v := strings.ToLower(strings.TrimSpace(theme))
	if v == "modern" {
		return "modern"
	}
	return "handdrawn"
}

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:16]
}

func renderMermaidImage(code, outPath string) error {
	if err := renderMermaidWithMMDC(code, outPath); err == nil {
		return nil
	}
	return renderMermaidWithKroki(code, outPath)
}

func renderMermaidWithMMDC(code, outPath string) error {
	if _, err := exec.LookPath("mmdc"); err != nil {
		return fmt.Errorf("mmdc not found")
	}

	tmpDir, err := os.MkdirTemp("", "mermaid-render-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	in := filepath.Join(tmpDir, "diagram.mmd")
	if err := os.WriteFile(in, []byte(code), 0o644); err != nil {
		return err
	}

	cmd := exec.Command("mmdc", "-i", in, "-o", outPath, "-b", "transparent", "-s", "3")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("mmdc failed: %s", msg)
	}
	return nil
}

func renderMermaidWithKroki(code, outPath string) error {
	client := &http.Client{Timeout: 20 * time.Second}
	req, err := http.NewRequest(http.MethodPost, "https://kroki.io/mermaid/png", strings.NewReader(code))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	req.Header.Set("Accept", "image/png")
	req.Header.Set("User-Agent", "wechat-preview/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("kroki request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("kroki response %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read png failed: %w", err)
	}

	if len(bytes.TrimSpace(data)) == 0 {
		return fmt.Errorf("empty png from kroki")
	}

	return os.WriteFile(outPath, data, 0o644)
}
