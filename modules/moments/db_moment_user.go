package moments

import (
	"hash/crc32"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	d "github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/db"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/gocraft/dbr/v2"
)

// DB
type momentUserDB struct {
	ctx     *config.Context
	session *dbr.Session
}

// New
func newMomentUserDB(ctx *config.Context) *momentUserDB {
	return &momentUserDB{
		ctx:     ctx,
		session: ctx.DB(),
	}
}

// 添加
func (m *momentUserDB) insertTx(mu *momentUserModel, tx *dbr.Tx) (int64, error) {
	result, err := tx.InsertInto(m.getTableName(mu.UID)).Columns(util.AttrToUnderscore(mu)...).Record(mu).Exec()
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	return id, err
}

// 添加
func (m *momentUserDB) insert(mu *momentUserModel) (int64, error) {
	result, _ := m.session.InsertInto(m.getTableName(mu.UID)).Columns(util.AttrToUnderscore(mu)...).Record(mu).Exec()
	id, err := result.LastInsertId()
	return id, err
}

// 删除
func (m *momentUserDB) deleteTx(tableName, publisher, momentNo string, tx *dbr.Tx) error {
	_, err := tx.DeleteFrom(tableName).Where("publisher=? and moment_no=?", publisher, momentNo).Exec()
	return err
}

// 删除某个人的好友所有朋友圈
func (m *momentUserDB) deleteWithUIDAndPublisherTx(uid, publisher string, tx *dbr.Tx) error {
	_, err := tx.DeleteFrom(m.getTableName(uid)).Where("publisher=? and uid=?", publisher, uid).Exec()
	return err
}

// 分页查询动态好友关系
func (m *momentUserDB) queryWithUIDAndPage(uid string, page, pageSize uint64) ([]*momentUserModel, error) {
	var momentUsers []*momentUserModel
	_, err := m.session.Select("*").From(m.getTableName(uid)).Where("uid=?", uid).OrderDir("sort_num", false).Offset((page - 1) * pageSize).Limit(pageSize).Load(&momentUsers)
	return momentUsers, err
}

// 查询不包含某些用户的好友关系
func (m *momentUserDB) queryWithUIDAndExcludeUIDs(uid string, uids []string, page, pageSize uint64) ([]*momentUserModel, error) {
	var momentUsers []*momentUserModel
	_, err := m.session.Select("*").From(m.getTableName(uid)).Where("uid=? and publisher not in ?", uid, uids).OrderDir("sort_num", false).Offset((page - 1) * pageSize).Limit(pageSize).Load(&momentUsers)
	return momentUsers, err
}

// 查询某个发布的动态
func (m *momentUserDB) queryWithPublisherAndPage(publisher string, page, pageSize uint64) ([]*momentUserModel, error) {
	var momentUsers []*momentUserModel
	_, err := m.session.Select("*").From(m.getTableName(publisher)).Where("uid=? and publisher=?", publisher, publisher).OrderDir("sort_num", false).Offset((page - 1) * pageSize).Limit(pageSize).Load(&momentUsers)
	return momentUsers, err
}

// 查询某个用户的前多少条动态
func (m *momentUserDB) queryWithUIDAndSize(uid string, size int64) ([]*momentUserModel, error) {
	var moments []*momentUserModel
	_, err := m.session.Select("*").From(m.getTableName(uid)).Where("publisher=? and uid=?", uid, uid).Limit(uint64(size)).OrderDir("sort_num", false).Load(&moments)
	return moments, err
}

// 获取用户所在的表名称
func (m *momentUserDB) getTableName(uid string) string {
	tableID := crc32.ChecksumIEEE([]byte(uid)) % 3
	if tableID == 0 {
		return "moment_user1"
	} else if tableID == 1 {
		return "moment_user2"
	} else {
		return "moment_user3"
	}
}

// 用户朋友圈关系model
type momentUserModel struct {
	UID       string //用户ID
	MomentNo  string //动态编号
	Publisher string //发布者uid
	SortNum   int64  //排序号
	d.BaseModel
}
