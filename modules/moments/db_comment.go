package moments

import (
	"hash/crc32"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	d "github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/db"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/gocraft/dbr/v2"
)

// commentDB DB
type commentDB struct {
	ctx     *config.Context
	session *dbr.Session
}

// CommentDB New
func newCommentDB(ctx *config.Context) *commentDB {
	return &commentDB{
		ctx:     ctx,
		session: ctx.DB(),
	}
}

// insert 添加评论或点赞
func (c *commentDB) insert(cm *commentModel) (int64, error) {
	result, _ := c.session.InsertInto(c.getTableName(cm.MomentNo)).Columns(util.AttrToUnderscore(cm)...).Record(cm).Exec()
	id, err := result.LastInsertId()
	return id, err
}

// delete 删除点赞或评论
func (c *commentDB) delete(uid, momentNo string, handleType int64) error {
	_, err := c.session.DeleteFrom(c.getTableName(momentNo)).Where("moment_no=? and uid=? and handle_type=?", momentNo, uid, handleType).Exec()
	return err
}

// 通过动态编号删除所有评论
func (c *commentDB) deleteWidthMomentNo(momentNo string, tx *dbr.Tx) error {
	_, err := tx.DeleteFrom(c.getTableName(momentNo)).Where("moment_no=? ", momentNo).Exec()
	return err
}

// deleteComment 删除评论
func (c *commentDB) deleteComment(uid, momentNo string, id int64) error {
	_, err := c.session.DeleteFrom(c.getTableName(momentNo)).Where("moment_no=? and uid=? and id=? and handle_type=1", momentNo, uid, id).Exec()
	return err
}

// 通过momentNo查询所有动态评论
func (c *commentDB) queryWithMomentNo(momentNo string) ([]*commentModel, error) {
	var commentModels []*commentModel
	_, err := c.session.Select("*").From(c.getTableName(momentNo)).Where("moment_no=?", momentNo).Load(&commentModels)
	return commentModels, err
}

// getTableName 获取用户所在表名
func (c *commentDB) getTableName(momentNo string) string {
	tableID := crc32.ChecksumIEEE([]byte(momentNo)) % 3
	if tableID == 0 {
		return "comments1"
	} else if tableID == 1 {
		return "comments2"
	} else {
		return "comments3"
	}
}

// CommentModel 朋友圈操作对象
type commentModel struct {
	MomentNo       string //朋友圈编号
	Content        string //评论内容
	UID            string //用户ID
	Name           string //用户名字
	HandleType     int    //操作类型【0:点赞】【1:评论】
	ReplyCommentID string // 回复评论的ID
	ReplyUID       string //被回复的uid
	ReplyName      string //被回复的名字
	d.BaseModel
}
