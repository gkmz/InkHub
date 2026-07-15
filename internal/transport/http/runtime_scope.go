package httptransport

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gkmz/InkHub/internal/provider/contracts"
	"github.com/gkmz/InkHub/internal/provider/source/folder"
)

type directoryInspectRequest struct {
	VaultPath string `json:"vault_path"`
}

type directoryCandidate struct {
	Path          string `json:"path"`
	MarkdownCount int    `json:"markdown_count"`
}

// inspectDirectories 只汇总目录级 Markdown 数量，不读取或返回文章内容。
func (h *runtimeHandler) inspectDirectories(response http.ResponseWriter, request *http.Request) {
	var input directoryInspectRequest
	if decodeJSON(request, &input) != nil || strings.TrimSpace(input.VaultPath) == "" {
		writeError(response, http.StatusBadRequest, "request.invalid", "目录检查请求无效")
		return
	}
	root, err := filepath.Abs(input.VaultPath)
	if err != nil {
		writeError(response, http.StatusBadRequest, "workspace.path_invalid", "内容库路径无效")
		return
	}
	if info, err := os.Stat(filepath.Join(root, ".obsidian")); err != nil || !info.IsDir() {
		writeError(response, http.StatusBadRequest, "workspace.not_obsidian", "所选目录不是 Obsidian Vault")
		return
	}
	candidates, err := inspectVaultDirectories(root)
	if err != nil {
		writeError(response, http.StatusUnprocessableEntity, "directory.inspect_failed", "无法检查内容目录")
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"directories": candidates})
}

func inspectVaultDirectories(root string) ([]directoryCandidate, error) {
	counts := map[string]int{}
	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && current != root && systemScopeDirectory(entry.Name()) {
			return filepath.SkipDir
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			return nil
		}
		relative, err := filepath.Rel(root, filepath.Dir(current))
		if err != nil || relative == "." {
			return err
		}
		parts := strings.Split(filepath.ToSlash(relative), "/")
		for index := range parts {
			counts[strings.Join(parts[:index+1], "/")]++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	candidates := make([]directoryCandidate, 0, len(counts))
	for path, count := range counts {
		candidates = append(candidates, directoryCandidate{Path: path, MarkdownCount: count})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Path < candidates[j].Path })
	return candidates, nil
}

type storedSourceScope struct {
	ContentRoots     []string `json:"content_roots"`
	IgnoredFolders   []string `json:"ignored_folders"`
	IgnoredFileNames []string `json:"ignored_file_names"`
}

func (h *runtimeHandler) settings(response http.ResponseWriter, request *http.Request) {
	var workspaceName, root, configJSON string
	err := h.db.QueryRowContext(request.Context(), `SELECT workspaces.name,sources.root_path,sources.config_json FROM workspaces JOIN sources ON sources.workspace_id=workspaces.id ORDER BY workspaces.last_used_at DESC LIMIT 1`).Scan(&workspaceName, &root, &configJSON)
	if err != nil {
		mapError(response, ErrNotFound)
		return
	}
	config := storedSourceScope{ContentRoots: []string{}, IgnoredFolders: []string{}}
	if json.Unmarshal([]byte(configJSON), &config) != nil {
		writeError(response, http.StatusInternalServerError, "workspace.config_invalid", "工作区目录配置损坏")
		return
	}
	if config.ContentRoots == nil {
		config.ContentRoots = []string{}
	}
	if config.IgnoredFolders == nil {
		config.IgnoredFolders = []string{}
	}
	if config.IgnoredFileNames == nil {
		config.IgnoredFileNames = folder.DefaultIgnoredFileNames()
	}
	directories, inspectErr := inspectVaultDirectories(root)
	settings := defaultSettings()
	settings["workspace_name"] = workspaceName
	settings["vault_path"] = root
	settings["content_roots"] = config.ContentRoots
	settings["ignored_folders"] = config.IgnoredFolders
	settings["ignored_file_names"] = config.IgnoredFileNames
	settings["directories"] = directories
	if inspectErr != nil {
		settings["directories"] = []directoryCandidate{}
		settings["diagnostics"] = []map[string]string{{"name": "内容目录", "state": "异常", "message": "无法读取 Vault 目录"}}
	}
	writeJSON(response, http.StatusOK, settings)
}

