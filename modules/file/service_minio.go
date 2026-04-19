package file

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/log"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/zap"
)

// ServiceMinio 文件上传
type ServiceMinio struct {
	log.Log
	ctx            *config.Context
	downloadClient *http.Client

	// 用于避免每次 DownloadURL 都重复 SetBucketPolicy（昂贵的网络调用）
	policyMu      sync.Mutex
	policyEnsured map[string]bool
}

// NewServiceMinio NewServiceMinio
func NewServiceMinio(ctx *config.Context) *ServiceMinio {
	return &ServiceMinio{
		Log: log.NewTLog("File"),
		ctx: ctx,
		downloadClient: &http.Client{
			Timeout: time.Second * 30,
		},
		policyEnsured: make(map[string]bool),
	}
}

func (sm *ServiceMinio) ensureBucketPolicy(bucketName string) error {
	sm.policyMu.Lock()
	already := sm.policyEnsured[bucketName]
	sm.policyMu.Unlock()
	if already {
		return nil
	}

	minioConfig := sm.ctx.GetConfig().Minio

	uploadUl, _ := url.Parse(minioConfig.UploadURL)
	endpoint := uploadUl.Host
	accessKeyID := minioConfig.AccessKeyID
	secretAccessKey := minioConfig.SecretAccessKey
	useSSL := false
	if strings.HasPrefix(uploadUl.Scheme, "https") {
		useSSL = true
	}

	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return err
	}

	// 匿名仅允许按已知 key 读对象（s3:GetObject）。不上开放 ListBucket，避免枚举桶内文件。
	// 上传、删除走带 AccessKey 的 SDK，由 MinIO 用户/服务账户策略授权。
	policy := `{
		"Version": "2012-10-17",
		"Statement": [{
			"Effect": "Allow",
			"Principal": {"AWS": ["*"]},
			"Action": ["s3:GetObject"],
			"Resource": ["arn:aws:s3:::%s/*"]
		}]
	}`
	err = minioClient.SetBucketPolicy(context.Background(), bucketName, fmt.Sprintf(policy, bucketName))
	if err != nil {
		return err
	}
	sm.policyMu.Lock()
	sm.policyEnsured[bucketName] = true
	sm.policyMu.Unlock()
	return nil
}

// minioObjectPathForPreview 将客户端请求的「逻辑路径」转为 UploadFile 写入 MinIO 时的 bucket+objectKey 形式。
// 上传约定见 api.uploadFile：physicalPath = type + path，例如 chat/1/uid/f.jpg → 桶 chat、对象键 1/uid/f.jpg；
// API 返回的 path 为 file/preview/<type>/...，若直接当作桶 file、键 preview/chat/... 会 NoSuchKey（Web/IM 图片 404）。
// 现网贴纸资源历史上存为：桶 file，下键 file/preview/sticker/...（注意多一层 file/ 前缀）。
// 因此 sticker 需单独映射，其他 chat/moment 仍按 kind/<sub>。
func minioObjectPathForPreview(ph string) string {
	ph = strings.TrimPrefix(strings.TrimSpace(ph), "/")
	if !strings.HasPrefix(ph, "file/preview/") {
		return ph
	}
	rest := strings.TrimPrefix(ph, "file/preview/")
	i := strings.IndexByte(rest, '/')
	if i <= 0 || i >= len(rest)-1 {
		return ph
	}
	kind := rest[:i]
	sub := rest[i+1:]
	switch kind {
	case "sticker":
		// 历史贴纸对象键：file/preview/sticker/...（bucket=file 后完整路径应为 file/file/preview/sticker/...）
		return "file/file/preview/sticker/" + sub
	case "chat", "moment", "momentcover", "report", "common", "chatbg", "download", "workplacebanner", "workplaceappicon":
		return kind + "/" + sub
	default:
		return ph
	}
}

// UploadFile 上传文件
func (sm *ServiceMinio) UploadFile(filePath string, contentType string, copyFileWriter func(io.Writer) error) (map[string]interface{}, error) {
	buff := bytes.NewBuffer(make([]byte, 0))
	err := copyFileWriter(buff)
	if err != nil {
		sm.Error("复制文件内容失败！", zap.Error(err))
		return nil, err
	}

	minioConfig := sm.ctx.GetConfig().Minio

	ctx := context.Background()
	uploadUl, _ := url.Parse(minioConfig.UploadURL)
	endpoint := uploadUl.Host
	accessKeyID := minioConfig.AccessKeyID
	secretAccessKey := minioConfig.SecretAccessKey
	useSSL := false

	if strings.HasPrefix(uploadUl.Scheme, "https") {
		useSSL = true
	}
	// 初使化minio client对象。
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		sm.Error("创建错误：", zap.Error(err))
		return nil, err
	}
	bucketName := "file"
	strs := strings.Split(filePath, "/")
	if len(strs) > 0 {
		bucketName = strs[0]
	}
	exists, err := minioClient.BucketExists(ctx, bucketName)
	if err != nil {
		sm.Error(fmt.Sprintf("检测 %s目录是否存在错误", bucketName))
		return nil, err
	}
	if !exists {
		err = minioClient.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{})
		if err != nil {
			sm.Error(fmt.Sprintf("创建 %s目录失败", bucketName))
			return nil, err
		}
	}
	if err := sm.ensureBucketPolicy(bucketName); err != nil {
		sm.Error("设置minio文件读写权限错误", zap.Error(err))
		return nil, err
	}

	fileName := strings.TrimPrefix(filePath, fmt.Sprintf("%s/", bucketName))
	n, err := minioClient.PutObject(ctx, bucketName, fileName, buff, int64(len(buff.Bytes())), minio.PutObjectOptions{ContentType: contentType, PartSize: 10 * 1024 * 1024})
	if err != nil {
		sm.Error("上传文件失败：", zap.Error(err))
		return map[string]interface{}{
			"path": "",
		}, err
	}
	return map[string]interface{}{
		"path": n.Key,
	}, err
}

