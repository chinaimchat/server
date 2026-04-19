package hotline

import (
	"bytes"
	"errors"
	"fmt"
	"hash/crc32"
	"html/template"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/base/app"
	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/file"
	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/user"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/common"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/log"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Hotline Hotline
type Hotline struct {
	ctx *config.Context
	log.Log
	configDB                  *configDB
	topicDB                   *topicDB
	visitorDB                 *visitorDB
	deviceDB                  *deviceDB
	historyDB                 *historyDB
	groupDB                   *groupDB
	agentDB                   *agentDB
	channelDB                 *channelDB
	roleDB                    *roleDB
	onlineDB                  *onlineDB
	sessionDB                 *sessionDB
	ruleDB                    *ruleDB
	infoCategoryDB            *infoCategoryDB
	intelliAssignDB           *intelliAssignDB
	routeService              IRouteService
	userService               user.IService
	appService                app.IService
	fieldDB                   *fieldDB
	quickReplyDB              *quickReplyDB
	hotlineVisitorCachePrefix string // 活跃的访客的缓存key
	visitorOnlineStatusPrefix string
	fileService               file.IService
	service                   IService
}

// New New
func New(ctx *config.Context) *Hotline {
	return &Hotline{
		Log:                       log.NewTLog("hotline"),
		ctx:                       ctx,
		configDB:                  newConfigDB(ctx),
		topicDB:                   newTopicDB(ctx),
		visitorDB:                 newVisitorDB(ctx),
		deviceDB:                  newDeviceDB(ctx),
		historyDB:                 newHistoryDB(ctx),
		groupDB:                   newGroupDB(ctx),
		agentDB:                   newAgentDB(ctx),
		onlineDB:                  newOnlineDB(ctx),
		sessionDB:                 newSessionDB(ctx),
		channelDB:                 newChannelDB(ctx),
		roleDB:                    newRoleDB(ctx),
		intelliAssignDB:           newIntelliAssignDB(ctx),
		fieldDB:                   newFieldDB(ctx),
		ruleDB:                    newRuleDB(ctx),
		routeService:              NewRouteService(ctx),
		userService:               user.NewService(ctx),
		appService:                app.NewService(ctx),
		infoCategoryDB:            newInfoCategoryDB(ctx),
		quickReplyDB:              newQuickReplyDB(ctx),
		fileService:               file.NewService(ctx),
		service:                   NewService(ctx),
		hotlineVisitorCachePrefix: "hotlineVistor:",
		visitorOnlineStatusPrefix: "visitorOnline:",
	}
}

// Route 路由配置
func (h *Hotline) Route(r *wkhttp.WKHttp) {
	v := r.Group("/v1/hotline")
	{
		// 前端widget
		v.GET("/widget/:app_id/config", h.widgetConfig) // widget配置
		v.GET("/widget", h.widget)                      // widget视图
		v.POST("/widget/:app_id/visitor", h.visitor)    // 获取或注册访客信息
		// v.POST("/widget/:app_id/route", h.route)        // 匹配客服
		v.PUT("/widget/:app_id/config", h.widgetConfigUpdate) // 更新配置

		v.GET("/visitors/:vid/im", h.visitorIMGet)

		v.GET("/agent/preactive", h.agentPreActive) // 准备激活
		v.POST("/agent/active", h.agentActive)      // 激活

		v.GET("/example", h.exampleDownload) // 下载例子

		v.GET("/visitors/:vid/avatar", h.visitorAvatar) // 访客头像

		v.GET("/apps/:app_id/avatar", h.appAvatar) // 应用头像
		// 某些客户端/代理会先 HEAD 探测资源是否存在；如果不注册 HEAD，会导致 404 并显示占位块。
		v.RouterGroup.HEAD("/apps/:app_id/avatar", r.WKHttpHandler(h.appAvatar))

	}
	auth := r.Group("/v1/hotline", h.ctx.AuthMiddleware(r))
	{
		// ---------- widget ----------
		auth.GET("/widget/:app_id/info", h.widgetInfoGet)                       // 获取widget信息
		auth.POST("/visitor/topic/channel", h.topicChannelCreate)               // 创建访客的topic频道
		auth.GET("/visitor/channel/:channel_id/:channel_type", h.channelGet)    // 频道详情
		auth.GET("/visitor/channels/:channel_id/members", h.syncChannelMembers) // 同步频道成员

		auth.POST("/agent/login", h.login) // 客服登录

		auth.POST("/onboarding", h.onboarding) // 提交引导页的相关数据

		auth.PUT("/coversation/clearUnread", h.clearSessionUnread) // 清除未读消息

	}
	agentAuth := r.Group("/v1/hotline", h.ctx.AuthMiddleware(r), h.agentRoleMiddleware()) // 客服权限
	{
		// ---------- 角色 ----------
		agentAuth.GET("/roles", h.roleList) // 角色列表

		// ---------- 客服相关 ----------
		agentAuth.POST("/agents", h.agentAdd) // 添加客服

		agentAuth.POST("/agent/invite", h.agentInvite) // 邀请客服

		agentAuth.GET("/agents", h.agentPage)     // 客服列表分页查询
		agentAuth.POST("/agent/solved", h.solved) // 解决
		agentAuth.POST("/agent/reopen", h.reopen) // 重开

		agentAuth.GET("/agent/my", h.agentMyGet)                // 我的客服资料
		agentAuth.PUT("/agent/my", h.agentMyUpdate)             // 修改我的客服资料
		agentAuth.PUT("/agent/password", h.agentPasswordUpdate) // 更新密码

		agentAuth.PUT("/agent/work/:status", h.workStatusSet) // 更新客服工作在他

		// ---------- 频道相关 ----------
		agentAuth.GET("/groups", h.groupList)      // 群组列表
		agentAuth.POST("/groups", h.groupAdd)      // 添加群组
		agentAuth.GET("/skills", h.skillGroupList) // 技能组
		agentAuth.GET("/channel", h.channelPage)            // 分页获取频道
		agentAuth.POST("/channel", h.channelCreate)         // 创建频道
		agentAuth.PUT("/channel/disable", h.channelDisable) // 频道禁用或开启

		// ---------- 访客相关 ----------
		agentAuth.GET("/visitors", h.visitorPage)     // 访客列表
		agentAuth.GET("/visitors/:vid", h.visitorGet) // 访客数据
		agentAuth.PUT("/visitors/:vid/phone/:value", func(c *wkhttp.Context) {
			h.visitorFieldUpdate(c, "phone")
		}) // 修改访客属性
		agentAuth.PUT("/visitors/:vid/email/:value", func(c *wkhttp.Context) {
			h.visitorFieldUpdate(c, "email")
		}) // 修改访客属性
		// 修改访客自定义属性
		agentAuth.PUT("/visitors/:vid/props/:field/:value", h.visitorPropsUpdate)
		agentAuth.GET("/vistor/realtime", h.visitorOnlineNoAgent) // 实时在线带分配的访客

		// ---------- 会话相关 ----------
		agentAuth.GET("/session", h.sessionPage)

		// ---------- 路由 ----------
		agentAuth.POST("/assign", h.assignTo)                   // 分配客服
		agentAuth.GET("/intelli", h.intelliAssignGet)           //  获取智能分配
		agentAuth.POST("/intelli", h.intelliAssignSet)          // 设置智能分配
		agentAuth.PUT("/intelli/enable", h.intelliAssignEnable) // 启用智能分配

		// ---------- 规则引擎 ----------
		agentAuth.POST("/rules", h.ruleAdd)                    // 添加规则
		agentAuth.GET("/rules", h.ruleList)                    // 规则列表
		agentAuth.DELETE("/rules/:rule_no", h.ruleDelete)      // 删除规则
		agentAuth.PUT("/rules/:rule_no/:status", h.ruleStatus) // 改变状态规则 status为 enable或disable

		// ---------- 话题 ----------
		agentAuth.POST("/topics", h.topicAdd)       // 添加
		agentAuth.DELETE("/topics/:id", h.topicDel) // 删除
		agentAuth.GET("/topics", h.topicPage)       // 分页查询
		agentAuth.PUT("/topics/:id", h.topicUpdate) // 更新

		// ---------- 快捷回复 ----------
		agentAuth.POST("/quick_replies", h.quickReplyAdd)       // 添加快捷回复
		agentAuth.POST("/quick_reply/sync", h.quickReplySync)   // 快捷回复同步
		agentAuth.GET("/quick_replies", h.quickReplyPage)       // 查询快捷回复列表
		agentAuth.DELETE("/quick_replies/:id", h.quickReplyDel) // 删除快捷回复

		// ---------- 信息类别 ----------
		agentAuth.POST("/info_categories", h.infoCategoryAdd)
		agentAuth.GET("/info_categories", h.infoCategoryList)

		// ---------- 其他 ----------
		agentAuth.GET("/fields", h.fieldList)

	}

	test := r.Group("/v1/hotline") // 测试接口
	{
		test.GET("/app/agent/update", h.appAgentChannelUpdate)

	}
	// 监听消息
	h.ctx.AddMessagesListener(h.messagesListen)
	// 监听在线状态
	h.ctx.AddOnlineStatusListener(h.onlineStatusNotify)
}

func (h *Hotline) login(c *wkhttp.Context) {
	agents, err := h.agentDB.queryWithUID(c.GetLoginUID())
	if err != nil {
		c.ResponseErrorf("查询客服信息失败！", err)
		return
	}
	if len(agents) <= 0 {
		c.Response(gin.H{
			"exist": 0,
		})
		return
	}
	agent := agents[0] // TODO: 目前这里只取一条，后面可以做多选

	err = h.agentDB.updateIsWork(1, c.GetLoginUID(), c.GetAppID())
	if err != nil {
		c.ResponseErrorf("更新为状态失败！", err)
		return
	}
	agent.IsWork = 1

	c.Response(gin.H{
		"exist": 1,
		"data": gin.H{
			"uid":     agent.UID,
			"app_id":  agent.AppID,
			"role":    agent.Role,
			"name":    agent.Name,
			"is_work": agent.IsWork,
		},
	})

}

func (h *Hotline) appAvatar(c *wkhttp.Context) {
	// 强制永远返回本地 logo.jpg，忽略 hotline_config.logo，避免配置差异导致头像不一致。
	avatarBytes, readErr := os.ReadFile("assets/assets/logo.jpg")
	if readErr != nil || len(avatarBytes) == 0 {
		h.Error("应用头像读取失败", zap.Error(readErr))
		c.ResponseError(errors.New("应用头像不存在！"))
		return
	}
	c.Header("Content-Type", "image/jpeg")
	c.Header("Cache-Control", "public, max-age=86400")
	c.Writer.WriteHeader(http.StatusOK)
	// HEAD 请求只返回 Header，不返回 body（避免某些客户端/代理误处理）。
	if c.Request != nil && c.Request.Method == http.MethodHead {
		return
	}
	_, _ = c.Writer.Write(avatarBytes)
}

func (h *Hotline) visitorAvatar(c *wkhttp.Context) {
	vid := c.Param("vid")

	// 访问默认头像（与 avatar 桶 default/test (n).png 及 defaultCount 预置一致）
	dc := h.ctx.GetConfig().Avatar.DefaultCount
	if dc < 1 {
		dc = 900
	}
	avatarID := crc32.ChecksumIEEE([]byte(vid)) % uint32(dc)
	downloadUrl, _ := h.fileService.DownloadURL(fmt.Sprintf("/avatar/default/test (%d).png", avatarID), fmt.Sprintf("test (%d).png", avatarID))
	c.Redirect(http.StatusFound, downloadUrl)
}

// 引导页的相关数据
func (h *Hotline) onboarding(c *wkhttp.Context) {
	var req onboardingReq
	if err := c.BindJSON(&req); err != nil {
		c.ResponseErrorf("数据格式有误！", err)
		return
	}
	if err := req.Check(); err != nil {
		c.ResponseError(err)
		return
	}

	appResp, err := h.appService.CreateApp(app.Req{
		AppID: req.AppID,
	})
	if err != nil {
		c.ResponseError(errors.New("创建app失败！"))
		return
	}

	err = h.ctx.IMCreateOrUpdateChannel(&config.ChannelCreateReq{
		ChannelID:   req.AppID,
		ChannelType: common.ChannelTypeGroup.Uint8(),
		Subscribers: []string{c.GetLoginUID()},
	})
	if err != nil {
		h.Error("IM创建频道失败！", zap.Error(err))
		c.ResponseError(errors.New("IM创建频道失败！"))
		return
	}

	tx, _ := h.ctx.DB().Begin()
	defer func() {
		if err := recover(); err != nil {
			tx.RollbackUnlessCommitted()
			panic(err)
		}
	}()

	err = h.configDB.insertTx(&configModel{
		AppID:   appResp.AppID,
		AppName: req.AppName,
		UID:     c.GetLoginUID(),
		Logo:    req.Logo,
		Color:   req.BrandColor,
		ChatBg:  req.ChatBgURL,
	}, tx)
	if err != nil {
		h.Error("添加配置失败！", zap.Error(err))
		tx.Rollback()
		c.ResponseError(errors.New("添加配置失败！"))
		return
	}

	err = h.agentDB.insertTx(&agentModel{
		AppID:  appResp.AppID,
		UID:    c.GetLoginUID(),
		Name:   c.GetLoginName(),
		Role:   RoleAdmin.String(),
		Status: 1,
	}, tx)
	if err != nil {
		h.Error("添加客服失败！", zap.Error(err))
		tx.Rollback()
		c.ResponseError(errors.New("添加客服失败！"))
		return
	}

	err = h.channelDB.insertTx(&channelModel{
		AppID:       appResp.AppID,
		Title:       fmt.Sprintf("%s团队", req.AppName),
		ChannelID:   appResp.AppID,
		ChannelType: common.ChannelTypeGroup.Uint8(),
	}, tx)
	if err != nil {
		tx.Rollback()
		h.Error("创建公司团队频道失败！", zap.Error(err))
		c.ResponseError(errors.New("创建公司团队频道失败！"))
		return
	}
	err = h.channelDB.insertSubscriberTx(&subscribersModel{
		AppID:          appResp.AppID,
		ChannelID:      appResp.AppID,
		ChannelType:    common.ChannelTypeGroup.Uint8(),
		SubscriberType: SubscriberTypeAgent.Int(),
		Subscriber:     c.GetLoginUID(),
	}, tx)
	if err != nil {
		tx.Rollback()
		h.Error("添加公司团队成员失败！", zap.Error(err))
		c.ResponseError(errors.New("添加公司团队成员失败！"))
		return
	}

	err = h.topicDB.insertTx(&topicModel{
		AppID:     appResp.AppID,
		Title:     "咨询",
		Welcome:   "有什么问题？可以咨询我",
		IsDefault: 1,
	}, tx)
	if err != nil {
		tx.Rollback()
		h.Error("添加默认话题失败！", zap.Error(err))
		c.ResponseError(errors.New("添加默认话题失败！"))
		return
	}

	if err := tx.Commit(); err != nil {
		tx.Rollback()
		h.Error("提交事务失败！", zap.Error(err))
		c.ResponseError(errors.New("提交事务失败！"))
		return
	}

	c.Response(onboardingResp{
		AppID: appResp.AppID,
		Role:  RoleAdmin.String(),
	})

}

func (h *Hotline) getVisitorOnlineCacheKey(vid string) string {
	return fmt.Sprintf("%s%s", h.hotlineVisitorCachePrefix, vid)
}

// 在线状态通知处理
func (h *Hotline) onlineStatusNotify(onlineStatusList []config.OnlineStatus) {
	if len(onlineStatusList) > 0 {
		for _, onlineStatus := range onlineStatusList {
			if h.ctx.GetConfig().IsVisitor(onlineStatus.UID) { // 是访客的uid
				if !onlineStatus.Online && onlineStatus.OnlineCount > 0 {
					continue
				}
				if onlineStatus.Online {
					err := h.onlineDB.updateOrAddOnline(UserTypeVisitor.Int(), onlineStatus.DeviceFlag, onlineStatus.UID)
					if err != nil {
						h.Warn("更新在线状态失败！", zap.Error(err))
					}
				} else {
					err := h.onlineDB.updateOrAddOffline(UserTypeVisitor.Int(), onlineStatus.DeviceFlag, onlineStatus.UID)
					if err != nil {
						h.Warn("更新离线状态失败！", zap.Error(err))
					}
				}
				visitorChannelM, err := h.channelDB.queryLastWithVid(onlineStatus.UID)
				if err != nil {
					h.Warn("查询访客频道数据失败！", zap.Error(err))
				}
				var online int
				if onlineStatus.Online {
					online = 1
				}
				if visitorChannelM != nil {
					h.ctx.SendCMD(config.MsgCMDReq{
						NoPersist:   true,
						ChannelID:   visitorChannelM.ChannelID,
						ChannelType: visitorChannelM.ChannelType,
						CMD:         common.CMDOnlineStatus,
						Param: map[string]interface{}{
							"online":             online,
							"device_flag":        onlineStatus.DeviceFlag,
							"uid":                onlineStatus.UID,
							"total_online_count": onlineStatus.TotalOnlineCount,
						},
					})
				}

			}
		}
	}
}

// 客服角色中间价
func (h *Hotline) agentRoleMiddleware() wkhttp.HandlerFunc {
	return func(c *wkhttp.Context) {
		appID := c.GetAppID()
		if strings.TrimSpace(appID) != "" {
			loginUID := c.GetLoginUID()
			agentM, err := h.agentDB.queryWithUIDAndAppID(loginUID, c.GetAppID())
			if err != nil {
				h.Warn("查询客服角色信息失败！", zap.Error(err))
			}
			if agentM == nil {
				h.Warn("没有查询到登录的客服权限！", zap.String("loginUID", loginUID), zap.String("appID", c.GetAppID()))
			}
			if agentM != nil {
				c.Set("agentRole", agentM.Role)
			}
		}

		c.Next() // 放行
	}
}

// 更新app对应的客服人员到IM服务器
func (h *Hotline) appAgentChannelUpdate(c *wkhttp.Context) {
	appID := c.Query("app_id")
	users, err := h.userService.GetUsersWithAppID(appID)
	if err != nil {
		c.ResponseErrorf("获取app对应的客服人员失败！", err)
		return
	}
	if len(users) > 0 {
		subscribers := make([]string, 0, len(users))
		for _, user := range users {
			subscribers = append(subscribers, user.UID)
		}
		err = h.ctx.IMCreateOrUpdateChannel(&config.ChannelCreateReq{
			ChannelID:   appID,
			ChannelType: common.ChannelTypeGroup.Uint8(),
			Subscribers: subscribers,
		})
		if err != nil {
			c.ResponseErrorf("创建或更新频道失败！", err)
			return
		}

		err = h.channelDB.deleteWithChannelID(appID)
		if err != nil {
			c.ResponseError(err)
			return
		}
		err = h.channelDB.deleteSubscribersWithChannelID(appID)
		if err != nil {
			c.ResponseError(err)
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
			Title:       "客服团队",
			ChannelID:   appID,
			ChannelType: common.ChannelTypeGroup.Uint8(),
		}, tx)
		if err != nil {
			tx.Rollback()
			c.ResponseError(err)
			return
		}
		if len(subscribers) > 0 {
			for _, subscriber := range subscribers {
				err = h.channelDB.insertSubscriberTx(&subscribersModel{
					ChannelID:      appID,
					ChannelType:    common.ChannelTypeGroup.Uint8(),
					SubscriberType: SubscriberTypeAgent.Int(),
					Subscriber:     subscriber,
				}, tx)
				if err != nil {
					tx.Rollback()
					c.ResponseError(err)
					return
				}
			}
		}
		if err := tx.Commit(); err != nil {
			tx.Rollback()
			c.ResponseError(err)
			return
		}

	}
	c.ResponseOK()

}

