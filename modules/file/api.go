package file

import (
	"bufio"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	wkutil "github.com/TangSengDaoDao/TangSengDaoDaoServer/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/log"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func previewPathExpectsRasterImage(ph string) bool {
	lower := strings.ToLower(ph)
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".gif", ".webp"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func isVideoExt(path string) bool {
	lower := strings.ToLower(path)
	for _, ext := range []string{".mp4", ".m4v", ".mov"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func sanitizeUploadPath(path string) string {
	raw := strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	if raw == "" {
		return ""
	}
	leadingSlash := strings.HasPrefix(raw, "/")
	parts := strings.Split(raw, "/")
	sanitized := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var b strings.Builder
		for _, r := range part {
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-' {
				b.WriteRune(r)
				continue
			}
			b.WriteRune('_')
		}
		seg := strings.Trim(strings.TrimSpace(b.String()), "._")
		if seg == "" {
			seg = "file"
		}
		sanitized = append(sanitized, seg)
	}
	if len(sanitized) == 0 {
		return ""
	}
	out := strings.Join(sanitized, "/")
	if leadingSlash {
		return "/" + out
	}
	return out
}

// setPreviewContentDisposition 使用逻辑路径的真实文件名（如 xxx.lim、xxx.png），避免请求里 ?filename=image.png
// 等参数让客户端/Glide 按位图解码 .lim（gzip+RLottie）→ 能 200 仍显示小蓝块。
func setPreviewContentDisposition(c *wkhttp.Context, logicalPath string) {
	base := filepath.Base(strings.ReplaceAll(strings.TrimSpace(logicalPath), "\\", "/"))
	if base == "" || base == "." {
		return
	}
	if strings.ContainsAny(base, `"`) {
		return
	}
	c.Writer.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, base))
}

// faststartProcess 将 moov atom 移到文件头部以支持流式播放。
// 返回处理后文件的 ReadSeeker、清理函数、错误。失败时调用方应回退到原始文件。
func faststartProcess(input io.Reader) (io.ReadSeeker, func(), error) {
	tmpDir, err := os.MkdirTemp("", "faststart-")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { os.RemoveAll(tmpDir) }

	inPath := filepath.Join(tmpDir, "input.mp4")
	outPath := filepath.Join(tmpDir, "output.mp4")

	inFile, err := os.Create(inPath)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	if _, err := io.Copy(inFile, input); err != nil {
		inFile.Close()
		cleanup()
		return nil, nil, err
	}
	inFile.Close()

	cmd := exec.Command("ffmpeg", "-i", inPath, "-c", "copy", "-movflags", "+faststart", "-y", outPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("ffmpeg faststart: %w: %s", err, string(out))
	}

	outFile, err := os.Open(outPath)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return outFile, func() {
		outFile.Close()
		cleanup()
	}, nil
}

// File 文件操作
type File struct {
	ctx *config.Context
	log.Log
	service IService
}

// New New
func New(ctx *config.Context) *File {
	return &File{
		ctx:     ctx,
		Log:     log.NewTLog("File"),
		service: NewService(ctx),
	}
}

// Route 路由
func (f *File) Route(r *wkhttp.WKHttp) {
	// 兼容 apiURL 未带 /v1/file 时：getFileURL = apiURL + path，path 为 file/preview/sticker/...
	// 实际请求为 GET /file/preview/...（无 /v1），否则 404 → 表情全是蓝色占位块。
	legacy := r.Group("")
	{
		legacy.Handle(http.MethodHead, "/file/preview/*path", r.WKHttpHandler(f.getFile))
		legacy.GET("/file/preview/*path", f.getFile)
	}
	api := r.Group("/v1/file")
	{ // 文件上传
		// api.POST("/upload/*path", f.upload)
		// 组合图片
		//	api.POST("/compose/*path", f.makeImageCompose)
		// HEAD：ServerLib 的 RouterGroup 未包装 HEAD，api.HEAD 会落到 gin 且类型不匹配；
		// 使用 Handle + WKHttpHandler，与 GET 一样走 wkhttp.Context（Glide/OkHttp 先发 HEAD 探测）。
		api.Handle(http.MethodHead, "/preview/*path", r.WKHttpHandler(f.getFile))
		api.GET("/preview/*path", f.getFile)
	}
	auth := r.Group("/v1/file", f.ctx.AuthMiddleware(r))
	{
		//获取上传文件地址
		auth.GET("/upload", f.getFilePath)
		//上传文件
		auth.POST("/upload", f.uploadFile)
	}
}

