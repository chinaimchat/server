package hotline

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/smtp"
	"strings"
	"sync"
	"time"

	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/user"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/common"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"github.com/jordan-wright/email"
	"go.uber.org/zap"
)

// 获取我的客服详情
func (h *Hotline) agentMyGet(c *wkhttp.Context) {
	loginUID := c.GetLoginUID()

	agentM, err := h.agentDB.queryWithUIDAndAppID(loginUID, c.GetAppID())
	if err != nil {
		c.ResponseErrorf("查询客服数据失败！", err)
		return
	}
	if agentM == nil {
		c.ResponseError(errors.New("客服不存在！"))
		return
	}
	userResp, err := h.userService.GetUser(loginUID)
	if err != nil {
		c.ResponseErrorf("获取用户数据！", err)
		return
	}
	if userResp == nil {
		c.ResponseError(errors.New("用户数据不存在！"))
		return
	}
	c.Response(newAgentResp(userResp, agentM))
}

func (h *Hotline) agentMyUpdate(c *wkhttp.Context) {
	var req struct {
		Name     string `json:"name"`
		Position string `json:"position"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseErrorf("数据格式有误！", err)
		return
	}
	if req.Name == "" {
		c.ResponseError(errors.New("名字不能为空！"))
		return
	}
	err := h.agentDB.updatePosition(req.Position, c.GetLoginUID(), c.GetAppID())
	if err != nil {
		c.ResponseErrorf("修改职位失败！", err)
		return
	}
	err = h.userService.UpdateUser(user.UserUpdateReq{
		UID:  c.GetLoginUID(),
		Name: &req.Name,
	})
	if err != nil {
		c.ResponseErrorf("更新用户失败！", err)
		return
	}
	c.ResponseOK()
}

func (h *Hotline) agentPasswordUpdate(c *wkhttp.Context) {
	var req struct {
		Password    string `json:"password"`
		NewPassword string `json:"new_password"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseErrorf("数据结构有误！", err)
		return
	}
	if req.Password == "" {
		c.ResponseError(errors.New("原密码不能为空！"))
		return
	}
	if req.NewPassword == "" {
		c.ResponseError(errors.New("新密码不能为空！"))
		return
	}

	err := h.userService.UpdateLoginPassword(user.UpdateLoginPasswordReq{
		UID:         c.GetLoginUID(),
		Password:    req.Password,
		NewPassword: req.NewPassword,
	})
	if err != nil {
		c.ResponseError(err)
		return
	}
	c.ResponseOK()
}

// 会话重开
func (h *Hotline) reopen(c *wkhttp.Context) {
	var req struct {
		ChannelID   string `json:"channel_id"`
		ChannelType uint8  `json:"channel_type"`
	}
	if err := c.BindJSON(&req); err != nil {
		h.Error("数据格式有误！", zap.Error(err))
		c.ResponseError(errors.New("数据格式有误！"))
		return
	}
	if req.ChannelID == "" {
		c.ResponseError(errors.New("频道ID不能为空！"))
		return
	}
	err := h.channelDB.updateCategoryAndAgentUID("", "", req.ChannelID, req.ChannelType)
	if err != nil {
		c.ResponseError(errors.New("更新会话类别失败！"))
		return
	}

	subscribers, _, err := h.channelDB.queryBindSubscribers(req.ChannelID, req.ChannelType)
	if err != nil {
		c.ResponseErrorf("查询频道失败！", err)
		return
	}

	if len(subscribers) > 0 {
		subscriberUIDs := make([]string, 0, len(subscribers))
		for _, subscriber := range subscribers {
			subscriberUIDs = append(subscriberUIDs, subscriber.Subscriber)
		}
		err = h.ctx.SendMessage(&config.MsgSendReq{
			ChannelID:   req.ChannelID,
			ChannelType: req.ChannelType,
			Subscribers: subscriberUIDs,
			Header: config.MsgHeader{
				RedDot: 1,
			},
			Payload: []byte(util.ToJson(map[string]interface{}{
				"type":    common.HotlineReopen,
				"content": "{0}重新打开了会话",
				"extra": []config.UserBaseVo{
					{
						UID:  c.GetLoginUID(),
						Name: c.GetLoginName(),
					},
				},
			})),
		})
		if err != nil {
			c.ResponseErrorf("发送消息失败！", err)
			return
		}
	}

}