// 角色判断
func (h *Hotline) hasRole(c *wkhttp.Context, roles ...RoleConst) bool {
	if len(roles) <= 0 {
		return false
	}
	role := c.GetString("agentRole")
	h.Info("获取到客服角色", zap.String("role", role))
	for _, r := range roles {
		if role == r.String() {
			return true
		}
	}
	return false
}
func (h *Hotline) messagesListen(messages []*config.MessageResp) {
	h.Debug("监听到消息", zap.Any("messages", messages))

	h.updateSessions(messages) // 更新session

	for _, message := range messages {
		fromUID := message.FromUID
		if message.ChannelType == common.ChannelTypeCustomerService.Uint8() { // 客服频道
			if !h.hasRoute(message) { // 未路由
				visitorUID, _ := h.ctx.GetConfig().GetCustomerServiceVisitorUID(message.ChannelID)
				if visitorUID == message.FromUID { // 访客发的消息才进行路由
					err := h.route(message)
					if err == nil {
						h.setRouted(fromUID)
					}

				}

			}
		}
	}
}

func (h *Hotline) updateSessions(messages []*config.MessageResp) {
	if len(messages) == 1 {
		message := messages[0]
		recvCount := 0
		sendCount := 0
		unreadCount := 0
		var lastSend int64 = 0
		var lastRecv int64 = 0
		if h.ctx.GetConfig().IsVisitor(message.FromUID) {
			recvCount = 1
			if message.Header.RedDot == 1 {
				unreadCount = 1
			}
			lastRecv = int64(message.Timestamp)
		} else {
			sendCount = 1
			lastSend = int64(message.Timestamp)
		}
		err := h.updateSession(messages[0], sendCount, lastSend, recvCount, lastRecv, unreadCount)
		if err != nil {
			h.Error("更新会话失败！", zap.Error(err))
			return
		}
		return
	}
	channelMessagesMap := map[string][]*config.MessageResp{}
	for _, message := range messages {
		if len(message.ChannelID) <= 0 {
			continue
		}
		key := fmt.Sprintf("%s-%d", message.ChannelID, message.ChannelType)
		messageList := channelMessagesMap[key]
		if messageList == nil {
			messageList = make([]*config.MessageResp, 0)
		}
		messageList = append(messageList, message)
		channelMessagesMap[key] = messageList
	}

	if len(channelMessagesMap) > 0 {
		for _, channelMessages := range channelMessagesMap {
			if len(channelMessages) > 0 {
				sendCount := 0
				recvCount := 0
				unreadCount := 0
				var lastSend int64 = 0
				var lastRecv int64 = 0
				for _, channelMessage := range channelMessages {
					if h.ctx.GetConfig().IsVisitor(channelMessage.FromUID) {
						recvCount++
						if channelMessage.Header.RedDot == 1 {
							unreadCount++
						}
						if int64(channelMessage.Timestamp) > lastRecv {
							lastRecv = int64(channelMessage.Timestamp)
						}

					} else {
						sendCount++
						if int64(channelMessage.Timestamp) > lastSend {
							lastSend = int64(channelMessage.Timestamp)
						}
					}
				}
				err := h.updateSession(channelMessages[len(channelMessages)-1], sendCount, lastSend, recvCount, lastRecv, unreadCount)
				if err != nil {
					h.Warn("更新会话失败！", zap.Error(err))
					continue
				}
			}
		}
	}
}

