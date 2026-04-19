package user

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gocraft/dbr/v2"

	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/base/event"
	common2 "github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/common"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/common"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/log"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkevent"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

// Manager 用户管理
type Manager struct {
	ctx *config.Context
	log.Log
	db            *managerDB
	userDB        *DB
	userSettingDB *SettingDB
	deviceDB      *deviceDB
	friendDB      *friendDB
	loginLogDB    *LoginLogDB
	privilegeDB   *privilegeDB
	onlineService IOnlineService
	commonService common2.IService
}

// NewManager NewManager
func NewManager(ctx *config.Context) *Manager {
	m := &Manager{
		ctx:           ctx,
		Log:           log.NewTLog("userManager"),
		db:            newManagerDB(ctx),
		deviceDB:      newDeviceDB(ctx),
		friendDB:      newFriendDB(ctx),
		loginLogDB:    NewLoginLogDB(ctx.DB()),
		privilegeDB:   newPrivilegeDB(ctx.DB()),
		userDB:        NewDB(ctx),
		userSettingDB: NewSettingDB(ctx.DB()),
		onlineService: NewOnlineService(ctx),
		commonService: common2.NewService(ctx),
	}
	m.createManagerAccount()
	return m
}

// Route 配置路由规则
func (m *Manager) Route(r *wkhttp.WKHttp) {
	user := r.Group("/v1/manager")
	{
		user.POST("/login", m.login) // 账号登录
	}
	auth := r.Group("/v1/manager", m.ctx.AuthMiddleware(r))
	{
		auth.POST("/user/admin", m.addAdminUser)                 // 添加一个管理员
		auth.GET("/user/admin", m.getAdminUsers)                 // 查询管理员用户
		auth.DELETE("/user/admin", m.deleteAdminUsers)           // 删除管理员用户
		auth.POST("/user/add", m.addUser)                        // 添加一个用户
		auth.POST("/user/resetpassword", m.resetUserPassword)    // 重置用户密码
		auth.POST("/user/impersonate", m.impersonateLoginAsUser) // 以此用户视角：签发 Web 端登录态（仅超级管理员）
		auth.GET("/user/list", m.list)                           // 用户列表
		auth.GET("/user/friends", m.friends)                     // 某个用户的好友
		auth.GET("/user/blacklist", m.blacklist)                 // 用户黑名单列表
		auth.GET("/user/disablelist", m.disableUsers)            // 封禁用户列表
		auth.GET("user/online", m.online)                        // 在线设备信息
		auth.PUT("/user/liftban/:uid/:status", m.liftBanUser)    // 解禁或封禁用户
		auth.POST("/user/updatepassword", m.updatePwd)           // 修改用户密码
		auth.GET("/user/devices", m.devices)                     // 查看某用户设备列表
		auth.GET("/user/privilege/list", m.privilegeList)
		auth.GET("/user/privilege/search-user", m.privilegeSearchUser)
		auth.POST("/user/privilege", m.privilegeCreate)
		auth.DELETE("/user/privilege", m.privilegeDelete)
		auth.PUT("/user/privilege/switch", m.privilegeSwitchUpdate)
		auth.GET("/user/privilege/global", m.privilegeGlobalGet)
		auth.PUT("/user/privilege/global", m.privilegeGlobalUpdate)
		auth.GET("/user/login-ip-history", m.loginIPHistory)
	}
}

func (m *Manager) devices(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	uid := c.Query("uid")
	if uid == "" {
		c.ResponseError(errors.New("请求用户uid不能为空"))
		return
	}
	devices, err := m.deviceDB.queryDeviceWithUID(uid)
	if err != nil {
		m.Error("查询用户设备列表错误", zap.Error(err))
		c.ResponseError(errors.New("查询用户设备列表错误"))
		return
	}
	list := make([]*managerDeviceResp, 0)
	if len(devices) == 0 {
		c.Response(list)
		return
	}
	for _, device := range devices {
		list = append(list, &managerDeviceResp{
			ID:          device.Id,
			DeviceID:    device.DeviceID,
			DeviceName:  device.DeviceName,
			DeviceModel: device.DeviceModel,
			LastLogin:   util.ToyyyyMMddHHmm(time.Unix(device.LastLogin, 0)),
		})
	}
	c.Response(list)
}

func (m *Manager) online(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	uid := c.Query("uid")
	if uid == "" {
		c.ResponseError(errors.New("请求用户uid不能为空"))
		return
	}
	list, err := m.db.queryUserOnline(uid)
	if err != nil {
		m.Error("查询用户在线设备信息错误", zap.Error(err))
		c.ResponseError(errors.New("查询用户在线设备信息错误"))
		return
	}
	result := make([]*userOnlineResp, 0)
	if len(list) > 0 {
		for _, user := range list {
			result = append(result, &userOnlineResp{
				Online:      user.Online,
				DeviceFlag:  user.DeviceFlag,
				LastOnline:  user.LastOffline,
				LastOffline: user.LastOffline,
				UID:         user.UID,
			})
		}
	}
	c.Response(result)
}

// 用户登录
func (m *Manager) login(c *wkhttp.Context) {
	var req managerLoginReq
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	if err := req.Check(); err != nil {
		c.ResponseError(err)
		return
	}
	userInfo, err := m.db.queryUserInfoWithNameAndPwd(req.Username)
	if err != nil {
		m.Error("登录错误", zap.Error(err))
		c.ResponseError(errors.New("登录错误！"))
		return
	}
	if userInfo == nil || userInfo.UID == "" {
		c.ResponseError(errors.New("登录用户不存在"))
		return
	}
	if userInfo.Password != util.MD5(util.MD5(req.Password)) {
		c.ResponseError(errors.New("用户名或密码错误"))
		return
	}
	if userInfo.Role != string(wkhttp.Admin) && userInfo.Role != string(wkhttp.SuperAdmin) {
		c.ResponseError(errors.New("登录账号未开通管理权限"))
		return
	}
	token := util.GenerUUID()
	// 将token设置到缓存
	err = m.ctx.Cache().SetAndExpire(m.ctx.GetConfig().Cache.TokenCachePrefix+token, fmt.Sprintf("%s@%s@%s", userInfo.UID, userInfo.Name, userInfo.Role), m.ctx.GetConfig().Cache.TokenExpire)
	if err != nil {
		m.Error("设置token缓存失败！", zap.Error(err))
		c.ResponseError(errors.New("设置token缓存失败！"))
		return
	}

	err = m.ctx.Cache().SetAndExpire(fmt.Sprintf("%s%d%s", m.ctx.GetConfig().Cache.UIDTokenCachePrefix, config.Web, userInfo.UID), token, m.ctx.GetConfig().Cache.TokenExpire)
	if err != nil {
		m.Error("设置uidtoken缓存失败！", zap.Error(err))
		c.ResponseError(errors.New("设置token缓存失败！"))
		return
	}

	c.Response(&managerLoginResp{
		UID:   userInfo.UID,
		Token: token,
		Name:  userInfo.Name,
		Role:  userInfo.Role,
	})
}

