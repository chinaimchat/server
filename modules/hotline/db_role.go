package hotline

import (
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/db"
	"github.com/gocraft/dbr/v2"
)

type roleDB struct {
	ctx *config.Context
	db  *dbr.Session
}

func newRoleDB(ctx *config.Context) *roleDB {
	return &roleDB{
		ctx: ctx,
		db:  ctx.DB(),
	}
}

func (r *roleDB) queryRolesWithAppID(appID string) ([]*roleModel, error) {
	var models []*roleModel
	_, err := r.db.Select("*").From("hotline_role").Where("app_id=?", appID).Load(&models)
	return models, err
}

type roleModel struct {
	AppID  string
	Role   string
	Remark string
	db.BaseModel
}