// 获取上传文件地址
func (f *File) getFilePath(c *wkhttp.Context) {
	loginUID := c.GetLoginUID()
	uploadPath := sanitizeUploadPath(c.Query("path"))
	fileType := c.Query("type")
	err := f.checkReq(Type(fileType), uploadPath)
	if err != nil {
		c.ResponseError(err)
		return
	}

	// 与主 API 同源：经 Nginx /api 反代时按 X-Forwarded-* 生成上传 URL，避免返回配置里的另一域名导致 POST 走 CDN 却未透传 token
	apiBase := wkutil.RequestAPIBaseURL(c.Request)
	if apiBase == "" {
		apiBase = f.ctx.GetConfig().External.APIBaseURL
	}
	baseURL := strings.TrimSuffix(apiBase, "/")

	var path string
	if Type(fileType) == TypeMomentCover {
		// 动态封面
		path = fmt.Sprintf("%s/file/upload?type=%s&path=/%s.png", baseURL, fileType, loginUID)
	} else if Type(fileType) == TypeSticker {
		// 自定义表情
		path = fmt.Sprintf("%s/file/upload?type=%s&path=/%s/%s.gif", baseURL, fileType, loginUID, util.GenerUUID())
	} else if Type(fileType) == TypeWorkplaceBanner {
		// 工作台横幅
		path = fmt.Sprintf("%s/file/upload?type=%s&path=/workplace/banner/%s", baseURL, fileType, uploadPath)
	} else if Type(fileType) == TypeWorkplaceAppIcon {
		// 工作台appIcon
		path = fmt.Sprintf("%s/file/upload?type=%s&path=/workplace/appicon/%s", baseURL, fileType, uploadPath)
	} else {
		path = fmt.Sprintf("%s/file/upload?type=%s&path=%s", baseURL, fileType, uploadPath)
	}
	c.Response(map[string]string{
		"url": path,
	})
}

// 上传文件
func (f *File) uploadFile(c *wkhttp.Context) {
	uploadPath := sanitizeUploadPath(c.Query("path"))
	fileType := c.Query("type")
	signature := c.Query("signature")
	var signatureInt int64 = 0
	if signature != "" {
		signatureInt, _ = strconv.ParseInt(signature, 10, 64)
	}
	contentType := c.DefaultPostForm("contenttype", "application/octet-stream")
	err := f.checkReq(Type(fileType), uploadPath)
	if err != nil {
		c.ResponseError(err)
		return
	}
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		f.Error("读取文件失败！", zap.Error(err))
		c.ResponseError(errors.New("读取文件失败！"))
		return
	}
	defer file.Close()

	var uploadReader io.ReadSeeker = file

	if isVideoExt(uploadPath) {
		processed, cleanup, fsErr := faststartProcess(file)
		if fsErr != nil {
			f.Warn("faststart 处理失败，使用原始文件", zap.String("path", uploadPath), zap.Error(fsErr))
			if _, seekErr := file.Seek(0, io.SeekStart); seekErr != nil {
				f.Error("回退 seek 失败", zap.Error(seekErr))
				c.ResponseError(errors.New("视频处理失败"))
				return
			}
		} else {
			defer cleanup()
			uploadReader = processed
		}
	}

	path := uploadPath
	if !strings.HasPrefix(path, "/") {
		path = fmt.Sprintf("/%s", path)
	}
	var sign []byte
	if signatureInt == 1 {
		h := sha512.New()
		_, err := io.Copy(h, uploadReader)
		if err != nil {
			f.Error("签名复制文件错误", zap.Error(err))
			c.ResponseError(errors.New("签名复制文件错误"))
			return
		}
		sign = h.Sum(nil)
	}
	_, err = f.service.UploadFile(fmt.Sprintf("%s%s", fileType, path), contentType, func(w io.Writer) error {
		_, err := uploadReader.Seek(0, io.SeekStart)
		if err != nil {
			f.Error("设置文件偏移量错误", zap.Error(err))
			return err
		}
		_, err = io.Copy(w, uploadReader)
		return err
	})
	if err != nil {
		f.Error("上传文件失败！", zap.Error(err))
		c.ResponseError(errors.New("上传文件失败！"))
		return
	}
	relativePath := fmt.Sprintf("file/preview/%s%s", fileType, path)
	absoluteURL := wkutil.FullAPIURL(f.ctx.GetConfig().External.APIBaseURL, relativePath)
	if signatureInt == 1 {
		encoded := base64.StdEncoding.EncodeToString(sign[:])
		c.Response(map[string]interface{}{
			"path":   relativePath,
			"url":    absoluteURL,
			"sha512": encoded,
		})
	} else {
		c.Response(map[string]string{
			"path": relativePath,
			"url":  absoluteURL,
		})
	}
}

