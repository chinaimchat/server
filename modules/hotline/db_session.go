package hotline

import (
	"fmt"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/db"
	"github.com/gocraft/dbr/v2"
)

type sessionDB struct {
	ctx *config.Context
	db  *dbr.Session
}

func newSessionDB(ctx *config.Context) *sessionDB {
	return &sessionDB{
		ctx: ctx,
		db:  ctx.DB(),
	}
}

// func (s *sessionDB) update(m *sessionModel, versionLock int64) error {
// 	_, err := s.db.Update("hotline_session").SetMap(map[string]interface{}{
// 		"send_count":             m.SendCount,
// 		"recv_count":             m.RecvCount,
// 		"unread_count":           m.UnreadCount,
// 		"last_message":           m.LastMessage,
// 		"last_content_type":      m.LastContentType,
// 		"last_session_timestamp": m.LastSessionTimestamp,
// 		"version_lock":           m.VersionLock,
// 	}).Where("owner=? and session_id=? and session_type=? and app_id=? and version_lock=?", m.Owner, m.SessionID, m.SessionType, m.AppID).Exec()
// 	return err
// }

func (s *sessionDB) insertOrUpdate(m *sessionModel) error {
	var err error
	var sql string
	var args = make([]interface{}, 0)
	if m.LastRecv != 0 {
		sql = `
		insert into hotline_session(channel_id,channel_type,app_id,send_count,recv_count,last_recv,unread_count,last_message,last_content_type,last_session_timestamp,vid) values(?,?,?,?,?,?,?,?,?,?,?) 
		ON DUPLICATE KEY UPDATE 
		send_count=send_count+VALUES(send_count),recv_count=recv_count+VALUES(recv_count),last_recv=VALUES(last_recv),unread_count=unread_count+VALUES(unread_count),last_message=VALUES(last_message),last_content_type=VALUES(last_content_type),last_session_timestamp=VALUES(last_session_timestamp)
		`
		args = append(args, m.ChannelID, m.ChannelType, m.AppID, m.SendCount, m.RecvCount, m.LastRecv, m.UnreadCount, m.LastMessage, m.LastContentType, m.LastSessionTimestamp, m.VID)

	} else {
		sql = `
		insert into hotline_session(channel_id,channel_type,app_id,send_count,last_send,recv_count,unread_count,last_message,last_content_type,last_session_timestamp,vid) values(?,?,?,?,?,?,?,?,?,?,?) 
		ON DUPLICATE KEY UPDATE 
		send_count=send_count+VALUES(send_count),last_send=VALUES(last_send),recv_count=recv_count+VALUES(recv_count),unread_count=unread_count+VALUES(unread_count),last_message=VALUES(last_message),last_content_type=VALUES(last_content_type),last_session_timestamp=VALUES(last_session_timestamp)
		`
		args = append(args, m.ChannelID, m.ChannelType, m.AppID, m.SendCount, m.LastSend, m.RecvCount, m.UnreadCount, m.LastMessage, m.LastContentType, m.LastSessionTimestamp, m.VID)
	}
	_, err = s.db.UpdateBySql(sql, args...).Exec()

	return err
}

func (s *sessionDB) queryWith(category string, appID string, pageIndex, pageSize int64, agentUID string) ([]*sessionModel, error) {
	var models []*sessionModel
	builder := s.db.Select("hotline_session.*").From("hotline_session").LeftJoin("hotline_channel", "hotline_channel.channel_id=hotline_session.channel_id and hotline_channel.app_id=hotline_session.app_id").Where("hotline_session.app_id=?", appID)
	if category != "" {
		if category == SessionCategoryNew.String() {
			builder = builder.Where("hotline_channel.category=? and hotline_channel.agent_uid=?", "", "")
		} else if category == SessionCategoryAssignMe.String() {
			builder = builder.Where("hotline_channel.agent_uid=? and hotline_channel.category<>?", agentUID, SessionCategorySolved.String())
		} else if category == SessionCategoryAllAssigned.String() {
			builder = builder.Where("hotline_channel.agent_uid<>?", "")
		} else if category == SessionCategoryRealtime.String() {
			builder = builder.Where("hotline_channel.agent_uid<>?", "")
		} else {
			builder = builder.Where("hotline_channel.category=?", category)
		}
	}
	_, err := builder.OrderDesc("hotline_session.last_session_timestamp").Offset(uint64((pageIndex - 1) * pageSize)).Limit(uint64(pageSize)).Load(&models)
	return models, err
}

func (s *sessionDB) queryWithChannel(channelID string, channelType uint8) (*sessionModel, error) {
	var m *sessionModel
	_, err := s.db.Select("*").From("hotline_session").Where("channel_id=? and channel_type=?", channelID, channelType).Load(&m)
	return m, err
}

func (s *sessionDB) updateUnreadCount(unreadCount int, channelID string, channelType uint8) error {
	_, err := s.db.Update("hotline_session").Set("unread_count", unreadCount).Where("channel_id=? and channel_type=?", channelID, channelType).Exec()
	return err
}

// 查询客服正在处理的会话总数
func (s *sessionDB) queryAgentSessionTotalWithAppID(appID string) ([]*sessionAgentTotal, error) {
	var models []*sessionAgentTotal
	_, err := s.db.Select("IFNULL(hotline_channel.agent_uid,'') agent_uid,count(*) session_count,max(hotline_session.last_session_timestamp) recent_session").From("hotline_session").LeftJoin("hotline_channel", "hotline_session.app_id=hotline_channel.app_id and hotline_session.channel_id=hotline_channel.channel_id and hotline_session.channel_type=hotline_channel.channel_type and hotline_channel.bus_type="+fmt.Sprintf("%d", ChannelBusTypeVisitor)+" and  hotline_channel.agent_uid<>''").Where("hotline_channel.agent_uid<>'' and hotline_session.app_id=? and hotline_channel.category<>?", appID, SessionCategorySolved.String()).GroupBy("hotline_channel.agent_uid").Load(&models)
	return models, err
}

type sessionAgentTotal struct {
	SessionCount  int    // 会话数量
	AgentUID      string // 客服uid
	RecentSession int64  // 最近的一次会话时间
}

type sessionModel struct {
	AppID                string
	VID                  string
	ChannelType          uint8
	ChannelID            string
	SendCount            int
	RecvCount            int
	UnreadCount          int
	LastMessage          string
	LastContentType      int
	LastSessionTimestamp int64
	LastRecv             int64 // 客服最后一次收消息时间（访客最后一次发消息时间）
	LastSend             int64 // 客服最后一次发消息时间
	VersionLock          int64
	db.BaseModel
}
