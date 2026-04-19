package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	filemod "github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/file"
	wkutil "github.com/TangSengDaoDao/TangSengDaoDaoServer/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/log"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"github.com/gocraft/dbr/v2"
	"go.uber.org/zap"
)

// Manager 通用后台管理api
type Manager struct {
	ctx *config.Context
	log.Log
	db          *db
	appconfigDB *appConfigDB
}

// NewManager NewManager
func NewManager(ctx *config.Context) *Manager {
	return &Manager{
		ctx:         ctx,
		Log:         log.NewTLog("commonManager"),
		db:          newDB(ctx.DB()),
		appconfigDB: newAppConfigDB(ctx),
	}
}

// Route 配置路由规则
func (m *Manager) Route(r *wkhttp.WKHttp) {
	auth := r.Group("/v1/manager", m.ctx.AuthMiddleware(r))
	{
		auth.GET("/common/appconfig", m.appconfig)               // 获取app配置
		auth.POST("/common/appconfig", m.updateConfig)           // 修改app配置
		auth.GET("/common/appmodule", m.getAppModule)            // 获取app模块
		auth.PUT("/common/appmodule", m.updateAppModule)         // 修改app模块
		auth.POST("/common/appmodule", m.addAppModule)           // 新增app模块
		auth.DELETE("/common/:sid/appmodule", m.deleteAppModule) // 删除app模块
		auth.GET("/invite/codes", m.inviteCodeList)
		auth.POST("/invite/codes", m.inviteCodeCreate)
		auth.PUT("/invite/codes", m.inviteCodeUpdate)
		auth.DELETE("/invite/codes/:id", m.inviteCodeDelete)
		auth.POST("/invite/code", m.inviteCodeCreate)
		auth.PUT("/invite/code", m.inviteCodeUpdate2)
		auth.DELETE("/invite/code/:code", m.inviteCodeDeleteByCode)
		auth.PUT("/invite/code/:code/status", m.inviteCodeToggleStatus)
		auth.GET("/invite/code/:code/users", m.inviteCodeUsers)
		// 邀请码新版路由（与外部管理端保持一致）
		auth.GET("/invitecode", m.inviteCodeListV2)
		auth.POST("/invitecode", m.inviteCodeCreateV2)
		auth.GET("/invitecode/:id", m.inviteCodeDetailV2)
		auth.PUT("/invitecode/:id", m.inviteCodeUpdateV2)
		auth.DELETE("/invitecode/:id", m.inviteCodeDeleteV2)
		auth.GET("/invitecode/:id/usage", m.inviteCodeUsageV2)
		auth.POST("/upload/image", m.uploadImage)
		auth.GET("/pay/recharge-channels", m.rechargeChannelList)
		auth.GET("/pay/recharge-channels/:id", m.rechargeChannelGet)
		auth.POST("/pay/recharge-channels", m.rechargeChannelCreate)
		auth.PUT("/pay/recharge-channels/:id", m.rechargeChannelUpdate)
		auth.PUT("/pay/recharge-channels/:id/status", m.rechargeChannelUpdateStatus)
		auth.DELETE("/pay/recharge-channels/:id", m.rechargeChannelDelete)
		auth.GET("/udun/config", m.udunConfigGet)
		auth.PUT("/udun/config/:id", m.udunConfigUpdate)
		auth.GET("/udun/coin-types", m.udunCoinTypeList)
		auth.POST("/udun/coin-types/sync", m.udunCoinTypeSync)
		auth.PUT("/udun/coin-types/:id", m.udunCoinTypeUpdate)
		auth.PUT("/udun/coin-types/:id/status", m.udunCoinTypeUpdateStatus)
		auth.DELETE("/udun/coin-types/:id", m.udunCoinTypeDelete)
		auth.GET("/aibot/config", m.aibotConfigGet)
		auth.POST("/aibot/config", m.aibotConfigSave)
		auth.POST("/aibot/toggle", m.aibotToggle)
		auth.GET("/aibot/history", m.aibotHistory)
		auth.GET("/web3/laboratory", m.web3LabList)
		auth.POST("/web3/laboratory", m.web3LabCreate)
		auth.PUT("/web3/laboratory/:id", m.web3LabUpdate)
		auth.PUT("/web3/laboratory/:id/status", m.web3LabUpdateStatus)
		auth.DELETE("/web3/laboratory/:id", m.web3LabDelete)
		auth.GET("/hotline/config/list", m.hotlineConfigList)
		auth.GET("/hotline/config/statistics", m.hotlineConfigStatistics)
		auth.POST("/hotline/config", m.hotlineConfigCreate)
		auth.PUT("/hotline/config/:id", m.hotlineConfigUpdate)
		auth.DELETE("/hotline/config/:id", m.hotlineConfigDelete)
		auth.GET("/hotline/agent", m.hotlineAgentList)
		auth.POST("/hotline/agent", m.hotlineAgentAdd)
		auth.DELETE("/hotline/agent/:id", m.hotlineAgentDelete)
		auth.GET("/hotline/topic", m.hotlineTopicList)
		auth.POST("/hotline/topic", m.hotlineTopicCreate)
		auth.PUT("/hotline/topic/:id", m.hotlineTopicUpdate)
		auth.DELETE("/hotline/topic/:id", m.hotlineTopicDelete)
	}
}
func (m *Manager) deleteAppModule(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}

	sid := c.Param("sid")
	if strings.TrimSpace(sid) == "" {
		c.ResponseError(errors.New("sid不能为空！"))
		return
	}
	module, err := m.db.queryAppModuleWithSid(sid)
	if err != nil {
		m.Error("查询app模块错误", zap.Error(err))
		c.ResponseError(errors.New("查询app模块错误"))
		return
	}
	if module == nil {
		c.ResponseError(errors.New("删除的模块不存在"))
		return
	}
	err = m.db.deleteAppModule(sid)
	if err != nil {
		m.Error("删除app模块错误", zap.Error(err))
		c.ResponseError(errors.New("删除app模块错误"))
		return
	}
	c.ResponseOK()
}

// 新增app模块
func (m *Manager) addAppModule(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type ReqVO struct {
		SID    string `json:"sid"`
		Name   string `json:"name"`
		Desc   string `json:"desc"`
		Status int    `json:"status"`
	}
	var req ReqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}

	if strings.TrimSpace(req.SID) == "" || strings.TrimSpace(req.Desc) == "" || strings.TrimSpace(req.Name) == "" {
		c.ResponseError(errors.New("名称/ID/介绍不能为空！"))
		return
	}
	module, err := m.db.queryAppModuleWithSid(req.SID)
	if err != nil {
		m.Error("查询app模块错误", zap.Error(err))
		c.ResponseError(errors.New("查询app模块错误"))
		return
	}
	if module != nil && module.SID != "" {
		c.ResponseError(errors.New("该sid模块已存在"))
		return
	}
	_, err = m.db.insertAppModule(&appModuleModel{
		SID:    req.SID,
		Name:   req.Name,
		Desc:   req.Desc,
		Status: req.Status,
	})
	if err != nil {
		m.Error("新增app模块错误", zap.Error(err))
		c.ResponseError(errors.New("新增app模块错误"))
		return
	}
	c.ResponseOK()
}
func (m *Manager) updateAppModule(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type ReqVO struct {
		SID    string `json:"sid"`
		Name   string `json:"name"`
		Desc   string `json:"desc"`
		Status int    `json:"status"`
	}
	var req ReqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}

	if strings.TrimSpace(req.SID) == "" || strings.TrimSpace(req.Desc) == "" || strings.TrimSpace(req.Name) == "" {
		c.ResponseError(errors.New("名称/ID/介绍不能为空！"))
		return
	}
	module, err := m.db.queryAppModuleWithSid(req.SID)
	if err != nil {
		m.Error("查询app模块错误", zap.Error(err))
		c.ResponseError(errors.New("查询app模块错误"))
		return
	}
	if module == nil {
		c.ResponseError(errors.New("不存在该模块"))
		return
	}
	module.Name = req.Name
	module.Desc = req.Desc
	module.Status = req.Status
	err = m.db.updateAppModule(module)
	if err != nil {
		m.Error("修改app模块错误", zap.Error(err))
		c.ResponseError(errors.New("修改app模块错误"))
		return
	}
	c.ResponseOK()
}

