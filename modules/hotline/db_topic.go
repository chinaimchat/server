package hotline

import (
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/db"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/gocraft/dbr/v2"
)

type topicDB struct {
	ctx *config.Context
	db  *dbr.Session
}

func newTopicDB(ctx *config.Context) *topicDB {
	return &topicDB{
		ctx: ctx,
		db:  ctx.DB(),
	}
}

func (t *topicDB) queryWithAppID(appID string) ([]*topicModel, error) {
	var models []*topicModel
	_, err := t.db.Select("*").From("hotline_topic").Where("app_id=? and is_deleted=0", appID).Load(&models)
	return models, err
}

func (t *topicDB) queryWithIDAndAppID(id int64, appID string) (*topicModel, error) {
	var m *topicModel
	_, err := t.db.Select("*").From("hotline_topic").Where("app_id=? and id=? and is_deleted=0", appID, id).Load(&m)
	return m, err
}

func (t *topicDB) insertTx(m *topicModel, tx *dbr.Tx) error {
	_, err := tx.InsertInto("hotline_topic").Columns(util.AttrToUnderscore(m)...).Record(m).Exec()
	return err
}
func (t *topicDB) insert(m *topicModel) error {
	_, err := t.db.InsertInto("hotline_topic").Columns(util.AttrToUnderscore(m)...).Record(m).Exec()
	return err
}

func (t *topicDB) delete(id int64, appID string) error {
	_, err := t.db.Update("hotline_topic").Set("is_deleted", 1).Where("id=? and app_id=?", id, appID).Exec()
	return err
}

func (t *topicDB) queryPageWithAppID(appID string, pageIndex uint64, pageSize uint64) ([]*topicModel, error) {
	var models []*topicModel
	_, err := t.db.Select("*").From("hotline_topic").Where("app_id=? and is_deleted=0", appID).Offset((pageIndex - 1) * pageSize).Limit(pageSize).Load(&models)
	return models, err
}

// 查询默认话题
func (t *topicDB) queryDefaultWithAppID(appID string) (*topicModel, error) {
	var m *topicModel
	_, err := t.db.Select("*").From("hotline_topic").Where("app_id=? and is_default=1", appID).Limit(1).Load(&m)
	return m, err
}

func (t *topicDB) queryCountWithAppID(appID string) (int64, error) {
	var cn int64
	_, err := t.db.Select("count(*)").From("hotline_topic").Where("app_id=? and is_deleted=0", appID).Load(&cn)
	return cn, err
}

func (t *topicDB) update(m *topicModel) error {
	_, err := t.db.Update("hotline_topic").SetMap(map[string]interface{}{
		"title":   m.Title,
		"welcome": m.Welcome,
	}).Where("id=?", m.Id).Exec()
	return err
}

type topicModel struct {
	AppID     string
	Title     string
	Welcome   string
	IsDefault int
	IsDeleted int
	db.BaseModel
}
