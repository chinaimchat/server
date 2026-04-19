package hotline

import (
	"errors"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/common"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	limlog "github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/log"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"go.uber.org/zap"
)

// 会话分页查询
func (h *Hotline) sessionPage(c *wkhttp.Context) {
	category := c.Query("category") // 会话类别
	pIndex, pSize := c.GetPage()

	sessions, err := h.sessionDB.queryWith(category, c.GetAppID(), pIndex, pSize, c.GetLoginUID())
	if err != nil {
		h.Error("查询会话数据失败！", zap.Error(err))
		c.ResponseError(errors.New("查询会话数据失败！"))
		return
	}
	sessionResps := make([]*sessionResp, 0, len(sessions))
	if len(sessions) > 0 {
		for _, session := range sessions {
			sessionResps = append(sessionResps, newSessionResp(session))
		}
	}
	c.Response(sessionResps)

}

// 清除最近会话未读数
func (h *Hotline) clearSessionUnread(c *wkhttp.Context) {
	var req struct {
		ChannelID   string `json:"channel_id"`
		ChannelType uint8  `json:"channel_type"`
	}
	if err := c.BindJSON(&req); err != nil {
		h.Error("hotline数据格式有误！", zap.Error(err))
		c.ResponseError(common.ErrData)
		return
	}
	err := h.sessionDB.updateUnreadCount(0, req.ChannelID, req.ChannelType)
	if err != nil {
		c.ResponseError(errors.New("更新未读消息失败！"))
		return
	}
	err = h.ctx.IMClearConversationUnread(config.ClearConversationUnreadReq{
		UID:         c.GetLoginUID(),
		ChannelID:   req.ChannelID,
		ChannelType: req.ChannelType,
	})
	if err != nil {
		c.ResponseError(err)
		return
	}
	// 发送清空红点的命令
	err = h.ctx.SendCMD(config.MsgCMDReq{
		NoPersist:   true,
		ChannelID:   c.GetLoginUID(),
		ChannelType: common.ChannelTypePerson.Uint8(),
		CMD:         common.CMDConversationUnreadClear,
		Param: map[string]interface{}{
			"channel_id":   req.ChannelID,
			"channel_type": req.ChannelType,
		},
	})
	if err != nil {
		h.Error("命令发送失败！", zap.String("cmd", common.CMDConversationUnreadClear))
		c.ResponseError(errors.New("命令发送失败！"))
		return
	}
	c.ResponseOK()
}

type messageResp struct {
	MessageID   int64                  `json:"message_id"`    // 服务端的消息ID(全局唯一)
	MessageSeq  uint32                 `json:"message_seq"`   // 消息序列号 （用户唯一，有序递增）
	ClientMsgNo string                 `json:"client_msg_no"` // 客户端消息唯一编号
	FromUID     string                 `json:"from_uid"`      // 发送者UID
	ToUID       string                 `json:"to_uid"`        // 接受者uid
	ChannelID   string                 `json:"channel_id"`    // 频道ID
	ChannelType uint8                  `json:"channel_type"`  // 频道类型
	Timestamp   int32                  `json:"timestamp"`     // 服务器消息时间戳(10位，到秒)
	Payload     map[string]interface{} `json:"payload"`       // 消息内容
	IsDeleted   int                    `json:"is_deleted"`    // 是否已删除
	VoiceStatus int                    `json:"voice_status"`  // 语音状态 0.未读 1.已读
}

func newMessageResp(m *config.MessageResp) *messageResp {
	payloadMap, err := m.GetPayloadMap()
	if err != nil {
		limlog.Warn("hotline解析消息的payload失败！", zap.String("payload", string(m.Payload)), zap.Error(err))
	}
	return &messageResp{
		MessageID:   m.MessageID,
		MessageSeq:  m.MessageSeq,
		ClientMsgNo: m.ClientMsgNo,
		FromUID:     m.FromUID,
		ToUID:       m.ToUID,
		ChannelID:   m.ChannelID,
		ChannelType: m.ChannelType,
		Timestamp:   m.Timestamp,
		Payload:     payloadMap,
		IsDeleted:   m.IsDeleted,
		VoiceStatus: m.VoiceStatus,
	}
}

type sessionResp struct {
	ChannelID   string       `json:"channel_id"`   // 频道ID
	ChannelType uint8        `json:"channel_type"` // 频道类型
	Unread      int64        `json:"unread"`       // 未读数
	Timestamp   int64        `json:"timestamp"`    // 最后一次会话时间戳
	LastMessage *messageResp `json:"last_message"` // 最后一条消息
}

func newSessionResp(m *sessionModel) *sessionResp {
	var lastMessage *messageResp
	if m.LastMessage != "" {
		if err := util.ReadJsonByByte([]byte(m.LastMessage), &lastMessage); err != nil {
			limlog.Warn("解码session的最后一条消息失败！", zap.Error(err))
		}
	}

	return &sessionResp{
		ChannelID:   m.ChannelID,
		ChannelType: m.ChannelType,
		Unread:      int64(m.UnreadCount),
		Timestamp:   m.LastSessionTimestamp,
		LastMessage: lastMessage,
	}
}