// 获取app模块
func (m *Manager) getAppModule(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	modules, err := m.db.queryAppModule()
	if err != nil {
		m.Error("查询app模块错误", zap.Error(err))
		c.ResponseError(errors.New("查询app模块错误"))
		return
	}
	list := make([]*managerAppModule, 0)
	if len(modules) > 0 {
		for _, module := range modules {
			list = append(list, &managerAppModule{
				SID:    module.SID,
				Name:   module.Name,
				Desc:   module.Desc,
				Status: module.Status,
			})
		}
	}
	c.Response(list)
}
func (m *Manager) updateConfig(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type reqVO struct {
		RevokeSecond                     int    `json:"revoke_second"`
		WelcomeMessage                   string `json:"welcome_message"`
		NewUserJoinSystemGroup           int    `json:"new_user_join_system_group"`
		SearchByPhone                    int    `json:"search_by_phone"`
		PrivilegeOnlyAddFriendOn         int    `json:"privilege_only_add_friend_on"`          // 仅特权用户可搜索添加好友
		FriendApplyAutoAcceptOn          int    `json:"friend_apply_auto_accept_on"`           // 添加好友免验证
		PrivilegeOnlyCreateInviteGroupOn int    `json:"privilege_only_create_invite_group_on"` // 仅特权用户可建群与邀请成员
		ShowLastOfflineOn                int    `json:"show_last_offline_on"`                  // 是否允许客户端看到对方上次在线时间
		InviteCodeSystemOn               int    `json:"invite_code_system_on"`                 // 邀请码系统总开关
		RegisterInviteOn                 int    `json:"register_invite_on"`                    // 开启注册邀请机制
		SendWelcomeMessageOn             int    `json:"send_welcome_message_on"`               // 开启注册登录发送欢迎语
		InviteSystemAccountJoinGroupOn   int    `json:"invite_system_account_join_group_on"`   // 开启系统账号加入群聊
		RegisterUserMustCompleteInfoOn   int    `json:"register_user_must_complete_info_on"`   // 注册用户必须填写完整信息
		ChannelPinnedMessageMaxCount     int    `json:"channel_pinned_message_max_count"`      // 频道置顶消息最大数量
		CanModifyApiUrl                  int    `json:"can_modify_api_url"`                    // 是否可以修改api地址
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	appConfigM, err := m.appconfigDB.query()
	if err != nil {
		m.Error("查询应用配置失败！", zap.Error(err))
		c.ResponseError(errors.New("查询应用配置失败！"))
		return
	}
	configMap := map[string]interface{}{}
	configMap["revoke_second"] = req.RevokeSecond
	configMap["welcome_message"] = req.WelcomeMessage
	configMap["new_user_join_system_group"] = req.NewUserJoinSystemGroup
	configMap["search_by_phone"] = req.SearchByPhone
	configMap["privilege_only_add_friend_on"] = req.PrivilegeOnlyAddFriendOn
	configMap["friend_apply_auto_accept_on"] = req.FriendApplyAutoAcceptOn
	configMap["privilege_only_create_invite_group_on"] = req.PrivilegeOnlyCreateInviteGroupOn
	configMap["show_last_offline_on"] = req.ShowLastOfflineOn
	configMap["invite_code_system_on"] = req.InviteCodeSystemOn
	configMap["register_invite_on"] = req.RegisterInviteOn
	configMap["send_welcome_message_on"] = req.SendWelcomeMessageOn
	configMap["invite_system_account_join_group_on"] = req.InviteSystemAccountJoinGroupOn
	configMap["register_user_must_complete_info_on"] = req.RegisterUserMustCompleteInfoOn
	configMap["channel_pinned_message_max_count"] = req.ChannelPinnedMessageMaxCount
	configMap["can_modify_api_url"] = req.CanModifyApiUrl
	err = m.appconfigDB.updateWithMap(configMap, appConfigM.Id)
	if err != nil {
		m.Error("修改app配置信息错误", zap.Error(err))
		c.ResponseError(errors.New("修改app配置信息错误"))
		return
	}
	c.ResponseOK()
}
func (m *Manager) appconfig(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	appconfig, err := m.appconfigDB.query()
	if err != nil {
		m.Error("查询应用配置失败！", zap.Error(err))
		c.ResponseError(errors.New("查询应用配置失败！"))
		return
	}
	var revokeSecond = 0
	var newUserJoinSystemGroup = 1
	var welcomeMessage = ""
	var searchByPhone = 1
	var registerInviteOn = 0
	var privilegeOnlyAddFriendOn = 0
	var friendApplyAutoAcceptOn = 0
	var privilegeOnlyCreateInviteGroupOn = 0
	var showLastOfflineOn = 1
	var inviteCodeSystemOn = 1
	var sendWelcomeMessageOn = 0
	var inviteSystemAccountJoinGroupOn = 0
	var registerUserMustCompleteInfoOn = 0
	var channelPinnedMessageMaxCount = 10
	var canModifyApiUrl = 0
	if appconfig != nil {
		revokeSecond = appconfig.RevokeSecond
		welcomeMessage = appconfig.WelcomeMessage
		newUserJoinSystemGroup = appconfig.NewUserJoinSystemGroup
		searchByPhone = appconfig.SearchByPhone
		registerInviteOn = appconfig.RegisterInviteOn
		privilegeOnlyAddFriendOn = appconfig.PrivilegeOnlyAddFriendOn
		friendApplyAutoAcceptOn = appconfig.FriendApplyAutoAcceptOn
		privilegeOnlyCreateInviteGroupOn = appconfig.PrivilegeOnlyCreateInviteGroupOn
		showLastOfflineOn = appconfig.ShowLastOfflineOn
		inviteCodeSystemOn = appconfig.InviteCodeSystemOn
		sendWelcomeMessageOn = appconfig.SendWelcomeMessageOn
		inviteSystemAccountJoinGroupOn = appconfig.InviteSystemAccountJoinGroupOn
		registerUserMustCompleteInfoOn = appconfig.RegisterUserMustCompleteInfoOn
		channelPinnedMessageMaxCount = appconfig.ChannelPinnedMessageMaxCount
		canModifyApiUrl = appconfig.CanModifyApiUrl
	}
	if revokeSecond == 0 {
		revokeSecond = 120
	}
	if welcomeMessage == "" {
		welcomeMessage = m.ctx.GetConfig().WelcomeMessage
	}
	if welcomeMessage == "" {
		welcomeMessage = "欢迎使用 AI私域课堂"
	}
	c.Response(&managerAppConfigResp{
		RevokeSecond:                     revokeSecond,
		WelcomeMessage:                   welcomeMessage,
		NewUserJoinSystemGroup:           newUserJoinSystemGroup,
		SearchByPhone:                    searchByPhone,
		RegisterInviteOn:                 registerInviteOn,
		PrivilegeOnlyAddFriendOn:         privilegeOnlyAddFriendOn,
		FriendApplyAutoAcceptOn:          friendApplyAutoAcceptOn,
		PrivilegeOnlyCreateInviteGroupOn: privilegeOnlyCreateInviteGroupOn,
		ShowLastOfflineOn:                showLastOfflineOn,
		InviteCodeSystemOn:               inviteCodeSystemOn,
		SendWelcomeMessageOn:             sendWelcomeMessageOn,
		InviteSystemAccountJoinGroupOn:   inviteSystemAccountJoinGroupOn,
		RegisterUserMustCompleteInfoOn:   registerUserMustCompleteInfoOn,
		ChannelPinnedMessageMaxCount:     channelPinnedMessageMaxCount,
		CanModifyApiUrl:                  canModifyApiUrl,
	})
}

func (m *Manager) inviteCodeList(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	pageIndex, pageSize := c.GetPage()
	offset := (pageIndex - 1) * pageSize

	var total int
	m.ctx.DB().Select("count(*)").From("invite_code").Load(&total)

	type codeModel struct {
		ID          int64  `db:"id" json:"id"`
		InviteCode  string `db:"invite_code" json:"invite_code"`
		UID         string `db:"uid" json:"uid"`
		MaxUseCount int    `db:"max_use_count" json:"max_use_count"`
		UsedCount   int    `db:"used_count" json:"used_count"`
		ExpireAt    int64  `db:"expire_at" json:"expire_at"`
		Status      int    `db:"status" json:"status"`
		Remark      string `db:"remark" json:"remark"`
		CreatedAt   int64  `db:"created_at" json:"created_at"`
		UpdatedAt   int64  `db:"updated_at" json:"updated_at"`
	}
	var list []*codeModel
	m.ctx.DB().Select("id,invite_code,uid,max_use_count,used_count,expire_at,status,remark,UNIX_TIMESTAMP(created_at) as created_at,UNIX_TIMESTAMP(updated_at) as updated_at").From("invite_code").OrderDir("created_at", false).Limit(uint64(pageSize)).Offset(uint64(offset)).Load(&list)
	if list == nil {
		list = make([]*codeModel, 0)
	}
	c.Response(map[string]interface{}{
		"page_index": pageIndex,
		"page_size":  pageSize,
		"total":      total,
		"data":       list,
	})
}

func (m *Manager) inviteCodeCreate(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type reqVO struct {
		InviteCode  string `json:"invite_code"`
		UID         string `json:"uid"`
		MaxUseCount int    `json:"max_use_count"`
		ExpireAt    int64  `json:"expire_at"`
		ExpireDays  int    `json:"expire_days"`
		Status      int    `json:"status"`
		Remark      string `json:"remark"`
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	code := req.InviteCode
	if code == "" {
		code = fmt.Sprintf("%06d", rand.Intn(1000000))
	}
	uid := req.UID
	if uid == "" {
		uid = c.GetLoginUID()
	}
	expireAt := req.ExpireAt
	if expireAt == 0 && req.ExpireDays > 0 {
		expireAt = int64(req.ExpireDays*86400) + time.Now().Unix()
	}
	status := req.Status
	if status == 0 && req.ExpireAt == 0 && req.ExpireDays == 0 {
		status = 1
	}
	_, err = m.ctx.DB().InsertInto("invite_code").Columns("invite_code", "uid", "max_use_count", "expire_at", "status", "remark").Values(code, uid, req.MaxUseCount, expireAt, status, req.Remark).Exec()
	if err != nil {
		m.Error("创建邀请码错误", zap.Error(err))
		c.ResponseError(errors.New("创建邀请码错误"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) inviteCodeUpdate(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type reqVO struct {
		ID     int64  `json:"id"`
		Status int    `json:"status"`
		Remark string `json:"remark"`
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	_, err = m.ctx.DB().Update("invite_code").Set("status", req.Status).Set("remark", req.Remark).Set("updated_at", dbr.Now).Where("id=?", req.ID).Exec()
	if err != nil {
		m.Error("更新邀请码错误", zap.Error(err))
		c.ResponseError(errors.New("更新邀请码错误"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) inviteCodeDelete(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	idStr := c.Param("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	_, err = m.ctx.DB().DeleteFrom("invite_code").Where("id=?", id).Exec()
	if err != nil {
		m.Error("删除邀请码错误", zap.Error(err))
		c.ResponseError(errors.New("删除邀请码错误"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) inviteCodeUpdate2(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type reqVO struct {
		InviteCode  string `json:"invite_code"`
		MaxUseCount int    `json:"max_use_count"`
		ExpireDays  int    `json:"expire_days"`
		Status      int    `json:"status"`
		Remark      string `json:"remark"`
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	if req.InviteCode == "" {
		c.ResponseError(errors.New("邀请码不能为空"))
		return
	}
	expireAt := int64(0)
	if req.ExpireDays > 0 {
		expireAt = int64(req.ExpireDays*86400) + time.Now().Unix()
	}
	builder := m.ctx.DB().Update("invite_code").
		Set("max_use_count", req.MaxUseCount).
		Set("status", req.Status).
		Set("remark", req.Remark).
		Set("updated_at", dbr.Now)
	if req.ExpireDays > 0 {
		builder = builder.Set("expire_at", expireAt)
	} else {
		builder = builder.Set("expire_at", 0)
	}
	_, err = builder.Where("invite_code=?", req.InviteCode).Exec()
	if err != nil {
		m.Error("更新邀请码错误", zap.Error(err))
		c.ResponseError(errors.New("更新邀请码错误"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) inviteCodeDeleteByCode(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	code := c.Param("code")
	if code == "" {
		c.ResponseError(errors.New("邀请码不能为空"))
		return
	}
	_, err = m.ctx.DB().DeleteFrom("invite_code").Where("invite_code=?", code).Exec()
	if err != nil {
		m.Error("删除邀请码错误", zap.Error(err))
		c.ResponseError(errors.New("删除邀请码错误"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) inviteCodeToggleStatus(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	code := c.Param("code")
	if code == "" {
		c.ResponseError(errors.New("邀请码不能为空"))
		return
	}
	type reqVO struct {
		Status int `json:"status"`
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	_, err = m.ctx.DB().Update("invite_code").Set("status", req.Status).Set("updated_at", dbr.Now).Where("invite_code=?", code).Exec()
	if err != nil {
		m.Error("切换邀请码状态错误", zap.Error(err))
		c.ResponseError(errors.New("切换邀请码状态错误"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) inviteCodeUsers(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	code := c.Param("code")
	if code == "" {
		c.ResponseError(errors.New("邀请码不能为空"))
		return
	}
	type userModel struct {
		UID       string `db:"uid" json:"uid"`
		Name      string `db:"name" json:"name"`
		Username  string `db:"username" json:"username"`
		Zone      string `db:"zone" json:"zone"`
		Phone     string `db:"phone" json:"phone"`
		CreatedAt int64  `db:"created_at" json:"created_at"`
	}
	var list []*userModel
	m.ctx.DB().Select("u.uid,u.name,u.username,u.zone,u.phone,UNIX_TIMESTAMP(u.created_at) as created_at").
		From("user u").
		Where("u.invite_code=?", code).
		OrderDir("u.created_at", false).
		Load(&list)
	if list == nil {
		list = make([]*userModel, 0)
	}
	c.Response(list)
}

type inviteCodeFriendCfg struct {
	FriendUID      string `json:"friend_uid"`
	WelcomeMessage string `json:"welcome_message"`
}

func parseInviteCodeGroups(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	out := make([]string, 0)
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func parseInviteCodeFriends(raw string) []inviteCodeFriendCfg {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []inviteCodeFriendCfg{}
	}
	out := make([]inviteCodeFriendCfg, 0)
	if err := json.Unmarshal([]byte(raw), &out); err == nil {
		return out
	}
	// 兼容历史结构字段（uid/to_uid/msg 等），避免编辑页回显丢失。
	var legacy []map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &legacy); err != nil {
		return []inviteCodeFriendCfg{}
	}
	converted := make([]inviteCodeFriendCfg, 0, len(legacy))
	for _, item := range legacy {
		friendUID := strings.TrimSpace(fmt.Sprint(item["friend_uid"]))
		if friendUID == "" {
			friendUID = strings.TrimSpace(fmt.Sprint(item["uid"]))
		}
		if friendUID == "" {
			friendUID = strings.TrimSpace(fmt.Sprint(item["to_uid"]))
		}
		if friendUID == "" {
			continue
		}
		welcomeMessage := strings.TrimSpace(fmt.Sprint(item["welcome_message"]))
		if welcomeMessage == "" {
			welcomeMessage = strings.TrimSpace(fmt.Sprint(item["message"]))
		}
		converted = append(converted, inviteCodeFriendCfg{
			FriendUID:      friendUID,
			WelcomeMessage: welcomeMessage,
		})
	}
	return converted
}

func marshalInviteCodeGroups(groups []string) string {
	if len(groups) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(groups)
	return string(b)
}

func marshalInviteCodeFriends(friends []inviteCodeFriendCfg) string {
	if len(friends) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(friends)
	return string(b)
}

func (m *Manager) inviteCodeListV2(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	pageIndex, pageSize := c.GetPage()
	offset := (pageIndex - 1) * pageSize
	keyword := strings.TrimSpace(c.Query("keyword"))

	var total int
	totalBuilder := m.ctx.DB().Select("count(*)").From("invite_code")
	if keyword != "" {
		totalBuilder = totalBuilder.Where("invite_code like ? or remark like ? or name like ?", "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}
	_, _ = totalBuilder.Load(&total)

	type codeModel struct {
		ID            int64  `db:"id"`
		InviteCode    string `db:"invite_code"`
		Name          string `db:"name"`
		Status        int    `db:"status"`
		Remark        string `db:"remark"`
		GroupsJSON    string `db:"groups_json"`
		FriendsJSON   string `db:"friends_json"`
		SystemWelcome string `db:"system_welcome"`
		UsageCount    int    `db:"used_count"`
		CreatedAt     string `db:"created_at"`
		UpdatedAt     string `db:"updated_at"`
	}
	var list []*codeModel
	builder := m.ctx.DB().
		Select("id,invite_code,name,status,remark,groups_json,friends_json,system_welcome,used_count,created_at,updated_at").
		From("invite_code").
		OrderDir("created_at", false).
		Limit(uint64(pageSize)).
		Offset(uint64(offset))
	if keyword != "" {
		builder = builder.Where("invite_code like ? or remark like ? or name like ?", "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}
	_, _ = builder.Load(&list)
	if list == nil {
		list = make([]*codeModel, 0)
	}
	respList := make([]map[string]interface{}, 0, len(list))
	for _, item := range list {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = item.Remark
		}
		respList = append(respList, map[string]interface{}{
			"id":             item.ID,
			"code":           item.InviteCode,
			"name":           name,
			"status":         item.Status,
			"groups":         parseInviteCodeGroups(item.GroupsJSON),
			"friends":        parseInviteCodeFriends(item.FriendsJSON),
			"system_welcome": item.SystemWelcome,
			"usage_count":    item.UsageCount,
			"created_at":     item.CreatedAt,
			"updated_at":     item.UpdatedAt,
		})
	}
	c.Response(map[string]interface{}{
		"list":       respList,
		"total":      total,
		"page_index": pageIndex,
		"page_size":  pageSize,
	})
}

func (m *Manager) inviteCodeCreateV2(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type reqVO struct {
		Code          string                `json:"code"`
		Name          string                `json:"name"`
		Status        int                   `json:"status"`
		Groups        []string              `json:"groups"`
		Friends       []inviteCodeFriendCfg `json:"friends"`
		SystemWelcome string                `json:"system_welcome"`
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	code := strings.TrimSpace(req.Code)
	if code == "" {
		code = fmt.Sprintf("%06d", rand.Intn(1000000))
	}
	status := req.Status
	if status != 0 && status != 1 {
		status = 1
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = code
	}
	groupsJSON := marshalInviteCodeGroups(req.Groups)
	friendsJSON := marshalInviteCodeFriends(req.Friends)
	_, err = m.ctx.DB().
		InsertInto("invite_code").
		Columns("invite_code", "uid", "status", "remark", "name", "groups_json", "friends_json", "system_welcome").
		Values(code, c.GetLoginUID(), status, name, name, groupsJSON, friendsJSON, strings.TrimSpace(req.SystemWelcome)).
		Exec()
	if err != nil {
		m.Error("创建邀请码错误", zap.Error(err))
		c.ResponseError(errors.New("创建邀请码错误"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) inviteCodeDetailV2(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if id <= 0 {
		c.ResponseError(errors.New("邀请码ID不能为空"))
		return
	}
	type modelRow struct {
		ID            int64  `db:"id"`
		InviteCode    string `db:"invite_code"`
		Name          string `db:"name"`
		Status        int    `db:"status"`
		Remark        string `db:"remark"`
		GroupsJSON    string `db:"groups_json"`
		FriendsJSON   string `db:"friends_json"`
		SystemWelcome string `db:"system_welcome"`
		CreatedAt     string `db:"created_at"`
		UpdatedAt     string `db:"updated_at"`
	}
	var rows []*modelRow
	_, _ = m.ctx.DB().Select("id,invite_code,name,status,remark,groups_json,friends_json,system_welcome,created_at,updated_at").
		From("invite_code").
		Where("id=?", id).
		Load(&rows)
	if len(rows) == 0 {
		c.ResponseError(errors.New("邀请码不存在"))
		return
	}
	item := rows[0]
	name := strings.TrimSpace(item.Name)
	if name == "" {
		name = item.Remark
	}
	c.Response(map[string]interface{}{
		"id":             item.ID,
		"code":           item.InviteCode,
		"name":           name,
		"status":         item.Status,
		"groups":         parseInviteCodeGroups(item.GroupsJSON),
		"friends":        parseInviteCodeFriends(item.FriendsJSON),
		"system_welcome": item.SystemWelcome,
		"created_at":     item.CreatedAt,
		"updated_at":     item.UpdatedAt,
	})
}

func (m *Manager) inviteCodeUpdateV2(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if id <= 0 {
		c.ResponseError(errors.New("邀请码ID不能为空"))
		return
	}
	type reqVO struct {
		Name          string                `json:"name"`
		Status        int                   `json:"status"`
		Groups        []string              `json:"groups"`
		Friends       []inviteCodeFriendCfg `json:"friends"`
		SystemWelcome string                `json:"system_welcome"`
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "-"
	}
	_, err = m.ctx.DB().Update("invite_code").
		Set("name", name).
		Set("remark", name).
		Set("status", req.Status).
		Set("groups_json", marshalInviteCodeGroups(req.Groups)).
		Set("friends_json", marshalInviteCodeFriends(req.Friends)).
		Set("system_welcome", strings.TrimSpace(req.SystemWelcome)).
		Set("updated_at", dbr.Now).
		Where("id=?", id).
		Exec()
	if err != nil {
		m.Error("更新邀请码错误", zap.Error(err))
		c.ResponseError(errors.New("更新邀请码错误"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) inviteCodeDeleteV2(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if id <= 0 {
		c.ResponseError(errors.New("邀请码ID不能为空"))
		return
	}
	_, err = m.ctx.DB().DeleteFrom("invite_code").Where("id=?", id).Exec()
	if err != nil {
		m.Error("删除邀请码错误", zap.Error(err))
		c.ResponseError(errors.New("删除邀请码错误"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) inviteCodeUsageV2(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if id <= 0 {
		c.ResponseError(errors.New("邀请码ID不能为空"))
		return
	}
	type codeRow struct {
		InviteCode string `db:"invite_code"`
	}
	var codeRows []*codeRow
	_, _ = m.ctx.DB().Select("invite_code").From("invite_code").Where("id=?", id).Load(&codeRows)
	if len(codeRows) == 0 || strings.TrimSpace(codeRows[0].InviteCode) == "" {
		c.Response(map[string]interface{}{"list": []interface{}{}, "total": 0})
		return
	}
	pageIndex, pageSize := c.GetPage()
	offset := (pageIndex - 1) * pageSize
	code := strings.TrimSpace(codeRows[0].InviteCode)
	type userModel struct {
		UID       string `db:"uid" json:"uid"`
		Name      string `db:"name" json:"name"`
		Username  string `db:"username" json:"username"`
		Zone      string `db:"zone" json:"zone"`
		Phone     string `db:"phone" json:"phone"`
		CreatedAt string `db:"created_at" json:"created_at"`
	}
	var total int
	_, _ = m.ctx.DB().Select("count(*)").From("user").Where("invite_code=?", code).Load(&total)
	var list []*userModel
	_, _ = m.ctx.DB().Select("uid,name,username,zone,phone,created_at").
		From("user").
		Where("invite_code=?", code).
		OrderDir("created_at", false).
		Limit(uint64(pageSize)).
		Offset(uint64(offset)).
		Load(&list)
	if list == nil {
		list = make([]*userModel, 0)
	}
	c.Response(map[string]interface{}{
		"list":       list,
		"total":      total,
		"page_index": pageIndex,
		"page_size":  pageSize,
	})
}

type managerAppConfigResp struct {
	RevokeSecond                     int    `json:"revoke_second"`
	WelcomeMessage                   string `json:"welcome_message"`
	NewUserJoinSystemGroup           int    `json:"new_user_join_system_group"`
	SearchByPhone                    int    `json:"search_by_phone"`
	PrivilegeOnlyAddFriendOn         int    `json:"privilege_only_add_friend_on"`          // 仅特权用户可搜索添加好友
	FriendApplyAutoAcceptOn          int    `json:"friend_apply_auto_accept_on"`           // 添加好友免验证
	PrivilegeOnlyCreateInviteGroupOn int    `json:"privilege_only_create_invite_group_on"` // 仅特权用户可建群与邀请成员
	ShowLastOfflineOn                int    `json:"show_last_offline_on"`                  // 是否允许客户端看到对方上次在线时间
	InviteCodeSystemOn               int    `json:"invite_code_system_on"`                 // 邀请码系统总开关
	RegisterInviteOn                 int    `json:"register_invite_on"`                    // 开启注册邀请机制
	SendWelcomeMessageOn             int    `json:"send_welcome_message_on"`               // 开启注册登录发送欢迎语
	InviteSystemAccountJoinGroupOn   int    `json:"invite_system_account_join_group_on"`   // 开启系统账号加入群聊
	RegisterUserMustCompleteInfoOn   int    `json:"register_user_must_complete_info_on"`   // 注册用户必须填写完整信息
	ChannelPinnedMessageMaxCount     int    `json:"channel_pinned_message_max_count"`      // 频道置顶消息最大数量
	CanModifyApiUrl                  int    `json:"can_modify_api_url"`                    // 是否可以修改api地址
}

type managerAppModule struct {
	SID    string `json:"sid"`
	Name   string `json:"name"`
	Desc   string `json:"desc"`
	Status int    `json:"status"` // 模块状态 1.可选 0.不可选 2.选中不可编辑
}

// ===== Recharge Channel =====

func (m *Manager) rechargeChannelList(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	pageIndex, pageSize := c.GetPage()
	offset := (pageIndex - 1) * pageSize

	var total int
	m.ctx.DB().Select("count(*)").From("recharge_channel").Load(&total)

	type channelModel struct {
		ID         int64  `db:"id" json:"id"`
		AppID      string `db:"app_id" json:"app_id"`
		PayType    int    `db:"pay_type" json:"pay_type"`
		Icon       string `db:"icon" json:"icon"`
		QrURL      string `db:"qr_url" json:"qr_url"`
		QrImageURL string `db:"qr_image_url" json:"qr_image_url"`
		PayAddress string `db:"pay_address" json:"pay_address"`
		InstallKey string `db:"install_key" json:"install_key"`
		Status     int    `db:"status" json:"status"`
		Title      string `db:"title" json:"title"`
		Remark     string `db:"remark" json:"remark"`
		CreatedAt  int64  `db:"created_at" json:"created_at"`
		UpdatedAt  int64  `db:"updated_at" json:"updated_at"`
	}
	var list []*channelModel
	m.ctx.DB().Select("id,app_id,pay_type,icon,qr_url,qr_image_url,pay_address,install_key,status,title,remark,UNIX_TIMESTAMP(created_at) as created_at,UNIX_TIMESTAMP(updated_at) as updated_at").From("recharge_channel").OrderDir("created_at", false).Limit(uint64(pageSize)).Offset(uint64(offset)).Load(&list)
	if list == nil {
		list = make([]*channelModel, 0)
	}
	c.Response(map[string]interface{}{
		"page_index": pageIndex,
		"page_size":  pageSize,
		"total":      total,
		"data":       list,
	})
}

func (m *Manager) rechargeChannelGet(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	idStr := c.Param("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	if id <= 0 {
		c.ResponseError(errors.New("无效的渠道ID"))
		return
	}
	type channelModel struct {
		ID         int64  `db:"id" json:"id"`
		AppID      string `db:"app_id" json:"app_id"`
		PayType    int    `db:"pay_type" json:"pay_type"`
		Icon       string `db:"icon" json:"icon"`
		QrURL      string `db:"qr_url" json:"qr_url"`
		QrImageURL string `db:"qr_image_url" json:"qr_image_url"`
		PayAddress string `db:"pay_address" json:"pay_address"`
		InstallKey string `db:"install_key" json:"install_key"`
		Status     int    `db:"status" json:"status"`
		Title      string `db:"title" json:"title"`
		Remark     string `db:"remark" json:"remark"`
		CreatedAt  int64  `db:"created_at" json:"created_at"`
		UpdatedAt  int64  `db:"updated_at" json:"updated_at"`
	}
	var list []*channelModel
	n, err := m.ctx.DB().Select("id,app_id,pay_type,icon,qr_url,qr_image_url,pay_address,install_key,status,title,remark,UNIX_TIMESTAMP(created_at) as created_at,UNIX_TIMESTAMP(updated_at) as updated_at").From("recharge_channel").Where("id=?", id).Load(&list)
	if err != nil || n == 0 {
		c.ResponseError(errors.New("渠道不存在"))
		return
	}
	ch := list[0]
	cfg := m.ctx.GetConfig()
	qrImageFull := ""
	if p := strings.TrimSpace(ch.QrImageURL); p != "" {
		qrImageFull = wkutil.FullAPIURLForFilePreview(cfg.External.APIBaseURL, p, cfg.Minio.UploadURL, cfg.Minio.DownloadURL)
	}
	c.Response(map[string]interface{}{
		"id":                   ch.ID,
		"app_id":               ch.AppID,
		"pay_type":             ch.PayType,
		"icon":                 ch.Icon,
		"qr_url":               ch.QrURL,
		"qr_image_url":         ch.QrImageURL,
		"qr_image_display_url": qrImageFull,
		"pay_address":          ch.PayAddress,
		"install_key":          ch.InstallKey,
		"status":               ch.Status,
		"title":                ch.Title,
		"remark":               ch.Remark,
		"created_at":           ch.CreatedAt,
		"updated_at":           ch.UpdatedAt,
	})
}

func (m *Manager) rechargeChannelCreate(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type reqVO struct {
		AppID      string `json:"app_id"`
		PayType    int    `json:"pay_type"`
		Icon       string `json:"icon"`
		QrURL      string `json:"qr_url"`
		QrImageURL string `json:"qr_image_url"`
		PayAddress string `json:"pay_address"`
		InstallKey string `json:"install_key"`
		Status     int    `json:"status"`
		Title      string `json:"title"`
		Remark     string `json:"remark"`
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		c.ResponseError(errors.New("标题不能为空"))
		return
	}
	appID := req.AppID
	if appID == "" {
		appID = "tsdd_app"
	}
	_, err = m.ctx.DB().InsertInto("recharge_channel").Columns("app_id", "pay_type", "icon", "qr_url", "qr_image_url", "pay_address", "install_key", "status", "title", "remark").Values(appID, req.PayType, req.Icon, req.QrURL, req.QrImageURL, req.PayAddress, req.InstallKey, req.Status, req.Title, req.Remark).Exec()
	if err != nil {
		m.Error("创建充值渠道错误", zap.Error(err))
		c.ResponseError(errors.New("创建充值渠道错误"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) rechargeChannelUpdate(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	idStr := c.Param("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	type reqVO struct {
		AppID      string `json:"app_id"`
		PayType    int    `json:"pay_type"`
		Icon       string `json:"icon"`
		QrURL      string `json:"qr_url"`
		QrImageURL string `json:"qr_image_url"`
		PayAddress string `json:"pay_address"`
		InstallKey string `json:"install_key"`
		Title      string `json:"title"`
		Remark     string `json:"remark"`
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	_, err = m.ctx.DB().Update("recharge_channel").
		Set("app_id", req.AppID).
		Set("pay_type", req.PayType).
		Set("icon", req.Icon).
		Set("qr_url", req.QrURL).
		Set("qr_image_url", req.QrImageURL).
		Set("pay_address", req.PayAddress).
		Set("install_key", req.InstallKey).
		Set("title", req.Title).
		Set("remark", req.Remark).
		Set("updated_at", dbr.Now).
		Where("id=?", id).Exec()
	if err != nil {
		m.Error("更新充值渠道错误", zap.Error(err))
		c.ResponseError(errors.New("更新充值渠道错误"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) rechargeChannelUpdateStatus(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	idStr := c.Param("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	type reqVO struct {
		Status int `json:"status"`
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	_, err = m.ctx.DB().Update("recharge_channel").Set("status", req.Status).Set("updated_at", dbr.Now).Where("id=?", id).Exec()
	if err != nil {
		m.Error("更新充值渠道状态错误", zap.Error(err))
		c.ResponseError(errors.New("更新充值渠道状态错误"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) rechargeChannelDelete(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	idStr := c.Param("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	_, err = m.ctx.DB().DeleteFrom("recharge_channel").Where("id=?", id).Exec()
	if err != nil {
		m.Error("删除充值渠道错误", zap.Error(err))
		c.ResponseError(errors.New("删除充值渠道错误"))
		return
	}
	c.ResponseOK()
}

// ===== Udun Config =====

func (m *Manager) udunConfigGet(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type configModel struct {
		ID          int64  `db:"id" json:"id"`
		BaseURL     string `db:"base_url" json:"base_url"`
		MerchantID  string `db:"merchant_id" json:"merchant_id"`
		SignKey     string `db:"sign_key" json:"sign_key"`
		CallbackURL string `db:"callback_url" json:"callback_url"`
		Timeout     int    `db:"timeout" json:"timeout"`
		Status      int    `db:"status" json:"status"`
		CreatedAt   int64  `db:"created_at" json:"created_at"`
		UpdatedAt   int64  `db:"updated_at" json:"updated_at"`
	}
	var list []*configModel
	m.ctx.DB().Select("id,base_url,merchant_id,sign_key,callback_url,timeout,status,UNIX_TIMESTAMP(created_at) as created_at,UNIX_TIMESTAMP(updated_at) as updated_at").From("udun_config").Load(&list)
	if len(list) > 0 {
		c.Response(list[0])
	} else {
		c.Response(map[string]interface{}{
			"id": 0, "base_url": "", "merchant_id": "", "sign_key": "",
			"callback_url": "", "timeout": 30, "status": 1,
		})
	}
}

func (m *Manager) udunConfigUpdate(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	idStr := c.Param("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	type reqVO struct {
		BaseURL     string `json:"base_url"`
		MerchantID  string `json:"merchant_id"`
		SignKey     string `json:"sign_key"`
		CallbackURL string `json:"callback_url"`
		Timeout     int    `json:"timeout"`
		Status      int    `json:"status"`
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	_, err = m.ctx.DB().Update("udun_config").
		Set("base_url", req.BaseURL).
		Set("merchant_id", req.MerchantID).
		Set("sign_key", req.SignKey).
		Set("callback_url", req.CallbackURL).
		Set("timeout", req.Timeout).
		Set("status", req.Status).
		Set("updated_at", dbr.Now).
		Where("id=?", id).Exec()
	if err != nil {
		m.Error("更新Udun配置错误", zap.Error(err))
		c.ResponseError(errors.New("更新Udun配置错误"))
		return
	}
	c.ResponseOK()
}

// ===== Udun Coin Type =====

func (m *Manager) udunCoinTypeList(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	pageIndex, pageSize := c.GetPage()
	offset := (pageIndex - 1) * pageSize

	var total int
	m.ctx.DB().Select("count(*)").From("udun_coin_type").Load(&total)

	type coinModel struct {
		ID         int64  `db:"id" json:"id"`
		Symbol     string `db:"symbol" json:"symbol"`
		CoinName   string `db:"coin_name" json:"coin_name"`
		Name       string `db:"name" json:"name"`
		MainSymbol string `db:"main_symbol" json:"main_symbol"`
		Decimals   string `db:"decimals" json:"decimals"`
		MinTrade   string `db:"min_trade" json:"min_trade"`
		Status     int    `db:"status" json:"status"`
		Tips       string `db:"tips" json:"tips"`
		CreatedAt  int64  `db:"created_at" json:"created_at"`
		UpdatedAt  int64  `db:"updated_at" json:"updated_at"`
	}
	var list []*coinModel
	m.ctx.DB().Select("id,symbol,coin_name,name,main_symbol,decimals,min_trade,status,tips,UNIX_TIMESTAMP(created_at) as created_at,UNIX_TIMESTAMP(updated_at) as updated_at").From("udun_coin_type").OrderDir("created_at", false).Limit(uint64(pageSize)).Offset(uint64(offset)).Load(&list)
	if list == nil {
		list = make([]*coinModel, 0)
	}
	c.Response(map[string]interface{}{
		"page_index": pageIndex,
		"page_size":  pageSize,
		"total":      total,
		"data":       list,
	})
}

func (m *Manager) udunCoinTypeSync(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	c.ResponseOK()
}

func (m *Manager) udunCoinTypeUpdate(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	idStr := c.Param("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	type reqVO struct {
		Symbol     string `json:"symbol"`
		CoinName   string `json:"coin_name"`
		Name       string `json:"name"`
		MainSymbol string `json:"main_symbol"`
		Decimals   string `json:"decimals"`
		MinTrade   string `json:"min_trade"`
		Tips       string `json:"tips"`
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	_, err = m.ctx.DB().Update("udun_coin_type").
		Set("symbol", req.Symbol).
		Set("coin_name", req.CoinName).
		Set("name", req.Name).
		Set("main_symbol", req.MainSymbol).
		Set("decimals", req.Decimals).
		Set("min_trade", req.MinTrade).
		Set("tips", req.Tips).
		Set("updated_at", dbr.Now).
		Where("id=?", id).Exec()
	if err != nil {
		m.Error("更新币种错误", zap.Error(err))
		c.ResponseError(errors.New("更新币种错误"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) udunCoinTypeUpdateStatus(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	idStr := c.Param("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	type reqVO struct {
		Status int `json:"status"`
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	_, err = m.ctx.DB().Update("udun_coin_type").Set("status", req.Status).Set("updated_at", dbr.Now).Where("id=?", id).Exec()
	if err != nil {
		m.Error("更新币种状态错误", zap.Error(err))
		c.ResponseError(errors.New("更新币种状态错误"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) udunCoinTypeDelete(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	idStr := c.Param("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	_, err = m.ctx.DB().DeleteFrom("udun_coin_type").Where("id=?", id).Exec()
	if err != nil {
		m.Error("删除币种错误", zap.Error(err))
		c.ResponseError(errors.New("删除币种错误"))
		return
	}
	c.ResponseOK()
}

// ===== AI Bot =====

func (m *Manager) aibotConfigGet(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}

	type configModel struct {
		ID          int64   `db:"id" json:"id"`
		Enabled     int     `db:"enabled" json:"enabled"`
		Provider    string  `db:"provider" json:"provider"`
		ApiKey      string  `db:"api_key" json:"-"`
		Model       string  `db:"model" json:"model"`
		MaxTokens   int     `db:"max_tokens" json:"max_tokens"`
		Temperature float64 `db:"temperature" json:"temperature"`
		SystemUID   string  `db:"system_uid" json:"system_uid"`
		CreatedAt   string  `db:"created_at" json:"created_at"`
		UpdatedAt   string  `db:"updated_at" json:"updated_at"`
	}
	var list []*configModel
	m.ctx.DB().Select("*").From("aibot_config").Where("id=1").Load(&list)

	if len(list) > 0 {
		cfg := list[0]
		c.Response(map[string]interface{}{
			"id": cfg.ID, "enabled": cfg.Enabled == 1, "provider": cfg.Provider,
			"has_api_key": cfg.ApiKey != "", "api_key": "", "model": cfg.Model,
			"max_tokens": cfg.MaxTokens, "temperature": cfg.Temperature,
			"system_uid": cfg.SystemUID, "created_at": cfg.CreatedAt, "updated_at": cfg.UpdatedAt,
		})
	} else {
		c.Response(map[string]interface{}{
			"enabled": false, "has_api_key": false, "api_key": "", "provider": "deepseek",
			"model": "deepseek-chat", "max_tokens": 2000, "temperature": 0.7,
			"system_uid": "u_10000",
		})
	}
}

func (m *Manager) aibotConfigSave(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type reqVO struct {
		Provider    string  `json:"provider"`
		ApiKey      string  `json:"api_key"`
		Model       string  `json:"model"`
		MaxTokens   int     `json:"max_tokens"`
		Temperature float64 `json:"temperature"`
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	builder := m.ctx.DB().Update("aibot_config").
		Set("provider", req.Provider).
		Set("model", req.Model).
		Set("max_tokens", req.MaxTokens).
		Set("temperature", req.Temperature).
		Set("updated_at", dbr.Now)
	if req.ApiKey != "" {
		builder = builder.Set("api_key", req.ApiKey)
	}
	_, err = builder.Where("id=?", 1).Exec()
	if err != nil {
		m.Error("保存AI机器人配置错误", zap.Error(err))
		c.ResponseError(errors.New("保存AI机器人配置错误"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) aibotToggle(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type reqVO struct {
		Enabled bool `json:"enabled"`
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	enabledVal := 0
	if req.Enabled {
		enabledVal = 1
	}
	_, err = m.ctx.DB().Update("aibot_config").Set("enabled", enabledVal).Set("updated_at", dbr.Now).Where("id=?", 1).Exec()
	if err != nil {
		m.Error("切换AI机器人状态错误", zap.Error(err))
		c.ResponseError(errors.New("切换AI机器人状态错误"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) aibotHistory(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	c.Response(map[string]interface{}{
		"list": make([]interface{}, 0),
	})
}

// ===== Web3 Laboratory =====

func (m *Manager) web3LabList(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	pageIndex, pageSize := c.GetPage()
	offset := (pageIndex - 1) * pageSize

	var total int
	m.ctx.DB().Select("count(*)").From("web3_laboratory").Load(&total)

	type labModel struct {
		ID        int64  `db:"id" json:"id"`
		ShortURL  string `db:"short_url" json:"short_url"`
		URL       string `db:"url" json:"url"`
		Status    int    `db:"status" json:"status"`
		Remark    string `db:"remark" json:"remark"`
		CreatedAt int64  `db:"created_at" json:"created_at"`
		UpdatedAt int64  `db:"updated_at" json:"updated_at"`
	}
	var list []*labModel
	m.ctx.DB().Select("id,short_url,url,status,remark,UNIX_TIMESTAMP(created_at) as created_at,UNIX_TIMESTAMP(updated_at) as updated_at").From("web3_laboratory").OrderDir("created_at", false).Limit(uint64(pageSize)).Offset(uint64(offset)).Load(&list)
	if list == nil {
		list = make([]*labModel, 0)
	}
	c.Response(map[string]interface{}{
		"page_index": pageIndex,
		"page_size":  pageSize,
		"total":      total,
		"data":       list,
	})
}

func (m *Manager) web3LabCreate(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type reqVO struct {
		ShortURL string `json:"short_url"`
		URL      string `json:"url"`
		Status   int    `json:"status"`
		Remark   string `json:"remark"`
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	if strings.TrimSpace(req.ShortURL) == "" {
		c.ResponseError(errors.New("短链接不能为空"))
		return
	}
	_, err = m.ctx.DB().InsertInto("web3_laboratory").Columns("short_url", "url", "status", "remark").Values(req.ShortURL, req.URL, req.Status, req.Remark).Exec()
	if err != nil {
		m.Error("创建Web3实验室项目错误", zap.Error(err))
		c.ResponseError(errors.New("创建Web3实验室项目错误"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) web3LabUpdate(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	idStr := c.Param("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	type reqVO struct {
		ShortURL string `json:"short_url"`
		URL      string `json:"url"`
		Remark   string `json:"remark"`
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	_, err = m.ctx.DB().Update("web3_laboratory").
		Set("short_url", req.ShortURL).
		Set("url", req.URL).
		Set("remark", req.Remark).
		Set("updated_at", dbr.Now).
		Where("id=?", id).Exec()
	if err != nil {
		m.Error("更新Web3实验室项目错误", zap.Error(err))
		c.ResponseError(errors.New("更新Web3实验室项目错误"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) web3LabUpdateStatus(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	idStr := c.Param("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	type reqVO struct {
		Status int `json:"status"`
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	_, err = m.ctx.DB().Update("web3_laboratory").Set("status", req.Status).Set("updated_at", dbr.Now).Where("id=?", id).Exec()
	if err != nil {
		m.Error("更新Web3实验室项目状态错误", zap.Error(err))
		c.ResponseError(errors.New("更新Web3实验室项目状态错误"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) web3LabDelete(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	idStr := c.Param("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	_, err = m.ctx.DB().DeleteFrom("web3_laboratory").Where("id=?", id).Exec()
	if err != nil {
		m.Error("删除Web3实验室项目错误", zap.Error(err))
		c.ResponseError(errors.New("删除Web3实验室项目错误"))
		return
	}
	c.ResponseOK()
}

// ===== Hotline Config (based on official hotline module tables) =====

func (m *Manager) hotlineConfigList(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	pageIndex, pageSize := c.GetPage()
	offset := (pageIndex - 1) * pageSize

	var total int
	m.ctx.DB().Select("count(*)").From("hotline_config").Load(&total)

	type configModel struct {
		ID        int64  `db:"id" json:"id"`
		AppID     string `db:"app_id" json:"app_id"`
		AppName   string `db:"app_name" json:"app_name"`
		UID       string `db:"uid" json:"uid"`
		Logo      string `db:"logo" json:"logo"`
		Color     string `db:"color" json:"color"`
		ChatBg    string `db:"chat_bg" json:"chat_bg"`
		CreatedAt int64  `db:"created_at" json:"created_at"`
		UpdatedAt int64  `db:"updated_at" json:"updated_at"`
	}
	var list []*configModel
	m.ctx.DB().Select("id,app_id,app_name,uid,logo,color,chat_bg,UNIX_TIMESTAMP(created_at) as created_at,UNIX_TIMESTAMP(updated_at) as updated_at").From("hotline_config").OrderDir("created_at", false).Limit(uint64(pageSize)).Offset(uint64(offset)).Load(&list)
	if list == nil {
		list = make([]*configModel, 0)
	}

	type configResp struct {
		ID         int64  `json:"id"`
		AppID      string `json:"app_id"`
		AppName    string `json:"app_name"`
		UID        string `json:"uid"`
		Logo       string `json:"logo"`
		Color      string `json:"color"`
		ChatBg     string `json:"chat_bg"`
		AgentCount int    `json:"agent_count"`
		CreatedAt  int64  `json:"created_at"`
		UpdatedAt  int64  `json:"updated_at"`
	}
	resps := make([]*configResp, 0, len(list))
	for _, cfg := range list {
		var agentCount int
		m.ctx.DB().Select("count(*)").From("hotline_agent").Where("app_id=?", cfg.AppID).Load(&agentCount)
		resps = append(resps, &configResp{
			ID: cfg.ID, AppID: cfg.AppID, AppName: cfg.AppName, UID: cfg.UID,
			Logo: cfg.Logo, Color: cfg.Color, ChatBg: cfg.ChatBg,
			AgentCount: agentCount, CreatedAt: cfg.CreatedAt, UpdatedAt: cfg.UpdatedAt,
		})
	}
	c.Response(map[string]interface{}{
		"page_index": pageIndex,
		"page_size":  pageSize,
		"total":      total,
		"data":       resps,
	})
}

func (m *Manager) hotlineConfigStatistics(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}

	var totalConfigs int
	m.ctx.DB().Select("count(*)").From("hotline_config").Load(&totalConfigs)

	var totalAgents int
	m.ctx.DB().Select("count(*)").From("hotline_agent").Load(&totalAgents)

	var workingAgents int
	m.ctx.DB().Select("count(*)").From("hotline_agent").Where("is_work=1 AND status=1").Load(&workingAgents)

	var todaySessions int
	m.ctx.DB().Select("count(*)").From("hotline_session").Where("DATE(FROM_UNIXTIME(last_session_timestamp))=CURDATE()").Load(&todaySessions)

	c.Response(map[string]interface{}{
		"total_configs":  totalConfigs,
		"total_agents":   totalAgents,
		"online_agents":  workingAgents,
		"today_sessions": todaySessions,
	})
}

func (m *Manager) hotlineConfigCreate(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type reqVO struct {
		AppID   string `json:"app_id"`
		AppName string `json:"app_name"`
		UID     string `json:"uid"`
		Logo    string `json:"logo"`
		Color   string `json:"color"`
		ChatBg  string `json:"chat_bg"`
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	if strings.TrimSpace(req.AppName) == "" {
		c.ResponseError(errors.New("应用名称不能为空"))
		return
	}
	if strings.TrimSpace(req.AppID) == "" {
		c.ResponseError(errors.New("应用ID不能为空"))
		return
	}
	if req.Color == "" {
		req.Color = "#409eff"
	}
	_, err = m.ctx.DB().InsertInto("hotline_config").Columns("app_id", "app_name", "uid", "logo", "color", "chat_bg").Values(req.AppID, req.AppName, req.UID, req.Logo, req.Color, req.ChatBg).Exec()
	if err != nil {
		m.Error("创建客服配置错误", zap.Error(err))
		c.ResponseError(errors.New("创建客服配置错误"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) hotlineConfigUpdate(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	idStr := c.Param("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	type reqVO struct {
		AppName string `json:"app_name"`
		UID     string `json:"uid"`
		Logo    string `json:"logo"`
		Color   string `json:"color"`
		ChatBg  string `json:"chat_bg"`
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	_, err = m.ctx.DB().Update("hotline_config").
		Set("app_name", req.AppName).
		Set("uid", req.UID).
		Set("logo", req.Logo).
		Set("color", req.Color).
		Set("chat_bg", req.ChatBg).
		Set("updated_at", dbr.Now).
		Where("id=?", id).Exec()
	if err != nil {
		m.Error("更新客服配置错误", zap.Error(err))
		c.ResponseError(errors.New("更新客服配置错误"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) hotlineConfigDelete(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	idStr := c.Param("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	type cfgRow struct {
		AppID string `db:"app_id"`
	}
	var cfg cfgRow
	m.ctx.DB().Select("app_id").From("hotline_config").Where("id=?", id).Load(&cfg)

	if cfg.AppID != "" {
		m.ctx.DB().DeleteFrom("hotline_agent").Where("app_id=?", cfg.AppID).Exec()
	}
	_, err = m.ctx.DB().DeleteFrom("hotline_config").Where("id=?", id).Exec()
	if err != nil {
		m.Error("删除客服配置错误", zap.Error(err))
		c.ResponseError(errors.New("删除客服配置错误"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) hotlineAgentList(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	appID := c.Query("app_id")

	type agentModel struct {
		ID         int64  `db:"id" json:"id"`
		AppID      string `db:"app_id" json:"app_id"`
		UID        string `db:"uid" json:"uid"`
		Name       string `db:"name" json:"name"`
		Role       string `db:"role" json:"role"`
		Position   string `db:"position" json:"position"`
		IsWork     int    `db:"is_work" json:"is_work"`
		LastActive int    `db:"last_active" json:"last_active"`
		Status     int    `db:"status" json:"status"`
		CreatedAt  int64  `db:"created_at" json:"created_at"`
	}
	var list []*agentModel
	q := m.ctx.DB().Select("id,app_id,uid,name,role,position,is_work,last_active,status,UNIX_TIMESTAMP(created_at) as created_at").From("hotline_agent")
	if appID != "" {
		q = q.Where("app_id=?", appID)
	}
	q.OrderDir("created_at", false).Load(&list)
	if list == nil {
		list = make([]*agentModel, 0)
	}
	c.Response(list)
}

func (m *Manager) hotlineAgentAdd(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type reqVO struct {
		AppID    string `json:"app_id"`
		UID      string `json:"uid"`
		Name     string `json:"name"`
		Role     string `json:"role"`
		Position string `json:"position"`
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	if strings.TrimSpace(req.AppID) == "" {
		c.ResponseError(errors.New("app_id不能为空"))
		return
	}
	if strings.TrimSpace(req.UID) == "" {
		c.ResponseError(errors.New("uid不能为空"))
		return
	}
	if req.Role == "" {
		req.Role = "agent"
	}
	_, err = m.ctx.DB().InsertInto("hotline_agent").Columns("app_id", "uid", "name", "role", "position", "is_work", "status").Values(req.AppID, req.UID, req.Name, req.Role, req.Position, 1, 1).Exec()
	if err != nil {
		m.Error("添加客服坐席错误", zap.Error(err))
		c.ResponseError(errors.New("添加客服坐席错误"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) hotlineAgentDelete(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	idStr := c.Param("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	_, err = m.ctx.DB().DeleteFrom("hotline_agent").Where("id=?", id).Exec()
	if err != nil {
		m.Error("删除客服坐席错误", zap.Error(err))
		c.ResponseError(errors.New("删除客服坐席错误"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) hotlineTopicList(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	appID := strings.TrimSpace(c.Query("app_id"))
	if appID == "" {
		c.ResponseError(errors.New("app_id不能为空"))
		return
	}
	type topicRow struct {
		ID        int64  `db:"id" json:"id"`
		Title     string `db:"title" json:"title"`
		Welcome   string `db:"welcome" json:"welcome"`
		IsDefault int    `db:"is_default" json:"is_default"`
	}
	var list []*topicRow
	_, err = m.ctx.DB().Select("id,title,welcome,is_default").From("hotline_topic").
		Where("app_id=? AND is_deleted=0", appID).OrderDir("id", true).Load(&list)
	if err != nil {
		m.Error("查询客服话题失败", zap.Error(err))
		c.ResponseError(errors.New("查询客服话题失败"))
		return
	}
	if list == nil {
		list = []*topicRow{}
	}
	c.Response(list)
}

func (m *Manager) hotlineTopicCreate(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	var req struct {
		AppID   string `json:"app_id"`
		Title   string `json:"title"`
		Welcome string `json:"welcome"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	appID := strings.TrimSpace(req.AppID)
	if appID == "" {
		c.ResponseError(errors.New("app_id不能为空"))
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		c.ResponseError(errors.New("话题标题不能为空"))
		return
	}
	if strings.TrimSpace(req.Welcome) == "" {
		c.ResponseError(errors.New("欢迎语不能为空"))
		return
	}
	_, err = m.ctx.DB().InsertInto("hotline_topic").Columns("app_id", "title", "welcome", "is_default", "is_deleted").
		Values(appID, strings.TrimSpace(req.Title), strings.TrimSpace(req.Welcome), 0, 0).Exec()
	if err != nil {
		m.Error("添加客服话题失败", zap.Error(err))
		c.ResponseError(errors.New("添加客服话题失败"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) hotlineTopicUpdate(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	idStr := c.Param("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	var req struct {
		AppID   string `json:"app_id"`
		Title   string `json:"title"`
		Welcome string `json:"welcome"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	appID := strings.TrimSpace(req.AppID)
	if appID == "" {
		c.ResponseError(errors.New("app_id不能为空"))
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		c.ResponseError(errors.New("话题标题不能为空"))
		return
	}
	if strings.TrimSpace(req.Welcome) == "" {
		c.ResponseError(errors.New("欢迎语不能为空"))
		return
	}
	r, err := m.ctx.DB().Update("hotline_topic").
		Set("title", strings.TrimSpace(req.Title)).
		Set("welcome", strings.TrimSpace(req.Welcome)).
		Set("updated_at", dbr.Now).
		Where("id=? AND app_id=? AND is_deleted=0", id, appID).Exec()
	if err != nil {
		m.Error("更新客服话题失败", zap.Error(err))
		c.ResponseError(errors.New("更新客服话题失败"))
		return
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		c.ResponseError(errors.New("话题不存在或不属于该应用"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) hotlineTopicDelete(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	idStr := c.Param("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	appID := strings.TrimSpace(c.Query("app_id"))
	if appID == "" {
		c.ResponseError(errors.New("app_id不能为空"))
		return
	}
	type defPick struct {
		IsDefault int `db:"is_default"`
	}
	var dr defPick
	cnt, _ := m.ctx.DB().Select("is_default").From("hotline_topic").Where("id=? AND app_id=? AND is_deleted=0", id, appID).Load(&dr)
	if cnt == 0 {
		c.ResponseError(errors.New("话题不存在"))
		return
	}
	if dr.IsDefault == 1 {
		c.ResponseError(errors.New("默认话题不能删除"))
		return
	}
	r, err := m.ctx.DB().Update("hotline_topic").Set("is_deleted", 1).Set("updated_at", dbr.Now).
		Where("id=? AND app_id=?", id, appID).Exec()
	if err != nil {
		m.Error("删除客服话题失败", zap.Error(err))
		c.ResponseError(errors.New("删除客服话题失败"))
		return
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		c.ResponseError(errors.New("话题不存在"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) uploadImage(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	f, header, err := c.Request.FormFile("file")
	if err != nil {
		c.ResponseError(errors.New("读取文件失败"))
		return
	}
	defer f.Close()

	contentType := c.DefaultPostForm("contenttype", header.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "image/png"
	}

	ext := ""
	if idx := strings.LastIndex(header.Filename, "."); idx >= 0 {
		ext = header.Filename[idx:]
	}
	if ext == "" {
		ext = ".png"
	}
	safeName := util.GenerUUID() + strings.ToLower(ext)
	storagePath := fmt.Sprintf("/manager/images/%s/%s", util.GenerUUID(), safeName)

	fileSvc := filemod.NewService(m.ctx)
	_, err = fileSvc.UploadFile("common"+storagePath, contentType, func(w io.Writer) error {
		_, err := io.Copy(w, f)
		return err
	})
	if err != nil {
		m.Error("上传图片失败", zap.Error(err))
		c.ResponseError(errors.New("上传图片失败"))
		return
	}

	relativePath := fmt.Sprintf("file/preview/common%s", storagePath)
	absoluteURL := wkutil.FullAPIURL(m.ctx.GetConfig().External.APIBaseURL, relativePath)
	c.JSON(http.StatusOK, map[string]string{
		"path": relativePath,
		"url":  absoluteURL,
	})
}
