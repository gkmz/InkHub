package httptransport

import (
	"net/http"

	"github.com/gkmz/InkHub/internal/editorial"
)

type publicationSettingsRequest struct {
	ExcludedSections []string `json:"excluded_sections"`
}

// savePublicationSettings 保存 Hugo、微信和小红书共享的发布内容规则。
func (h *runtimeHandler) savePublicationSettings(response http.ResponseWriter, request *http.Request) {
	var input publicationSettingsRequest
	if decodeJSON(request, &input) != nil || input.ExcludedSections == nil {
		writeError(response, http.StatusBadRequest, "request.invalid", "发布内容设置请求无效")
		return
	}
	workspaceID, err := h.currentWorkspaceID(request.Context())
	if err != nil {
		mapError(response, ErrNotFound)
		return
	}
	settings, err := editorial.SavePublicationSettings(request.Context(), h.db, workspaceID, editorial.PublicationSettings{ExcludedSections: input.ExcludedSections})
	if err != nil {
		writeError(response, http.StatusBadRequest, "publication.settings_invalid", err.Error())
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"excluded_sections": settings.ExcludedSections})
}
