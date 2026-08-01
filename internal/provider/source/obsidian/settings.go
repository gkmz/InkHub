package obsidian

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// AttachmentLocationKind 描述 Obsidian 附件目录相对 Vault 的位置语义。
type AttachmentLocationKind string

const (
	// AttachmentAtVaultRoot 表示附件直接存放在 Vault 根目录。
	AttachmentAtVaultRoot AttachmentLocationKind = "vault_root"
	// AttachmentAtCurrentFolder 表示附件存放在当前笔记所在目录。
	AttachmentAtCurrentFolder AttachmentLocationKind = "current_folder"
	// AttachmentAtCurrentSubfolder 表示附件存放在当前笔记目录下的子目录。
	AttachmentAtCurrentSubfolder AttachmentLocationKind = "current_subfolder"
	// AttachmentAtConfiguredFolder 表示附件存放在指定的 Vault 相对目录。
	AttachmentAtConfiguredFolder AttachmentLocationKind = "configured_folder"
)

// LinkFormat 描述 Obsidian 生成新链接时使用的路径偏好。
type LinkFormat string

const (
	// LinkFormatRelative 表示相对当前笔记的路径。
	LinkFormatRelative LinkFormat = "relative"
	// LinkFormatShortest 表示能够唯一定位资源的最短路径。
	LinkFormatShortest LinkFormat = "shortest"
	// LinkFormatAbsolute 表示相对 Vault 根目录的路径。
	LinkFormatAbsolute LinkFormat = "absolute"
)

// AttachmentLocation 保存规范化后的附件目录配置。
type AttachmentLocation struct {
	Kind AttachmentLocationKind
	Path string
}

// ObsidianSettings 保存 .obsidian/app.json 中影响资源解析的设置。
type ObsidianSettings struct {
	AttachmentFolder AttachmentLocation
	LinkFormat       LinkFormat
	UseMarkdownLinks bool
	Fingerprint      string
}

// ReadSettings 读取指定 Vault 的 Obsidian 资源解析设置，供运行期诊断使用。
func ReadSettings(root string) (ObsidianSettings, error) {
	return readObsidianSettings(root)
}

// AttachmentLocationLabel 返回面向用户的附件位置说明。
func (s ObsidianSettings) AttachmentLocationLabel() string {
	switch s.AttachmentFolder.Kind {
	case AttachmentAtVaultRoot:
		return "Vault 根目录"
	case AttachmentAtCurrentFolder:
		return "当前笔记所在文件夹"
	case AttachmentAtCurrentSubfolder:
		return "当前文件夹下的 " + s.AttachmentFolder.Path
	case AttachmentAtConfiguredFolder:
		return "指定文件夹 " + s.AttachmentFolder.Path
	default:
		return "未知"
	}
}

// LinkFormatLabel 返回面向用户的链接类型说明。
func (s ObsidianSettings) LinkFormatLabel() string {
	switch s.LinkFormat {
	case LinkFormatRelative:
		return "基于当前笔记的相对路径"
	case LinkFormatAbsolute:
		return "基于 Vault 根目录的绝对路径"
	default:
		return "尽可能简短的形式"
	}
}

// readObsidianSettings 读取并规范化 Obsidian 的附件和链接配置。
func readObsidianSettings(root string) (ObsidianSettings, error) {
	pathName := filepath.Join(root, ".obsidian", "app.json")
	content, err := os.ReadFile(pathName)
	if err != nil {
		return defaultObsidianSettings(), fmt.Errorf("读取 Obsidian 设置: %w", err)
	}
	var raw struct {
		AttachmentFolderPath string `json:"attachmentFolderPath"`
		NewLinkFormat        string `json:"newLinkFormat"`
		UseMarkdownLinks     bool   `json:"useMarkdownLinks"`
	}
	if err := json.Unmarshal(content, &raw); err != nil {
		sum := sha256.Sum256(content)
		settingsFingerprint := hex.EncodeToString(sum[:])
		settings := defaultObsidianSettings()
		settings.Fingerprint = settingsFingerprint
		return settings, fmt.Errorf("解析 Obsidian 设置: %w", err)
	}
	relevant, _ := json.Marshal(raw)
	sum := sha256.Sum256(relevant)
	settingsFingerprint := hex.EncodeToString(sum[:])
	settings := defaultObsidianSettings()
	settings.AttachmentFolder = normalizeAttachmentLocation(raw.AttachmentFolderPath)
	settings.LinkFormat = normalizeLinkFormat(raw.NewLinkFormat)
	settings.UseMarkdownLinks = raw.UseMarkdownLinks
	settings.Fingerprint = settingsFingerprint
	return settings, nil
}

func defaultObsidianSettings() ObsidianSettings {
	return ObsidianSettings{
		AttachmentFolder: AttachmentLocation{Kind: AttachmentAtVaultRoot},
		LinkFormat:       LinkFormatShortest,
	}
}

func normalizeAttachmentLocation(value string) AttachmentLocation {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	switch value {
	case "", ".", "/":
		return AttachmentLocation{Kind: AttachmentAtVaultRoot}
	case "./":
		return AttachmentLocation{Kind: AttachmentAtCurrentFolder}
	}
	if strings.HasPrefix(value, "./") {
		subfolder := cleanVaultPath(strings.TrimPrefix(value, "./"))
		if subfolder == "" || subfolder == "." {
			return AttachmentLocation{Kind: AttachmentAtCurrentFolder}
		}
		return AttachmentLocation{Kind: AttachmentAtCurrentSubfolder, Path: subfolder}
	}
	return AttachmentLocation{Kind: AttachmentAtConfiguredFolder, Path: cleanVaultPath(value)}
}

func normalizeLinkFormat(value string) LinkFormat {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(LinkFormatRelative):
		return LinkFormatRelative
	case string(LinkFormatAbsolute):
		return LinkFormatAbsolute
	default:
		return LinkFormatShortest
	}
}

func cleanVaultPath(value string) string {
	value = strings.TrimSpace(strings.TrimPrefix(strings.ReplaceAll(value, "\\", "/"), "/"))
	if value == "" {
		return ""
	}
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return ""
	}
	return cleaned
}