func (h *Hotline) updateSession(lastMessage *config.MessageResp, sendCount int, lastSend int64, recvCount int, lastRecv int64, unreadCount int) error {
	subscribers, channelM, err := h.channelDB.queryBindSubscribers(lastMessage.ChannelID, lastMessage.ChannelType)
	if err != nil {
		return err
	}
	if len(subscribers) <= 0 {
		h.Warn("频道内无订阅者！", zap.String("channelID", lastMessage.ChannelID), zap.Uint8("channelType", lastMessage.ChannelType))
	}
	if channelM != nil {
		err = h.sessionDB.insertOrUpdate(&sessionModel{
			AppID:                channelM.AppID,
			VID:                  channelM.VID,
			ChannelType:          lastMessage.ChannelType,
			ChannelID:            lastMessage.ChannelID,
			SendCount:            sendCount,
			LastSend:             lastSend,
			RecvCount:            recvCount,
			LastRecv:             lastRecv,
			UnreadCount:          unreadCount,
			LastMessage:          util.ToJson(newMessageResp(lastMessage)),
			LastContentType:      lastMessage.GetContentType(),
			LastSessionTimestamp: time.Now().Unix(),
		})
		if err != nil {
			h.Warn("添加或更新会话失败！", zap.Error(err))
		}
	}

	return nil

}

