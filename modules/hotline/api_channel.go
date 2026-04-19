package hotline

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/common"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// 频道分页查询
func (h *Hotline) channelPage(c *wkhttp.Context) {
	busTypeStr := c.Query("bus_type")
	pIndex, pSize := c.GetPage()

	appID := c.GetAppID()

	models, err := h.channelDB.queryPage(busTypeStr, appID, pIndex, pSize)
	if err != nil {
		c.ResponseErrorf("查询频道数据失败！", err)
		return
	}
	cn, err := h.channelDB.queryCount(busTypeStr, appID)
	if err != nil {
		c.ResponseErrorf("查询频道数量失败！", err)
		return
	}
	resps := make([]*channelResp, 0, len(models))
	if len(models) > 0 {
		for _, model := range models {
			resps = append(resps, newChannelResp(model))
		}
	}
	c.Response(common.NewPageResult(pIndex, pSize, cn, resps))
}

func (h *Hotline) channelDisable(c *wkhttp.Context) {
	var req struct {
		ChannelID   string `json:"channel_id"`
		ChannelType uint8  `json:"channel_type"`
		Disable     int    `json:"disable"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseErrorf("数据格式有误！", err)
		return
	}
	err := h.channelDB.updateDisable(req.Disable, req.ChannelID, req.ChannelType)
	if err != nil {
		c.ResponseErrorf("更新频道禁用状态失败！", err)
		return
	}
	c.ResponseOK()
}

func (h *Hotline) channelCreate(c *wkhttp.Context) {
	var req channelReq
	if err := c.BindJSON(&req); err != nil {
		c.ResponseErrorf("数据格式有误！", err)
		return
	}
	if err := req.check(); err != nil {
		c.ResponseError(err)
		return
	}
	appID := c.GetAppID()
	channelID := fmt.Sprintf("%s@ht", util.GenerUUID())
	channelType := common.ChannelTypeGroup.Uint8()

	subscribers := make([]string, 0, len(req.Subscribers))
	if len(req.Subscribers) > 0 {
		for _, subscriber := range req.Subscribers {
			subscribers = append(subscribers, subscriber.Subscriber)
		}
	}

	err := h.ctx.IMCreateOrUpdateChannel(&config.ChannelCreateReq{
		ChannelID:   channelID,
		ChannelType: channelType,
		// Bind:        appID,
		Subscribers: subscribers,
	})
	if err != nil {
		c.ResponseErrorf("创建频道失败！", err)
		return
	}
	if req.BusType == ChannelBusTypeSkill.Int() { // 如果是技能组 则需要将原来技能组里的订阅者移除
		for _, subscriber := range subscribers {
			err = h.channelDB.deleteSubscriberWithBusType(subscriber, SubscriberTypeAgent.Int(), ChannelBusTypeSkill.Int(), appID)
			if err != nil {
				h.Error("删除原技能组的成员失败！", zap.Error(err))
				c.ResponseError(errors.New("删除原技能组的成员失败！"))
				return
			}
		}

	}
	tx, _ := h.ctx.DB().Begin()
	defer func() {
		if err := recover(); err != nil {
			tx.RollbackUnlessCommitted()
			panic(err)
		}
	}()

	err = h.channelDB.insertTx(&channelModel{
		AppID:              appID,
		TopicID:            req.TopicID,
		Title:              req.Title,
		ChannelID:          channelID,
		ChannelType:        channelType,
		BusType:            req.BusType,
		SessionActiveLimit: req.SessionActiveLimit,
	}, tx)
	if err != nil {
		tx.Rollback()
		c.ResponseErrorf("添加访客频道失败！", err)
		return
	}

	if len(req.Subscribers) > 0 {
		for _, subscriber := range req.Subscribers {
			err = h.channelDB.insertSubscriberTx(&subscribersModel{
				AppID:          appID,
				ChannelID:      channelID,
				ChannelType:    common.ChannelTypeGroup.Uint8(),
				SubscriberType: subscriber.SubscriberType,
				Subscriber:     subscriber.Subscriber,
			}, tx)
			if err != nil {
				tx.Rollback()
				c.ResponseErrorf("添加订阅者失败！！", err)
				return
			}
		}

	}

	if err := tx.Commit(); err != nil {
		tx.Rollback()
		c.ResponseErrorf("提交事务失败！", err)
		return
	}

	c.Response(gin.H{
		"channel_id":   channelID,
		"channel_type": channelType,
	})
}

func (h *Hotline) topicChannelCreate(c *wkhttp.Context) {
	appID := c.Request.Header.Get("appid")
	loginUID := c.GetLoginUID()
	var req struct {
		TopicID int64 `json:"topic_id"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseErrorf("数据格式有误！", err)
		return
	}

	var topicID int64
	var topicM *topicModel
	var err error
	if req.TopicID <= 0 {
		topicM, err = h.topicDB.queryDefaultWithAppID(appID)
		if err != nil {
			h.Error("获取默认的topic失败！", zap.Error(err))
			c.ResponseError(errors.New("获取默认的topic失败！"))
			return
		}
		if topicM == nil {
			topics, err := h.topicDB.queryWithAppID(appID)
			if err != nil {
				h.Error("查询话题失败！", zap.Error(err))
				c.ResponseError(errors.New("查询话题失败！"))
				return
			}
			if len(topics) > 0 {
				topicM = topics[0]
			}
		}
		if topicM != nil {
			topicID = topicM.Id
		}
	} else {
		topicID = req.TopicID
		topicM, err = h.topicDB.queryWithIDAndAppID(topicID, appID)
		if err != nil {
			h.Error("查询话题失败！", zap.Error(err))
			c.ResponseError(errors.New("查询话题失败！"))
			return
		}
		if topicM == nil {
			c.ResponseError(errors.New("话题不存在！"))
			return
		}
	}

	visitorChannel, err := h.channelDB.queryVisitorChannel(c.GetLoginUID(), topicID, appID)
	if err != nil {
		c.ResponseErrorf("查询是否存在访客频道失败！", err)
		return
	}
	if visitorChannel != nil {
		c.JSON(http.StatusOK, gin.H{
			"channel_id":   visitorChannel.ChannelID,
			"channel_type": visitorChannel.ChannelType,
		})
		return
	}

	visitorM, err := h.visitorDB.queryWithVID(c.GetLoginUID())
	if err != nil {
		c.ResponseErrorf("查询访客数据失败！", err)
		return
	}
	if visitorM == nil {
		h.Error("访客不存在！", zap.String("vid", c.GetLoginUID()))
		c.ResponseError(errors.New("访客不存在！"))
		return
	}

	channelID := h.ctx.GetConfig().ComposeCustomerServiceChannelID(loginUID, fmt.Sprintf("%s_ht", util.GenerUUID()))
	channelType := common.ChannelTypeCustomerService.Uint8()

	err = h.ctx.IMCreateOrUpdateChannel(&config.ChannelCreateReq{
		ChannelID:   channelID,
		ChannelType: channelType,
	})
	if err != nil {
		c.ResponseErrorf("创建频道失败！", err)
		return
	}

	tx, _ := h.ctx.DB().Begin()
	defer func() {
		if err := recover(); err != nil {
			tx.RollbackUnlessCommitted()
			panic(err)
		}
	}()

	err = h.channelDB.insertTx(&channelModel{
		AppID:       appID,
		VID:         c.GetLoginUID(),
		TopicID:     topicID,
		Title:       visitorM.Name,
		ChannelID:   channelID,
		ChannelType: channelType,
	}, tx)
	if err != nil {
		tx.Rollback()
		c.ResponseErrorf("添加访客频道失败！", err)
		return
	}

	// err = h.channelDB.insertSubscriberTx(&subscribersModel{
	// 	AppID:          appID,
	// 	ChannelID:      channelID,
	// 	ChannelType:    common.ChannelTypeGroup.Uint8(),
	// 	SubscriberType: SubscriberTypeVisitor.Int(),
	// 	Subscriber:     c.GetLoginUID(),
	// }, tx)
	// if err != nil {
	// 	tx.Rollback()
	// 	c.ResponseErrorf("添加订阅者失败！！", err)
	// 	return
	// }
	if err := tx.Commit(); err != nil {
		tx.Rollback()
		c.ResponseErrorf("提交事务失败！", err)
		return
	}

	configM, err := h.configDB.queryWithAppID(appID)
	if err != nil {
		h.Error("查询配置数据失败！", zap.Error(err))
		return
	}
	go func(topicM *topicModel, configM *configModel) {
		if topicM != nil && configM != nil {
			fromUID := strings.TrimSpace(configM.UID)
			if fromUID == "" {
				fromUID = strings.TrimSpace(h.ctx.GetConfig().Account.SystemUID)
			}
			if fromUID == "" {
				h.Warn("给访客发送欢迎消息跳过：缺少发送者UID", zap.String("appID", appID), zap.Int64("topicID", topicID))
				return
			}
			err = h.ctx.SendMessage(&config.MsgSendReq{
				ChannelID:   channelID,
				ChannelType: channelType,
				FromUID:     fromUID,
				Header: config.MsgHeader{
					RedDot: 1,
				},
				Payload: []byte(util.ToJson(map[string]interface{}{
					"type":    common.Text,
					"content": topicM.Welcome,
				})),
			})
			if err != nil {
				h.Warn("给访客发送欢迎消息失败！", zap.Error(err))
			}
		}
	}(topicM, configM)

	// go func() {
	// 	topicM, err := h.topicDB.queryWithIDAndAppID(req.TopicID, appID)
	// 	if err != nil {
	// 		h.Error("查询话题失败！", zap.Error(err))
	// 		return
	// 	}
	// 	configM, err := h.configDB.queryWithAppID(appID)
	// 	if err != nil {
	// 		h.Error("查询配置数据失败！", zap.Error(err))
	// 		return
	// 	}
	// 	if topicM != nil && configM != nil {
	// 		go func(configM *configModel) {
	// 			time.Sleep(time.Second)
	// 			err = h.ctx.SendMessage(&config.MsgSendReq{
	// 				ChannelID:   channelID,
	// 				ChannelType: channelType,
	// 				FromUID:     configM.UID,
	// 				Payload: []byte(util.ToJson(map[string]interface{}{
	// 					"type":    common.Text,
	// 					"content": topicM.Welcome,
	// 				})),
	// 			})
	// 			if err != nil {
	// 				h.Error("发送消息失败！", zap.Error(err))
	// 				return
	// 			}
	// 		}(configM)
	// 	}
	// }()

	c.Response(gin.H{
		"channel_id":   channelID,
		"channel_type": channelType,
	})

}

