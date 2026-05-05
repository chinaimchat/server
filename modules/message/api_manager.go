package message

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/base/event"
	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/group"
	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/user"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/common"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/model"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/log"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/register"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkevent"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"github.com/gocraft/dbr/v2"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

// Manager 消息管理
type Manager struct {
	ctx *config.Context
	log.Log
	userService  user.IService
	groupService group.IService
	managerDB    *managerDB
	pinnedDB     *pinnedDB
}

// NewManager NewManager
func NewManager(ctx *config.Context) *Manager {
	return &Manager{
		ctx:          ctx,
		Log:          log.NewTLog("MessageManager"),
		userService:  user.NewService(ctx),
		groupService: group.NewService(ctx),
		managerDB:    newManagerDB(ctx),
		pinnedDB:     newPinnedDB(ctx),
	}
}

// Route 路由配置
func (m *Manager) Route(r *wkhttp.WKHttp) {
	auth := r.Group("/v1/manager", m.ctx.AuthMiddleware(r))
	{
		auth.POST("/message/send", m.sendMsg)                 // 发送消息
		auth.POST("message/sendfriends", m.sendMsgToFriends)  // 给某个用户代发消息
		auth.GET("/message", m.list)                          // 代发消息记录
		auth.POST("/message/sendall", m.sendMsgToAllUsers)    // 给所有用户发送一条消息
		auth.GET("/message/record", m.record)                 // 消息记录
		auth.GET("/message/recordpersonal", m.recordpersonal) // 单聊聊天记录
		auth.DELETE("/message", m.delete)                     // 删除消息
		auth.GET("/message/prohibit_words", m.prohibitWordsList)
		auth.POST("/message/prohibit_words", m.prohibitWordsAdd)
		auth.DELETE("/message/prohibit_words", m.prohibitWordsDelete)
		auth.GET("/message/sensitive_words", m.sensitiveWordsList)
		auth.POST("/message/sensitive_words", m.sensitiveWordsAdd)
		auth.PUT("/message/sensitive_words", m.sensitiveWordsUpdate)
		auth.POST("/message/sensitive_words/batch", m.sensitiveWordsBatchAdd)
		auth.DELETE("/message/sensitive_words", m.sensitiveWordsDelete)
	}
}
func (m *Manager) sendMsgToFriends(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type ReqVO struct {
		UID     string   `json:"uid"`
		ToUIDs  []string `json:"to_uids"`
		Content string   `json:"content"`
	}
	var req ReqVO
	if err := c.BindJSON(&req); err != nil {
		m.Error("数据格式有误！", zap.Error(err))
		c.ResponseError(errors.New("数据格式有误！"))
		return
	}
	if req.UID == "" {
		c.ResponseError(errors.New("发送者不能为空"))
		return
	}
	if req.Content == "" {
		c.ResponseError(errors.New("发送内容不能为空"))
		return
	}
	if len(req.ToUIDs) == 0 {
		c.ResponseError(errors.New("发送消息的订阅者不能为空"))
		return
	}
	go m.sendMessageToFriends(req.ToUIDs, req.UID, req.Content)
	c.ResponseOK()
}

