package hotline

import (
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/db"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/gocraft/dbr/v2"
)

type deviceDB struct {
	ctx *config.Context
	db  *dbr.Session
}

func newDeviceDB(ctx *config.Context) *deviceDB {
	return &deviceDB{
		ctx: ctx,
		db:  ctx.DB(),
	}
}

func (d *deviceDB) insertTx(m *deviceModel, tx *dbr.Tx) error {
	_, err := tx.InsertInto("hotline_device").Columns(util.AttrToUnderscore(m)...).Record(m).Exec()
	return err
}

func (d *deviceDB) queryWithVIDAndAppID(vid string, appID string) (*deviceModel, error) {
	var m *deviceModel
	_, err := d.db.Select("*").From("hotline_device").Where("vid=? and app_id=?", vid, appID).Load(&m)
	return m, err
}
func (d *deviceDB) queryWithVID(vid string) (*deviceModel, error) {
	var m *deviceModel
	_, err := d.db.Select("*").From("hotline_device").Where("vid=?", vid).Load(&m)
	return m, err
}

type deviceModel struct {
	AppID   string
	VID     string
	Device  string
	OS      string
	Model   string
	Version string
	db.BaseModel
}
