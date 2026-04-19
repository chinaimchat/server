package util

import "bytes"

// IsRasterImageMagic 根据文件头判断是否为常见位图（PNG/JPEG/GIF/WebP），
// 用于避免把 XML/JSON 错误体或空响应在已设 image/* 头的情况下写给客户端。
func IsRasterImageMagic(b []byte) bool {
	if len(b) < 3 {
		return false
	}
	// PNG
	if len(b) >= 8 && b[0] == 0x89 && b[1] == 0x50 && b[2] == 0x4E && b[3] == 0x47 && b[4] == 0x0D && b[5] == 0x0A && b[6] == 0x1A && b[7] == 0x0A {
		return true
	}
	// JPEG
	if b[0] == 0xFF && b[1] == 0xD8 {
		return true
	}
	// GIF
	if len(b) >= 6 && (string(b[0:6]) == "GIF87a" || string(b[0:6]) == "GIF89a") {
		return true
	}
	// WebP: RIFF....WEBP
	if len(b) >= 12 && string(b[0:4]) == "RIFF" && string(b[8:12]) == "WEBP" {
		return true
	}
	return false
}

// RasterImageContentType 按文件头返回 MIME；无法识别返回空字符串。
// 必须与 IsRasterImageMagic 判定范围一致，用于避免「路径是 .png、实际是 JPEG」时仍发 Content-Type: image/png 导致 Skia unimplemented。
func RasterImageContentType(b []byte) string {
	if len(b) < 3 {
		return ""
	}
	if len(b) >= 8 && b[0] == 0x89 && b[1] == 0x50 && b[2] == 0x4E && b[3] == 0x47 && b[4] == 0x0D && b[5] == 0x0A && b[6] == 0x1A && b[7] == 0x0A {
		return "image/png"
	}
	if b[0] == 0xFF && b[1] == 0xD8 {
		return "image/jpeg"
	}
	if len(b) >= 6 && (string(b[0:6]) == "GIF87a" || string(b[0:6]) == "GIF89a") {
		return "image/gif"
	}
	if len(b) >= 12 && string(b[0:4]) == "RIFF" && string(b[8:12]) == "WEBP" {
		return "image/webp"
	}
	return ""
}

// LooksLikeS3OrHTTPErrorBody 判断首字节是否像 S3/MinIO XML 错误体或 JSON 错误（避免 200+假图片）。
func LooksLikeS3OrHTTPErrorBody(b []byte) bool {
	b = bytes.TrimSpace(b)
	if len(b) == 0 {
		return true
	}
	if bytes.HasPrefix(b, []byte("<?xml")) || bytes.HasPrefix(b, []byte("<Error")) {
		return true
	}
	if b[0] == '{' || b[0] == '[' {
		return true
	}
	return false
}
