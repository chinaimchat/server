package message

import (
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/db"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/gocraft/dbr/v2"
)

// managerDB 管理员代发消息记录
type managerDB struct {
	session *dbr.Session
	db      *DB
}

// newManagerDB newManagerDB
func newManagerDB(ctx *config.Context) *managerDB {
	return &managerDB{
		session: ctx.DB(),
		db:      NewDB(ctx),
	}
}

// 添加一条发送消息记录
func (m *managerDB) insertMsgHistory(message *managerMsgModel) error {
	_, err := m.session.InsertInto("send_history").Columns(util.AttrToUnderscore(message)...).Record(message).Exec()
	return err
}

// 查询代发消息记录
func (m *managerDB) queryMsgWithPage(pageSize, page uint64) ([]*managerMsgModel, error) {
	var list []*managerMsgModel
	_, err := m.session.Select("*").From("send_history").Offset((page-1)*pageSize).Limit(pageSize).OrderDir("created_at", false).Load(&list)
	return list, err
}

// 查询消息数量
func (m *managerDB) queryMsgCount() (int64, error) {
	var count int64
	_, err := m.session.Select("count(*)").From("send_history").Load(&count)
	return count, err
}

func (m *managerDB) queryWithChannelID(channelID string, page, pageSize uint64) ([]*messageModel, error) {
	var list []*messageModel
	var table = m.db.getTable(channelID)
	_, err := m.session.Select("*").From(table).Where("channel_id=?", channelID).Offset((page-1)*pageSize).Limit(pageSize).OrderDir("created_at", false).Load(&list)
	return list, err
}

func (m *managerDB) queryRecordCount(channelID string) (int64, error) {
	var count int64
	_, err := m.session.Select("count(*)").From(m.db.getTable(channelID)).Where("channel_id=?", channelID).Load(&count)
	return count, err
}

func (m *managerDB) queryMsgExtrWithMsgIds(msgIds []string) ([]*messageExtraModel, error) {
	var list []*messageExtraModel
	_, err := m.session.Select("*").From("message_extra").Where("message_id in ?", msgIds).Load(&list)
	return list, err
}

func (m *managerDB) updateMsgExtraVersionAndDeletedTx(md *messageExtraModel, tx *dbr.Tx) error {
	_, err := tx.InsertBySql("INSERT INTO message_extra (message_id,message_seq,channel_id,channel_type,is_deleted,version) VALUES (?,?,?,?,?,?) ON DUPLICATE KEY UPDATE is_deleted=VALUES(is_deleted),version=VALUES(version)", md.MessageID, md.MessageSeq, md.ChannelID, md.ChannelType, md.IsDeleted, md.Version).Exec()
	return err
}

// 管理员代发消息记录
type managerMsgModel struct {
	Receiver            string // 接受者uid
	ReceiverName        string // 接受者名字
	ReceiverChannelType int    // 接受者频道类型
	Sender              string // 发送者uid
	SenderName          string // 发送者名字
	HandlerUID          string // 操作者uid
	HandlerName         string // 操作者名字
	Content             string // 发送内容
	db.BaseModel
}