func (m *Manager) sendMessageToFriends(toUids []string, fromUID string, content string) error {
	err := m.ctx.SendMessageBatch(&config.MsgSendBatch{
		Header: config.MsgHeader{
			RedDot: 1,
		},
		FromUID: fromUID,
		Payload: []byte(util.ToJson(map[string]interface{}{
			"content": content,
			"type":    1,
		})),
		Subscribers: toUids,
	})
	if err != nil {
		m.Error("发送消息错误", zap.Error(err))
		return errors.New("发送消息错误")
	}
	return nil
}
func (m *Manager) delete(c *wkhttp.Context) {
	loginUID := c.GetLoginUID()
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type msgVO struct {
		MessageID  string `json:"message_id"`
		MessageSeq uint32 `json:"message_seq"`
	}
	type reqVO struct {
		List        []*msgVO `json:"list"`
		ChannelID   string   `json:"channel_id"`
		FromUID     string   `json:"from_uid"`
		ChannelType uint8    `json:"channel_type"`
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		m.Error("数据格式有误！", zap.Error(err))
		c.ResponseError(errors.New("数据格式有误！"))
		return
	}
	if len(req.List) == 0 {
		c.ResponseError(errors.New("删除的msgIds不能为空"))
		return
	}
	if req.ChannelType == uint8(common.ChannelTypePerson) && (req.FromUID == "" || req.ChannelID == req.FromUID) {
		c.ResponseError(errors.New("单聊fromuid不能为空且不能和channelId一致"))
		return
	}
	fakeChannelID := req.ChannelID
	if req.ChannelType == common.ChannelTypePerson.Uint8() {
		fakeChannelID = common.GetFakeChannelIDWith(req.ChannelID, req.FromUID)
	}
	tx, err := m.ctx.DB().Begin()
	if err != nil {
		m.Error("开启事务失败！", zap.Error(err))
		c.ResponseError(errors.New("开启事务失败！"))
		return
	}
	defer func() {
		if err := recover(); err != nil {
			tx.RollbackUnlessCommitted()
			panic(err)
		}
	}()
	msgIds := make([]string, 0)
	for _, msg := range req.List {
		version := m.genMessageExtraSeq(fakeChannelID)
		msgIds = append(msgIds, msg.MessageID)
		err := m.managerDB.updateMsgExtraVersionAndDeletedTx(&messageExtraModel{
			ChannelID:   fakeChannelID,
			ChannelType: req.ChannelType,
			MessageID:   msg.MessageID,
			MessageSeq:  msg.MessageSeq,
			IsDeleted:   1,
			Version:     version,
		}, tx)
		if err != nil {
			tx.Rollback()
			m.Error(common.ErrData.Error(), zap.Error(err))
			c.ResponseError(errors.New("删除消息错误"))
			return
		}
	}
	pinnedMsgs, err := m.pinnedDB.queryWithMessageIds(fakeChannelID, req.ChannelType, msgIds)
	if err != nil {
		tx.Rollback()
		m.Error("查询置顶消息错误", zap.Error(err))
		c.ResponseError(errors.New("查询置顶消息错误"))
		return
	}
	isSendSyncPinnedMsgCMD := false
	if len(pinnedMsgs) > 0 {
		for _, pinnedMsg := range pinnedMsgs {
			if pinnedMsg.IsDeleted == 0 {
				pinnedMsg.IsDeleted = 1
				pinnedMsg.Version = time.Now().UnixMilli()
				isSendSyncPinnedMsgCMD = true
				err = m.pinnedDB.updateTx(pinnedMsg, tx)
				if err != nil {
					tx.Rollback()
					m.Error("删除置顶消息错误", zap.Error(err))
					c.ResponseError(errors.New("删除置顶消息错误"))
					return
				}
			}
		}
	}
	if isSendSyncPinnedMsgCMD {
		err = m.ctx.SendCMD(config.MsgCMDReq{
			NoPersist:   true,
			ChannelID:   req.ChannelID,
			ChannelType: req.ChannelType,
			FromUID:     loginUID,
			CMD:         common.CMDSyncPinnedMessage,
		})

		if err != nil {
			m.Warn("发送cmd失败！", zap.Error(err))
		}
	}
	var eventID int64 = 0
	if m.ctx.GetConfig().ZincSearch.SearchOn {
		eventID, err = m.ctx.EventBegin(&wkevent.Data{
			Event: event.EventUpdateSearchMessage,
			Data: &config.UpdateSearchMessageReq{
				MessageIDs: msgIds,
				ChannelID:  req.ChannelID,
			},
			Type: wkevent.None,
		}, tx)
		if err != nil {
			tx.Rollback()
			m.Error("开启事件失败！", zap.Error(err))
			c.ResponseError(errors.New("开启事件失败！"))
			return
		}
	}
	if err := tx.Commit(); err != nil {
		tx.Rollback()
		m.Error("提交事务失败！", zap.Error(err))
		c.ResponseError(errors.New("提交事务失败！"))
		return
	}
	if eventID > 0 {
		m.ctx.EventCommit(eventID)
	}
	if req.ChannelType == common.ChannelTypePerson.Uint8() {
		err = m.ctx.SendCMD(config.MsgCMDReq{
			NoPersist:   false,
			ChannelID:   req.ChannelID,
			ChannelType: req.ChannelType,
			CMD:         common.CMDSyncMessageExtra,
			FromUID:     req.FromUID,
			Param: map[string]interface{}{
				"channel_id":   req.ChannelID,
				"channel_type": req.ChannelType,
			},
		})
	} else {
		err = m.ctx.SendCMD(config.MsgCMDReq{
			NoPersist:   false,
			ChannelID:   req.ChannelID,
			ChannelType: req.ChannelType,
			CMD:         common.CMDSyncMessageExtra,
			Param: map[string]interface{}{
				"channel_id":   req.ChannelID,
				"channel_type": req.ChannelType,
			},
		})
	}

	if err != nil {
		m.Error("发送cmd失败！", zap.Error(err))
		c.ResponseError(err)
		return
	}
	c.ResponseOK()
}

