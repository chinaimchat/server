package hotline

import (
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/db"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/gocraft/dbr/v2"
)

type infoCategoryDB struct {
	ctx *config.Context
	db  *dbr.Session
}

func newInfoCategoryDB(ctx *config.Context) *infoCategoryDB {
	return &infoCategoryDB{
		ctx: ctx,
		db:  ctx.DB(),
	}
}

func (i *infoCategoryDB) insert(m *infoCategoryModel) error {
	_, err := i.db.InsertInto("hotline_info_category").Columns(util.AttrToUnderscore(m)...).Record(m).Exec()
	return err
}

// 查询我的类别
func (i *infoCategoryDB) queryMyWithAppID(uid string, appID string) ([]*infoCategoryModel, error) {
	var models []*infoCategoryModel
	_, err := i.db.Select("*").From("hotline_info_category").Where("app_id=? and (creater=? or share=1)", appID, uid).Load(&models)
	return models, err
}

type infoCategoryModel struct {
	AppID        string
	CategoryNo   string
	CategoryName string
	Creater      string
	Share        int
	db.BaseModel
}