// 例子下载
var exampleOnce sync.Once
var exampleOnceTpl *template.Template

func (h *Hotline) exampleDownload(c *wkhttp.Context) {
	appID := c.Query("app_id")

	configM, err := h.configDB.queryWithAppID(appID)
	if err != nil {
		c.ResponseErrorf("查询app数据失败！", err)
		return
	}
	if configM == nil {
		c.ResponseError(errors.New("app信息不存在！"))
		return
	}
	exampleOnce.Do(func() {
		exampleOnceTpl = template.Must(template.ParseFiles("assets/webroot/hotline/exmaple_tpl.html"))
	})
	buff := bytes.NewBuffer(make([]byte, 0))
	err = exampleOnceTpl.ExecuteTemplate(buff, "hotline/exmaple_tpl.html", map[string]interface{}{
		"app_id":    appID,
		"app_name":  configM.AppName,
		"widget_js": fmt.Sprintf("%s/hotline/scripts/widget.js", h.ctx.GetConfig().External.BaseURL),
	})
	if err != nil {
		c.ResponseErrorf("渲染模版失败！", err)
		return
	}
	c.Header("Content-Type", "application/octet-stream")
	c.Header("content-Disposition", "attachment;filename=index.html")

	c.Writer.Write(buff.Bytes())
}

type onboardingReq struct {
	AppID      string `json:"app_id"`
	AppName    string `json:"app_name"`    // app 名字
	Logo       string `json:"logo"`        // logo
	AgentName  string `json:"agent_name"`  // 客服名称
	BrandColor string `json:"brand_color"` // 品牌颜色 例如: #f1f1f1
	ChatBgURL  string `json:"chat_bg_url"` // 聊天背景url
}

func (o onboardingReq) Check() error {
	if len(o.AppID) <= 0 {
		return errors.New("appid不能为空！")
	}
	if len(o.AppName) <= 0 {
		return errors.New("app名字不能为空！")
	}
	// if len(o.AgentName) <= 0 {
	// 	return errors.New("客服名称不能为空！")
	// }
	if len(o.BrandColor) <= 0 {
		return errors.New("品牌颜色不能为空！")
	}
	return nil
}

type onboardingResp struct {
	AppID string `json:"app_id"`
	Role  string `json:"role"`
}