// mergePersonPreviewMessages 合并「被查看用户↔对方」「当前超管↔对方」以及 channel_id=对方 的落库记录，避免管理员代发落在非会话 fake 频道时预览页看不到。
func (m *Manager) mergePersonPreviewMessages(subjectUID, peerUID, loginUID string, pageIndex, pageSize uint64) ([]*messageModel, int64, error) {
	ch1 := common.GetFakeChannelIDWith(subjectUID, peerUID)
	ch2 := common.GetFakeChannelIDWith(loginUID, peerUID)
	ch3 := peerUID
	c1, err := m.managerDB.queryRecordCount(ch1)
	if err != nil {
		return nil, 0, err
	}
	c2, err := m.managerDB.queryRecordCount(ch2)
	if err != nil {
		return nil, 0, err
	}
	c3, err := m.managerDB.queryRecordCount(ch3)
	if err != nil {
		return nil, 0, err
	}
	channels := []string{ch1}
	if ch2 != ch1 {
		channels = append(channels, ch2)
	}
	if ch3 != ch1 && ch3 != ch2 {
		channels = append(channels, ch3)
	}
	var total int64
	for _, ch := range channels {
		switch ch {
		case ch1:
			total += c1
		case ch2:
			total += c2
		case ch3:
			total += c3
		}
	}
	if len(channels) == 1 {
		msgs, err := m.managerDB.queryWithChannelID(channels[0], pageIndex, pageSize)
		return msgs, total, err
	}
	// 多频道合并分页：每条频道取「各自最新 need 条」其中 need = pageIndex*pageSize（有上限），
	// 合并去重后按时间排序，再取全局第 [(P-1)*S, P*S) 条，与单表 OFFSET/LIMIT 数学一致。
	need := pageIndex * pageSize
	const maxNeed uint64 = 10000
	if need > maxNeed {
		need = maxNeed
	}
	if need < pageSize {
		need = pageSize
	}
	var all []*messageModel
	for _, ch := range channels {
		part, qerr := m.managerDB.queryWithChannelID(ch, 1, need)
		if qerr != nil {
			return nil, 0, qerr
		}
		all = append(all, part...)
	}
	byID := make(map[int64]*messageModel)
	for _, msg := range all {
		if msg == nil {
			continue
		}
		if old, ok := byID[msg.MessageID]; !ok || msg.Timestamp > old.Timestamp {
			byID[msg.MessageID] = msg
		}
	}
	merged := make([]*messageModel, 0, len(byID))
	for _, msg := range byID {
		merged = append(merged, msg)
	}
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].Timestamp != merged[j].Timestamp {
			return merged[i].Timestamp > merged[j].Timestamp
		}
		return merged[i].MessageID > merged[j].MessageID
	})
	start := int((pageIndex - 1) * pageSize)
	if start >= len(merged) {
		return []*messageModel{}, total, nil
	}
	end := start + int(pageSize)
	if end > len(merged) {
		end = len(merged)
	}
	return merged[start:end], total, nil
}

