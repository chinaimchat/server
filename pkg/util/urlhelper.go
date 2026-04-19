package util

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// RequestAPIBaseURL 根据当前请求的 Host（X-Forwarded-Proto / X-Forwarded-Host / X-Forwarded-Prefix）生成 API base，
// 使返回的文件 URL 与客户端请求同源；直连 8090 无 Prefix 时为 scheme://host/v1，经 Nginx /api 转发时应配 X-Forwarded-Prefix: /api。
func RequestAPIBaseURL(r *http.Request) string {
	if r == nil {
		return ""
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if s := r.Header.Get("X-Forwarded-Proto"); s != "" {
		scheme = strings.TrimSpace(strings.ToLower(s))
		if scheme != "https" && scheme != "http" {
			scheme = "http"
		}
	}
	host := r.Host
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = strings.TrimSpace(h)
		if idx := strings.Index(host, ","); idx > 0 {
			host = strings.TrimSpace(host[:idx])
		}
	}
	if host == "" {
		return ""
	}
	prefix := strings.TrimSuffix(strings.TrimSpace(r.Header.Get("X-Forwarded-Prefix")), "/")
	if prefix != "" {
		return fmt.Sprintf("%s://%s%s/v1", scheme, host, prefix)
	}
	return fmt.Sprintf("%s://%s/v1", scheme, host)
}

// SameSiteFilePreviewPath 将文件预览绝对 URL 转为根相对路径（保留 /api 等 X-Forwarded-Prefix），
// 供 Web 端 <img src> 与当前页面同源，避免返回带 :8090 或缺少 /api 的绝对地址导致裂图。
// 当主机名与当前请求的 API base 不一致时原样返回 abs（外站或 MinIO 直链等）。
func SameSiteFilePreviewPath(r *http.Request, abs string) string {
	abs = strings.TrimSpace(abs)
	if r == nil || abs == "" {
		return abs
	}
	if !strings.HasPrefix(abs, "http://") && !strings.HasPrefix(abs, "https://") {
		return abs
	}
	outU, err := url.Parse(abs)
	if err != nil || outU.Host == "" || outU.Path == "" {
		return abs
	}
	reqBase := RequestAPIBaseURL(r)
	baseU, err2 := url.Parse(reqBase)
	if err2 != nil || baseU.Host == "" {
		return abs
	}
	if !strings.EqualFold(outU.Hostname(), baseU.Hostname()) {
		return abs
	}
	path := outU.EscapedPath()
	if outU.RawQuery != "" {
		path += "?" + outU.RawQuery
	}
	basePath := strings.TrimSuffix(strings.TrimSpace(baseU.Path), "/")
	if basePath == "" {
		return path
	}
	if strings.HasPrefix(path, basePath+"/") || path == basePath || strings.HasPrefix(path, basePath+"?") {
		return path
	}
	if strings.HasPrefix(path, "/v1/") {
		publicPrefix := strings.TrimSuffix(basePath, "/v1")
		if publicPrefix != "" {
			path = publicPrefix + path
		}
	}
	return path
}

// AvatarAPIRelativePath 用户头像 GET 的相对路径（带 .png 后缀），对应路由 GET /v1/users/:uid/avatar.png。
// 无后缀的 .../avatar 易被 Glide 等库误判为流媒体而走 MediaMetadataRetriever，出现 setDataSource 0x80000000 与 Skia unimplemented。
func AvatarAPIRelativePath(uid string) string {
	if uid == "" {
		return ""
	}
	return fmt.Sprintf("users/%s/avatar.png", uid)
}

// LoopbackAPIBaseURL 本进程访问自身 HTTP API 的 base（以 /v1 结尾）。
// 群头像拼图等后台任务若用 External 公网地址，在 Docker 内常会 dial 宿主机公网 IP 超时；应走 127.0.0.1。
// 可通过环境变量 TS_INTERNAL_API_BASE_URL 覆盖（例如 http://127.0.0.1:8090/v1）。
func LoopbackAPIBaseURL(listenAddr string) string {
	if s := strings.TrimSpace(os.Getenv("TS_INTERNAL_API_BASE_URL")); s != "" {
		return strings.TrimSuffix(s, "/")
	}
	addr := strings.TrimSpace(listenAddr)
	if strings.HasPrefix(addr, ":") {
		return "http://127.0.0.1" + addr + "/v1"
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return "http://127.0.0.1:8090/v1"
	}
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("http://%s:%s/v1", host, port)
}

// FullAPIURL constructs an absolute URL from a relative API path using External.APIBaseURL.
// Handles dedup of /v1 prefix and normalizes slashes.
// e.g. ("http://x.x.x.x:8090/v1", "users/uid/avatar") → "http://x.x.x.x:8090/v1/users/uid/avatar"
// e.g. ("http://x.x.x.x:8090/v1", "/v1/users/uid/avatar") → "http://x.x.x.x:8090/v1/users/uid/avatar"
func FullAPIURL(apiBaseURL string, relativePath string) string {
	if relativePath == "" {
		return ""
	}
	if strings.HasPrefix(relativePath, "http://") || strings.HasPrefix(relativePath, "https://") {
		return relativePath
	}
	base := strings.TrimSuffix(strings.TrimSpace(apiBaseURL), "/")
	if base == "" {
		return ""
	}
	path := strings.TrimPrefix(strings.TrimSpace(relativePath), "/")
	if path == "" {
		return ""
	}
	if strings.HasSuffix(base, "/v1") && strings.HasPrefix(path, "v1/") {
		path = strings.TrimPrefix(path, "v1/")
	}
	uBase, err := url.Parse(base + "/")
	if err != nil {
		// fallback to old behavior if base is malformed
		return fmt.Sprintf("%s/%s", base, path)
	}
	// Use url.URL to ensure spaces and other special chars are percent-encoded.
	uBase.Path = strings.TrimSuffix(uBase.Path, "/") + "/" + path
	return uBase.String()
}

