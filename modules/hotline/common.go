package hotline

import (
	"net/url"
	"strings"

	"go.uber.org/zap"
)

func (h *Hotline) GetSourceAndSearchKeyword(referrer string) (string, string) {
	referrerURL, err := url.Parse(referrer)
	if err != nil {
		h.Warn("解析ReferrerURL失败！", zap.Error(err))
	}
	if referrerURL == nil {
		return "", ""
	}

	var source string
	var keyword string
	if strings.Contains(referrer, "baidu.com") {
		source = "百度"
		keyword = referrerURL.Query().Get("wd")
	} else if strings.Contains(referrer, "sogou.com") {
		source = "搜狗"
		keyword = referrerURL.Query().Get("query")
	} else if strings.Contains(referrer, "so.com") {
		source = "360搜索"
		keyword = referrerURL.Query().Get("q")
	} else if strings.Contains(referrer, "google.com") {
		source = "google"
	} else if strings.Contains(referrer, "bing.com") {
		source = "必应"
	} else if strings.Contains(referrer, "sm.cn") {
		source = "UC搜索"
	} else {
		source = "官网"
	}
	return source, keyword
}