func (m *Manager) recordpersonal(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	uid := c.Query("uid")
	touid := c.Query("touid")
	pageIndex, pageSize := c.GetPage()
	if strings.TrimSpace(uid) == "" || strings.TrimSpace(touid) == "" {
		c.ResponseError(errors.New("uid不能为空"))
		return
	}
	channelID := common.GetFakeChannelIDWith(uid, touid)
	blend := c.Query("blend_admin_sends") == "1"
	var msgs []*messageModel
	var count int64
	if blend {
		if err := c.CheckLoginRoleIsSuperAdmin(); err != nil {
			c.ResponseError(err)
			return
		}
		loginUID := strings.TrimSpace(c.GetLoginUID())
		if loginUID != "" && loginUID != uid {
			msgs, count, err = m.mergePersonPreviewMessages(uid, touid, loginUID, uint64(pageIndex), uint64(pageSize))
		} else {
			msgs, err = m.managerDB.queryWithChannelID(channelID, uint64(pageIndex), uint64(pageSize))
			if err != nil {
				m.Error(common.ErrData.Error(), zap.Error(err))
				c.ResponseError(errors.New("查询消息记录错误"))
				return
			}
			count, err = m.managerDB.queryRecordCount(channelID)
		}
	} else {
		msgs, err = m.managerDB.queryWithChannelID(channelID, uint64(pageIndex), uint64(pageSize))
		if err != nil {
			m.Error(common.ErrData.Error(), zap.Error(err))
			c.ResponseError(errors.New("查询消息记录错误"))
			return
		}
		count, err = m.managerDB.queryRecordCount(channelID)
	}
	if err != nil {
		m.Error(common.ErrData.Error(), zap.Error(err))
		c.ResponseError(errors.New("查询消息记录错误"))
		return
	}
	list := make([]*recordVO, 0)
	if len(msgs) == 0 {
		c.Response(&recordResp{
			Count: count,
			List:  list,
		})
		return
	}
	uids := make([]string, 0)
	msgIds := make([]string, 0)
	for _, msg := range msgs {
		uids = append(uids, msg.FromUID)
		msgIds = append(msgIds, strconv.FormatInt(msg.MessageID, 10))
	}
	msgExtrs, err := m.managerDB.queryMsgExtrWithMsgIds(msgIds)
	if err != nil {
		m.Error(common.ErrData.Error(), zap.Error(err))
		c.ResponseError(errors.New("查询消息扩展错误"))
		return
	}
	userList, err := m.userService.GetUsers(uids)
	if err != nil {
		m.Error(common.ErrData.Error(), zap.Error(err))
		c.ResponseError(errors.New("查询发送者信息错误"))
		return
	}
	ids := make([]int64, 0)
	for _, msg := range msgs {
		sendName := ""
		for _, user := range userList {
			if user.UID == msg.FromUID {
				sendName = user.Name
			}
		}
		isDeleted := 0
		revoke := 0
		editedAt := 0
		readedCount := 0
		var payloadMap map[string]interface{}
		for _, extr := range msgExtrs {
			msgID, _ := strconv.ParseInt(extr.MessageID, 10, 64)
			if msgID == msg.MessageID {
				isDeleted = extr.IsDeleted
				revoke = extr.Revoke
				editedAt = extr.EditedAt
				readedCount = extr.ReadedCount
				if extr.ContentEdit.String != "" {
					err := util.ReadJsonByByte([]byte(extr.ContentEdit.String), &payloadMap)
					if err != nil {
						log.Warn("负荷数据不是json格式！", zap.Error(err), zap.String("payload", string(extr.ContentEdit.String)))
					}
				}
			}
		}
		if payloadMap == nil {
			err := util.ReadJsonByByte(msg.Payload, &payloadMap)
			if err != nil {
				log.Warn("负荷数据不是json格式！", zap.Error(err), zap.String("payload", string(msg.Payload)))
			}
		}
		var deviceDBID int64 = 0
		if strings.Contains(msg.ClientMsgNo, "_") {
			tempStrs := strings.Split(msg.ClientMsgNo, "_")
			if len(tempStrs) > 2 {
				str := tempStrs[1]
				if str != "" {
					deviceDBID, err = strconv.ParseInt(str, 10, 64)
					if err == nil {
						ids = append(ids, deviceDBID)
					} else {
						deviceDBID = 0
					}
				}
			}
		}
		println("消息设备ID", deviceDBID)
		messageId := strconv.FormatInt(msg.MessageID, 10)
		list = append(list, &recordVO{
			MessageID:   messageId,
			Sender:      msg.FromUID,
			SenderName:  sendName,
			Payload:     payloadMap,
			Signal:      msg.Signal,
			IsDeleted:   isDeleted,
			CreatedAt:   msg.CreatedAt.String(),
			EditedAt:    editedAt,
			Revoke:      revoke,
			DeviceDBID:  deviceDBID,
			ReadedCount: readedCount,
		})
	}
	var devices []*model.DeviceResp
	if len(ids) > 0 {
		modules := register.GetModules(m.ctx)
		for _, module := range modules {
			if module.BussDataSource.GetDevice != nil {
				devices, _ = module.BussDataSource.GetDevice(ids)
				break
			}
		}
	}
	if len(devices) > 0 && len(list) > 0 {
		for _, device := range devices {
			for _, msg := range list {
				if msg.DeviceDBID == device.ID {
					msg.DeviceID = device.DeviceID
					msg.DeviceName = device.DeviceName
					msg.DeviceModel = device.DeviceModel
					break
				}
			}
		}
	}
	c.Response(&recordResp{
		Count: count,
		List:  list,
	})
}
func (m *Manager) record(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	var channelID = c.Query("channel_id")
	pageIndex, pageSize := c.GetPage()
	msgs, err := m.managerDB.queryWithChannelID(channelID, uint64(pageIndex), uint64(pageSize))
	if err != nil {
		m.Error(common.ErrData.Error(), zap.Error(err))
		c.ResponseError(errors.New("查询消息记录错误"))
		return
	}
	count, err := m.managerDB.queryRecordCount(channelID)
	if err != nil {
		m.Error(common.ErrData.Error(), zap.Error(err))
		c.ResponseError(errors.New("查询消息总量错误"))
		return
	}

	list := make([]*recordVO, 0)
	if len(msgs) == 0 {
		c.Response(list)
		return
	}
	uids := make([]string, 0)
	msgIds := make([]string, 0)
	for _, msg := range msgs {
		uids = append(uids, msg.FromUID)
		msgIds = append(msgIds, strconv.FormatInt(msg.MessageID, 10))
	}
	msgExtrs, err := m.managerDB.queryMsgExtrWithMsgIds(msgIds)
	if err != nil {
		m.Error(common.ErrData.Error(), zap.Error(err))
		c.ResponseError(errors.New("查询消息扩展错误"))
		return
	}
	userList, err := m.userService.GetUsers(uids)
	if err != nil {
		m.Error(common.ErrData.Error(), zap.Error(err))
		c.ResponseError(errors.New("查询发送者信息错误"))
		return
	}
	ids := make([]int64, 0)
	for _, msg := range msgs {
		sendName := ""
		for _, user := range userList {
			if user.UID == msg.FromUID {
				sendName = user.Name
			}
		}
		isDeleted := 0
		revoke := 0
		editedAt := 0
		readedCount := 0
		var payloadMap map[string]interface{}
		for _, extr := range msgExtrs {
			msgID, _ := strconv.ParseInt(extr.MessageID, 10, 64)
			if msgID == msg.MessageID {
				isDeleted = extr.IsDeleted
				revoke = extr.Revoke
				editedAt = extr.EditedAt
				readedCount = extr.ReadedCount
				if extr.ContentEdit.String != "" {
					err := util.ReadJsonByByte([]byte(extr.ContentEdit.String), &payloadMap)
					if err != nil {
						log.Warn("负荷数据不是json格式！", zap.Error(err), zap.String("payload", string(extr.ContentEdit.String)))
					}
				}
			}
		}
		if payloadMap == nil {
			err := util.ReadJsonByByte(msg.Payload, &payloadMap)
			if err != nil {
				log.Warn("负荷数据不是json格式！", zap.Error(err), zap.String("payload", string(msg.Payload)))
			}
		}
		var deviceDBID int64 = 0
		if strings.Contains(msg.ClientMsgNo, "_") {
			tempStrs := strings.Split(msg.ClientMsgNo, "_")
			if len(tempStrs) > 2 {
				str := tempStrs[1]
				if str != "" {
					deviceDBID, err = strconv.ParseInt(str, 10, 64)
					if err == nil {
						ids = append(ids, deviceDBID)
					} else {
						deviceDBID = 0
					}
				}
			}
		}
		println("消息设备ID", deviceDBID)
		messageId := strconv.FormatInt(msg.MessageID, 10)

		list = append(list, &recordVO{
			MessageID:   messageId,
			MessageSeq:  msg.MessageSeq,
			Sender:      msg.FromUID,
			SenderName:  sendName,
			Payload:     payloadMap,
			Signal:      0,
			IsDeleted:   isDeleted,
			CreatedAt:   msg.CreatedAt.String(),
			EditedAt:    editedAt,
			Revoke:      revoke,
			DeviceDBID:  deviceDBID,
			ReadedCount: readedCount,
		})
	}

	var devices []*model.DeviceResp
	if len(ids) > 0 {
		modules := register.GetModules(m.ctx)
		for _, module := range modules {
			if module.BussDataSource.GetDevice != nil {
				devices, _ = module.BussDataSource.GetDevice(ids)
				break
			}
		}
	}
	if len(devices) > 0 && len(list) > 0 {
		for _, device := range devices {
			for _, msg := range list {
				if msg.DeviceDBID == device.ID {
					msg.DeviceID = device.DeviceID
					msg.DeviceName = device.DeviceName
					msg.DeviceModel = device.DeviceModel
					break
				}
			}
		}
	}
	c.Response(&recordResp{
		Count: count,
		List:  list,
	})
}
func (m *Manager) sendMsgToAllUsers(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type SendMsgReq struct {
		Content string `json:"content"`
	}
	var req SendMsgReq
	if err := c.BindJSON(&req); err != nil {
		m.Error(common.ErrData.Error(), zap.Error(err))
		c.ResponseError(common.ErrData)
		return
	}
	userList, err := m.userService.GetAllUsers()
	if err != nil {
		c.ResponseError(err)
		return
	}
	uids := make([][]string, 0)
	tempUserList := make([]string, 0)
	for _, user := range userList {
		if len(tempUserList) == 1000 {
			uids = append(uids, tempUserList)
			tempUserList = make([]string, 0)
		}
		tempUserList = append(tempUserList, user.UID)
	}
	if len(tempUserList) > 0 {
		uids = append(uids, tempUserList)
	}
	go m.sendMessageBatch(uids, req.Content)
	c.ResponseOK()
}
func (m *Manager) sendMessageBatch(uids [][]string, content string) error {
	for _, list := range uids {
		err := m.ctx.SendMessageBatch(&config.MsgSendBatch{
			Header: config.MsgHeader{
				RedDot: 1,
			},
			FromUID: m.ctx.GetConfig().Account.SystemUID,
			Payload: []byte(util.ToJson(map[string]interface{}{
				"content": content,
				"type":    1,
			})),
			Subscribers: list,
		})
		if err != nil {
			m.Error("发送消息错误", zap.Error(err))
			return errors.New("发送消息错误")
		}
		time.Sleep(time.Second)
	}
	return nil
}

