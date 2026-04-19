package hotline

import (
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/db"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/gocraft/dbr/v2"
)

type historyDB struct {
	ctx *config.Context
	db  *dbr.Session
}

func newHistoryDB(ctx *config.Context) *historyDB {
	return &historyDB{
		ctx: ctx,
		db:  ctx.DB(),
	}
}

func (h *historyDB) insertTx(m *historyModel, tx *dbr.Tx) error {
	_, err := tx.InsertInto("hotline_history").Columns(util.AttrToUnderscore(m)...).Record(m).Exec()
	return err
}

func (h *historyDB) insert(m *historyModel) error {
	_, err := h.db.InsertInto("hotline_history").Columns(util.AttrToUnderscore(m)...).Record(m).Exec()
	return err
}

// 查询 查询指定条数的历史记录
func (h *historyDB) queryRecent(count uint64, vid, appID string) ([]*historyModel, error) {
	var models []*historyModel
	_, err := h.db.Select("*").From("hotline_history").Where("vid=? and app_id=?", vid, appID).OrderDesc("created_at").Limit(count).Load(&models)
	return models, err
}

// 查询第一条历史
func (h *historyDB) queryFirst(vid, appID string) (*historyModel, error) {
	var m *historyModel
	_, err := h.db.Select("*").From("hotline_history").Where("vid=? and app_id=?", vid, appID).OrderAsc("created_at").Limit(1).Load(&m)
	return m, err
}
func (h *historyDB) queryLast(vid, appID string) (*historyModel, error) {
	var m *historyModel
	_, err := h.db.Select("*").From("hotline_history").Where("vid=? and app_id=?", vid, appID).OrderDesc("created_at").Limit(1).Load(&m)
	return m, err
}

type historyModel struct {
	AppID     string
	VID       string
	SiteURL   string
	SiteTitle string
	Referrer  string
	db.BaseModel
}