func (h *Hotline) channelGet(c *wkhttp.Context) {
	channelID := c.Param("channel_id")
	channelTypeI64, _ := strconv.ParseInt(c.Param("channel_type"), 10, 64)
	channelType := uint8(channelTypeI64)

	channelM, err := h.channelDB.queryDetailWithChannelID(channelID, uint8(channelType))
	if err != nil {
		c.ResponseErrorf("查询频道失败！", err)
		return
	}
	if channelM == nil {
		h.Warn("频道不存在！", zap.String("channelID", channelID))
		c.ResponseError(errors.New("频道不存在！"))
		return
	}
	onlineM, err := h.onlineDB.queryWithUserIDAndDeviceFlag(channelM.VID, config.Web.Uint8())
	if err != nil {
		c.ResponseErrorf("查询在线数据失败！", err)
		return
	}
	var online int
	var lastOffline int
	if onlineM != nil {
		online = onlineM.Online
		if online != 1 {
			lastOffline = onlineM.LastOffline
		}
	}

	c.Response(groupResp{
		GroupNo:     channelID,
		Name:        channelM.Title,
		Online:      online,
		LastOffline: lastOffline,
		Extra: map[string]interface{}{
			"topic_id":   channelM.TopicID,
			"topic_name": channelM.TopicName,
			"agent_uid":  channelM.AgentUID,
			"agent_name": channelM.AgentName,
			"category":   channelM.Category,
			"vid":        channelM.VID,
			"source":     channelM.Source,
		},
	})
}