// 发送消息
func (m *Manager) sendMsg(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	var req managerSendMsgReq
	if err := c.BindJSON(&req); err != nil {
		m.Error(common.ErrData.Error(), zap.Error(err))
		c.ResponseError(common.ErrData)
		return
	}
	if err := req.check(); err != nil {
		c.ResponseError(err)
		return
	}
	var receiverName string = ""
	if req.ReceivedChannelType == int(common.ChannelTypePerson) {
		user, err := m.userService.GetUser(req.ReceivedChannelID)
		if err != nil {
			// 「以此用户视角」单聊历史里可能包含已删除或系统账号，允许继续发送到 fake 频道
			if strings.TrimSpace(req.ConversationSubjectUID) == "" {
				m.Error("查询接受的者信息错误", zap.Error(err), zap.String("uid", req.ReceivedChannelID))
				c.ResponseError(errors.New("查询接受的者信息错误"))
				return
			}
			m.Warn("消息接受者查询失败，按会话视角继续发送", zap.Error(err), zap.String("uid", req.ReceivedChannelID))
		}
		if user == nil && strings.TrimSpace(req.ConversationSubjectUID) == "" {
			c.ResponseError(errors.New("消息接受者用户不存在"))
			return
		}
		if user != nil {
			receiverName = user.Name
		}
	}
	if req.ReceivedChannelType == int(common.ChannelTypeGroup) {
		group, err := m.groupService.GetGroupWithGroupNo(req.ReceivedChannelID)
		if err != nil {
			m.Error("查询接受群信息错误", zap.Error(err), zap.String("groupNo", req.ReceivedChannelID))
			c.ResponseError(errors.New("查询接受群信息错误"))
			return
		}
		if group == nil {
			c.ResponseError(errors.New("消息接受群不存在"))
			return
		}
		receiverName = group.Name
	}
	channelID := req.ReceivedChannelID
	if req.ReceivedChannelType == int(common.ChannelTypePerson) && strings.TrimSpace(req.ConversationSubjectUID) != "" {
		// 管理端「以此用户视角」单聊：消息写入「被查看用户」与「会话对方」的私聊频道，发送者仍为 Sender（管理员）
		channelID = common.GetFakeChannelIDWith(strings.TrimSpace(req.ConversationSubjectUID), req.ReceivedChannelID)
	}
	payload := map[string]interface{}{
		"content":  req.Content,
		"type":     1,
		"from_uid": req.Sender,
	}
	if req.Payload != nil {
		payload = req.Payload
		if _, ok := payload["from_uid"]; !ok {
			payload["from_uid"] = req.Sender
		}
	}
	err = m.ctx.SendMessage(&config.MsgSendReq{
		Header: config.MsgHeader{
			RedDot: 1,
		},
		FromUID:     req.Sender,
		ChannelID:   channelID,
		ChannelType: uint8(req.ReceivedChannelType),
		Payload:     []byte(util.ToJson(payload)),
	})
	if err != nil &&
		req.ReceivedChannelType == int(common.ChannelTypePerson) &&
		strings.TrimSpace(req.ConversationSubjectUID) != "" {
		chTry2 := common.GetFakeChannelIDWith(strings.TrimSpace(req.Sender), req.ReceivedChannelID)
		if chTry2 != channelID {
			m.Warn("会话视角发送失败，尝试管理员与对方私聊频道", zap.Error(err), zap.String("channelID", channelID), zap.String("try2", chTry2))
			err = m.ctx.SendMessage(&config.MsgSendReq{
				Header: config.MsgHeader{
					RedDot: 1,
				},
				FromUID:     req.Sender,
				ChannelID:   chTry2,
				ChannelType: uint8(req.ReceivedChannelType),
				Payload:     []byte(util.ToJson(payload)),
			})
		}
	}
	if err != nil &&
		req.ReceivedChannelType == int(common.ChannelTypePerson) &&
		strings.TrimSpace(req.ConversationSubjectUID) != "" {
		m.Warn("仍会失败，回退 channel_id 为对方 uid", zap.Error(err), zap.String("received_channel_id", req.ReceivedChannelID))
		err = m.ctx.SendMessage(&config.MsgSendReq{
			Header: config.MsgHeader{
				RedDot: 1,
			},
			FromUID:     req.Sender,
			ChannelID:   req.ReceivedChannelID,
			ChannelType: uint8(req.ReceivedChannelType),
			Payload:     []byte(util.ToJson(payload)),
		})
	}
	if err != nil {
		m.Error("发送消息错误", zap.Error(err))
		c.ResponseError(err)
		return
	}
	// 添加发送消息记录
	err = m.managerDB.insertMsgHistory(&managerMsgModel{
		Sender:              req.Sender,
		SenderName:          req.SenderName,
		ReceiverChannelType: req.ReceivedChannelType,
		Receiver:            req.ReceivedChannelID,
		ReceiverName:        receiverName,
		HandlerUID:          c.GetLoginUID(),
		HandlerName:         c.GetLoginName(),
		Content:             req.Content,
	})
	if err != nil {
		m.Error("添加发送消息记录错误", zap.Error(err))
		c.ResponseError(errors.New("添加发送消息记录错误"))
		return
	}
	c.ResponseOK()
}

