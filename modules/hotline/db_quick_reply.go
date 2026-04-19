package hotline

import (
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/db"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/gocraft/dbr/v2"
)

type quickReplyDB struct {
	ctx *config.Context
	db  *dbr.Session
}

func newQuickReplyDB(ctx *config.Context) *quickReplyDB {
	return &quickReplyDB{
		ctx: ctx,
		db:  ctx.DB(),
	}
}

func (q *quickReplyDB) insert(m *quickReplyModel) error {
	_, err := q.db.InsertInto("hotline_quick_reply").Columns(util.AttrToUnderscore(m)...).Record(m).Exec()
	return err
}

func (q *quickReplyDB) queryPageWithCategoryNo(categoryNo string, appID string, pageIndex, pageSize uint64) ([]*quickReplyModel, error) {
	var models []*quickReplyModel
	_, err := q.db.Select("*").From("hotline_quick_reply").Where("category_no=? and app_id=?", categoryNo, appID).Offset((pageIndex - 1) * pageSize).Limit(pageSize).Load(&models)
	return models, err
}

// 查询我可见的快捷回复
func (q *quickReplyDB) queryMyWithAppID(uid string, appID string) ([]*quickReplyDetailModel, error) {
	var models []*quickReplyDetailModel
	_, err := q.db.Select("hotline_quick_reply.*,hotline_info_category.category_name,hotline_info_category.share").From("hotline_quick_reply").
		Join("hotline_info_category", "hotline_quick_reply.app_id=hotline_info_category.app_id and hotline_quick_reply.category_no=hotline_info_category.category_no").
		Where("hotline_quick_reply.app_id=? and (hotline_info_category.creater=? or hotline_info_category.share=1)", appID, uid).Load(&models)
	return models, err
}

func (q *quickReplyDB) deleteWithIDAndAppID(id int64, appID string) error {
	_, err := q.db.DeleteFrom("hotline_quick_reply").Where("id=? and app_id=?", id, appID).Exec()
	return err
}

type quickReplyDetailModel struct {
	quickReplyModel
	CategoryName string
}

type quickReplyModel struct {
	AppID      string
	Title      string
	Content    string
	CategoryNo string
	Shortcode  string
	Creater    string
	db.BaseModel
}