// 更新工作状态
func (h *Hotline) workStatusSet(c *wkhttp.Context) {
	status := c.Param("status")

	statusI := 0
	if status == "on" {
		statusI = 1
	}
	err := h.agentDB.updateIsWork(statusI, c.GetLoginUID(), c.GetAppID())
	if err != nil {
		c.ResponseErrorf("更新客服工作状态失败！", err)
		return
	}
	c.ResponseOK()
}

// 解决
func (h *Hotline) solved(c *wkhttp.Context) {
	var req struct {
		ChannelID   string `json:"channel_id"`
		ChannelType uint8  `json:"channel_type"`
	}
	if err := c.BindJSON(&req); err != nil {
		h.Error("数据格式有误！", zap.Error(err))
		c.ResponseError(errors.New("数据格式有误！"))
		return
	}
	if req.ChannelID == "" {
		c.ResponseError(errors.New("频道ID不能为空！"))
		return
	}
	err := h.channelDB.updateCategory(SessionCategorySolved.String(), req.ChannelID, req.ChannelType)
	if err != nil {
		c.ResponseError(errors.New("更新会话类别失败！"))
		return
	}

	subscribers, _, err := h.channelDB.queryBindSubscribers(req.ChannelID, req.ChannelType)
	if err != nil {
		c.ResponseErrorf("查询频道失败！", err)
		return
	}

	if len(subscribers) > 0 {
		subscriberUIDs := make([]string, 0, len(subscribers))
		for _, subscriber := range subscribers {
			subscriberUIDs = append(subscriberUIDs, subscriber.Subscriber)
		}
		err = h.ctx.SendMessage(&config.MsgSendReq{
			ChannelID:   req.ChannelID,
			ChannelType: req.ChannelType,
			Subscribers: subscriberUIDs,
			Header: config.MsgHeader{
				RedDot: 1,
			},
			Payload: []byte(util.ToJson(map[string]interface{}{
				"type":    common.HotlineSolved,
				"content": "{0}已解决会话",
				"extra": []config.UserBaseVo{
					{
						UID:  c.GetLoginUID(),
						Name: c.GetLoginName(),
					},
				},
			})),
		})
		if err != nil {
			c.ResponseErrorf("发送消息失败！", err)
			return
		}
	}

	c.ResponseOK()
}

// 客服分页查询
func (h *Hotline) agentPage(c *wkhttp.Context) {
	role := c.Query("role")        // 角色代号
	skillNo := c.Query("skill_no") // 技能组编号
	groupNo := c.Query("group_no") // 组编号
	statusStr := c.Query("status") // 客服状态 0.禁用 1.启用 空为查询所有
	name := c.Query("name")
	working := c.Query("working") // 是否工作中 0.否 1.是
	pIndex, pSize := c.GetPage()

	agentDetailModels, err := h.agentDB.queryWith(role, name, skillNo, groupNo, statusStr, working, pIndex, pSize, c.GetAppID())
	if err != nil {
		c.ResponseErrorf("查询客服数据失败！", err)
		return
	}
	total, err := h.agentDB.queryTotalWith(role, name, skillNo, groupNo, statusStr, working, c.GetAppID())
	if err != nil {
		c.ResponseErrorf("查询客服数量失败！", err)
		return
	}
	resps := make([]*agentDetailResp, 0, len(agentDetailModels))
	if len(agentDetailModels) > 0 {
		for _, agentDetailM := range agentDetailModels {
			resps = append(resps, newAgentDetailResp(agentDetailM))
		}
	}
	c.Response(common.NewPageResult(pIndex, pSize, total, resps))
}

// 邀请客服
var emailTmpOnce sync.Once
var emaillTpl *template.Template