// 代发消息列表
func (m *Manager) list(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	pageIndex, pageSize := c.GetPage()
	list, err := m.managerDB.queryMsgWithPage(uint64(pageSize), uint64(pageIndex))
	if err != nil {
		m.Error("查询代发消息记录错误", zap.Error(err))
		c.ResponseError(errors.New("查询代发消息记录错误"))
		return
	}
	count, err := m.managerDB.queryMsgCount()
	if err != nil {
		m.Error("查询代发消息总数错误", zap.Error(err))
		c.ResponseError(errors.New("查询代发消息总数错误"))
		return
	}
	result := make([]*managerSendMsgResp, 0)
	for _, model := range list {
		result = append(result, &managerSendMsgResp{
			Sender:              model.Sender,
			SenderName:          model.SenderName,
			Receiver:            model.Receiver,
			ReceiverName:        model.ReceiverName,
			ReceiverChannelType: model.ReceiverChannelType,
			HandlerUID:          model.HandlerUID,
			HandlerName:         model.HandlerName,
			Content:             model.Content,
			CreatedAt:           model.CreatedAt.String(),
		})
	}
	c.Response(map[string]interface{}{
		"count": count,
		"list":  result,
	})
}

func (m *managerSendMsgReq) check() error {
	if m.ReceivedChannelID == "" {
		return errors.New("接受者ID不能为空")
	}
	if m.Sender == "" {
		return errors.New("发送者ID不能为空")
	}
	if m.SenderName == "" {
		return errors.New("发送者名字不能为空")
	}
	if m.ReceivedChannelType != int(common.ChannelTypeGroup) && m.ReceivedChannelType != int(common.ChannelTypePerson) && m.ReceivedChannelType != int(common.ChannelTypeNone) {
		return errors.New("接受者类型错误")
	}
	if strings.TrimSpace(m.Content) == "" && m.Payload == nil {
		return errors.New("发送内容不能为空")
	}
	return nil
}