func (h *Hotline) syncChannelMembers(c *wkhttp.Context) {
	channelID := c.Param("channel_id")

	channel, err := h.channelDB.queryWithChannelID(channelID, common.ChannelTypeGroup.Uint8())
	if err != nil {
		c.ResponseErrorf("查询频道数据失败！", err)
		return
	}
	if channel == nil {
		c.ResponseError(errors.New("频道不存在！"))
		return
	}
	subscribers := make([]*subscribersModel, 0)
	if strings.TrimSpace(channel.Bind) != "" {
		subs, err := h.channelDB.querySubscribers(channel.Bind, common.ChannelTypeGroup.Uint8())
		if err != nil {
			c.ResponseErrorf("查询订阅者失败！", err)
			return
		}
		if len(subs) > 0 {
			subscribers = append(subscribers, subs...)
		}
	}

	subs, err := h.channelDB.querySubscribers(channelID, common.ChannelTypeGroup.Uint8())
	if err != nil {
		c.ResponseErrorf("查询订阅者失败！", err)
		return
	}
	if len(subs) > 0 {
		subscribers = append(subscribers, subs...)
	}

	members := make([]*memberDetailResp, 0, len(subscribers))
	for _, subscriber := range subscribers {
		members = append(members, &memberDetailResp{
			UID:     subscriber.Subscriber,
			GroupNo: channelID,
			Name:    "暂无",
		})
	}

	c.Response(members)
}