// 获取文件
func (f *File) getFile(c *wkhttp.Context) {
	ph := c.Param("path")
	if ph == "" {
		c.JSON(http.StatusBadRequest, gin.H{"msg": "访问路径不能为空", "status": http.StatusBadRequest})
		return
	}
	if dec, err := url.PathUnescape(ph); err == nil {
		ph = dec
	}
	ph = strings.TrimPrefix(strings.TrimSpace(ph), "/")
	// 仅折叠重复段：/v1/file/preview/file/preview/sticker/... 误拼
	for strings.Contains(ph, "file/preview/file/preview/") {
		ph = strings.Replace(ph, "file/preview/file/preview/", "file/preview/", 1)
	}
	// 兼容历史 path：file/file/preview/...（数据库里遗留的双 file 前缀）
	for strings.Contains(ph, "file/file/preview/") {
		ph = strings.Replace(ph, "file/file/preview/", "file/preview/", 1)
	}
	// 禁止再 Strip 掉唯一的 file/preview/：否则 file/preview/sticker/... 会变成 sticker/...，
	// MinIO 会把第一段当成桶名（sticker），对象键错位 → HEAD/GET 失败，安卓 Glide 显示蓝色占位块。
	if strings.HasPrefix(ph, "preview/") {
		ph = "file/" + ph
	} else if !strings.HasPrefix(ph, "file/") {
		// 常见：请求 /v1/file/preview/sticker/duck/cover.png，param 为 sticker/duck/cover.png
		ph = "file/preview/" + ph
	}
	if strings.HasPrefix(ph, "avatar/") || strings.HasPrefix(ph, "file/preview/avatar/") || strings.Contains(ph, "/avatar/default/") {
		f.Info("file preview request", zap.String("method", c.Request.Method), zap.String("path", ph), zap.String("range", c.GetHeader("Range")))
	}
	filename := c.Query("filename")
	if filename == "" {
		paths := strings.Split(ph, "/")
		if len(paths) > 0 {
			filename = paths[len(paths)-1]
		}
	}
	// Range 请求（视频 seek、断点续传等）必须走 HTTP 回退路径，
	// 因为 MinIO SDK GetObject 不支持 HTTP Range 语义。
	hasRange := c.GetHeader("Range") != ""

	// HEAD：Glide/OkHttp 等会先探测资源；仅注册 GET 时 Gin 对 HEAD 返回 404，导致客户端显示占位色块。
	if c.Request.Method == http.MethodHead && !hasRange {
		if size, ctHead, errSt := f.service.StatObjectDirect(ph); errSt == nil {
			if previewPathExpectsRasterImage(ph) {
				if inf := contentTypeByObjectKey(ph); inf != "" {
					ctHead = inf
				}
			} else if inf := contentTypeByObjectKey(ph); inf != "" {
				ctHead = inf
			}
			if ctHead != "" {
				c.Writer.Header().Set("Content-Type", ctHead)
			}
			setPreviewContentDisposition(c, ph)
			if size > 0 {
				c.Writer.Header().Set("Content-Length", strconv.FormatInt(size, 10))
			}
			c.Writer.Header().Set("Accept-Ranges", "bytes")
			c.Writer.Header().Set("Cache-Control", "public, max-age=86400")
			c.Writer.WriteHeader(http.StatusOK)
			return
		}
	}

	// MinIO：优先 SDK 直连读对象，避免 DownloadURL 公网/空配置导致容器内 http.Get 失败。
	// 但跳过 Range 请求——SDK GetObject 不支持 HTTP Range 语义，视频 seek / 断点续传需走 HTTP 回退。
	// HEAD 已在上方尝试 StatObjectDirect；此处不可再 OpenObjectDirect，否则会读完整对象体（违反 HEAD 语义）。
	if !hasRange && c.Request.Method != http.MethodHead {
		if rc, ct, objSize, errDirect := f.service.OpenObjectDirect(ph); errDirect == nil && rc != nil {
			defer rc.Close()
			br := bufio.NewReaderSize(rc, 512)
			head, _ := br.Peek(512)
			if len(head) > 0 && wkutil.LooksLikeS3OrHTTPErrorBody(head) {
				f.Warn("preview 对象体疑似错误文档", zap.String("path", ph))
				c.String(http.StatusBadGateway, "object store returned an error document")
				return
			}
			if previewPathExpectsRasterImage(ph) && len(head) > 0 && !wkutil.IsRasterImageMagic(head) {
				f.Warn("preview 期望位图但魔数异常", zap.String("path", ph))
				c.String(http.StatusBadGateway, "object is not a valid image")
				return
			}
			if previewPathExpectsRasterImage(ph) {
				if m := wkutil.RasterImageContentType(head); m != "" {
					ct = m
				} else if inf := contentTypeByObjectKey(ph); inf != "" {
					ct = inf
				}
			} else if inf := contentTypeByObjectKey(ph); inf != "" {
				ct = inf
			}
			if ct != "" {
				c.Writer.Header().Set("Content-Type", ct)
			}
			setPreviewContentDisposition(c, ph)
			if objSize > 0 {
				c.Writer.Header().Set("Content-Length", strconv.FormatInt(objSize, 10))
			}
			c.Writer.Header().Set("Accept-Ranges", "bytes")
			c.Writer.Header().Set("Cache-Control", "public, max-age=86400")
			c.Writer.WriteHeader(http.StatusOK)
			_, _ = io.Copy(c.Writer, br)
			return
		} else if strings.HasPrefix(ph, "avatar/") || strings.HasPrefix(ph, "file/preview/avatar/") || strings.Contains(ph, "/avatar/default/") {
			f.Warn("OpenObjectDirect failed", zap.String("path", ph), zap.Error(errDirect))
		}
	}

	downloadURL, err := f.service.DownloadURLForServerFetch(ph, filename)
	if err != nil {
		f.Warn("构建预览拉流地址失败", zap.String("path", ph), zap.Error(err))
		c.String(http.StatusBadGateway, err.Error())
		return
	}

	// 移动端（安卓）有时不方便处理 302 跳转到 minio/oss（尤其是跳到 http 资源时会触发 cleartext 限制）。
	// 因此这里改为：后端直接抓取 downloadURL 的内容，然后以 200/二进制流返回给客户端。
	upstreamMethod := http.MethodGet
	if c.Request.Method == http.MethodHead {
		upstreamMethod = http.MethodHead
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), upstreamMethod, downloadURL, nil)
	if err != nil {
		c.String(http.StatusBadGateway, err.Error())
		return
	}
	// 透传 Range，支持播放器边下边播/断点续传（Minio 可能返回 206）。
	if r := c.GetHeader("Range"); r != "" {
		req.Header.Set("Range", r)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.String(http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if strings.HasPrefix(ph, "avatar/") || strings.HasPrefix(ph, "file/preview/avatar/") || strings.Contains(ph, "/avatar/default/") {
			f.Warn("file preview upstream non-2xx", zap.String("path", ph), zap.Int("status", resp.StatusCode), zap.String("downloadURL", downloadURL))
		}
		c.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		c.Writer.WriteHeader(resp.StatusCode)
		if c.Request.Method != http.MethodHead {
			_, _ = io.CopyN(c.Writer, resp.Body, 1024)
		}
		return
	}

	// 透传长度与缓存等；Content-Type 见下方按扩展名修正（不从 MinIO 原样透传）。
	for _, h := range []string{
		"Content-Length",
		"Content-Range",
		"Accept-Ranges",
		"Cache-Control",
		"ETag",
		"Last-Modified",
	} {
		if v := resp.Header.Get(h); v != "" {
			c.Writer.Header().Set(h, v)
		}
	}
	// MinIO 通过 HTTP 拉取时，对象元数据常为 application/octet-stream；与 OpenObjectDirect 分支不一致时，
	// Glide/Fresco/系统会按「流」解码位图 → 花屏或蓝块。按逻辑路径扩展名强制与 contentTypeByObjectKey 对齐。
	if inf := contentTypeByObjectKey(ph); inf != "" {
		c.Writer.Header().Set("Content-Type", inf)
	} else if ct := resp.Header.Get("Content-Type"); ct != "" {
		c.Writer.Header().Set("Content-Type", ct)
	}
	// 不用 MinIO 回传的 Content-Disposition（可能受 URL 上 filename 影响），统一按逻辑路径真实文件名
	setPreviewContentDisposition(c, ph)

	// 关键：透传下游状态码（例如 206 Partial Content），避免播放器误判。
	c.Writer.WriteHeader(resp.StatusCode)
	if c.Request.Method != http.MethodHead {
		_, _ = io.Copy(c.Writer, resp.Body)
	}
}

func (f *File) checkReq(fileType Type, path string) error {
	if fileType == "" {
		return errors.New("文件类型不能为空")
	}
	if path == "" && fileType != TypeMomentCover && fileType != TypeSticker {
		return errors.New("上传路径不能为空")
	}
	if fileType != TypeChat && fileType != TypeMoment && fileType != TypeMomentCover && fileType != TypeSticker && fileType != TypeReport && fileType != TypeChatBg && fileType != TypeCommon && fileType != TypeDownload {
		return errors.New("文件类型错误")
	}
	return nil
}