func (m *Manager) genMessageExtraSeq(channelID string) int64 {
	return m.ctx.GenSeq(fmt.Sprintf("%s:%s", common.MessageExtraSeqKey, channelID))
}

func (m *Manager) prohibitWordsList(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	pageIndex, pageSize := c.GetPage()
	offset := (pageIndex - 1) * pageSize
	searchKey := c.Query("search_key")

	countQ := m.ctx.DB().Select("count(*)").From("prohibit_words")
	if searchKey != "" {
		countQ = countQ.Where("content LIKE ?", "%"+searchKey+"%")
	}
	var count int
	countQ.Load(&count)

	type wordModel struct {
		ID        int64  `db:"id" json:"id"`
		Content   string `db:"content" json:"content"`
		IsRegex   int    `db:"is_regex" json:"is_regex"`
		IsDeleted int    `db:"is_deleted" json:"is_deleted"`
		CreatedAt string `db:"created_at" json:"created_at"`
	}
	var list []*wordModel
	q := m.ctx.DB().Select("*").From("prohibit_words")
	if searchKey != "" {
		q = q.Where("content LIKE ?", "%"+searchKey+"%")
	}
	q.OrderDir("created_at", false).Limit(uint64(pageSize)).Offset(uint64(offset)).Load(&list)

	if list == nil {
		list = make([]*wordModel, 0)
	}
	c.Response(map[string]interface{}{
		"count": count,
		"list":  list,
	})
}

