package message

import (
	"net/http"
	"strconv"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

// prohibitWordsVersionSeqKey 违禁词版本序列，供后台增删与客户端增量同步共用。
const prohibitWordsVersionSeqKey = "prohibit_words:version"

type prohibitWordSyncItem struct {
	ID        int64  `db:"id" json:"id"`
	Content   string `db:"content" json:"content"`
	IsRegex   int    `db:"is_regex" json:"is_regex"`
	IsDeleted int    `db:"is_deleted" json:"is_deleted"`
	Version   int64  `db:"version" json:"version"`
	CreatedAt string `db:"created_at" json:"created_at"`
}

// prohibitWordsSync 客户端拉取违禁词增量（与 Web ProhibitwordsService.sync 一致）。
// version=0 时返回全量；version>0 时仅返回 version 更大的记录。
func (m *Message) prohibitWordsSync(c *wkhttp.Context) {
	since, _ := strconv.ParseInt(c.Query("version"), 10, 64)
	q := m.ctx.DB().Select("*").From("prohibit_words")
	if since > 0 {
		q = q.Where("version > ?", since)
	}
	var list []*prohibitWordSyncItem
	_, err := q.OrderDir("version", true).OrderDir("id", true).Load(&list)
	if err != nil {
		m.Error("同步违禁词失败", zap.Error(err))
		c.ResponseError(errors.New("同步违禁词失败"))
		return
	}
	if list == nil {
		list = []*prohibitWordSyncItem{}
	}
	c.JSON(http.StatusOK, list)
}
