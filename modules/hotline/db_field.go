package hotline

import (
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/db"
	"github.com/gocraft/dbr/v2"
)

type fieldDB struct {
	ctx *config.Context
	db  *dbr.Session
}

func newFieldDB(ctx *config.Context) *fieldDB {
	return &fieldDB{
		ctx: ctx,
		db:  ctx.DB(),
	}
}

func (f *fieldDB) queryAll() ([]*fieldModel, error) {
	var models []*fieldModel
	_, err := f.db.Select("*").From("hotline_field").Load(&models)
	return models, err
}

type fieldModel struct {
	Field      string
	Name       string
	Group      string
	Type       string
	Datasource string
	Options    string
	Symbols    string
	db.BaseModel
}
