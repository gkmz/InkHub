package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/gkmz/InkHub/internal/domain/article"
	inksqlite "github.com/gkmz/InkHub/internal/storage/sqlite"
)

func TestRuntimeHandlerCreatesWorkspaceIdempotentlyAndRestoresSession(t *testing.T) {
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	vault := t.TempDir()
	if err := os.Mkdir(filepath.Join(vault, ".obsidian"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vault, ".obsidian", "app.json"), []byte(`{"attachmentFolderPath":"assets","newLinkFormat":"relative","useMarkdownLinks":false}`), 0o600); err != nil {
		t.Fatal(err)
	}
	article := "---\nid: article_01JTEST\ntitle: 真实扫描文章\ndescription: 测试首次扫描\ntags: [Go]\nkeywords: [InkHub]\n---\n正文"
	if err := os.MkdirAll(filepath.Join(vault, "Areas"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vault, "Areas", "文章.md"), []byte(article), 0o600); err != nil {
		t.Fatal(err)
	}
	indexArticle := strings.Replace(article, "article_01JTEST", "article_INDEX", 1)
	if err := os.WriteFile(filepath.Join(vault, "Areas", "index.md"), []byte(indexArticle), 0o600); err != nil {
		t.Fatal(err)
	}
	refreshCalls := 0
	handler := NewRuntimeHandler(db, NewRouter(emptyRuntimeAPI{}), RuntimeOptions{ProviderRuntime: testProviderRuntime(t), AfterWorkspaceCreated: func(context.Context) (string, error) {
		refreshCalls++
		return "ready", nil
	}})

	session := httptest.NewRecorder()
	handler.ServeHTTP(session, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/session", nil))
	if !strings.Contains(session.Body.String(), `"has_workspace":false`) {
		t.Fatalf("初始会话错误: %s", session.Body.String())
	}

	body := []byte(`{"name":"写作空间","vault_path":"` + filepath.ToSlash(vault) + `","content_roots":["Areas"],"ignored_folders":[],"wechat_template":"default","ai_enabled":false}`)
	for range 2 {
		request := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/workspaces", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", "http://localhost")
		request.Header.Set("Idempotency-Key", "same-request")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusCreated {
			t.Fatalf("创建工作区失败: %d %s", response.Code, response.Body.String())
		}
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM workspaces`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("工作区未幂等创建: count=%d err=%v", count, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM articles WHERE title='真实扫描文章'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("首次扫描未写入文章: count=%d err=%v", count, err)
	}
	if refreshCalls != 2 {
		t.Fatalf("工作区创建后未触发 taxonomy 刷新: calls=%d", refreshCalls)
	}
	settingsResponse := httptest.NewRecorder()
	handler.ServeHTTP(settingsResponse, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/settings", nil))
	if settingsResponse.Code != http.StatusOK || !strings.Contains(settingsResponse.Body.String(), `"attachment_location":"指定文件夹 assets"`) || !strings.Contains(settingsResponse.Body.String(), `"link_format":"基于当前笔记的相对路径"`) {
		t.Fatalf("Obsidian 设置未返回: %d %s", settingsResponse.Code, settingsResponse.Body.String())
	}
	var articleID string
	if err := db.QueryRow(`SELECT id FROM articles LIMIT 1`).Scan(&articleID); err != nil {
		t.Fatal(err)
	}
	detail := httptest.NewRecorder()
	handler.ServeHTTP(detail, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/articles/"+articleID, nil))
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), "真实扫描文章") || !strings.Contains(detail.Body.String(), "正文") {
		t.Fatalf("文章详情错误: %d %s", detail.Code, detail.Body.String())
	}
	metadataBody := `{"metadata":{"title":"写回后的标题","description":"新摘要","category":"工程","series":"InkHub","tags":["Go"],"keywords":["本地"],"slug":"updated-title","cover":""}}`
	metadataRequest := httptest.NewRequest(http.MethodPut, "http://localhost/api/v1/articles/"+articleID+"/metadata", strings.NewReader(metadataBody))
	metadataRequest.Header.Set("Content-Type", "application/json")
	metadataRequest.Header.Set("Origin", "http://localhost")
	metadataResponse := httptest.NewRecorder()
	handler.ServeHTTP(metadataResponse, metadataRequest)
	updated, readErr := os.ReadFile(filepath.Join(vault, "Areas", "文章.md"))
	if metadataResponse.Code != http.StatusOK || readErr != nil || !strings.Contains(string(updated), "写回后的标题") {
		t.Fatalf("元数据未原子写回: code=%d body=%s file=%s err=%v", metadataResponse.Code, metadataResponse.Body.String(), updated, readErr)
	}
	if !strings.Contains(string(updated), "tags: [go]") || !strings.Contains(metadataResponse.Body.String(), `"tags":["go"]`) {
		t.Fatalf("Tag 未在写回边界规范化: body=%s file=%s", metadataResponse.Body.String(), updated)
	}
	invalidMetadata := httptest.NewRequest(http.MethodPut, "http://localhost/api/v1/articles/"+articleID+"/metadata", strings.NewReader(`{"metadata":{"title":"无效标签","tags":["---"]}}`))
	invalidMetadata.Header.Set("Content-Type", "application/json")
	invalidMetadata.Header.Set("Origin", "http://localhost")
	invalidResponse := httptest.NewRecorder()
	handler.ServeHTTP(invalidResponse, invalidMetadata)
	if invalidResponse.Code != http.StatusBadRequest || !strings.Contains(invalidResponse.Body.String(), "metadata.tags_invalid") {
		t.Fatalf("无效 Tag 未被拒绝: code=%d body=%s", invalidResponse.Code, invalidResponse.Body.String())
	}
	scopeRequest := httptest.NewRequest(http.MethodPut, "http://localhost/api/v1/settings/content-scope", strings.NewReader(`{"content_roots":["Areas"],"ignored_folders":[],"ignored_file_names":["toc.md"]}`))
	scopeRequest.Header.Set("Content-Type", "application/json")
	scopeRequest.Header.Set("Origin", "http://localhost")
	scopeResponse := httptest.NewRecorder()
	handler.ServeHTTP(scopeResponse, scopeRequest)
	var sourceConfig string
	if err := db.QueryRow(`SELECT config_json FROM sources LIMIT 1`).Scan(&sourceConfig); scopeResponse.Code != http.StatusOK || err != nil || !strings.Contains(sourceConfig, `"ignored_file_names":["toc.md"]`) {
		t.Fatalf("忽略文件名未持久化: code=%d config=%s err=%v", scopeResponse.Code, sourceConfig, err)
	}
}

func TestRuntimeWorkspaceInitializationCompletesFrontmatterAndStableIDs(t *testing.T) {
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	vault := t.TempDir()
	for _, directory := range []string{".obsidian", "Areas"} {
		if err := os.MkdirAll(filepath.Join(vault, directory), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	plainPath := filepath.Join(vault, "Areas", "无属性.md")
	existingPath := filepath.Join(vault, "Areas", "已有属性.md")
	if err := os.WriteFile(plainPath, []byte("正文保持不变\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(existingPath, []byte("---\nid: null\ntitle: 已有标题\ncustom_field: 保留值\n---\n已有正文\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := NewRuntimeHandler(db, NewRouter(emptyRuntimeAPI{}), RuntimeOptions{ProviderRuntime: testProviderRuntime(t)})
	request := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/workspaces", strings.NewReader(`{"name":"初始化测试","vault_path":"`+filepath.ToSlash(vault)+`","content_roots":["Areas"],"ignored_folders":[]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost")
	request.Header.Set("Idempotency-Key", "initialize-frontmatter")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("创建工作区失败: code=%d body=%s", response.Code, response.Body.String())
	}

	stablePattern := regexp.MustCompile(`(?m)^id: article_[A-Za-z0-9]+$`)
	for _, check := range []struct {
		path string
		want string
	}{{plainPath, "正文保持不变"}, {existingPath, "custom_field: 保留值"}} {
		content, readErr := os.ReadFile(check.path)
		if readErr != nil || !stablePattern.Match(content) || !strings.Contains(string(content), check.want) {
			t.Fatalf("初始化未兼容写回 %s: content=%s err=%v", check.path, content, readErr)
		}
		if strings.Count(string(content), "\nid:") != 1 {
			t.Fatalf("初始化产生重复 Stable ID 字段: path=%s content=%s", check.path, content)
		}
	}
	var articleCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM articles WHERE stable_id<>'' AND deleted_at IS NULL`).Scan(&articleCount); err != nil || articleCount != 2 {
		t.Fatalf("稳定文章索引数量错误: count=%d err=%v", articleCount, err)
	}
	job := httptest.NewRecorder()
	var jobID string
	if err := db.QueryRow(`SELECT id FROM jobs WHERE kind='workspace.initialize'`).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	handler.ServeHTTP(job, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/jobs/"+jobID, nil))
	if job.Code != http.StatusOK || !strings.Contains(job.Body.String(), `"assigned_ids":2`) || !strings.Contains(job.Body.String(), `"state":"succeeded"`) {
		t.Fatalf("初始化任务结果错误: code=%d body=%s", job.Code, job.Body.String())
	}
	session := httptest.NewRecorder()
	handler.ServeHTTP(session, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/session", nil))
	if !strings.Contains(session.Body.String(), `"required":false`) {
		t.Fatalf("初始化成功后会话仍被阻断: %s", session.Body.String())
	}
}

func TestRuntimeWorkspaceInitializationBlocksDuplicateStableIDs(t *testing.T) {
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	vault := t.TempDir()
	for _, directory := range []string{".obsidian", "Areas"} {
		if err := os.MkdirAll(filepath.Join(vault, directory), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	duplicate := []byte("---\nid: article_DUPLICATE\n---\n正文\n")
	secondPath := filepath.Join(vault, "Areas", "二.md")
	for _, name := range []string{"一.md", "二.md"} {
		if err := os.WriteFile(filepath.Join(vault, "Areas", name), duplicate, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	handler := NewRuntimeHandler(db, NewRouter(emptyRuntimeAPI{}), RuntimeOptions{ProviderRuntime: testProviderRuntime(t)})
	request := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/workspaces", strings.NewReader(`{"name":"冲突测试","vault_path":"`+filepath.ToSlash(vault)+`","content_roots":["Areas"],"ignored_folders":[]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost")
	request.Header.Set("Idempotency-Key", "initialize-duplicate")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("工作区基础记录创建失败: code=%d body=%s", response.Code, response.Body.String())
	}
	session := httptest.NewRecorder()
	handler.ServeHTTP(session, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/session", nil))
	if !strings.Contains(session.Body.String(), `"required":true`) || !strings.Contains(session.Body.String(), `"state":"failed"`) {
		t.Fatalf("重复 Stable ID 未阻断主界面: %s", session.Body.String())
	}
	var result string
	if err := db.QueryRow(`SELECT result_json FROM jobs WHERE kind='workspace.initialize'`).Scan(&result); err != nil || !strings.Contains(result, "obsidian.duplicate_stable_id") {
		t.Fatalf("初始化冲突未保留文件诊断: result=%s err=%v", result, err)
	}
	if err := os.WriteFile(secondPath, []byte("---\nid: article_UNIQUE\n---\n正文\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	retry := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/workspace/initialize", strings.NewReader("{}"))
	retry.Header.Set("Content-Type", "application/json")
	retry.Header.Set("Origin", "http://localhost")
	retryResponse := httptest.NewRecorder()
	handler.ServeHTTP(retryResponse, retry)
	if retryResponse.Code != http.StatusOK || !strings.Contains(retryResponse.Body.String(), `"state":"succeeded"`) {
		t.Fatalf("修复冲突后无法重试初始化: code=%d body=%s", retryResponse.Code, retryResponse.Body.String())
	}
	session = httptest.NewRecorder()
	handler.ServeHTTP(session, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/session", nil))
	if !strings.Contains(session.Body.String(), `"required":false`) {
		t.Fatalf("重试成功后初始化门禁未解除: %s", session.Body.String())
	}
}

func TestRuntimeMetadataSaveGeneratesStableIDWithoutChangingArticleID(t *testing.T) {
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	vault := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vault, ".obsidian"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(vault, "Areas"), 0o700); err != nil {
		t.Fatal(err)
	}
	articlePath := filepath.Join(vault, "Areas", "待补充身份.md")
	source := "---\ntitle: 待补充身份\ndescription: 已有摘要\ntags: [go]\nkeywords: [InkHub]\npublish:\n  status: ready\n  slug: identity-repair\n---\n正文\n"
	if err := os.WriteFile(articlePath, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := NewRuntimeHandler(db, NewRouter(emptyRuntimeAPI{}), RuntimeOptions{ProviderRuntime: testProviderRuntime(t)})
	createRequest := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/workspaces", strings.NewReader(`{"name":"身份测试","vault_path":"`+filepath.ToSlash(vault)+`","content_roots":["Areas"],"ignored_folders":[],"wechat_template":"default","ai_enabled":false}`))
	createRequest.Header.Set("Content-Type", "application/json")
	createRequest.Header.Set("Origin", "http://localhost")
	createRequest.Header.Set("Idempotency-Key", "identity-test")
	createResponse := httptest.NewRecorder()
	handler.ServeHTTP(createResponse, createRequest)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("创建工作区失败: code=%d body=%s", createResponse.Code, createResponse.Body.String())
	}
	var articleID string
	if err := db.QueryRow(`SELECT id FROM articles WHERE title='待补充身份'`).Scan(&articleID); err != nil {
		t.Fatal(err)
	}
	var contentHash, frontmatterHash string
	if err := db.QueryRow(`SELECT content_hash,frontmatter_hash FROM articles WHERE id=?`, articleID).Scan(&contentHash, &frontmatterHash); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO editorial_reviews(article_id,state,approved_content_hash,approved_frontmatter_hash,approved_at,approved_by,updated_at) VALUES(?,'approved',?,?,'2026-08-03T00:00:00Z','user','2026-08-03T00:00:00Z')`, articleID, contentHash, frontmatterHash); err != nil {
		t.Fatal(err)
	}
	metadataBody := `{"metadata":{"title":"待补充身份","description":"已有摘要","category":"","series":"","tags":["go"],"keywords":["InkHub"],"slug":"identity-repair","cover":""}}`
	responses := make([]*httptest.ResponseRecorder, 8)
	var wait sync.WaitGroup
	start := make(chan struct{})
	for index := range responses {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			request := httptest.NewRequest(http.MethodPut, "http://localhost/api/v1/articles/"+articleID+"/metadata", strings.NewReader(metadataBody))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Origin", "http://localhost")
			responses[index] = httptest.NewRecorder()
			handler.ServeHTTP(responses[index], request)
		}(index)
	}
	close(start)
	wait.Wait()
	metadataResponse := responses[0]
	for index, response := range responses {
		if response.Code != http.StatusOK {
			t.Fatalf("并发补充稳定 ID 第 %d 个请求失败: code=%d body=%s", index, response.Code, response.Body.String())
		}
	}

	var stableID, storedArticleID string
	if err := db.QueryRow(`SELECT id,stable_id FROM articles WHERE relative_path='Areas/待补充身份.md' AND deleted_at IS NULL`).Scan(&storedArticleID, &stableID); err != nil {
		t.Fatal(err)
	}
	if storedArticleID != articleID {
		t.Fatalf("补充稳定 ID 后内部文章 ID 改变: got=%s want=%s", storedArticleID, articleID)
	}
	if err := article.StableID(stableID).Validate(); err != nil {
		t.Fatalf("生成的稳定 ID 无效: %q err=%v", stableID, err)
	}
	var reviewState string
	if err := db.QueryRow(`SELECT state FROM editorial_reviews WHERE article_id=?`, articleID).Scan(&reviewState); err != nil || reviewState != "approved" {
		t.Fatalf("仅补充稳定 ID 后同版本审核状态改变: state=%s err=%v", reviewState, err)
	}
	written, err := os.ReadFile(articlePath)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`(?m)^id: article_[A-Za-z0-9]+$`).Match(written) || !strings.Contains(metadataResponse.Body.String(), `"stable_id":"`+stableID+`"`) {
		t.Fatalf("源文件或详情响应未返回稳定 ID: file=%s body=%s", written, metadataResponse.Body.String())
	}
	for index, response := range responses {
		if !strings.Contains(response.Body.String(), `"stable_id":"`+stableID+`"`) {
			t.Fatalf("并发请求返回了不同稳定 ID: index=%d body=%s want=%s", index, response.Body.String(), stableID)
		}
	}

	// 模拟扫描后目标文章指纹没有落库，接口必须明确失败而不是返回旧详情。
	if _, err := db.Exec(`CREATE TRIGGER keep_old_article_fingerprint AFTER UPDATE OF source_fingerprint ON articles
BEGIN
  UPDATE articles SET source_fingerprint=OLD.source_fingerprint WHERE id=NEW.id;
END`); err != nil {
		t.Fatal(err)
	}
	partialBody := strings.Replace(metadataBody, "待补充身份", "索引失败后的标题", 1)
	partialRequest := httptest.NewRequest(http.MethodPut, "http://localhost/api/v1/articles/"+articleID+"/metadata", strings.NewReader(partialBody))
	partialRequest.Header.Set("Content-Type", "application/json")
	partialRequest.Header.Set("Origin", "http://localhost")
	partialResponse := httptest.NewRecorder()
	handler.ServeHTTP(partialResponse, partialRequest)
	if partialResponse.Code != http.StatusInternalServerError || !strings.Contains(partialResponse.Body.String(), `"code":"article.index_refresh_failed"`) {
		t.Fatalf("索引未正确落库时仍返回成功: code=%d body=%s", partialResponse.Code, partialResponse.Body.String())
	}
}

func TestRuntimeReviewGeneratesStableIDWithoutChangingArticleID(t *testing.T) {
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	vault := t.TempDir()
	if err := os.Mkdir(filepath.Join(vault, ".obsidian"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(vault, "Areas"), 0o700); err != nil {
		t.Fatal(err)
	}
	articlePath := filepath.Join(vault, "Areas", "article.md")
	if err := os.WriteFile(articlePath, []byte("---\ntitle: 已有标题\ndescription: 用于审核的摘要\nkeywords:\n  - review\npublish:\n  status: ready\n  slug: review-test\n---\n正文\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := NewRuntimeHandler(db, NewRouter(emptyRuntimeAPI{}), RuntimeOptions{ProviderRuntime: testProviderRuntime(t)})
	create := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/workspaces", strings.NewReader(`{"name":"审核测试","vault_path":"`+filepath.ToSlash(vault)+`","content_roots":["Areas"],"ignored_folders":[],"wechat_template":"default","ai_enabled":false}`))
	create.Header.Set("Content-Type", "application/json")
	create.Header.Set("Origin", "http://localhost")
	create.Header.Set("Idempotency-Key", "review-identity-test")
	createResponse := httptest.NewRecorder()
	handler.ServeHTTP(createResponse, create)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("创建审核测试工作区: code=%d body=%s", createResponse.Code, createResponse.Body.String())
	}
	var articleID string
	if err := db.QueryRow(`SELECT id FROM articles WHERE relative_path='Areas/article.md'`).Scan(&articleID); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/articles/"+articleID+"/review", nil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"state":"approved"`) {
		t.Fatalf("审核未自动补充稳定 ID: code=%d body=%s", response.Code, response.Body.String())
	}
	var storedArticleID, stableID, reviewState string
	if err := db.QueryRow(`SELECT articles.id,articles.stable_id,editorial_reviews.state FROM articles JOIN editorial_reviews ON editorial_reviews.article_id=articles.id WHERE articles.relative_path='Areas/article.md'`).Scan(&storedArticleID, &stableID, &reviewState); err != nil {
		t.Fatal(err)
	}
	if storedArticleID != articleID || article.StableID(stableID).Validate() != nil || reviewState != "approved" {
		t.Fatalf("审核后的文章身份或状态错误: id=%s stable_id=%s state=%s", storedArticleID, stableID, reviewState)
	}
	written, err := os.ReadFile(articlePath)
	if err != nil || !strings.Contains(string(written), "id: "+stableID) {
		t.Fatalf("稳定 ID 未写入源文件: content=%s err=%v", written, err)
	}
}

func TestRuntimeReviewRejectsArticleWithInvalidStableID(t *testing.T) {
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := "2026-08-03T00:00:00Z"
	_, err = db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES('w1','审核测试','/tmp',?,?,?);
INSERT INTO sources(id,workspace_id,provider_type,root_path,created_at,updated_at) VALUES('s1','w1','obsidian','/tmp',?,?);
INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,title,content_hash,frontmatter_hash,indexed_at,created_at,updated_at,content_stage)
VALUES('a1','w1','s1','legacy-invalid','article.md','已有标题','content-hash','frontmatter-hash',?,?,?,'ready')`, now, now, now, now, now, now, now, now)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewRuntimeHandler(db, NewRouter(emptyRuntimeAPI{}))
	request := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/articles/a1/review", nil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), `"code":"article.identity_invalid"`) {
		t.Fatalf("非法稳定 ID 的文章仍可审核: code=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRuntimeHandlerRefreshesWorkspace(t *testing.T) {
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	vault := t.TempDir()
	if err := os.Mkdir(filepath.Join(vault, ".obsidian"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(vault, "Areas"), 0o700); err != nil {
		t.Fatal(err)
	}
	articlePath := filepath.Join(vault, "Areas", "文章.md")
	draft := "---\nid: article_REFRESH\ntitle: 手动刷新\npublish:\n  status: draft\n---\n正文\n"
	if err := os.WriteFile(articlePath, []byte(draft), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := NewRuntimeHandler(db, NewRouter(emptyRuntimeAPI{}), RuntimeOptions{ProviderRuntime: testProviderRuntime(t)})
	createRequest := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/workspaces", strings.NewReader(`{"name":"刷新测试","vault_path":"`+filepath.ToSlash(vault)+`","content_roots":["Areas"],"ignored_folders":[],"wechat_template":"default","ai_enabled":false}`))
	createRequest.Header.Set("Content-Type", "application/json")
	createRequest.Header.Set("Origin", "http://localhost")
	createRequest.Header.Set("Idempotency-Key", "refresh-test")
	createResponse := httptest.NewRecorder()
	handler.ServeHTTP(createResponse, createRequest)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("创建刷新测试工作区失败: %d %s", createResponse.Code, createResponse.Body.String())
	}
	var stage string
	if err := db.QueryRow(`SELECT content_stage FROM articles WHERE title='手动刷新'`).Scan(&stage); err != nil || stage != "draft" {
		t.Fatalf("初始文章阶段错误: stage=%s err=%v", stage, err)
	}
	ready := strings.Replace(draft, "status: draft", "status: ready", 1)
	if err := os.WriteFile(articlePath, []byte(ready), 0o600); err != nil {
		t.Fatal(err)
	}
	refreshRequest := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/workspace/refresh", strings.NewReader("{}"))
	refreshRequest.Header.Set("Content-Type", "application/json")
	refreshRequest.Header.Set("Origin", "http://localhost")
	refreshResponse := httptest.NewRecorder()
	handler.ServeHTTP(refreshResponse, refreshRequest)
	if refreshResponse.Code != http.StatusOK || !strings.Contains(refreshResponse.Body.String(), `"indexed":1`) {
		t.Fatalf("工作区刷新失败: %d %s", refreshResponse.Code, refreshResponse.Body.String())
	}
	if err := db.QueryRow(`SELECT content_stage FROM articles WHERE title='手动刷新'`).Scan(&stage); err != nil || stage != "ready" {
		t.Fatalf("刷新后文章阶段错误: stage=%s err=%v", stage, err)
	}
}

func TestRuntimeHandlerInspectsVaultDirectoriesWithoutExposingFileNames(t *testing.T) {
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	vault := t.TempDir()
	if err := os.Mkdir(filepath.Join(vault, ".obsidian"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{"Areas/写作/秘密标题.md", "Areas/另一篇.md", "Resources/资料.md"} {
		full := filepath.Join(vault, relative)
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("private body"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	handler := NewRuntimeHandler(db, NewRouter(emptyRuntimeAPI{}))
	body := `{"vault_path":"` + filepath.ToSlash(vault) + `"}`
	request := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/directories/inspect", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"path":"Areas"`) || !strings.Contains(response.Body.String(), `"markdown_count":2`) {
		t.Fatalf("目录检查响应错误: code=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "秘密标题") || strings.Contains(response.Body.String(), "private body") {
		t.Fatalf("目录检查泄露文章信息: %s", response.Body.String())
	}
}

func TestRuntimeSettingsNormalizesLegacyNullScopeArrays(t *testing.T) {
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	vault := t.TempDir()
	if err := os.Mkdir(filepath.Join(vault, ".obsidian"), 0o700); err != nil {
		t.Fatal(err)
	}
	now := "2026-01-01T00:00:00Z"
	if _, err := db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES('w1','旧空间',?,?,?,?)`, t.TempDir(), now, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sources(id,workspace_id,provider_type,root_path,config_json,created_at,updated_at) VALUES('s1','w1','obsidian',?,'{"content_roots":["Areas"],"ignored_folders":null}',?,?)`, vault, now, now); err != nil {
		t.Fatal(err)
	}
	handler := NewRuntimeHandler(db, NewRouter(emptyRuntimeAPI{}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/settings", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"ignored_folders":[]`) {
		t.Fatalf("旧 null 配置未规范化: code=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRuntimeHandlerPicksDirectoryThroughInjectedNativeAdapter(t *testing.T) {
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	picker := &fakeDirectoryPicker{path: "/Users/test/Documents/Vault"}
	handler := NewRuntimeHandler(db, NewRouter(emptyRuntimeAPI{}), RuntimeOptions{DirectoryPicker: picker})

	request := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/directories/pick", strings.NewReader(`{"purpose":"hugo"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"path":"/Users/test/Documents/Vault"`) {
		t.Fatalf("目录选择接口响应错误: code=%d body=%s", response.Code, response.Body.String())
	}
	if picker.title != "选择 Hugo 项目根目录" {
		t.Fatalf("目录选择器标题错误: %q", picker.title)
	}

	invalid := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/directories/pick", strings.NewReader(`{"purpose":"unknown"}`))
	invalid.Header.Set("Content-Type", "application/json")
	invalid.Header.Set("Origin", "http://localhost")
	invalidResponse := httptest.NewRecorder()
	handler.ServeHTTP(invalidResponse, invalid)
	if invalidResponse.Code != http.StatusBadRequest || !strings.Contains(invalidResponse.Body.String(), "request.invalid") {
		t.Fatalf("未知目录用途未拒绝: code=%d body=%s", invalidResponse.Code, invalidResponse.Body.String())
	}

	blocked := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/directories/pick", strings.NewReader(`{}`))
	blocked.Header.Set("Content-Type", "application/json")
	blocked.Header.Set("Origin", "https://evil.example")
	blockedResponse := httptest.NewRecorder()
	handler.ServeHTTP(blockedResponse, blocked)
	if blockedResponse.Code != http.StatusForbidden {
		t.Fatalf("跨源目录选择请求未拒绝: %d", blockedResponse.Code)
	}
}

func TestRuntimeDashboardPassesThroughToCoreRouter(t *testing.T) {
	t.Parallel()
	core := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		writeJSON(response, http.StatusOK, map[string]string{"source": "core"})
	})
	response := httptest.NewRecorder()
	NewRuntimeHandler(nil, core).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/dashboard", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"source":"core"`) {
		t.Fatalf("工作台请求未进入核心 Router: code=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRuntimeArticleDetailReturnsOnlyEffectiveDisposition(t *testing.T) {
	tests := []struct {
		name            string
		kind            string
		dispositionHash string
		wantDisposition bool
		wantKind        string
		wantChannels    []string
	}{
		{name: "当前版本已发表返回渠道", kind: "published", dispositionHash: "hash-1", wantDisposition: true, wantKind: "published", wantChannels: []string{"hugo", "wechat"}},
		{name: "旧版本已发表不再有效", kind: "published", dispositionHash: "hash-old", wantDisposition: false},
		{name: "忽略跨版本持续且不返回渠道", kind: "ignored", dispositionHash: "hash-old", wantDisposition: true, wantKind: "ignored", wantChannels: []string{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			vault := t.TempDir()
			if err := os.Mkdir(filepath.Join(vault, ".obsidian"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(vault, "article.md"), []byte("---\nid: stable-1\ntitle: 处置详情\n---\n正文"), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err = db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES('w1','当前','/tmp','2026-07-30','2026-07-30','2026-07-30');
INSERT INTO sources(id,workspace_id,provider_type,root_path,created_at,updated_at) VALUES('s1','w1','obsidian',?,'2026-07-30','2026-07-30');
INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,title,content_hash,indexed_at,created_at,updated_at) VALUES('a1','w1','s1','stable-1','article.md','处置详情','hash-1','2026-07-30','2026-07-30','2026-07-30');
INSERT INTO provider_instances(id,workspace_id,provider_type,name,created_at,updated_at) VALUES
('h1','w1','hugo','Hugo','2026-07-30','2026-07-30'),('m1','w1','wechat','微信','2026-07-30','2026-07-30');
INSERT INTO publications(id,article_id,provider_instance_id,workspace_id,state,content_hash,created_at,updated_at) VALUES
				('p1','a1','h1','w1','published','hash-1','2026-07-30','2026-07-30'),('p2','a1','m1','w1','published','hash-1','2026-07-30','2026-07-30')`, vault)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO article_dispositions(article_id,workspace_id,kind,content_hash,created_at,updated_at) VALUES('a1','w1',?,?,'2026-07-30','2026-07-30')`, test.kind, test.dispositionHash); err != nil {
				t.Fatal(err)
			}
			handler := NewRuntimeHandler(db, NewRouter(emptyRuntimeAPI{}), RuntimeOptions{ProviderRuntime: testProviderRuntime(t)})
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/articles/a1", nil))
			if response.Code != http.StatusOK {
				t.Fatalf("文章详情响应错误: code=%d body=%s", response.Code, response.Body.String())
			}
			var body struct {
				Disposition *struct {
					Kind     string   `json:"kind"`
					Channels []string `json:"channels"`
				} `json:"disposition"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if !test.wantDisposition {
				if body.Disposition != nil {
					t.Fatalf("旧处置不应返回: %+v", body.Disposition)
				}
				return
			}
			if body.Disposition == nil || body.Disposition.Kind != test.wantKind || !equalStrings(body.Disposition.Channels, test.wantChannels) {
				t.Fatalf("disposition=%+v want kind=%s channels=%v", body.Disposition, test.wantKind, test.wantChannels)
			}
		})
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

type fakeDirectoryPicker struct {
	path  string
	title string
}

func (p *fakeDirectoryPicker) Pick(_ context.Context, title string) (string, error) {
	p.title = title
	return p.path, nil
}

type emptyRuntimeAPI struct{}

func (emptyRuntimeAPI) ListArticles(context.Context, ArticleListQuery) (ArticlePage, error) {
	return ArticlePage{}, nil
}

func (emptyRuntimeAPI) BatchDisposition(context.Context, BatchDispositionCommand) (BatchDispositionResult, error) {
	return BatchDispositionResult{}, nil
}
func (emptyRuntimeAPI) Dashboard(context.Context) (DashboardView, error) {
	return DashboardView{}, nil
}
func (emptyRuntimeAPI) QueuePublication(context.Context, PublicationCommand) (string, error) {
	return "", ErrNotFound
}
func (emptyRuntimeAPI) ConfirmWeChat(context.Context, ConfirmCommand) error    { return ErrNotFound }
func (emptyRuntimeAPI) MarkWeChatCopied(context.Context, ConfirmCommand) error { return ErrNotFound }