// 重置用户密码
func (m *Manager) resetUserPassword(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}

	type reqRUP struct {
		NewPassword              string `json:"new_password"`
		NewPassswordConfirmation string `json:"new_password_confirmation"`
		Uid                      string `json:"uid"`
	}
	var req reqRUP
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	if len(req.NewPassword) < 6 {
		c.ResponseError(errors.New("密码长度必须大于6位"))
		return
	}
	if req.NewPassword != req.NewPassswordConfirmation {
		c.ResponseError(errors.New("两次密码不一致！"))
		return
	}
	if req.Uid == "" {
		c.ResponseError(errors.New("用户uid不能为空！"))
		return
	}
	user, err := m.userDB.QueryByUID(req.Uid)
	if err != nil {
		m.Error("查询用户信息错误", zap.Error(err))
		c.ResponseError(errors.New("查询用户信息错误"))
		return
	}
	if user == nil {
		c.ResponseError(errors.New("操作用户不存在"))
		return
	}

	err = m.userDB.UpdateUsersWithField("password", util.MD5(util.MD5(req.NewPassword)), req.Uid)
	if err != nil {
		m.Error("重置用户密码错误", zap.Error(err))
		c.Response("重置用户密码错误")
		return
	}
	c.ResponseOK()
}

// issueWebLoginToken 为指定用户签发与 Web 登录一致的 token（与 User.execLogin 中 Web 分支逻辑对齐）
func issueWebLoginToken(ctx *config.Context, userInfo *Model) (*loginUserDetailResp, error) {
	if userInfo.Status == int(common.UserDisable) {
		return nil, errors.New("该用户已被禁用")
	}
	flag := config.Web
	deviceLevel := config.DeviceLevelSlave

	token := util.GenerUUID()
	oldToken, err := ctx.Cache().Get(fmt.Sprintf("%s%d%s", ctx.GetConfig().Cache.UIDTokenCachePrefix, flag, userInfo.UID))
	if err != nil {
		return nil, errors.New("获取旧token错误")
	}
	if flag != config.APP {
		if strings.TrimSpace(oldToken) != "" {
			token = oldToken
		}
	}

	err = ctx.Cache().SetAndExpire(ctx.GetConfig().Cache.TokenCachePrefix+token, fmt.Sprintf("%s@%s@%s", userInfo.UID, userInfo.Name, userInfo.Role), ctx.GetConfig().Cache.TokenExpire)
	if err != nil {
		return nil, errors.New("设置token缓存失败！")
	}
	err = ctx.Cache().SetAndExpire(fmt.Sprintf("%s%d%s", ctx.GetConfig().Cache.UIDTokenCachePrefix, flag, userInfo.UID), token, ctx.GetConfig().Cache.TokenExpire)
	if err != nil {
		return nil, errors.New("设置uidtoken缓存失败！")
	}

	imResp, err := ctx.UpdateIMToken(config.UpdateIMTokenReq{
		UID:         userInfo.UID,
		Token:       token,
		DeviceFlag:  config.DeviceFlag(flag),
		DeviceLevel: deviceLevel,
	})
	if err != nil {
		return nil, errors.New("更新IM的token失败！")
	}
	if imResp.Status == config.UpdateTokenStatusBan {
		return nil, errors.New("此账号已经被封禁！")
	}
	// 与 Web 登录一致：不保留原生 App 离线推送注册
	_ = ctx.GetRedisConn().Del(fmt.Sprintf("%s%s", common.UserDeviceTokenPrefix, userInfo.UID))
	return newLoginUserDetailResp(userInfo, token, ctx), nil
}