// openObject 使用 SDK 直连 MinIO 读对象，避免 DownloadURL 配置错误或容器内无法访问公网地址导致预览失败。
// 返回 (reader, contentType, sizeBytes, error)
func (sm *ServiceMinio) openObject(ph string) (io.ReadCloser, string, int64, error) {
	raw := strings.TrimPrefix(strings.TrimSpace(ph), "/")
	if raw == "" {
		return nil, "", 0, fmt.Errorf("empty path")
	}

	// sticker 预览对象在 MinIO 上存在历史/现网两种 key 布局：
	// 1) 旧：file/file/preview/sticker/<sub>  （对应代码 minioObjectPathForPreview 的 legacy 映射）
	// 2) 新：file/preview/sticker/<sub>       （对应我们现有 miniodata 目录 file/preview/sticker/...）
	// 为兼容两种布局：先尝试 legacy key，不存在则回退到新 key。
	candidates := []string{minioObjectPathForPreview(raw)}
	if strings.HasPrefix(raw, "file/preview/sticker/") {
		sub := strings.TrimPrefix(raw, "file/preview/sticker/")
		if sub != "" {
			candidates = append(candidates, "file/preview/sticker/"+sub)
		}
	}

	minioConfig := sm.ctx.GetConfig().Minio
	uploadUl, err := url.Parse(minioConfig.UploadURL)
	if err != nil {
		return nil, "", 0, err
	}
	endpoint := uploadUl.Host
	accessKeyID := minioConfig.AccessKeyID
	secretAccessKey := minioConfig.SecretAccessKey
	useSSL := strings.HasPrefix(uploadUl.Scheme, "https")

	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, "", 0, err
	}
	ctx := context.Background()

	var lastErr error
	for _, mapped := range candidates {
		if mapped == "" {
			continue
		}
		parts := strings.Split(mapped, "/")
		if len(parts) < 2 {
			lastErr = fmt.Errorf("invalid object path")
			continue
		}
		bucketName := parts[0]
		objectKey := strings.Join(parts[1:], "/")

		if err := sm.ensureBucketPolicy(bucketName); err != nil {
			sm.Warn("ensureBucketPolicy", zap.String("bucket", bucketName), zap.Error(err))
		}

		obj, err := minioClient.GetObject(ctx, bucketName, objectKey, minio.GetObjectOptions{})
		if err != nil {
			lastErr = err
			continue
		}
		st, statErr := obj.Stat()
		if statErr != nil {
			_ = obj.Close()
			lastErr = statErr
			continue
		}
		ct := "application/octet-stream"
		if st.ContentType != "" {
			ct = st.ContentType
		}
		return obj, ct, st.Size, nil
	}

	return nil, "", 0, lastErr
}

// statObject 仅 Stat，供 HEAD /v1/file/preview/ 使用（安卓 Glide 等会先 HEAD 再 GET）。
func (sm *ServiceMinio) statObject(ph string) (int64, string, error) {
	minioConfig := sm.ctx.GetConfig().Minio
	raw := strings.TrimPrefix(strings.TrimSpace(ph), "/")
	if raw == "" {
		return 0, "", fmt.Errorf("empty path")
	}

	// 与 openObject 同理：sticker 预览 key 兼容 legacy/new 两种布局。
	candidates := []string{minioObjectPathForPreview(raw)}
	if strings.HasPrefix(raw, "file/preview/sticker/") {
		sub := strings.TrimPrefix(raw, "file/preview/sticker/")
		if sub != "" {
			candidates = append(candidates, "file/preview/sticker/"+sub)
		}
	}

	uploadUl, err := url.Parse(minioConfig.UploadURL)
	if err != nil {
		return 0, "", err
	}
	endpoint := uploadUl.Host
	accessKeyID := minioConfig.AccessKeyID
	secretAccessKey := minioConfig.SecretAccessKey
	useSSL := strings.HasPrefix(uploadUl.Scheme, "https")

	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return 0, "", err
	}

	ctx := context.Background()
	var lastErr error
	for _, mapped := range candidates {
		if mapped == "" {
			continue
		}
		parts := strings.Split(mapped, "/")
		if len(parts) < 2 {
			lastErr = fmt.Errorf("invalid object path")
			continue
		}
		bucketName := parts[0]
		objectKey := strings.Join(parts[1:], "/")

		if err := sm.ensureBucketPolicy(bucketName); err != nil {
			sm.Warn("ensureBucketPolicy", zap.String("bucket", bucketName), zap.Error(err))
		}

		st, err := minioClient.StatObject(ctx, bucketName, objectKey, minio.StatObjectOptions{})
		if err != nil {
			lastErr = err
			continue
		}
		ct := "application/octet-stream"
		if st.ContentType != "" {
			ct = st.ContentType
		}
		return st.Size, ct, nil
	}

	return 0, "", lastErr
}