func (h *runtimeHandler) saveContentScope(response http.ResponseWriter, request *http.Request) {
	var input storedSourceScope
	if decodeJSON(request, &input) != nil || len(input.ContentRoots) == 0 {
		writeError(response, http.StatusBadRequest, "request.invalid", "内容目录配置无效")
		return
	}
	if input.IgnoredFileNames == nil {
		input.IgnoredFileNames = folder.DefaultIgnoredFileNames()
	}
	scope, err := folder.NewScopeWithFileNames(input.ContentRoots, input.IgnoredFolders, input.IgnoredFileNames)
	if err != nil {
		writeError(response, http.StatusBadRequest, "workspace.scope_invalid", "内容目录规则无效")
		return
	}
	var sourceID, workspaceID, root string
	if err := h.db.QueryRowContext(request.Context(), `SELECT sources.id,sources.workspace_id,sources.root_path FROM sources JOIN workspaces ON workspaces.id=sources.workspace_id ORDER BY workspaces.last_used_at DESC LIMIT 1`).Scan(&sourceID, &workspaceID, &root); err != nil {
		mapError(response, ErrNotFound)
		return
	}
	configJSON, _ := json.Marshal(storedSourceScope{ContentRoots: scope.ContentRoots(), IgnoredFolders: scope.IgnoredFolders(), IgnoredFileNames: scope.IgnoredFileNames()})
	if _, err := h.db.ExecContext(request.Context(), `UPDATE sources SET config_json=?,updated_at=? WHERE id=?`, string(configJSON), time.Now().UTC().Format(time.RFC3339Nano), sourceID); err != nil {
		mapError(response, err)
		return
	}
	report, err := h.scanWorkspace(request.Context(), sourceID, workspaceID, configJSON)
	if err != nil {
		mapError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]int{"indexed": report.Indexed, "failed": report.Failed})
}

func (h *runtimeHandler) previewContentScope(response http.ResponseWriter, request *http.Request) {
	var input storedSourceScope
	if decodeJSON(request, &input) != nil || len(input.ContentRoots) == 0 {
		writeError(response, http.StatusBadRequest, "request.invalid", "内容目录配置无效")
		return
	}
	if input.IgnoredFileNames == nil {
		input.IgnoredFileNames = folder.DefaultIgnoredFileNames()
	}
	scope, err := folder.NewScopeWithFileNames(input.ContentRoots, input.IgnoredFolders, input.IgnoredFileNames)
	if err != nil {
		writeError(response, http.StatusBadRequest, "workspace.scope_invalid", "内容目录规则无效")
		return
	}
	var sourceID string
	if err := h.db.QueryRowContext(request.Context(), `SELECT sources.id FROM sources JOIN workspaces ON workspaces.id=sources.workspace_id ORDER BY workspaces.last_used_at DESC LIMIT 1`).Scan(&sourceID); err != nil {
		mapError(response, ErrNotFound)
		return
	}
	configJSON, _ := json.Marshal(storedSourceScope{ContentRoots: scope.ContentRoots(), IgnoredFolders: scope.IgnoredFolders(), IgnoredFileNames: scope.IgnoredFileNames()})
	source, err := h.buildSource(request.Context(), sourceID, configJSON)
	if err != nil {
		mapError(response, err)
		return
	}
	result, err := source.Scan(request.Context(), contracts.ScanCursor{})
	if err != nil {
		mapError(response, err)
		return
	}
	active := map[string]bool{}
	rows, err := h.db.QueryContext(request.Context(), `SELECT relative_path FROM articles WHERE source_id=? AND deleted_at IS NULL`, sourceID)
	if err != nil {
		mapError(response, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var relative string
		if rows.Scan(&relative) == nil {
			active[relative] = true
		}
	}
	added := 0
	for _, document := range result.Documents {
		if !active[document.Ref.RelativePath] && !hasBlockingRuntimeDiagnostic(document.Diagnostics) {
			added++
		}
	}
	removed := 0
	for relative := range active {
		if !scope.Includes(relative) {
			removed++
		}
	}
	writeJSON(response, http.StatusOK, map[string]int{"added": added, "removed": removed})
}

func hasBlockingRuntimeDiagnostic(diagnostics []contracts.Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Blocking {
			return true
		}
	}
	return false
}

func systemScopeDirectory(name string) bool {
	return name == ".obsidian" || name == ".git" || name == ".trash" || strings.HasPrefix(name, ".inkhub-")
}
