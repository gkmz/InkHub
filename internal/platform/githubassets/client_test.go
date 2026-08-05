package githubassets

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func TestValidateRejectsPrivateRepository(t *testing.T) {
	t.Parallel()

	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://api.github.com/repos/gkmz/images" {
			t.Fatalf("意外请求: %s", request.URL)
		}
		if request.Header.Get("Authorization") != "Bearer secret-token" {
			t.Fatal("GitHub API 未使用 Bearer Token")
		}
		return jsonResponse(http.StatusOK, `{"private":true,"permissions":{"push":true}}`), nil
	})}
	uploader, err := New(Config{Owner: "gkmz", Repository: "images", Branch: "main", Prefix: "inkhub", Token: "secret-token"}, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = uploader.Validate(context.Background())
	assertGitHubError(t, err, "github.repository_private", "secret-token")
}

func TestValidateChecksConfiguredBranch(t *testing.T) {
	t.Parallel()

	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		if requests == 1 {
			return jsonResponse(http.StatusOK, `{"private":false,"permissions":{"push":true}}`), nil
		}
		if request.URL.String() != "https://api.github.com/repos/gkmz/images/branches/publish" {
			t.Fatalf("分支请求错误: %s", request.URL)
		}
		return jsonResponse(http.StatusNotFound, `{"message":"Not Found"}`), nil
	})}
	uploader, err := New(Config{Owner: "gkmz", Repository: "images", Branch: "publish", Prefix: "inkhub", Token: "secret-token"}, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = uploader.Validate(context.Background())
	assertGitHubError(t, err, "github.config_invalid", "secret-token")
	if requests != 2 {
		t.Fatalf("请求次数=%d，未检查分支", requests)
	}
}

func TestInspectReusesMatchingPublicAsset(t *testing.T) {
	t.Parallel()

	content := []byte("valid-image-content")
	sum := sha256.Sum256(content)
	digest := hex.EncodeToString(sum[:])
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		if !strings.HasPrefix(request.URL.String(), "https://api.github.com/repos/gkmz/images/contents/") {
			t.Fatalf("请求未发往 GitHub Contents API: %s", request.URL)
		}
		body := `{"type":"file","encoding":"base64","content":"` + base64.StdEncoding.EncodeToString(content) + `"}`
		return jsonResponse(http.StatusOK, body), nil
	})}
	uploader, err := New(Config{Owner: "gkmz", Repository: "images", Branch: "main", Prefix: "inkhub", Token: "secret-token"}, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, found, err := uploader.Inspect(context.Background(), contracts.AssetUploadRequest{Digest: digest, Extension: ".png", MediaType: "image/png"})
	if err != nil || !found || !result.Reused {
		t.Fatalf("未复用已有资源: result=%+v found=%v err=%v", result, found, err)
	}
	if !strings.HasPrefix(result.URL, "https://raw.githubusercontent.com/gkmz/images/main/inkhub/") || requests != 1 {
		t.Fatalf("复用结果不正确: result=%+v requests=%d", result, requests)
	}
}

func TestInspectIncludesUpstreamStatusForAccessFailure(t *testing.T) {
	t.Parallel()

	content := []byte("image")
	sum := sha256.Sum256(content)
	digest := hex.EncodeToString(sum[:])
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusForbidden, `{"message":"Resource not accessible by personal access token"}`), nil
	})}
	uploader, err := New(Config{Owner: "gkmz", Repository: "images", Branch: "main", Prefix: "inkhub", Token: "secret-token"}, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = uploader.Inspect(context.Background(), contracts.AssetUploadRequest{Digest: digest, Extension: ".png", MediaType: "image/png"})
	providerErr, ok := err.(*contracts.ProviderError)
	if !ok || providerErr.UpstreamStatus != http.StatusForbidden || providerErr.Category != contracts.ErrorUnauthorizedResource {
		t.Fatalf("未保留 GitHub 状态: %#v", err)
	}
}

func TestInspectReadsLargeAssetThroughGitBlobAPI(t *testing.T) {
	t.Parallel()

	content := []byte("large-image-content")
	sum := sha256.Sum256(content)
	digest := hex.EncodeToString(sum[:])
	blobSHA := strings.Repeat("a", 40)
	requests := []string{}
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests = append(requests, request.URL.Path)
		if strings.Contains(request.URL.Path, "/contents/") {
			return jsonResponse(http.StatusOK, `{"type":"file","encoding":"none","sha":"`+blobSHA+`"}`), nil
		}
		if strings.HasSuffix(request.URL.Path, "/git/blobs/"+blobSHA) {
			return jsonResponse(http.StatusOK, `{"encoding":"base64","content":"`+base64.StdEncoding.EncodeToString(content)+`"}`), nil
		}
		t.Fatalf("意外请求: %s", request.URL)
		return nil, nil
	})}
	uploader, err := New(Config{Owner: "gkmz", Repository: "images", Branch: "main", Prefix: "inkhub", Token: "secret-token"}, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, found, err := uploader.Inspect(context.Background(), contracts.AssetUploadRequest{Digest: digest, Extension: ".png", MediaType: "image/png"})
	if err != nil || !found || !result.Reused || len(requests) != 2 {
		t.Fatalf("大图片未通过 Git Blob 检查: result=%+v found=%v err=%v requests=%v", result, found, err, requests)
	}
}

func TestUploadCreatesAssetAndChecksAnonymousRawURL(t *testing.T) {
	t.Parallel()

	content := []byte("new-image-content")
	sum := sha256.Sum256(content)
	digest := hex.EncodeToString(sum[:])
	localPath := filepath.Join(t.TempDir(), "cover.png")
	if err := os.WriteFile(localPath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	var methods []string
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		methods = append(methods, request.Method+" "+request.URL.Host)
		switch request.URL.Host {
		case "api.github.com":
			if request.Header.Get("Authorization") != "Bearer secret-token" {
				t.Fatal("GitHub API 未携带 Token")
			}
			if request.Method == http.MethodGet {
				return jsonResponse(http.StatusNotFound, `{"message":"Not Found"}`), nil
			}
			if request.Method == http.MethodPut {
				return jsonResponse(http.StatusCreated, `{"content":{"sha":"blob"}}`), nil
			}
		case "raw.githubusercontent.com":
			if request.Header.Get("Authorization") != "" {
				t.Fatal("匿名 Raw URL 不得携带 Token")
			}
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ok"))}, nil
		}
		t.Fatalf("意外请求: %s %s", request.Method, request.URL)
		return nil, nil
	})}
	uploader, err := New(Config{Owner: "gkmz", Repository: "images", Branch: "main", Prefix: "inkhub", Token: "secret-token"}, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := uploader.Upload(context.Background(), contracts.AssetUploadRequest{LocalPath: localPath, Digest: digest, Extension: ".png", MediaType: "image/png"})
	if err != nil {
		t.Fatalf("上传资源: %v", err)
	}
	if result.Reused || !strings.HasPrefix(result.URL, "https://raw.githubusercontent.com/") {
		t.Fatalf("上传结果不正确: %+v", result)
	}
	if strings.Join(methods, ",") != "GET api.github.com,PUT api.github.com,GET raw.githubusercontent.com" {
		t.Fatalf("请求序列=%v", methods)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func assertGitHubError(t *testing.T, err error, code, secret string) {
	t.Helper()
	providerErr, ok := err.(*contracts.ProviderError)
	if !ok || providerErr.Code != code {
		t.Fatalf("错误=%v，期望 code=%s", err, code)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("错误泄露 Secret: %v", err)
	}
}