func (m *Manager) prohibitWordsAdd(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type reqVO struct {
		Content string `json:"content"`
		IsRegex int    `json:"is_regex"`
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		m.Error("数据格式有误！", zap.Error(err))
		c.ResponseError(errors.New("数据格式有误！"))
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		c.ResponseError(errors.New("违禁词不能为空"))
		return
	}
	version := m.ctx.GenSeq(prohibitWordsVersionSeqKey)
	_, err = m.ctx.DB().InsertInto("prohibit_words").Columns("content", "is_regex", "version").Values(req.Content, req.IsRegex, version).Exec()
	if err != nil {
		m.Error("添加违禁词错误", zap.Error(err))
		c.ResponseError(errors.New("添加违禁词错误"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) prohibitWordsDelete(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type reqVO struct {
		ID        int64 `json:"id"`
		IsDeleted int   `json:"is_deleted"`
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		m.Error("数据格式有误！", zap.Error(err))
		c.ResponseError(errors.New("数据格式有误！"))
		return
	}
	version := m.ctx.GenSeq(prohibitWordsVersionSeqKey)
	_, err = m.ctx.DB().Update("prohibit_words").Set("is_deleted", req.IsDeleted).Set("version", version).Where("id=?", req.ID).Exec()
	if err != nil {
		m.Error("操作违禁词错误", zap.Error(err))
		c.ResponseError(errors.New("操作违禁词错误"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) sensitiveWordsList(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	pageIndex, pageSize := c.GetPage()
	offset := (pageIndex - 1) * pageSize
	searchKey := c.Query("search_key")
	category := c.Query("category")

	countQ := m.ctx.DB().Select("count(*)").From("sensitive_words")
	if searchKey != "" {
		countQ = countQ.Where("content LIKE ?", "%"+searchKey+"%")
	}
	if category != "" {
		countQ = countQ.Where("category=?", category)
	}
	var count int
	countQ.Load(&count)

	type wordModel struct {
		ID        int64  `db:"id" json:"id"`
		Content   string `db:"content" json:"content"`
		Category  string `db:"category" json:"category"`
		Level     int    `db:"level" json:"level"`
		IsDeleted int    `db:"is_deleted" json:"is_deleted"`
		Version   int    `db:"version" json:"version"`
		CreatedAt string `db:"created_at" json:"created_at"`
	}
	var list []*wordModel
	q := m.ctx.DB().Select("*").From("sensitive_words")
	if searchKey != "" {
		q = q.Where("content LIKE ?", "%"+searchKey+"%")
	}
	if category != "" {
		q = q.Where("category=?", category)
	}
	q.OrderDir("created_at", false).Limit(uint64(pageSize)).Offset(uint64(offset)).Load(&list)

	if list == nil {
		list = make([]*wordModel, 0)
	}
	c.Response(map[string]interface{}{
		"count": count,
		"list":  list,
	})
}

func (m *Manager) sensitiveWordsAdd(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type reqVO struct {
		Content  string `json:"content"`
		Category string `json:"category"`
		Level    int    `json:"level"`
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		m.Error("数据格式有误！", zap.Error(err))
		c.ResponseError(errors.New("数据格式有误！"))
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		c.ResponseError(errors.New("敏感词不能为空"))
		return
	}
	if req.Category == "" {
		req.Category = "default"
	}
	if req.Level == 0 {
		req.Level = 1
	}
	_, err = m.ctx.DB().InsertInto("sensitive_words").Columns("content", "category", "level").Values(req.Content, req.Category, req.Level).Exec()
	if err != nil {
		m.Error("添加敏感词错误", zap.Error(err))
		c.ResponseError(errors.New("添加敏感词错误"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) sensitiveWordsBatchAdd(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type reqVO struct {
		Words    []string `json:"words"`
		Category string   `json:"category"`
		Level    int      `json:"level"`
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		m.Error("数据格式有误！", zap.Error(err))
		c.ResponseError(errors.New("数据格式有误！"))
		return
	}
	if len(req.Words) == 0 {
		c.ResponseError(errors.New("敏感词列表不能为空"))
		return
	}
	if req.Category == "" {
		req.Category = "default"
	}
	if req.Level == 0 {
		req.Level = 1
	}
	successCount := 0
	for _, word := range req.Words {
		w := strings.TrimSpace(word)
		if w == "" {
			continue
		}
		_, e := m.ctx.DB().InsertInto("sensitive_words").Columns("content", "category", "level").Values(w, req.Category, req.Level).Exec()
		if e == nil {
			successCount++
		}
	}
	c.Response(map[string]interface{}{
		"success_count": successCount,
		"total_count":   len(req.Words),
	})
}

func (m *Manager) sensitiveWordsUpdate(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type reqVO struct {
		ID       int64  `json:"id"`
		Content  string `json:"content"`
		Category string `json:"category"`
		Level    int    `json:"level"`
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		m.Error("数据格式有误！", zap.Error(err))
		c.ResponseError(errors.New("数据格式有误！"))
		return
	}
	if req.ID == 0 {
		c.ResponseError(errors.New("ID不能为空"))
		return
	}
	_, err = m.ctx.DB().Update("sensitive_words").
		Set("content", req.Content).
		Set("category", req.Category).
		Set("level", req.Level).
		Set("version", dbr.Expr("version+1")).
		Set("updated_at", dbr.Now).
		Where("id=?", req.ID).Exec()
	if err != nil {
		m.Error("更新敏感词错误", zap.Error(err))
		c.ResponseError(errors.New("更新敏感词错误"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) sensitiveWordsDelete(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type reqVO struct {
		ID int64 `json:"id"`
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		m.Error("数据格式有误！", zap.Error(err))
		c.ResponseError(errors.New("数据格式有误！"))
		return
	}
	var isDeleted int
	m.ctx.DB().Select("is_deleted").From("sensitive_words").Where("id=?", req.ID).Load(&isDeleted)
	newVal := 1
	if isDeleted == 1 {
		newVal = 0
	}
	_, err = m.ctx.DB().Update("sensitive_words").Set("is_deleted", newVal).Set("updated_at", dbr.Now).Where("id=?", req.ID).Exec()
	if err != nil {
		m.Error("操作敏感词错误", zap.Error(err))
		c.ResponseError(errors.New("操作敏感词错误"))
		return
	}
	c.ResponseOK()
}

type managerSendMsgReq struct {
	Sender                 string                 `json:"sender"`                             // 发送者uid
	SenderName             string                 `json:"sender_name"`                        // 发送者名字
	ReceivedChannelID      string                 `json:"received_channel_id"`                // 接受者id（单聊为对方 uid；群聊为群号）
	ReceivedChannelType    int                    `json:"received_channel_type"`              // 接受类型
	Content                string                 `json:"content"`                            // 发送内容
	Payload                map[string]interface{} `json:"payload"`                            // 可选：自定义消息体（例如图片消息）
	ConversationSubjectUID string                 `json:"conversation_subject_uid,omitempty"` // 单聊可选：被查看用户 uid，与 received_channel_id 组成 fake 频道
}

type managerSendMsgResp struct {
	Receiver            string `json:"receiver"`              // 接受者uid
	ReceiverName        string `json:"receiver_name"`         // 接受者名字
	ReceiverChannelType int    `json:"receiver_channel_type"` // 接受者频道类型
	Sender              string `json:"sender"`                // 发送者uid
	SenderName          string `json:"sender_name"`           // 发送者名字
	HandlerUID          string `json:"handler_uid"`           // 操作者uid
	HandlerName         string `json:"handler_name"`          // 操作者名字
	Content             string `json:"content"`               // 发送内容
	CreatedAt           string `json:"created_at"`            // 发送时间
}
type recordResp struct {
	Count int64       `json:"count"`
	List  []*recordVO `json:"list"`
}
type recordVO struct {
	MessageID   string                 `json:"message_id"`   // 消息编号
	MessageSeq  uint32                 `json:"message_seq"`  // 消息序号
	Sender      string                 `json:"sender"`       // 发送者uid
	SenderName  string                 `json:"sender_name"`  // 发送者名字
	Signal      int                    `json:"signal"`       // 是否加密
	Payload     map[string]interface{} `json:"payload"`      // 发送内容
	IsDeleted   int                    `json:"is_deleted"`   // 是否删除
	ReadedCount int                    `json:"readed_count"` // 已读人数
	Revoke      int                    `json:"revoke"`       // 是否撤回
	DeviceDBID  int64                  `json:"device_db_id"` // 设备数据库id
	DeviceID    string                 `json:"device_id"`    // 设备id
	DeviceName  string                 `json:"device_name"`  // 设备名称
	DeviceModel string                 `json:"device_model"` // 设备型号
	CreatedAt   string                 `json:"created_at"`   // 发送时间
	EditedAt    int                    `json:"edited_at"`    // 编辑时间
}
