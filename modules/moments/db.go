package moments

import (
	"hash/crc32"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	d "github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/db"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/gocraft/dbr/v2"
)

// db db
type db struct {
	ctx     *config.Context
	session *dbr.Session
}

// newDB New
func newDB(ctx *config.Context) *db {
	return &db{
		ctx:     ctx,
		session: ctx.DB(),
	}
}

// 添加朋友圈
func (d *db) insertTx(m *model, tx *dbr.Tx) error {
	_, err := tx.InsertInto(d.getTableName(m.MomentNo)).Columns(util.AttrToUnderscore(m)...).Record(m).Exec()
	return err
}

// 添加朋友圈
func (d *db) insert(m *model) error {
	_, err := d.session.InsertInto(d.getTableName(m.MomentNo)).Columns(util.AttrToUnderscore(m)...).Record(m).Exec()
	return err
}

// 删除朋友圈
func (d *db) deleteTx(uid, momentNo string, tx *dbr.Tx) error {
	_, err := tx.DeleteFrom(d.getTableName(momentNo)).Where("moment_no=? and publisher=?", momentNo, uid).Exec()
	return err
}

// 查询某条动态
func (d *db) queryWithMomentNo(momentNo string) (*model, error) {
	var moment *model
	_, err := d.session.Select("*").From(d.getTableName(momentNo)).Where("moment_no=?", momentNo).Load(&moment)
	return moment, err
}

// 朋友圈列表
func (d *db) list(tableName string, momentNos []string) ([]*model, error) {
	var moments []*model
	_, err := d.session.Select("*").From(tableName).Where("moment_no in ?", momentNos).Load(&moments)
	return moments, err
}

// 查看某个人的动态列表
func (d *db) listWithUID(tableName string, loginUID string, momentNos []string) ([]*model, error) {
	var moments []*model
	_, err := d.session.Select("*").From(tableName).Where("moment_no in ? and (privacy_type='public' or (privacy_type='internal' and find_in_set(?, privacy_uids)) or (privacy_type='prohibit' and privacy_uids not like ?))", momentNos, loginUID, "%"+loginUID+"%").Load(&moments)
	return moments, err
}

// 查看某个人只含图片的列表
func (d *db) listWithUIDAndImgs(tableName string, uid string, loginUID string) ([]*model, error) {
	var moments []*model
	// _, err := d.session.SelectBySql("select * from ? where publisher=? and (imgs <> '' or video_cover_path <> '') and (privacy_type='public' or (privacy_type='internal' and find_in_set(?, privacy_uids)) or (privacy_type='prohibit' and privacy_uids not like ?)) order by created_at desc limit 4", tableName, uid, loginUID, "%"+loginUID+"%").Load(&moments)
	_, err := d.session.Select("*").From(tableName).Where("(imgs<>'' or video_cover_path <>'') and publisher=? and (privacy_type='public' or (privacy_type='internal' and find_in_set(?, privacy_uids)) or (privacy_type='prohibit' and privacy_uids not like ?))", uid, loginUID, "%"+loginUID+"%").OrderBy("created_at desc").Limit(4).Load(&moments)
	return moments, err
}

// 获取用户所在的表名称
func (d *db) getTableName(momentNo string) string {
	tableID := crc32.ChecksumIEEE([]byte(momentNo)) % 3
	if tableID == 0 {
		return "moments1"
	} else if tableID == 1 {
		return "moments2"
	} else {
		return "moments3"
	}
}

// 朋友圈对象
type model struct {
	MomentNo       string //朋友圈编号
	Publisher      string //发布者UID
	PublisherName  string //发布者名称
	VideoPath      string //视频地址
	VideoCoverPath string //视频封面
	Content        string //发布内容
	Imgs           string //图片集合
	PrivacyType    string //隐私类型 【public：公开】【private：私有】【internal：部分可见】【prohibit：不给谁看】
	PrivacyUids    string //部分可见和不给谁看对应的用户集合
	Address        string //地址
	Longitude      string //经度
	Latitude       string //纬度
	RemindUids     string //提醒谁看用户集合
	d.BaseModel
}