// impersonateLoginAsUser 超级管理员获取目标用户的 Web 登录信息（用于管理后台「以此用户视角查看」）
func (m *Manager) impersonateLoginAsUser(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	var req struct {
		UID string `json:"uid"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	if strings.TrimSpace(req.UID) == "" {
		c.ResponseError(errors.New("用户uid不能为空！"))
		return
	}
	user, err := m.userDB.QueryByUID(req.UID)
	if err != nil {
		m.Error("查询用户信息错误", zap.Error(err))
		c.ResponseError(errors.New("查询用户信息错误"))
		return
	}
	if user == nil {
		c.ResponseError(errors.New("操作用户不存在"))
		return
	}
	if user.IsDestroy == 1 {
		c.ResponseError(errors.New("该用户已注销"))
		return
	}
	resp, err := issueWebLoginToken(m.ctx, user)
	if err != nil {
		c.ResponseError(err)
		return
	}
	c.Response(resp)
}

// 删除管理员用户
func (m *Manager) deleteAdminUsers(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	uid := c.Query("uid")
	if uid == "" {
		c.ResponseError(errors.New("删除用户uid不能为空"))
		return
	}
	user, err := m.userDB.QueryByUID(uid)
	if err != nil {
		m.Error("查询管理员用户错误", zap.Error(err))
		c.ResponseError(errors.New("查询管理员用户错误"))
		return
	}
	if user == nil || len(user.UID) == 0 {
		c.ResponseError(errors.New("该用户不存在"))
		return
	}
	if user.Role == "" {
		c.ResponseError(errors.New("该用户不是管理员账号不能删除"))
		return
	}
	if user.Role == string(wkhttp.SuperAdmin) {
		c.ResponseError(errors.New("超级管理员账号不能删除"))
		return
	}
	err = m.db.deleteUserWithUIDAndRole(uid, string(wkhttp.Admin))
	if err != nil {
		m.Error("删除管理员错误", zap.Error(err))
		c.ResponseError(errors.New("删除管理员错误"))
		return
	}
	oldToken, err := m.ctx.Cache().Get(fmt.Sprintf("%s%d%s", m.ctx.GetConfig().Cache.UIDTokenCachePrefix, config.Web, user.UID))
	if err != nil {
		m.Error("获取旧token错误", zap.Error(err))
		c.ResponseError(errors.New("获取旧token错误"))
		return
	}
	if oldToken != "" {
		err = m.ctx.Cache().Delete(m.ctx.GetConfig().Cache.TokenCachePrefix + oldToken)
		if err != nil {
			m.Error("清除旧token数据错误", zap.Error(err))
			c.ResponseError(errors.New("清除旧token数据错误"))
			return
		}
	}
	c.ResponseOK()
}

// 查询管理员列表
func (m *Manager) getAdminUsers(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	users, err := m.db.queryUsersWithRole(string(wkhttp.Admin))
	if err != nil {
		m.Error("查询管理员用户错误", zap.Error(err))
		c.ResponseError(errors.New("查询管理员用户错误"))
		return
	}
	list := make([]*adminUserResp, 0)
	if len(users) > 0 {
		for _, user := range users {
			list = append(list, &adminUserResp{
				UID:          user.UID,
				Name:         user.Name,
				Username:     user.Username,
				RegisterTime: user.CreatedAt.String(),
			})
		}
	}
	c.Response(list)
}

// 添加一个管理员
func (m *Manager) addAdminUser(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type reqVO struct {
		LoginName string `json:"login_name"`
		Name      string `json:"name"`
		Password  string `json:"password"`
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	if req.LoginName == "" {
		c.ResponseError(errors.New("登录用户名不能为空"))
		return
	}
	if req.Name == "" {
		c.ResponseError(errors.New("用户名不能为空"))
		return
	}
	if req.Password == "" {
		c.ResponseError(errors.New("密码不能为空"))
		return
	}
	user, err := m.db.queryUserWithNameAndRole(req.LoginName, string(wkhttp.Admin))
	if err != nil {
		m.Error("查询用户是否存在错误", zap.String("username", req.LoginName))
		c.ResponseError(errors.New("查询用户是否存在错误"))
		return
	}
	if user != nil && len(user.UID) > 0 {
		c.ResponseError(errors.New("该用户名已存在"))
		return
	}
	tx, err := m.userDB.session.Begin()
	if err != nil {
		m.Error("开启事务错误", zap.Error(err))
		c.ResponseError(errors.New("开启事务错误"))
		return
	}
	defer func() {
		if err := recover(); err != nil {
			tx.Rollback()
			panic(err)
		}
	}()
	uid, err := m.userDB.AllocateStagedNumericUID(tx)
	if err != nil {
		tx.Rollback()
		m.Error("分配管理员UID失败", zap.Error(err))
		c.ResponseError(err)
		return
	}
	userModel := &Model{}
	userModel.UID = uid
	userModel.Name = req.Name
	userModel.Vercode = fmt.Sprintf("%s@%d", util.GenerUUID(), common.User)
	userModel.QRVercode = fmt.Sprintf("%s@%d", util.GenerUUID(), common.QRCode)
	userModel.Phone = ""
	userModel.Username = req.LoginName
	userModel.Zone = ""
	userModel.Role = string(wkhttp.Admin)
	userModel.Password = util.MD5(util.MD5(req.Password))
	userModel.ShortNo = uid
	userModel.IsUploadAvatar = 0
	userModel.NewMsgNotice = 0
	userModel.MsgShowDetail = 0
	userModel.SearchByPhone = 0
	userModel.SearchByShort = 0
	userModel.VoiceOn = 0
	userModel.ShockOn = 0
	userModel.Sex = 1
	userModel.Status = int(common.UserAvailable)
	err = m.userDB.insertTx(userModel, tx)
	if err != nil {
		tx.Rollback()
		m.Error("添加管理员错误", zap.String("username", req.Name))
		c.ResponseError(err)
		return
	}
	err = tx.Commit()
	if err != nil {
		tx.Rollback()
		m.Error("数据库事务提交失败", zap.Error(err))
		c.ResponseError(errors.New("数据库事务提交失败"))
		return
	}
	c.ResponseOK()
}

// 添加一个用户
func (m *Manager) addUser(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	var req managerAddUserReq
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Password = strings.TrimSpace(req.Password)
	req.Phone = strings.TrimSpace(req.Phone)
	req.Zone = strings.TrimSpace(req.Zone)
	if req.Zone == "" {
		req.Zone = "0086"
	}
	if err := req.checkAddUserReq(); err != nil {
		c.ResponseError(err)
		return
	}
	userInfo, err := m.userDB.QueryByUsername(fmt.Sprintf("%s%s", req.Zone, req.Phone))
	if err != nil {
		m.Error("查询用户信息失败！", zap.String("username", req.Phone))
		c.ResponseError(err)
		return
	}
	if userInfo != nil {
		c.ResponseError(errors.New("该用户已存在"))
		return
	}
	var shortNumStatus = 0
	if m.ctx.GetConfig().ShortNo.EditOff {
		shortNumStatus = 1
	}
	tx, err := m.db.session.Begin()
	if err != nil {
		m.Error("开启事物错误", zap.Error(err))
		c.ResponseError(errors.New("开启事物错误"))
		return
	}
	defer func() {
		if err := recover(); err != nil {
			tx.Rollback()
			panic(err)
		}
	}()
	uid, err := m.userDB.AllocateStagedNumericUID(tx)
	if err != nil {
		tx.Rollback()
		m.Error("分配UID失败", zap.Error(err))
		c.ResponseError(err)
		return
	}
	userModel := &Model{}
	userModel.UID = uid
	userModel.Name = req.Name
	userModel.Vercode = fmt.Sprintf("%s@%d", util.GenerUUID(), common.User)
	userModel.QRVercode = fmt.Sprintf("%s@%d", util.GenerUUID(), common.QRCode)
	userModel.Phone = req.Phone
	userModel.Username = fmt.Sprintf("%s%s", req.Zone, req.Phone)
	userModel.Zone = req.Zone
	userModel.Password = util.MD5(util.MD5(req.Password))
	userModel.ShortNo = uid
	userModel.IsUploadAvatar = 0
	userModel.NewMsgNotice = 1
	userModel.MsgShowDetail = 1
	userModel.SearchByPhone = 1
	userModel.ShortStatus = shortNumStatus
	userModel.SearchByShort = 1
	userModel.VoiceOn = 1
	userModel.ShockOn = 1
	userModel.Sex = req.Sex
	userModel.Status = int(common.UserAvailable)
	err = m.userDB.insertTx(userModel, tx)
	if err != nil {
		tx.Rollback()
		m.Error("添加用户错误", zap.String("username", req.Phone))
		c.ResponseError(err)
		return
	}

	err = m.addSystemFriend(uid)
	if err != nil {
		tx.Rollback()
		c.ResponseError(errors.New("添加后台生成用户和系统账号为好友关系失败"))
		return
	}
	err = m.addFileHelperFriend(uid)
	if err != nil {
		tx.Rollback()
		c.ResponseError(errors.New("添加后台生成用户和文件助手为好友关系失败"))
		return
	}
	//发送用户注册事件
	eventID, err := m.ctx.EventBegin(&wkevent.Data{
		Event: event.EventUserRegister,
		Type:  wkevent.Message,
		Data: map[string]interface{}{
			"uid": uid,
		},
	}, tx)
	if err != nil {
		tx.RollbackUnlessCommitted()
		m.Error("开启事件失败！", zap.Error(err))
		c.ResponseError(errors.New("开启事件失败！"))
		return
	}
	err = tx.Commit()
	if err != nil {
		tx.Rollback()
		m.Error("数据库事物提交失败", zap.Error(err))
		c.ResponseError(errors.New("数据库事物提交失败"))
		return
	}
	m.ctx.EventCommit(eventID)
	c.ResponseOK()
}

// 用户列表
func (m *Manager) list(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	keyword := c.Query("keyword")
	onlineStr := c.Query("online")

	var online int64 = -1
	if strings.TrimSpace(onlineStr) != "" {
		online, _ = strconv.ParseInt(onlineStr, 10, 64)
	}
	pageIndex, pageSize := c.GetPage()
	var userList []*managerUserModel
	var count int64
	if keyword == "" {
		userList, err = m.db.queryUserListWithPage(uint64(pageSize), uint64(pageIndex), int(online))
		if err != nil {
			m.Error("查询用户列表报错", zap.Error(err))
			c.ResponseError(err)
			return
		}

		count, err = m.userDB.queryUserCount()
		if err != nil {
			m.Error("查询用户数量错误", zap.Error(err))
			c.ResponseError(errors.New("查询用户数量错误"))
			return
		}
	} else {
		userList, err = m.db.queryUserListWithPageAndKeyword(keyword, int(online), uint64(pageSize), uint64(pageIndex))
		if err != nil {
			m.Error("查询用户列表报错", zap.Error(err))
			c.ResponseError(err)
			return
		}

		count, err = m.db.queryUserCountWithKeyWord(keyword)
		if err != nil {
			m.Error("查询用户数量错误", zap.Error(err))
			c.ResponseError(errors.New("查询用户数量错误"))
			return
		}
	}

	result := make([]*managerUserResp, 0)
	if len(userList) > 0 {
		uids := make([]string, 0)
		for _, user := range userList {
			uids = append(uids, user.UID)
		}
		resps, err := m.onlineService.GetUserLastOnlineStatus(uids)
		respsdata := map[string]*config.OnlinestatusResp{}
		if len(resps) > 0 {
			for _, v := range resps {
				respsdata[v.UID] = v
			}
		}
		if err != nil {
			m.Error("查询用户在线状态失败", zap.Error(err))
			c.ResponseError(errors.New("查询用户在线状态失败"))
			return
		}
		devices, err := m.deviceDB.queryDeviceLastLoginWithUids(uids)
		if err != nil {
			m.Error("查询用户最后一次登录设备信息错误", zap.Error(err))
			c.ResponseError(errors.New("查询用户最后一次登录设备信息错误"))
			return
		}
		var i = 0
		for _, user := range userList {
			var device *deviceModel
			if len(devices) > 0 {
				for _, model := range devices {
					if model.UID == user.UID {
						device = model
						break
					}
				}
			}
			var lastLoginTime string
			var deviceName string = ""
			var deviceModel string = ""
			var online int
			var lastOnlineTime string = ""
			if device != nil {
				deviceModel = device.DeviceModel
				deviceName = device.DeviceName
				lastLoginTime = util.ToyyyyMMddHHmm(time.Unix(device.LastLogin, 0))
			}
			/* if i < len(resps) {
				online = resps[i].Online
				lastOnlineTime = util.ToyyyyMMddHHmm(time.Unix(int64(resps[i].LastOffline), 0))
			} */
			if respsdata[user.UID] != nil {
				online = respsdata[user.UID].Online
				lastOnlineTime = util.ToyyyyMMddHHmm(time.Unix(int64(respsdata[user.UID].LastOffline), 0))
			}
			showPhone := getShowPhoneNum(user.Phone)
			result = append(result, &managerUserResp{
				UID:            user.UID,
				Username:       user.Username,
				Name:           user.Name,
				Phone:          showPhone,
				Sex:            user.Sex,
				ShortNo:        user.ShortNo,
				LastLoginTime:  lastLoginTime,
				DeviceName:     deviceName,
				DeviceModel:    deviceModel,
				Online:         online,
				LastOnlineTime: lastOnlineTime,
				RegisterTime:   user.CreatedAt.String(),
				Status:         user.Status,
				IsDestroy:      user.IsDestroy,
				GiteeUID:       user.GiteeUID,
				GithubUID:      user.GithubUID,
				WXOpenid:       user.WXOpenid,
			})
			i++
		}
	}
	c.Response(map[string]interface{}{
		"list":  result,
		"count": count,
	})
}

// 查询某个用户的好友
func (m *Manager) friends(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	uid := c.Query("uid")
	if uid == "" {
		c.ResponseError(errors.New("查询用户ID不能为空"))
		return
	}
	sortType := c.Query("sort_type")
	if sortType == "" {
		sortType = "1"
	}
	sortTypeInt, err := strconv.Atoi(sortType)
	if err != nil {
		sortTypeInt = 0
	}
	list, err := m.friendDB.QueryFriends(uid)
	if err != nil {
		m.Error("查询用户好友错误", zap.String("uid", uid))
		c.ResponseError(err)
		return
	}
	result := make([]*managerFriendResp, 0)
	if len(list) == 0 {
		c.Response(result)
		return
	}
	if sortTypeInt == 0 {
		for _, friend := range list {
			result = append(result, &managerFriendResp{
				UID:              friend.ToUID,
				Remark:           friend.Remark,
				Name:             friend.ToName,
				RelationshipTime: friend.CreatedAt.String(),
			})
		}
		c.Response(result)
		return
	}
	// 查询最近会话
	conversations, err := m.ctx.IMSyncUserConversation(uid, 0, 1, "", nil)
	if err != nil {
		m.Error("同步离线后的最近会话失败！", zap.Error(err), zap.String("loginUID", uid))
		c.ResponseError(errors.New("同步离线后的最近会话失败！"))
		return
	}
	if len(conversations) == 0 {
		for _, friend := range list {
			result = append(result, &managerFriendResp{
				UID:              friend.ToUID,
				Remark:           friend.Remark,
				Name:             friend.ToName,
				RelationshipTime: friend.CreatedAt.String(),
			})
		}
		c.Response(result)
		return
	}
	sort.SliceStable(conversations, func(i, j int) bool {
		return conversations[i].Timestamp > conversations[j].Timestamp
	})
	for _, conv := range conversations {
		if conv.ChannelType != common.ChannelTypePerson.Uint8() {
			continue
		}
		var f *DetailModel
		for _, friend := range list {
			if friend.ToUID == conv.ChannelID {
				f = friend
				break
			}
		}
		if f != nil {
			result = append(result, &managerFriendResp{
				UID:              f.ToUID,
				Remark:           f.Remark,
				Name:             f.ToName,
				RelationshipTime: f.CreatedAt.String(),
			})
		}
	}
	for _, f := range list {
		isAdd := true
		for _, r := range result {
			if r.UID == f.ToUID {
				isAdd = false
				break
			}
		}
		if isAdd {
			result = append(result, &managerFriendResp{
				UID:              f.ToUID,
				Remark:           f.Remark,
				Name:             f.ToName,
				RelationshipTime: f.CreatedAt.String(),
			})
		}
	}
	c.Response(result)
}

// 查询某个用户的黑名单
func (m *Manager) blacklist(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	uid := c.Query("uid")
	if uid == "" {
		c.ResponseError(errors.New("查询用户ID不能为空"))
		return
	}
	list, err := m.db.queryUserBlacklists(uid)
	if err != nil {
		m.Error("查询黑名单列表失败！", zap.Error(err))
		c.ResponseError(errors.New("查询黑名单列表失败！"))
		return
	}
	blacklists := []*managerBlackUserResp{}
	for _, result := range list {
		blacklists = append(blacklists, &managerBlackUserResp{
			UID:      result.UID,
			Name:     result.Name,
			CreateAt: result.UpdatedAt.String(),
		})
	}
	c.Response(blacklists)
}

// 查看封禁用户列表
func (m *Manager) disableUsers(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	pageIndex, pageSize := c.GetPage()
	list, err := m.db.queryUserListWithStatus(int(common.UserDisable), uint64(pageSize), uint64(pageIndex))
	if err != nil {
		m.Error("通过状态查询用户列表错误", zap.Error(err))
		c.ResponseError(errors.New("通过状态查询用户列表错误"))
		return
	}
	count, err := m.db.queryUserCountWithStatus(int(common.UserDisable))
	if err != nil {
		m.Error("查询用户数量错误", zap.Error(err))
		c.ResponseError(errors.New("查询用户数量错误"))
		return
	}
	result := make([]*managerDisableUserResp, 0)
	if len(list) > 0 {
		for _, user := range list {
			showPhone := getShowPhoneNum(user.Phone)
			result = append(result, &managerDisableUserResp{
				Name:         user.Name,
				ShortNo:      user.ShortNo,
				Phone:        showPhone,
				UID:          user.UID,
				ClosureTime:  user.UpdatedAt.String(),
				RegisterTime: user.CreatedAt.String(),
			})
		}
	}
	c.Response(map[string]interface{}{
		"list":  result,
		"count": count,
	})
}

// 封禁或解禁用户
func (m *Manager) liftBanUser(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	uid := c.Param("uid")
	status := c.Param("status")
	if uid == "" {
		c.ResponseError(errors.New("操作用户id不能为空"))
		return
	}
	if status == "" {
		c.ResponseError(errors.New("修改状态类型不能为空"))
		return
	}
	userStatus, _ := strconv.Atoi(status)
	if userStatus != int(common.UserAvailable) && userStatus != int(common.UserDisable) {
		c.ResponseError(errors.New("修改状态类型不匹配"))
		return
	}
	userInfo, err := m.userDB.QueryByUID(uid)
	if err != nil {
		m.Error("查询用户信息失败！", zap.String("uid", uid))
		c.ResponseError(errors.New("查询用户信息错误"))
		return
	}
	if userInfo == nil {
		c.ResponseError(errors.New("操作用户不存在"))
		return
	}
	if userInfo.Status == userStatus {
		c.ResponseOK()
		return
	}
	err = m.userDB.UpdateUsersWithField("status", status, uid)
	if err != nil {
		m.Error("修改用户状态错误", zap.Error(err))
		c.ResponseError(errors.New("修改用户状态错误"))
		return
	}

	ban := 0
	if userStatus == int(common.UserDisable) {
		ban = 1
	}

	err = m.ctx.IMCreateOrUpdateChannelInfo(&config.ChannelInfoCreateReq{
		ChannelID:   uid,
		ChannelType: common.ChannelTypePerson.Uint8(),
		Ban:         ban,
	})
	if err != nil {
		m.Error("更新WebIM的token失败！", zap.Error(err))
		c.ResponseError(errors.New("更新IM的token失败！"))
		return
	}
	err = m.ctx.QuitUserDevice(userInfo.UID, -1)
	if err != nil {
		m.Error("下线用户所有登录设备错误", zap.Error(err), zap.String("uid", uid))
		c.ResponseError(errors.New("下线用户所有登录设备错误"))
		return
	}
	c.ResponseOK()
}

// 修改登录密码
func (m *Manager) updatePwd(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	loginUID := c.GetLoginUID()
	type updatePwdReq struct {
		Password    string `json:"password"`
		NewPassword string `json:"new_password"`
	}
	var req updatePwdReq
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	if req.Password == "" || req.NewPassword == "" {
		c.ResponseError(errors.New("密码不能为空"))
		return
	}
	user, err := m.userDB.QueryByUID(loginUID)
	if err != nil {
		m.Error("查询用户信息错误", zap.Error(err))
		c.ResponseError(errors.New("查询用户信息错误"))
		return
	}
	if user == nil {
		c.ResponseError(errors.New("操作用户不存在"))
		return
	}
	if util.MD5(util.MD5(req.Password)) != user.Password {
		c.ResponseError(errors.New("原密码错误"))
		return
	}
	if len(req.NewPassword) < 6 {
		c.ResponseError(errors.New("密码长度必须大于6位"))
		return
	}
	if req.Password == req.NewPassword {
		c.ResponseError(errors.New("新密码不能和旧密码一样"))
		return
	}
	err = m.userDB.UpdateUsersWithField("password", util.MD5(util.MD5(req.NewPassword)), loginUID)
	if err != nil {
		m.Error("修改用户密码错误", zap.Error(err))
		c.Response("修改用户密码错误")
		return
	}
	// 清除token缓存
	oldToken, err := m.ctx.Cache().Get(fmt.Sprintf("%s%d%s", m.ctx.GetConfig().Cache.UIDTokenCachePrefix, config.Web, user.UID))
	if err != nil {
		m.Error("获取旧token错误", zap.Error(err))
		c.ResponseError(errors.New("获取旧token错误"))
		return
	}
	if oldToken != "" {
		err = m.ctx.Cache().Delete(m.ctx.GetConfig().Cache.TokenCachePrefix + oldToken)
		if err != nil {
			m.Error("清除旧token数据错误", zap.Error(err))
			c.ResponseError(errors.New("清除旧token数据错误"))
			return
		}
	}
	c.ResponseOK()
}
func (r managerAddUserReq) checkAddUserReq() error {
	if strings.TrimSpace(r.Name) == "" {
		return errors.New("用户名不能为空！")
	}
	if strings.TrimSpace(r.Password) == "" {
		return errors.New("密码不能为空！")
	}
	if strings.TrimSpace(r.Phone) == "" {
		return errors.New("手机号不能为空！")
	}
	if !regexp.MustCompile(`^\d+$`).MatchString(strings.TrimSpace(r.Phone)) {
		return errors.New("手机号格式有误")
	}
	if strings.TrimSpace(r.Zone) == "" {
		return errors.New("区号不能为空！")
	}

	return nil
}
func (r managerLoginReq) Check() error {
	if strings.TrimSpace(r.Username) == "" {
		return errors.New("用户名不能为空！")
	}
	if strings.TrimSpace(r.Password) == "" {
		return errors.New("密码不能为空！")
	}
	return nil
}

// 处理注册用户和文件助手互为好友
func (m *Manager) addFileHelperFriend(uid string) error {
	if uid == "" {
		m.Error("用户ID不能为空")
		return errors.New("用户ID不能为空")
	}
	isFriend, err := m.friendDB.IsFriend(uid, m.ctx.GetConfig().Account.FileHelperUID)
	if err != nil {
		m.Error("查询用户关系失败")
		return err
	}
	if !isFriend {
		version := m.ctx.GenSeq(common.FriendSeqKey)
		err := m.friendDB.Insert(&FriendModel{
			UID:     uid,
			ToUID:   m.ctx.GetConfig().Account.FileHelperUID,
			Version: version,
		})
		if err != nil {
			m.Error("注册用户和文件助手成为好友失败")
			return err
		}
	}
	return nil
}

// addSystemFriend 处理注册用户和系统账号互为好友
func (m *Manager) addSystemFriend(uid string) error {

	if uid == "" {
		m.Error("用户ID不能为空")
		return errors.New("用户ID不能为空")
	}
	isFriend, err := m.friendDB.IsFriend(uid, m.ctx.GetConfig().Account.SystemUID)
	if err != nil {
		m.Error("查询用户关系失败")
		return err
	}
	tx, err := m.friendDB.session.Begin()
	if err != nil {
		m.Error("开启事物错误", zap.Error(err))
		return errors.New("开启事物错误")
	}
	defer func() {
		if err := recover(); err != nil {
			tx.Rollback()
			panic(err)
		}
	}()
	if !isFriend {
		version := m.ctx.GenSeq(common.FriendSeqKey)
		err := m.friendDB.InsertTx(&FriendModel{
			UID:     uid,
			ToUID:   m.ctx.GetConfig().Account.SystemUID,
			Version: version,
		}, tx)
		if err != nil {
			m.Error("注册用户和系统账号成为好友失败")
			tx.Rollback()
			return err
		}
	}
	// systemIsFriend, err := u.friendDB.IsFriend(u.ctx.GetConfig().SystemUID, uid)
	// if err != nil {
	// 	u.Error("查询系统账号和注册用户关系失败")
	// 	tx.Rollback()
	// 	return err
	// }
	// if !systemIsFriend {
	// 	version := u.ctx.GenSeq(common.FriendSeqKey)
	// 	err := u.friendDB.InsertTx(&FriendModel{
	// 		UID:     u.ctx.GetConfig().SystemUID,
	// 		ToUID:   uid,
	// 		Version: version,
	// 	}, tx)
	// 	if err != nil {
	// 		u.Error("系统账号和注册用户成为好友失败")
	// 		tx.Rollback()
	// 		return err
	// 	}
	// }
	err = tx.Commit()
	if err != nil {
		tx.Rollback()
		m.Error("用户注册数据库事物提交失败", zap.Error(err))
		return err
	}
	return nil
}

// 创建一个系统管理账户
func (m *Manager) createManagerAccount() {
	user, err := m.userDB.QueryByUID(m.ctx.GetConfig().Account.AdminUID)
	if err != nil {
		m.Error("查询系统管理账号错误", zap.Error(err))
		return
	}
	if (user != nil && user.UID != "") || m.ctx.GetConfig().AdminPwd == "" {
		return
	}

	username := string(wkhttp.SuperAdmin)
	role := string(wkhttp.SuperAdmin)
	var pwd = m.ctx.GetConfig().AdminPwd
	err = m.userDB.Insert(&Model{
		UID:      m.ctx.GetConfig().Account.AdminUID,
		Name:     "超级管理员",
		ShortNo:  "30000",
		Category: "system",
		Role:     role,
		Username: username,
		Zone:     "0086",
		Phone:    "13000000002",
		Status:   1,
		Password: util.MD5(util.MD5(pwd)),
	})
	if err != nil {
		m.Error("新增系统管理员错误", zap.Error(err))
		return
	}
}
func (m *Manager) privilegeList(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	page, pageSize := c.GetPage()
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	keyword := strings.TrimSpace(c.Query("keyword"))
	offset := (page - 1) * pageSize
	rows, err := m.privilegeDB.list(int(pageSize), int(offset), keyword)
	if err != nil {
		m.Error("查询特权用户列表失败", zap.Error(err))
		c.ResponseError(errors.New("查询特权用户列表失败"))
		return
	}
	total, err := m.privilegeDB.count(keyword)
	if err != nil {
		m.Error("查询特权用户总数失败", zap.Error(err))
		c.ResponseError(errors.New("查询特权用户总数失败"))
		return
	}
	c.Response(map[string]interface{}{
		"data":      rows,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (m *Manager) privilegeSearchUser(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	keyword := strings.TrimSpace(c.Query("keyword"))
	if keyword == "" {
		c.ResponseError(errors.New("keyword不能为空"))
		return
	}
	row, err := m.privilegeDB.searchUser(keyword)
	if err != nil {
		m.Error("搜索用户失败", zap.Error(err))
		c.ResponseError(errors.New("搜索用户失败"))
		return
	}
	if row == nil {
		c.Response(map[string]interface{}{"data": nil})
		return
	}
	c.Response(map[string]interface{}{"data": row})
}

func normalizeSwitchValue(v int) int {
	if v == 1 {
		return 1
	}
	return 0
}

func (m *Manager) privilegeCreate(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	var req struct {
		UID                  string `json:"uid"`
		GroupManageOn        int    `json:"group_manage_on"`
		AllMemberInviteOn    int    `json:"all_member_invite_on"`
		MutualDeletePersonOn int    `json:"mutual_delete_person_on"`
		MutualDeleteGroupOn  int    `json:"mutual_delete_group_on"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误"))
		return
	}
	uid := strings.TrimSpace(req.UID)
	if uid == "" {
		c.ResponseError(errors.New("uid不能为空"))
		return
	}
	user, err := m.userDB.QueryByUID(uid)
	if err != nil {
		m.Error("查询用户失败", zap.Error(err))
		c.ResponseError(errors.New("查询用户失败"))
		return
	}
	if user == nil || user.UID == "" {
		c.ResponseError(errors.New("用户不存在"))
		return
	}
	exists, err := m.privilegeDB.queryByUID(uid)
	if err != nil {
		m.Error("查询特权用户失败", zap.Error(err))
		c.ResponseError(errors.New("查询特权用户失败"))
		return
	}
	if exists != nil && exists.UID != "" {
		c.ResponseError(errors.New("该用户已是特权用户"))
		return
	}
	err = m.privilegeDB.insert(&privilegeModel{
		UID:                  uid,
		GroupManageOn:        normalizeSwitchValue(req.GroupManageOn),
		AllMemberInviteOn:    normalizeSwitchValue(req.AllMemberInviteOn),
		MutualDeletePersonOn: normalizeSwitchValue(req.MutualDeletePersonOn),
		MutualDeleteGroupOn:  normalizeSwitchValue(req.MutualDeleteGroupOn),
	})
	if err != nil {
		m.Error("新增特权用户失败", zap.Error(err))
		c.ResponseError(errors.New("新增特权用户失败"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) privilegeDelete(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	uid := strings.TrimSpace(c.Query("uid"))
	if uid == "" {
		c.ResponseError(errors.New("uid不能为空"))
		return
	}
	err = m.privilegeDB.deleteByUID(uid)
	if err != nil {
		m.Error("删除特权用户失败", zap.Error(err))
		c.ResponseError(errors.New("删除特权用户失败"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) privilegeSwitchUpdate(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	var req struct {
		UID   string `json:"uid"`
		Field string `json:"field"`
		Value int    `json:"value"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误"))
		return
	}
	uid := strings.TrimSpace(req.UID)
	if uid == "" {
		c.ResponseError(errors.New("uid不能为空"))
		return
	}
	allowed := map[string]bool{
		"group_manage_on":         true,
		"all_member_invite_on":    true,
		"mutual_delete_person_on": true,
		"mutual_delete_group_on":  true,
	}
	if !allowed[req.Field] {
		c.ResponseError(errors.New("field不合法"))
		return
	}
	exists, err := m.privilegeDB.queryByUID(uid)
	if err != nil {
		m.Error("查询特权用户失败", zap.Error(err))
		c.ResponseError(errors.New("查询特权用户失败"))
		return
	}
	if exists == nil || exists.UID == "" {
		c.ResponseError(errors.New("特权用户不存在"))
		return
	}
	err = m.privilegeDB.updateSwitch(uid, req.Field, normalizeSwitchValue(req.Value))
	if err != nil {
		m.Error("更新特权开关失败", zap.Error(err))
		c.ResponseError(errors.New("更新特权开关失败"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) privilegeGlobalGet(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	cfg, err := m.commonService.GetAppConfig()
	if err != nil {
		m.Error("获取全局配置失败", zap.Error(err))
		c.ResponseError(errors.New("获取全局配置失败"))
		return
	}
	c.Response(map[string]interface{}{
		"privilege_only_add_friend_on":          cfg.PrivilegeOnlyAddFriendOn,
		"friend_apply_auto_accept_on":           cfg.FriendApplyAutoAcceptOn,
		"privilege_only_create_invite_group_on": cfg.PrivilegeOnlyCreateInviteGroupOn,
		"show_last_offline_on":                  cfg.ShowLastOfflineOn,
	})
}

func (m *Manager) privilegeGlobalUpdate(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	var req struct {
		PrivilegeOnlyAddFriendOn         *int `json:"privilege_only_add_friend_on"`
		FriendApplyAutoAcceptOn          *int `json:"friend_apply_auto_accept_on"`
		PrivilegeOnlyCreateInviteGroupOn *int `json:"privilege_only_create_invite_group_on"`
		ShowLastOfflineOn                *int `json:"show_last_offline_on"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误"))
		return
	}
	if req.PrivilegeOnlyAddFriendOn == nil && req.FriendApplyAutoAcceptOn == nil && req.PrivilegeOnlyCreateInviteGroupOn == nil && req.ShowLastOfflineOn == nil {
		c.ResponseError(errors.New("至少传入一个开关字段"))
		return
	}
	setMap := map[string]interface{}{"updated_at": dbr.Now}
	if req.PrivilegeOnlyAddFriendOn != nil {
		setMap["privilege_only_add_friend_on"] = normalizeSwitchValue(*req.PrivilegeOnlyAddFriendOn)
	}
	if req.FriendApplyAutoAcceptOn != nil {
		setMap["friend_apply_auto_accept_on"] = normalizeSwitchValue(*req.FriendApplyAutoAcceptOn)
	}
	if req.PrivilegeOnlyCreateInviteGroupOn != nil {
		setMap["privilege_only_create_invite_group_on"] = normalizeSwitchValue(*req.PrivilegeOnlyCreateInviteGroupOn)
	}
	if req.ShowLastOfflineOn != nil {
		setMap["show_last_offline_on"] = normalizeSwitchValue(*req.ShowLastOfflineOn)
	}
	_, err = m.ctx.DB().Update("app_config").SetMap(setMap).Where("id > ?", 0).Exec()
	if err != nil {
		m.Error("更新全局配置失败", zap.Error(err))
		c.ResponseError(errors.New("更新全局配置失败"))
		return
	}
	c.ResponseOK()
}

func (m *Manager) loginIPHistory(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	page, pageSize := c.GetPage()
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	uid := strings.TrimSpace(c.Query("uid"))
	loginIP := strings.TrimSpace(c.Query("login_ip"))

	type row struct {
		Name        string `db:"name" json:"name"`
		Username    string `db:"username" json:"username"`
		UID         string `db:"uid" json:"uid"`
		LoginIP     string `db:"login_ip" json:"login_ip"`
		LoginRegion string `json:"login_region"`
		CreatedAt   string `db:"created_at" json:"created_at"`
	}

	q := m.ctx.DB().Select("user.name,user.username,login_log.uid,login_log.login_ip,DATE_FORMAT(login_log.created_at,'%Y-%m-%d %H:%i:%s') as created_at").
		From("login_log").
		LeftJoin("user", "login_log.uid=user.uid")
	cq := m.ctx.DB().Select("count(*)").From("login_log").LeftJoin("user", "login_log.uid=user.uid")
	if uid != "" {
		like := "%" + uid + "%"
		q = q.Where("login_log.uid like ? or user.username like ?", like, like)
		cq = cq.Where("login_log.uid like ? or user.username like ?", like, like)
	}
	if loginIP != "" {
		likeIP := "%" + loginIP + "%"
		q = q.Where("login_log.login_ip like ?", likeIP)
		cq = cq.Where("login_log.login_ip like ?", likeIP)
	}
	var total int64
	_, err = cq.Load(&total)
	if err != nil {
		m.Error("查询登录IP历史总数失败", zap.Error(err))
		c.ResponseError(errors.New("查询登录IP历史总数失败"))
		return
	}
	var list []*row
	_, err = q.OrderDir("login_log.created_at", false).Limit(uint64(pageSize)).Offset(uint64(offset)).Load(&list)
	if err != nil {
		m.Error("查询登录IP历史失败", zap.Error(err))
		c.ResponseError(errors.New("查询登录IP历史失败"))
		return
	}
	if list == nil {
		list = make([]*row, 0)
	}
	regionCache := map[string]string{}
	for _, item := range list {
		ip := strings.TrimSpace(item.LoginIP)
		if ip == "" {
			item.LoginRegion = "-"
			continue
		}
		if cached, ok := regionCache[ip]; ok {
			item.LoginRegion = cached
			continue
		}
		if util.IsIntranet(ip) || ip == "127.0.0.1" || ip == "::1" {
			item.LoginRegion = "内网IP"
			regionCache[ip] = item.LoginRegion
			continue
		}
		province, city, rErr := util.GetIPAddress(ip)
		if rErr != nil {
			item.LoginRegion = "未知"
			regionCache[ip] = item.LoginRegion
			continue
		}
		region := strings.TrimSpace(strings.TrimSpace(province) + " " + strings.TrimSpace(city))
		if region == "" {
			item.LoginRegion = "未知"
		} else {
			item.LoginRegion = region
		}
		regionCache[ip] = item.LoginRegion
	}
	c.Response(map[string]interface{}{
		"data":      list,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func getShowPhoneNum(mobile string) string {
	if len(mobile) <= 3 {
		return mobile
	}
	phone := mobile[:3]
	var length = len(mobile) - 3
	if length > 4 {
		length = 4
	}
	for i := 0; i < length; i++ {
		phone = fmt.Sprintf("%s*", phone)
	}
	var index = 3 + length
	if index > 0 && index < len(mobile) {
		return phone + mobile[index:]
	}
	return phone
}

type managerLoginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type managerLoginResp struct {
	UID   string `json:"uid"`
	Token string `json:"token"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}
type managerAddUserReq struct {
	Name     string `json:"name"`
	Password string `json:"password"`
	Phone    string `json:"phone"`
	Zone     string `json:"zone"`
	Sex      int    `json:"sex"`
}
type managerBlackUserResp struct {
	Name     string `json:"name"`
	UID      string `json:"uid"`
	CreateAt string `json:"create_at"`
}
type adminUserResp struct {
	Name         string `json:"name"`
	UID          string `json:"uid"`
	Username     string `json:"username"`
	RegisterTime string `json:"register_time"`
}
type managerUserResp struct {
	Name           string `json:"name"`
	UID            string `json:"uid"`
	Phone          string `json:"phone"`
	Username       string `json:"username"`
	ShortNo        string `json:"short_no"`
	Sex            int    `json:"sex"`
	RegisterTime   string `json:"register_time"`
	LastLoginTime  string `json:"last_login_time"`
	DeviceName     string `json:"device_name"`
	DeviceModel    string `json:"device_model"`
	Online         int    `json:"online"`
	LastOnlineTime string `json:"last_online_time"`
	Status         int    `json:"status"`
	IsDestroy      int    `json:"is_destroy"`
	WXOpenid       string `json:"wx_openid"`  // 微信openid
	GiteeUID       string `json:"gitee_uid"`  // gitee uid
	GithubUID      string `json:"github_uid"` // github uid
}

type managerFriendResp struct {
	Name             string `json:"name"`
	UID              string `json:"uid"`
	Remark           string `json:"remark"`
	RelationshipTime string `json:"relationship_time"`
}

type managerDisableUserResp struct {
	Name         string `json:"name"`
	UID          string `json:"uid"`
	ShortNo      string `json:"short_no"`
	Sex          int    `json:"sex"`
	RegisterTime string `json:"register_time"`
	Phone        string `json:"phone"`
	ClosureTime  string `json:"closure_time"`
}

type managerDeviceResp struct {
	ID          int64  `json:"id"`
	DeviceID    string `json:"device_id"`    // 设备ID
	DeviceName  string `json:"device_name"`  // 设备名称
	DeviceModel string `json:"device_model"` // 设备型号
	LastLogin   string `json:"last_login"`   // 设备最后一次登录时间
	Self        int    `json:"self"`         // 是否是本机
}

type userOnlineResp struct {
	UID         string `json:"uid"`
	DeviceFlag  uint8  `json:"device_flag"`
	LastOnline  int    `json:"last_online"`
	LastOffline int    `json:"last_offline"`
	Online      int    `json:"online"`
}

func newUserOnlineResp(m *onlineStatusWeightModel) *userOnlineResp {

	return &userOnlineResp{
		UID:         m.UID,
		DeviceFlag:  m.DeviceFlag,
		LastOnline:  m.LastOnline,
		LastOffline: m.LastOffline,
		Online:      m.Online,
	}
}