type groupResp struct {
	GroupNo            string                 `json:"group_no"`             // 群编号
	Name               string                 `json:"name"`                 // 群名称
	Notice             string                 `json:"notice"`               // 群公告
	Mute               int                    `json:"mute"`                 // 免打扰
	Top                int                    `json:"top"`                  // 置顶
	ShowNick           int                    `json:"show_nick"`            // 显示昵称
	Save               int                    `json:"save"`                 // 是否保存
	Forbidden          int                    `json:"forbidden"`            // 是否全员禁言
	Invite             int                    `json:"invite"`               // 群聊邀请确认
	ChatPwd            int                    `json:"chat_pwd"`             //是否开启聊天密码
	Screenshot         int                    `json:"screenshot"`           //截屏通知
	RevokeRemind       int                    `json:"revoke_remind"`        //撤回提醒
	JoinGroupRemind    int                    `json:"join_group_remind"`    //进群提醒
	ForbiddenAddFriend int                    `json:"forbidden_add_friend"` //群内禁止加好友
	CreatedAt          string                 `json:"created_at"`
	UpdatedAt          string                 `json:"updated_at"`
	Version            int64                  `json:"version"`      // 群数据版本
	Extra              map[string]interface{} `json:"extra"`        // 扩展数据
	Online             int                    `json:"online"`       //是否在线
	LastOffline        int                    `json:"last_offline"` //最后一次离线时间
}

// 成员详情model
type memberDetailResp struct {
	ID        uint64 `json:"id"`
	UID       string `json:"uid"`        // 成员uid
	GroupNo   string `json:"group_no"`   // 群唯一编号
	Name      string `json:"name"`       // 群成员名称
	Remark    string `json:"remark"`     // 成员备注
	Role      int    `json:"role"`       // 成员角色
	Version   int64  `json:"version"`    // 版本号
	IsDeleted int    `json:"is_deleted"` // 是否删除
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type channelReq struct {
	Title              string          `json:"title"`
	VID                string          `json:"vid"`
	TopicID            int64           `json:"topic_id"`
	BusType            int             `json:"bus_type"`
	SessionActiveLimit int             `json:"session_active_limit"` // 最大会话限制
	Subscribers        []subscriberReq `json:"subscribers"`
}

func (c channelReq) check() error {
	if c.Title == "" {
		return errors.New("频道标题不能为空！")
	}
	return nil
}

type subscriberReq struct {
	SubscriberType int    `json:"subscriber_type"` // 0.访客 1.客服
	Subscriber     string `json:"subscriber"`      // 订阅者ID 0.位vid 1.为uid
}

type channelResp struct {
	VID                string `json:"vid"`
	TopicID            int64  `json:"topic_id"`
	TopicName          string `json:"topic_name"`
	Title              string `json:"title"`
	ChannelID          string `json:"channel_id"`
	ChannelType        uint8  `json:"channel_type"`
	AgentUID           string `json:"agent_uid"`
	AgentName          string `json:"agent_name"`
	Category           string `json:"category"`
	Bind               string `json:"bind"`
	Disable            int    `json:"disable"`
	BusType            int    `json:"bus_type"`
	MemberCount        int    `json:"member_count"`
	SessionActiveLimit int    `json:"session_active_limit"` // 最大会话限制
}

func newChannelResp(m *channelDetailModel) *channelResp {
	return &channelResp{
		VID:                m.VID,
		TopicID:            m.TopicID,
		TopicName:          m.TopicName,
		Title:              m.Title,
		ChannelID:          m.ChannelID,
		ChannelType:        m.ChannelType,
		AgentUID:           m.AgentUID,
		AgentName:          m.AgentName,
		Category:           m.Category,
		Bind:               m.Bind,
		Disable:            m.Disable,
		BusType:            m.BusType,
		MemberCount:        m.MemberCount,
		SessionActiveLimit: m.SessionActiveLimit,
	}
}