// FilePreviewAPIBaseURL 生成「浏览器 / 客户端直接 GET 文件预览」用的 API base（以 /v1 结尾）。
// 经 Nginx 反代时常见 Host 仅为域名或 IP、不含端口，RequestAPIBaseURL 会得到 http://ip/v1，
// 浏览器加载图片会走默认 80，而 TangSeng 业务 API 在 8090 → 502。若 External.APIBaseURL 与请求同主机且带显式端口（如 :8090），则改用配置。
func FilePreviewAPIBaseURL(r *http.Request, externalAPIBase string) string {
	reqBase := RequestAPIBaseURL(r)
	ext := strings.TrimSpace(externalAPIBase)
	if ext == "" {
		return reqBase
	}
	if reqBase == "" {
		return strings.TrimSuffix(ext, "/")
	}
	uReq, err1 := url.Parse(reqBase)
	uExt, err2 := url.Parse(ext)
	if err1 != nil || err2 != nil || uReq.Host == "" || uExt.Host == "" {
		return reqBase
	}
	if !strings.EqualFold(uReq.Hostname(), uExt.Hostname()) {
		return reqBase
	}
	portReq := uReq.Port()
	portExt := uExt.Port()
	if uReq.Scheme == "http" && uExt.Scheme == "http" {
		if (portReq == "" || portReq == "80") && portExt != "" && portExt != "80" {
			return strings.TrimSuffix(ext, "/")
		}
	}
	return reqBase
}

func parsedURLHost(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host
}

// isUnsignedMinIODownloadURLShape matches MinIO 直链上仅带 response-content-disposition 的未签名 GET（私有桶会 403）。
func isUnsignedMinIODownloadURLShape(u *url.URL) bool {
	if u == nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	q := u.Query()
	for k := range q {
		if k != "response-content-disposition" {
			return false
		}
	}
	if q.Get("response-content-disposition") == "" {
		return false
	}
	p := strings.Trim(strings.TrimPrefix(u.Path, "/"), "/")
	return p != "" && strings.Contains(p, "/")
}

func hasObjectStoragePresignQuery(u *url.URL) bool {
	if u == nil {
		return false
	}
	for k := range u.Query() {
		kl := strings.ToLower(k)
		if strings.HasPrefix(kl, "x-amz-") {
			return true
		}
		if kl == "signature" || strings.HasPrefix(kl, "x-goog-") {
			return true
		}
	}
	return false
}

// FullAPIURLForFilePreview 将充值二维码等「文件预览」地址规范为走 API 的 /v1/file/preview/...。
// 若库里误存了 MinIO DownloadURL 形式的绝对地址（仅 query 含 response-content-disposition、无预签名参数），
// 匿名访问对象存储会 403；此处改回经业务网关由服务端带凭证读对象。
// 配置 Minio.DownloadURL 的 Host 与对外的文件域名一致时，无 query 的直链也会转换。
func FullAPIURLForFilePreview(apiBaseURL, raw, minioUploadURL, minioDownloadURL string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		p := strings.TrimPrefix(strings.TrimSpace(raw), "/")
		if p == "" {
			return ""
		}
		// Relative object paths in DB (e.g. workplace/appicon/xxx) must go through file/preview.
		if strings.HasPrefix(p, "file/preview/") || strings.HasPrefix(p, "v1/file/preview/") {
			return FullAPIURL(apiBaseURL, p)
		}
		return FullAPIURL(apiBaseURL, "file/preview/"+p)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return FullAPIURL(apiBaseURL, raw)
	}
	hosts := make(map[string]struct{})
	for _, s := range []string{minioUploadURL, minioDownloadURL} {
		if h := parsedURLHost(s); h != "" {
			hosts[h] = struct{}{}
		}
	}
	objPath := strings.TrimPrefix(u.Path, "/")
	if objPath == "" || !strings.Contains(objPath, "/") {
		return raw
	}
	_, hostMatch := hosts[u.Host]
	if hostMatch && !hasObjectStoragePresignQuery(u) {
		return FullAPIURL(apiBaseURL, "file/preview/"+objPath)
	}
	if isUnsignedMinIODownloadURLShape(u) {
		return FullAPIURL(apiBaseURL, "file/preview/"+objPath)
	}
	return raw
}

// FullBaseURL constructs an absolute URL from a relative path using External.BaseURL (user-facing domain).
// Handles dedup of /v1 prefix and normalizes slashes.
func FullBaseURL(baseURL string, relativePath string) string {
	if relativePath == "" {
		return ""
	}
	if strings.HasPrefix(relativePath, "http://") || strings.HasPrefix(relativePath, "https://") {
		return relativePath
	}
	base := strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return ""
	}
	path := strings.TrimPrefix(strings.TrimSpace(relativePath), "/")
	if path == "" {
		return ""
	}
	if strings.HasSuffix(base, "/v1") && strings.HasPrefix(path, "v1/") {
		path = strings.TrimPrefix(path, "v1/")
	}
	uBase, err := url.Parse(base + "/")
	if err != nil {
		return fmt.Sprintf("%s/%s", base, path)
	}
	uBase.Path = strings.TrimSuffix(uBase.Path, "/") + "/" + path
	return uBase.String()
}
