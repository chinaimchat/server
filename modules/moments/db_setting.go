package moments

import (
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	d "github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/db"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/gocraft/dbr/v2"
)

type settingDB struct {
	ctx     *config.Context
	session *dbr.Session
}

func newSettingDB(ctx *config.Context) *settingDB {
	return &settingDB{
		ctx:     ctx,
		session: ctx.DB(),
	}
}

// 查询对某人的朋友圈设置
func (d *settingDB) queryWithUIDAndToUID(uid, toUID string) (*settingModel, error) {
	var m *settingModel
	_, err := d.session.Select("*").From("moment_setting").Where("uid=? and to_uid=?", uid, toUID).Load(&m)
	return m, err
}

// 添加
func (d *settingDB) insert(m *settingModel) error {
	_, err := d.session.InsertInto("moment_setting").Columns(util.AttrToUnderscore(m)...).Record(m).Exec()
	return err
}

// 查询某人的所有设置
func (d *settingDB) queryWithUID(uid string) ([]*settingModel, error) {
	var list []*settingModel
	_, err := d.session.Select("*").From("moment_setting").Where("uid=?", uid).Load(&list)
	return list, err
}

// 查询对某人的所有设置
func (d *settingDB) queryWithToUID(toUID string) ([]*settingModel, error) {
	var list []*settingModel
	_, err := d.session.Select("*").From("moment_setting").Where("to_uid=?", toUID).Load(&list)
	return list, err
}

// Update 更新群信息
func (d *settingDB) update(model *settingModel) error {
	_, err := d.session.Update("moment_setting").SetMap(map[string]interface{}{
		"is_hide_my":  model.IsHideMy,
		"is_hide_his": model.IsHideHis,
	}).Where("id=?", model.Id).Exec()
	return err
}

// 查询一批用户对某人的设置
func (d *settingDB) queryWithUIDsAndToUID(uids []string, toUID string) ([]*settingModel, error) {
	var list []*settingModel
	_, err := d.session.Select("*").From("moment_setting").Where("uid in ? and to_uid=?", uids, toUID).Load(&list)
	return list, err
}

type settingModel struct {
	UID       string
	ToUID     string
	IsHideMy  int
	IsHideHis int
	d.BaseModel
}
