package hotline

import (
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/db"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/gocraft/dbr/v2"
)

type configDB struct {
	ctx *config.Context
	db  *dbr.Session
}

func newConfigDB(ctx *config.Context) *configDB {
	return &configDB{
		ctx: ctx,
		db:  ctx.DB(),
	}
}

func (d *configDB) queryWithAppID(appID string) (*configModel, error) {
	var configModel *configModel
	_, err := d.db.Select("*").From("hotline_config").Where("app_id=?", appID).Load(&configModel)
	return configModel, err
}

func (d *configDB) insertTx(m *configModel, tx *dbr.Tx) error {
	_, err := tx.InsertInto("hotline_config").Columns(util.AttrToUnderscore(m)...).Record(m).Exec()
	return err
}

func (d *configDB) update(m *configModel) error {
	_, err := d.db.Update("hotline_config").SetMap(map[string]interface{}{
		"app_name": m.AppName,
		"logo":     m.Logo,
		"color":    m.Color,
		"chat_bg":  m.ChatBg,
	}).Where("app_id=?", m.AppID).Exec()
	return err
}

type configModel struct {
	AppID   string
	AppName string
	UID     string
	Logo    string
	Color   string
	ChatBg  string
	db.BaseModel
}