func (h *Hotline) agentInvite(c *wkhttp.Context) {
	var req struct {
		Emails   []string `json:"emails"`   // 邮箱
		Role     string   `json:"role"`     // 角色
		SkillNo  string   `json:"skill_no"` // 技能组
		GroupNos []string `json:"group_nos"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseErrorf("数据格式有误！", err)
		return
	}
	if len(req.Emails) <= 0 {
		c.ResponseError(errors.New("邮箱不能为空！"))
		return
	}
	if len(req.Role) <= 0 {
		c.ResponseError(errors.New("角色不能为空！"))
		return
	}
	if !h.containRoles(req.Role) {
		c.ResponseError(errors.New("无效的角色代号！"))
		return
	}
	if req.Role == RoleAdmin.String() {
		c.ResponseError(errors.New("不能添加超级管理员！"))
		return
	}
	if !h.hasRole(c, RoleAdmin, RoleManager, RoleUsermanager) {
		c.ResponseError(errors.New("没有添加客服的权限！"))
		return
	}

	emailTmpOnce.Do(func() {
		emaillTpl = template.Must(template.ParseFiles("assets/webroot/hotline/invite_tpl.html"))
	})

	appConfig, err := h.configDB.queryWithAppID(c.GetAppID())
	if err != nil {
		c.ResponseErrorf("查询app配置失败！", err)
		return
	}

	for _, email := range req.Emails {
		activeCode := util.GenerUUID()
		err := h.sendInviteEmail(activeCode, email, appConfig.AppName, activeCacheData{
			AppID:    c.GetAppID(),
			Role:     req.Role,
			SkillNo:  req.SkillNo,
			Email:    email,
			GroupNos: req.GroupNos,
		})
		if err != nil {
			c.ResponseErrorf("发送邮件失败！", err)
			return
		}
	}
	c.ResponseOK()

}

func (h *Hotline) sendInviteEmail(activeCode, emailAddr, orgName string, data activeCacheData) error {
	buff := bytes.NewBuffer(make([]byte, 0))
	err := emaillTpl.ExecuteTemplate(buff, "hotline/invite_tpl.html", map[string]interface{}{
		"app_name":      h.ctx.GetConfig().AppName,
		"active_url":    fmt.Sprintf("%s/v1/hotline/agent/preactive?code=%s", h.ctx.GetConfig().External.BaseURL, activeCode),
		"org_name":      orgName,
		"support_email": h.ctx.GetConfig().Support.Email,
	})
	if err != nil {
		h.Error("获取激活模版失败！", zap.Error(err))
		return err
	}
	err = h.ctx.GetRedisConn().SetAndExpire(fmt.Sprintf("agentActive:%s", activeCode), util.ToJson(data), time.Hour*24*2)
	if err != nil {
		h.Error("设置激活码的缓存失败！", zap.Error(err))
		return err
	}
	e := email.NewEmail()
	e.From = h.ctx.GetConfig().Support.Email
	e.To = []string{emailAddr}
	e.Subject = "激活您的账号"
	e.HTML = buff.Bytes()

	smtpAddrs := strings.Split(h.ctx.GetConfig().Support.EmailSmtp, ":")

	err = e.Send(h.ctx.GetConfig().Support.EmailSmtp, smtp.PlainAuth("", h.ctx.GetConfig().Support.Email, h.ctx.GetConfig().Support.EmailPwd, smtpAddrs[0]))
	if err != nil {
		h.Error("发送邮件失败！", zap.Error(err))
		return err
	}
	return nil
}

// 准备激活
func (h *Hotline) agentPreActive(c *wkhttp.Context) {
	activeCode := c.Query("code") // 激活码

	activeStr, err := h.ctx.GetRedisConn().GetString(fmt.Sprintf("agentActive:%s", activeCode))
	if err != nil {
		c.ResponseError(errors.New("获取激活码绑定的数据失败！"))
		return
	}
	if activeStr == "" {
		c.ResponseError(errors.New("激活码已过期！"))
		return
	}
	var activeCacheResp activeCacheData
	if err := util.ReadJsonByByte([]byte(activeStr), &activeCacheResp); err != nil {
		c.ResponseErrorf("解码激活数据失败！", err)
		return
	}

	userResp, err := h.userService.GetUserWithUsername(activeCacheResp.Email)
	if err != nil {
		c.ResponseErrorf("查询用户信息失败！", err)
		return
	}

	if userResp != nil {
		agentM, err := h.agentDB.queryWithUID(userResp.UID)
		if err != nil {
			c.ResponseErrorf("查询客服失败！", err)
			return
		}
		if agentM == nil {
			tx, _ := h.ctx.DB().Begin()
			defer func() {
				if err := recover(); err != nil {
					tx.RollbackUnlessCommitted()
					panic(err)
				}
			}()
			err = h.agentDB.insertTx(&agentModel{
				AppID:  activeCacheResp.AppID,
				UID:    userResp.UID,
				Name:   userResp.Name,
				Role:   activeCacheResp.Role,
				IsWork: 1,
				Status: 1,
			}, tx)
			if err != nil {
				tx.Rollback()
				c.ResponseErrorf("添加客服失败！", err)
				return
			}
			if activeCacheResp.SkillNo != "" {
				err = h.channelDB.insertSubscriberTx(&subscribersModel{
					AppID:          activeCacheResp.AppID,
					ChannelID:      activeCacheResp.SkillNo,
					ChannelType:    common.ChannelTypeGroup.Uint8(),
					SubscriberType: SubscriberTypeAgent.Int(),
					Subscriber:     userResp.UID,
				}, tx)
				if err != nil {
					tx.Rollback()
					c.ResponseErrorf("添加技能组成员失败！", err)
					return
				}
			}
			if err := tx.Commit(); err != nil {
				tx.Rollback()
				c.ResponseErrorf("提交事务失败！", err)
				return
			}
			c.Redirect(http.StatusFound, h.ctx.GetConfig().External.WebLoginURL)
			return
		} else {
			c.Redirect(http.StatusFound, h.ctx.GetConfig().External.WebLoginURL)
			return
		}
	}

	c.HTML(http.StatusOK, "hotline/invite_active_tpl.html", map[string]interface{}{
		"active_url":    fmt.Sprintf("%s/v1/hotline/agent/active", h.ctx.GetConfig().External.BaseURL),
		"active_code":   activeCode,
		"web_login_url": h.ctx.GetConfig().External.WebLoginURL,
		"base_url":      h.ctx.GetConfig().External.BaseURL + "/hotline/",
	})
}

// 激活
func (h *Hotline) agentActive(c *wkhttp.Context) {
	var req struct {
		Name       string `json:"name"`
		Password   string `json:"password"`
		ActiveCode string `json:"active_code"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseErrorf("数据格式有误！", err)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		c.ResponseError(errors.New("姓名不能为空！"))
		return
	}
	if strings.TrimSpace(req.Password) == "" {
		c.ResponseError(errors.New("密码不能为空！"))
		return
	}
	if strings.TrimSpace(req.ActiveCode) == "" {
		c.ResponseError(errors.New("激活码不能为空！"))
		return
	}
	activeStr, err := h.ctx.GetRedisConn().GetString(fmt.Sprintf("agentActive:%s", req.ActiveCode))
	if err != nil {
		c.ResponseError(errors.New("获取激活码绑定的数据失败！"))
		return
	}
	if activeStr == "" {
		c.ResponseError(errors.New("激活码已过期！"))
		return
	}
	var activeCacheResp activeCacheData
	if err := util.ReadJsonByByte([]byte(activeStr), &activeCacheResp); err != nil {
		c.ResponseErrorf("解码激活数据失败！", err)
		return
	}

	userResp, err := h.userService.GetUserWithUsername(activeCacheResp.Email)
	if err != nil {
		c.ResponseErrorf("查询用户失败！", err)
		return
	}
	uid := util.GenerUUID()
	if userResp == nil {
		err = h.userService.AddUser(&user.AddUserReq{
			Name:     req.Name,
			UID:      uid,
			Email:    activeCacheResp.Email,
			Password: req.Password,
		})
		if err != nil {
			c.ResponseErrorf("添加用户失败！", err)
			return
		}
	} else {
		uid = userResp.UID
	}
	agentM, err := h.agentDB.queryWithUID(uid)
	if err != nil {
		c.ResponseErrorf("查询客服失败！", err)
		return
	}
	if agentM == nil {
		tx, _ := h.ctx.DB().Begin()
		defer func() {
			if err := recover(); err != nil {
				tx.RollbackUnlessCommitted()
				panic(err)
			}
		}()
		err = h.agentDB.insertTx(&agentModel{
			AppID:  activeCacheResp.AppID,
			UID:    uid,
			Name:   req.Name,
			Role:   activeCacheResp.Role,
			IsWork: 1,
			Status: 1,
		}, tx)
		if err != nil {
			tx.Rollback()
			c.ResponseErrorf("添加客服失败！", err)
			return
		}
		if activeCacheResp.SkillNo != "" {
			err = h.channelDB.insertSubscriberTx(&subscribersModel{
				AppID:          activeCacheResp.AppID,
				ChannelID:      activeCacheResp.SkillNo,
				ChannelType:    common.ChannelTypeGroup.Uint8(),
				SubscriberType: SubscriberTypeAgent.Int(),
				Subscriber:     uid,
			}, tx)
			if err != nil {
				tx.Rollback()
				c.ResponseErrorf("添加技能组成员失败！", err)
				return
			}
		}
		if err := tx.Commit(); err != nil {
			tx.Rollback()
			c.ResponseErrorf("提交事务失败！", err)
			return
		}
	}
	err = h.ctx.GetRedisConn().Del(fmt.Sprintf("agentActive:%s", req.ActiveCode))
	if err != nil {
		c.ResponseErrorf("删除缓存的激活码失败！", err)
		return
	}
	c.ResponseOK()

}

