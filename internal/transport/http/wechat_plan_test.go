package httptransport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gkmz/InkHub/internal/app/publication"
	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func TestWeChatPlanHTTPReturnsSafeImageListAndConfirmsOpaqueToken(t *testing.T) {
	api := &staticWeChatPlanAPI{plan: publication.WeChatPlanView{
		Token: "opaque-token", TemplateID: "default", Ready: true, ExpiresAt: time.Date(2026, 7, 18, 10, 10, 0, 0, time.UTC),
		Images: []contracts.AssetPlanItem{{Reference: "images/封面.png", MediaType: "image/png", Size: 120, State: "upload"}},
	}}
	handler := NewRuntimeHandler(nil, NewRouter(emptyRuntimeAPI{}), RuntimeOptions{WeChatPlans: api})
	request := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/articles/a1/wechat-plans", strings.NewReader(`{"template_id":"default"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"reference":"images/封面.png"`) || strings.Contains(response.Body.String(), "/secret/vault") {
		t.Fatalf("计划响应错误: %d %s", response.Code, response.Body.String())
	}

	confirm := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/articles/a1/wechat-plans/confirm", strings.NewReader(`{"plan_token":"opaque-token"}`))
	confirm.Header.Set("Content-Type", "application/json")
	confirm.Header.Set("Origin", "http://localhost")
	confirmed := httptest.NewRecorder()
	handler.ServeHTTP(confirmed, confirm)
	if confirmed.Code != http.StatusAccepted || !strings.Contains(confirmed.Body.String(), `"state":"queued"`) || strings.Contains(confirmed.Body.String(), "job-safe") || api.token != "opaque-token" {
		t.Fatalf("确认响应错误: %d %s token=%s", confirmed.Code, confirmed.Body.String(), api.token)
	}
}

type staticWeChatPlanAPI struct {
	plan  publication.WeChatPlanView
	token string
}

func (api *staticWeChatPlanAPI) Plan(context.Context, string, string, string) (publication.WeChatPlanView, error) {
	return api.plan, nil
}

func (api *staticWeChatPlanAPI) Confirm(_ context.Context, _ string, token string) (string, error) {
	api.token = token
	return "job-safe", nil
}