func (sm *ServiceMinio) DownloadURL(ph string, filename string) (string, error) {
	return sm.buildMinioGETURL(ph, filename, false)
}

// DownloadURLForServerFetch 仅用于服务端容器内向 MinIO 拉对象；基址固定为 UploadURL，避免公网 DownloadURL 在容器内不可达。
func (sm *ServiceMinio) DownloadURLForServerFetch(ph string, filename string) (string, error) {
	return sm.buildMinioGETURL(ph, filename, true)
}

func (sm *ServiceMinio) buildMinioGETURL(ph string, filename string, forServerFetch bool) (string, error) {
	minioConfig := sm.ctx.GetConfig().Minio
	// ph 可能带前导 /（如 /avatar/default/xxx.jpg）。第一段须为 bucket 名，否则会把策略错误打到 file 桶导致 403。
	phClean := minioObjectPathForPreview(strings.TrimPrefix(strings.TrimSpace(ph), "/"))
	bucketName := "file"
	if parts := strings.Split(phClean, "/"); len(parts) > 0 && parts[0] != "" {
		bucketName = parts[0]
	}
	// 为避免历史桶没有读策略，确保在下载前完成 policy 注入
	_ = sm.ensureBucketPolicy(bucketName)

	vals := url.Values{}
	dispName := filename
	for _, r := range filename {
		if r > 127 {
			dispName = "preview"
			break
		}
	}
	// 与对象 key 扩展名一致：避免 uid.png 文件名 + test (n).jpg 对象导致类型/扩展名与解码器预期不符
	if kb := objectKeyBaseName(phClean); kb != "" && isASCIIFilename(kb) {
		switch strings.ToLower(filepath.Ext(kb)) {
		case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".lim":
			dispName = kb
		}
	}
	vals.Set("response-content-disposition", fmt.Sprintf("inline; filename=\"%s\"", dispName))

	var base string
	if forServerFetch {
		base = strings.TrimSuffix(strings.TrimSpace(minioConfig.UploadURL), "/")
	} else {
		// 给浏览器/客户端跳转或外链：优先对外 DownloadURL；未配置则与 UploadURL 一致
		base = strings.TrimSuffix(strings.TrimSpace(minioConfig.DownloadURL), "/")
		if base == "" {
			base = strings.TrimSuffix(strings.TrimSpace(minioConfig.UploadURL), "/")
		}
	}
	if base == "" {
		return "", fmt.Errorf("minio upload url is empty")
	}
	seg := strings.Split(phClean, "/")
	result, err := url.JoinPath(base, seg...)
	if err != nil {
		return "", err
	}
	// S3/MinIO：用查询参数覆盖响应 Content-Type，避免对象元数据错误（如误标 image/jpeg）与
	// response-content-disposition 里 *.png 文件名矛盾，导致客户端按错误类型解码失败。
	if ct := contentTypeByObjectKey(phClean); ct != "" {
		vals.Set("response-content-type", ct)
	}

	return fmt.Sprintf("%s?%s", result, vals.Encode()), nil
}

// contentTypeByObjectKey 根据对象路径扩展名推断 MIME（常见图片 + 聊天视频），
// 避免 MinIO 上误标为 application/octet-stream 时，客户端/Glide 用错误解码器或 MediaMetadataRetriever 解析失败。
func contentTypeByObjectKey(objectPath string) string {
	lower := strings.ToLower(objectPath)
	switch {
	case strings.HasSuffix(lower, ".png"):
		return "image/png"
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(lower, ".gif"):
		return "image/gif"
	case strings.HasSuffix(lower, ".webp"):
		return "image/webp"
	case strings.HasSuffix(lower, ".mp4"), strings.HasSuffix(lower, ".m4v"):
		return "video/mp4"
	case strings.HasSuffix(lower, ".mov"):
		return "video/quicktime"
	case strings.HasSuffix(lower, ".webm"):
		return "video/webm"
	case strings.HasSuffix(lower, ".3gp"):
		return "video/3gpp"
	case strings.HasSuffix(lower, ".lim"):
		// 矢量表情（RLottie）；桶里常被标成 gzip/octet-stream，与安卓端按后缀识别 lim 的逻辑对齐
		return "application/octet-stream"
	default:
		return ""
	}
}

func objectKeyBaseName(objectPath string) string {
	p := strings.Trim(strings.ReplaceAll(objectPath, "\\", "/"), "/")
	if p == "" {
		return ""
	}
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

func isASCIIFilename(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}
