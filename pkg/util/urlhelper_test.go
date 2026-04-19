package util

import (
	"net/http"
	"testing"
)

func TestFilePreviewAPIBaseURL(t *testing.T) {
	ext := "http://192.0.2.1:8090/v1"
	req := func(host string) *http.Request {
		r, _ := http.NewRequest(http.MethodGet, "http://example/v1/x", nil)
		r.Host = host
		return r
	}
	if got := FilePreviewAPIBaseURL(req("192.0.2.1"), ext); got != "http://192.0.2.1:8090/v1" {
		t.Fatalf("Host without port: got %q want http://192.0.2.1:8090/v1", got)
	}
	if got := FilePreviewAPIBaseURL(req("192.0.2.1:8090"), ext); got != "http://192.0.2.1:8090/v1" {
		t.Fatalf("Host with API port: got %q", got)
	}
	if got := FilePreviewAPIBaseURL(req("10.0.2.2:8090"), ext); got != "http://10.0.2.2:8090/v1" {
		t.Fatalf("Different host must keep request base: got %q", got)
	}
}

func TestSameSiteFilePreviewPath(t *testing.T) {
	rWithAPIPrefix := func() *http.Request {
		r, _ := http.NewRequest(http.MethodGet, "http://192.0.2.1:82/api/v1/workplace/app", nil)
		r.Host = "192.0.2.1:82"
		r.Header.Set("X-Forwarded-Prefix", "/api")
		return r
	}
	// 同主机但端口为8090 的绝对地址 → 根相对路径，便于页面在 :82 走 nginx
	absWrong := "http://192.0.2.1:8090/v1/file/preview/bucket/obj"
	if got := SameSiteFilePreviewPath(rWithAPIPrefix(), absWrong); got != "/api/v1/file/preview/bucket/obj" {
		t.Fatalf("same host different port: got %q", got)
	}
	absOK := "http://192.0.2.1:82/api/v1/file/preview/bucket/obj"
	if got := SameSiteFilePreviewPath(rWithAPIPrefix(), absOK); got != "/api/v1/file/preview/bucket/obj" {
		t.Fatalf("strip to path: got %q", got)
	}
	other, _ := http.NewRequest(http.MethodGet, "http://192.0.2.1:82/api/v1/x", nil)
	other.Host = "192.0.2.1:82"
	other.Header.Set("X-Forwarded-Prefix", "/api")
	if got := SameSiteFilePreviewPath(other, "https://cdn.example.com/a/b"); got != "https://cdn.example.com/a/b" {
		t.Fatalf("foreign host: got %q", got)
	}
	rNoPrefix, _ := http.NewRequest(http.MethodGet, "http://192.0.2.1:8090/v1/x", nil)
	rNoPrefix.Host = "192.0.2.1:8090"
	if got := SameSiteFilePreviewPath(rNoPrefix, "http://192.0.2.1:8090/v1/file/preview/o"); got != "/v1/file/preview/o" {
		t.Fatalf("no api prefix: got %q", got)
	}
}

func TestFullAPIURLForFilePreviewRelativePath(t *testing.T) {
	base := "http://192.0.2.1:8090/v1"
	if got := FullAPIURLForFilePreview(base, "workplace/appicon/a.png", "", ""); got != "http://192.0.2.1:8090/v1/file/preview/workplace/appicon/a.png" {
		t.Fatalf("relative object path must use file/preview: got %q", got)
	}
	if got := FullAPIURLForFilePreview(base, "/file/preview/workplace/appicon/a.png", "", ""); got != "http://192.0.2.1:8090/v1/file/preview/workplace/appicon/a.png" {
		t.Fatalf("existing file/preview path should stay stable: got %q", got)
	}
}