// 添加客服
func (h *Hotline) agentAdd(c *wkhttp.Context) {
	var req struct {
		UID      string   `json:"uid"`      // 用户名
		Name     string   `json:"name"`     // 客服名称
		Role     string   `json:"role"`     // 角色
		SkillNo  string   `json:"skill_no"` // 技能组
		GroupNos []string `json:"group_nos"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseErrorf("数据格式有误！", err)
		return
	}
	if len(req.UID) <= 0 {
		c.ResponseError(errors.New("用户uid不能为空！"))
		return
	}
	if len(req.Name) <= 0 {
		c.ResponseError(errors.New("客服名称不能为空！"))
		return
	}
	if len(req.Role) <= 0 {
		c.ResponseError(errors.New("角色不能为空！"))
		return
	}
	if !h.containRoles(req.Role) {
		c.ResponseError(errors.New("无效的角色代号！"))
		return
	}
	if req.Role == RoleAdmin.String() {
		c.ResponseError(errors.New("不能添加超级管理员！"))
		return
	}
	if !h.hasRole(c, RoleAdmin, RoleManager, RoleUsermanager) {
		c.ResponseError(errors.New("没有添加客服的权限！"))
		return
	}
	if req.UID == c.GetLoginUID() {
		c.ResponseError(errors.New("不能添加自己！"))
		return
	}
	agentM, err := h.agentDB.queryWithUIDAndAppID(req.UID, c.GetAppID())
	if err != nil {
		h.Error("查询客服信息失败！", zap.Error(err))
		c.ResponseError(errors.New("查询客服信息失败！"))
		return
	}
	if agentM != nil {
		c.ResponseError(errors.New("客服已经存在！"))
		return
	}

	err = h.ctx.IMAddSubscriber(&config.SubscriberAddReq{
		ChannelID:   c.GetAppID(),
		ChannelType: common.ChannelTypeGroup.Uint8(),
		Subscribers: []string{req.UID},
	})
	if err != nil {
		h.Error("添加订阅者失败！", zap.Error(err))
		c.ResponseError(errors.New("添加订阅者失败！"))
		return
	}

	tx, _ := h.ctx.DB().Begin()
	defer func() {
		if err := recover(); err != nil {
			tx.RollbackUnlessCommitted()
			panic(err)
		}
	}()

	err = h.agentDB.insertTx(&agentModel{
		AppID:  c.GetAppID(),
		UID:    req.UID,
		Name:   req.Name,
		Role:   req.Role,
		IsWork: 1,
		Status: 1,
	}, tx)
	if err != nil {
		tx.Rollback()
		c.ResponseErrorf("客服添加失败！", err)
		return
	}

	err = h.channelDB.insertSubscriberTx(&subscribersModel{
		AppID:          c.GetAppID(),
		ChannelID:      c.GetAppID(),
		ChannelType:    common.ChannelTypeGroup.Uint8(),
		SubscriberType: SubscriberTypeAgent.Int(),
		Subscriber:     req.UID,
	}, tx)
	if err != nil {
		tx.Rollback()
		h.Error("添加频道成员失败！", zap.Error(err))
		c.ResponseError(errors.New("添加频道成员失败！"))
		return
	}

	if len(req.GroupNos) > 0 {
		for _, groupNo := range req.GroupNos {
			err = h.groupDB.insertMemberTx(&memberModel{
				AppID:   c.GetAppID(),
				GroupNo: groupNo,
				UID:     req.UID,
			}, tx)
			if err != nil {
				tx.Rollback()
				c.ResponseErrorf("添加成员失败！", err)
				return
			}
		}
	}
	if len(req.SkillNo) > 0 {
		err = h.groupDB.insertMemberTx(&memberModel{
			AppID:   c.GetAppID(),
			GroupNo: req.SkillNo,
			UID:     req.UID,
		}, tx)
		if err != nil {
			tx.Rollback()
			c.ResponseErrorf("添加成员失败！", err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		tx.Rollback()
		c.ResponseErrorf("提交事务失败！", err)
		return
	}

	c.ResponseOK()
}

func (h *Hotline) containRoles(r string) bool {
	switch r {
	case RoleAdmin.String():
		return true
	case RoleAgent.String():
		return true
	case RoleManager.String():
		return true
	case RoleUsermanager.String():
		return true
	}
	return false
}

type agentDetailResp struct {
	UID        string   `json:"uid"`         // 客服uid
	Name       string   `json:"name"`        // 客服名称
	LastActive int      `json:"last_active"` // 最后一次活动时间 10位时间戳（单位秒）
	Role       string   `json:"role"`
	RoleName   string   `json:"role_name"`
	IsWork     int      `json:"is_work"`     // 是否是工作状态
	SkillNo    string   `json:"skill_no"`    // 技能组编号
	SkillName  string   `json:"skill_name"`  // 技能组名称
	GroupNos   []string `json:"group_nos"`   // 群组编号集合
	GroupNames []string `json:"group_names"` // 群组名称集合
}

func newAgentDetailResp(m *agentDetailModel) *agentDetailResp {
	groupNos := make([]string, 0)
	groupNames := make([]string, 0)
	if len(m.GroupNos) > 0 {
		groupNos = strings.Split(m.GroupNos, ",")
	}
	if len(m.GroupNames) > 0 {
		groupNames = strings.Split(m.GroupNames, ",")
	}
	return &agentDetailResp{
		UID:        m.UID,
		Name:       m.Name,
		LastActive: m.LastActive,
		Role:       m.Role,
		RoleName:   RoleConst(m.Role).RoleName(),
		IsWork:     m.IsWork,
		SkillNo:    m.SkillNo,
		SkillName:  m.SkillName,
		GroupNos:   groupNos,
		GroupNames: groupNames,
	}
}

type activeCacheData struct {
	AppID    string   `json:"app_id"`
	Email    string   `json:"email"`
	Role     string   `json:"role"`
	SkillNo  string   `json:"skill_no"`
	GroupNos []string `json:"group_nos"`
}

type agentResp struct {
	UID            string `json:"uid"`
	Name           string `json:"name"`
	Zone           string `json:"zone,omitempty"`
	Phone          string `json:"phone,omitempty"`
	Email          string `json:"email,omitempty"`
	Position       string `json:"position"` // 职位
	IsUploadAvatar int    `json:"is_upload_avatar"`
}

func newAgentResp(userResp *user.Resp, m *agentModel) *agentResp {
	return &agentResp{
		UID:            m.UID,
		Name:           m.Name,
		Zone:           userResp.Zone,
		Phone:          userResp.Phone,
		Email:          userResp.Email,
		Position:       m.Position,
		IsUploadAvatar: userResp.IsUploadAvatar,
	}
}
