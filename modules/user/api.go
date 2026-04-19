package user

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"hash/crc32"
	"image"
	"image/draw"
	"image/png"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/file"
	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/source"
	wkutil "github.com/TangSengDaoDao/TangSengDaoDaoServer/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/model"
	"github.com/disintegration/imaging"
	"github.com/gocraft/dbr/v2"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"

	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/base/app"
	commonapi "github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/base/common"
	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/base/event"
	common2 "github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/common"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/common"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/log"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/network"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/register"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkevent"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

var (
	ErrUserNeedVerification = errors.New("user need verification") // 用户需要验证
)

// normalizeAvatarToPNG 解码后统一为 8 位 NRGBA 再写 PNG，避免 16 位/调色板/灰度等变体在部分 Android Skia 上报 unimplemented。
func normalizeAvatarToPNG(r io.Reader) ([]byte, error) {
	img, err := imaging.Decode(r, imaging.AutoOrientation(true))
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	nrgba := image.NewNRGBA(b)
	draw.Draw(nrgba, b, img, b.Min, draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, nrgba); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

var qrcodeChanMap = map[string]chan *common.QRCodeModel{}
var qrcodeChanLock sync.RWMutex

// User 用户相关API
type User struct {
	db            *DB
	friendDB      *friendDB
	deviceDB      *deviceDB
	smsServie     commonapi.ISMSService
	fileService   file.IService
	settingDB     *SettingDB
	onlineDB      *onlineDB
	userService   IService
	onlineService *OnlineService
	giteeDB       *giteeDB
	githubDB      *githubDB

	setting *Setting
	log.Log
	ctx                      *config.Context
	userDeviceTokenPrefix    string
	loginUUIDPrefix          string
	openapiAuthcodePrefix    string
	openapiAccessTokenPrefix string
	loginLog                 *LoginLog
	privilegeDB              *privilegeDB
	onetimePrekeysDB         *onetimePrekeysDB
	maillistDB               *maillistDB
	commonService            common2.IService
	deviceFlagDB             *deviceFlagDB
	deviceFlagsCache         []*deviceFlagModel
	appService               app.IService
}

// New New
func New(ctx *config.Context) *User {
	u := &User{
		ctx:                      ctx,
		db:                       NewDB(ctx),
		deviceDB:                 newDeviceDB(ctx),
		friendDB:                 newFriendDB(ctx),
		smsServie:                commonapi.NewSMSService(ctx),
		settingDB:                NewSettingDB(ctx.DB()),
		setting:                  NewSetting(ctx),
		userDeviceTokenPrefix:    common.UserDeviceTokenPrefix,
		loginUUIDPrefix:          "loginUUID:",
		openapiAuthcodePrefix:    "openapi:authcodePrefix:",
		openapiAccessTokenPrefix: "openapi:accessTokenPrefix:",
		onlineDB:                 newOnlineDB(ctx),
		onlineService:            NewOnlineService(ctx),
		Log:                      log.NewTLog("User"),
		fileService:              file.NewService(ctx),
		userService:              NewService(ctx),
		loginLog:                 NewLoginLog(ctx),
		privilegeDB:              newPrivilegeDB(ctx.DB()),
		onetimePrekeysDB:         newOnetimePrekeysDB(ctx),
		maillistDB:               newMaillistDB(ctx),
		deviceFlagDB:             newDeviceFlagDB(ctx),
		giteeDB:                  newGiteeDB(ctx),
		githubDB:                 newGithubDB(ctx),
		commonService:            common2.NewService(ctx),
		appService:               app.NewService(ctx),
	}
	u.updateSystemUserToken()
	source.SetUserProvider(u)
	return u
}

// Route 路由配置
func (u *User) Route(r *wkhttp.WKHttp) {
	auth := r.Group("/v1", u.ctx.AuthMiddleware(r))
	{

		auth.GET("/users/:uid/im", u.userIM) // 获取当前登录用户 IM 节点（需鉴权；拉路由前同步 token 到悟空）
		auth.GET("/users/:uid", u.get) // 根据uid查询用户信息
		auth.GET("/invite", u.myInviteCode)
		// 获取用户的会话信息
		// auth.GET("/users/:uid/conversation", u.userConversationInfoGet)

		auth.POST("/users/:uid/avatar", u.uploadAvatar)              //上传用户头像
		auth.PUT("/users/:uid/setting", u.setting.userSettingUpdate) // 更新用户设置
	}

	user := r.Group("/v1/user", u.ctx.AuthMiddleware(r))
	{
		user.POST("/device_token", u.registerUserDeviceToken)      // 注册用户设备
		user.DELETE("/device_token", u.unregisterUserDeviceToken)  // 卸载用户设备
		user.POST("/device_badge", u.registerUserDeviceBadge)      // 上传设备红点数量
		user.GET("/grant_login", u.grantLogin)                     // 授权登录
		user.PUT("/current", u.userUpdateWithField)                //修改用户信息
		user.GET("/qrcode", u.qrcodeMy)                            // 我的二维码
		user.PUT("/my/setting", u.userUpdateSetting)               // 更新我的设置
		user.POST("/blacklist/:uid", u.addBlacklist)               //添加黑名单
		user.DELETE("/blacklist/:uid", u.removeBlacklist)          //移除黑名单
		user.GET("/blacklists", u.blacklists)                      //黑名单列表
		user.POST("/chatpwd", u.setChatPwd)                        //设置聊天密码
		user.POST("/lockscreenpwd", u.setLockScreenPwd)            //设置锁屏密码
		user.PUT("/lock_after_minute", u.lockScreenAfterMinuteSet) // 设置多久后锁屏
		user.DELETE("/lockscreenpwd", u.closeLockScreenPwd)        //关闭锁屏密码
		user.GET("/customerservices", u.customerservices)          //客服列表
		user.DELETE("/destroy/:code", u.destroyAccount)            // 注销用户
		user.POST("/sms/destroy", u.sendDestroyCode)               //获取注销账号短信验证码
		user.PUT("/updatepassword", u.updatePwd)                   // 修改登录密码
		user.POST("/web3publickey", u.uploadWeb3PublicKey)         // 上传web3公钥
		user.POST("/quit", u.quit)                                 // 退出登录
		// #################### 登录设备管理 ####################
		user.GET("/devices", u.deviceList)                 // 用户登录设备
		user.DELETE("/devices/:device_id", u.deviceDelete) // 删除登录设备
		user.GET("/devices/:device_id", u.getDevice)       // 查询某个登录设备
		user.GET("/online", u.onlineList)                  // 用户在线列表（我的设备和我的好友）
		user.POST("/online", u.onlinelistWithUIDs)         // 获取指定的uid在线状态
		user.POST("/pc/quit", u.pcQuit)                    // 退出pc登录

		// #################### 用户通讯录 ####################
		user.GET("/search", u.search) // 搜索用户
		user.POST("/maillist", u.addMaillist)
		user.GET("/maillist", u.getMailList)

		// #################### 用户红点 ####################
		user.GET("/reddot/:category", u.getRedDot)      // 获取用户红点
		user.DELETE("/reddot/:category", u.clearRedDot) // 清除红点
	}
	v := r.Group("/v1")
	{

		v.POST("/user/register", u.register)                 //用户注册
		v.POST("/user/login", u.login)                       // 用户登录
		v.POST("/user/usernamelogin", u.usernameLogin)       // 用户名登录
		v.POST("/user/usernameregister", u.usernameRegister) // 用户名注册

		v.POST("/user/pwdforget_web3", u.resetPwdWithWeb3PublicKey) // 通过web3公钥重置密码
		v.GET("/user/web3verifytext", u.getVerifyText)              // 获取验证字符串
		v.POST("/user/web3verifysign", u.web3verifySignature)       // 验证签名
		//v.POST("user/wxlogin", u.wxLogin)
		v.POST("/user/sms/forgetpwd", u.getForgetPwdSMS) //获取忘记密码验证码
		v.POST("/user/pwdforget", u.pwdforget)           //重置登录密码
		v.GET("/users/:uid/avatar", u.UserAvatar)        // 用户头像（兼容旧客户端）
		v.RouterGroup.HEAD("/users/:uid/avatar", r.WKHttpHandler(u.UserAvatar))
		v.GET("/users/:uid/avatar.png", u.UserAvatar) // 同上，带后缀便于 Glide/按扩展名选解码器
		v.RouterGroup.HEAD("/users/:uid/avatar.png", r.WKHttpHandler(u.UserAvatar))
		v.GET("/user/loginuuid", u.getLoginUUID) // 获取扫描用的登录uuid
		v.GET("/user/loginstatus", u.getloginStatus)
		v.POST("/user/sms/registercode", u.sendRegisterCode)             //获取注册短信验证码
		v.POST("/user/login_authcode/:auth_code", u.loginWithAuthCode)   // 通过认证码登录
		v.POST("/user/sms/login_check_phone", u.sendLoginCheckPhoneCode) //发送登录设备验证验证码
		v.POST("/user/login/check_phone", u.loginCheckPhone)             //登录验证设备手机号

		// #################### 第三方授权 ####################
		v.GET("/user/thirdlogin/authcode", u.thirdAuthcode)     // 第三方授权码获取
		v.GET("/user/thirdlogin/authstatus", u.thirdAuthStatus) // github认证页面
		// github
		v.GET("/user/github", u.github)            // github认证页面
		v.GET("/user/oauth/github", u.githubOAuth) // github登录
		// gitee
		v.GET("/user/gitee", u.gitee)            // gitee认证页面
		v.GET("/user/oauth/gitee", u.giteeOAuth) // gitee登录

	}

	u.ctx.AddOnlineStatusListener(u.onlineService.listenOnlineStatus) // 监听在线状态
	u.ctx.AddOnlineStatusListener(u.handleOnlineStatus)               // 需要放在listenOnlineStatus之后
	u.ctx.Schedule(time.Minute*5, u.onlineStatusCheck)                // 在线状态定时检查

}

// 我的邀请码
// 返回格式固定为 {invite_code: string, status: int}，避免把错误文案污染到 invite_code。
func (u *User) myInviteCode(c *wkhttp.Context) {
	loginUID := strings.TrimSpace(c.GetLoginUID())
	if loginUID == "" {
		c.ResponseError(errors.New("请先登录"))
		return
	}

	type inviteRow struct {
		InviteCode string `db:"invite_code"`
		Status     int    `db:"status"`
	}
	var rows []*inviteRow
	_, err := u.ctx.DB().
		Select("invite_code,status").
		From("invite_code").
		Where("uid=?", loginUID).
		OrderDir("created_at", false).
		Limit(1).
		Load(&rows)
	if err != nil {
		u.Error("查询我的邀请码失败", zap.String("uid", loginUID), zap.Error(err))
		c.ResponseError(errors.New("查询我的邀请码失败"))
		return
	}
	if len(rows) == 0 {
		c.Response(map[string]interface{}{
			"invite_code": "",
			"status":      0,
		})
		return
	}
	c.Response(map[string]interface{}{
		"invite_code": strings.TrimSpace(rows[0].InviteCode),
		"status":      rows[0].Status,
	})
}

// app退出登录
func (u *User) quit(c *wkhttp.Context) {
	loginUID := c.GetLoginUID()
	err := u.ctx.QuitUserDevice(loginUID, int(config.Web)) // 退出web
	if err != nil {
		u.Error("退出web设备失败", zap.Error(err))
		c.ResponseError(errors.New("退出web设备失败"))
		return
	}

	err = u.ctx.QuitUserDevice(loginUID, int(config.PC))
	if err != nil {
		u.Error("退出PC设备失败", zap.Error(err))
		c.ResponseError(errors.New("退出PC设备失败"))
		return
	}

	err = u.ctx.GetRedisConn().Del(fmt.Sprintf("%s%s", u.userDeviceTokenPrefix, loginUID))
	if err != nil {
		u.Error("删除设备token失败！", zap.Error(err))
		c.ResponseError(errors.New("删除设备token失败！"))
		return
	}
	c.ResponseOK()
}

// 清除红点
func (u *User) clearRedDot(c *wkhttp.Context) {
	loginUID := c.GetLoginUID()
	category := c.Param("category")
	if category == "" {
		c.ResponseError(errors.New("分类不能为空"))
		return
	}
	userRedDot, err := u.db.queryUserRedDot(loginUID, category)
	if err != nil {
		u.Error("查询用户红点错误", zap.Error(err))
		c.ResponseError(errors.New("查询用户红点错误"))
		return
	}
	if userRedDot != nil {
		userRedDot.Count = 0
		err = u.db.updateUserRedDot(userRedDot)
		if err != nil {
			u.Error("修改用户红点错误", zap.Error(err))
			c.ResponseError(errors.New("查询用户红点错误"))
			return
		}
	}
	c.ResponseOK()
}

// 获取用户红点
func (u *User) getRedDot(c *wkhttp.Context) {
	loginUID := c.GetLoginUID()
	category := c.Param("category")
	if category == "" {
		c.ResponseError(errors.New("分类不能为空"))
		return
	}
	userRedDot, err := u.db.queryUserRedDot(loginUID, UserRedDotCategoryFriendApply)
	if err != nil {
		u.Error("查询用户红点错误", zap.Error(err))
		c.ResponseError(errors.New("查询用户红点错误"))
		return
	}
	count := 0
	isDot := 0
	if userRedDot != nil {
		count = userRedDot.Count
		isDot = userRedDot.IsDot
	}
	c.Response(map[string]interface{}{
		"count":  count,
		"is_dot": isDot,
	})
}

// updateSystemUserToken 更新系统账号token
func (u *User) updateSystemUserToken() {
	_, err := u.ctx.UpdateIMToken(config.UpdateIMTokenReq{
		UID:         u.ctx.GetConfig().Account.SystemUID,
		DeviceFlag:  config.APP,
		DeviceLevel: config.DeviceLevelMaster,
		Token:       util.GenerUUID(),
	})
	if err != nil {
		u.Error("更新IM的token失败！", zap.Error(err))
	}

	_, err = u.ctx.UpdateIMToken(config.UpdateIMTokenReq{
		UID:         u.ctx.GetConfig().Account.FileHelperUID,
		DeviceFlag:  config.APP,
		DeviceLevel: config.DeviceLevelMaster,
		Token:       util.GenerUUID(),
	})
	if err != nil {
		u.Error("更新IM的token失败！", zap.Error(err))
	}

	// 系统管理员
	_, err = u.ctx.UpdateIMToken(config.UpdateIMTokenReq{
		UID:         u.ctx.GetConfig().Account.AdminUID,
		DeviceFlag:  config.APP,
		DeviceLevel: config.DeviceLevelMaster,
		Token:       util.GenerUUID(),
	})
	if err != nil {
		u.Error("更新IM的token失败！", zap.Error(err))
	}

}

const maxAvatarReadBytes = 6 << 20

func (u *User) finishAvatarNormalizedPNG(c *wkhttp.Context, body []byte, ph, downloadFileName string) bool {
	pngBody, err := normalizeAvatarToPNG(bytes.NewReader(body))
	if err != nil {
		u.Warn("规范化 PNG 失败", zap.String("path", ph), zap.Error(err))
		c.String(http.StatusBadGateway, "avatar decode or normalize failed")
		return false
	}
	fn := strings.TrimSpace(downloadFileName)
	if i := strings.LastIndex(fn, "."); i > 0 {
		fn = fn[:i] + ".png"
	} else if fn != "" {
		fn = fn + ".png"
	}
	if fn != "" {
		c.Header("Content-Disposition", fmt.Sprintf("inline; filename=%q", fn))
	}
	c.Header("Cache-Control", "public, max-age=86400")
	c.Header("Content-Length", strconv.Itoa(len(pngBody)))
	c.Data(http.StatusOK, "image/png", pngBody)
	return true
}

// redirectAvatarStorage 普通用户头像：优先 MinIO SDK 直连读对象并以 200 返回图片二进制，
// 避免 302 到 HTTP 地址被 Android cleartext 策略拦截导致头像不显示。
// 返回前校验魔数：若上游/缓存错误导致体为 XML/JSON/空，不再发 200+image/*，避免安卓 Glide/Skia 报 unimplemented。
func (u *User) redirectAvatarStorage(c *wkhttp.Context, ph, downloadFileName, v string, downloadURLPrebuilt string) bool {
	if rc, _, _, err := u.fileService.OpenObjectDirect(ph); err == nil && rc != nil {
		defer rc.Close()
		body, errRead := io.ReadAll(io.LimitReader(rc, maxAvatarReadBytes+1))
		if errRead != nil {
			u.Error("读取头像失败", zap.Error(errRead), zap.String("path", ph))
			c.String(http.StatusBadGateway, "failed to read avatar")
			return false
		}
		if len(body) > maxAvatarReadBytes {
			u.Error("头像过大", zap.String("path", ph), zap.Int("size", len(body)))
			c.String(http.StatusBadGateway, "avatar too large")
			return false
		}
		if wkutil.LooksLikeS3OrHTTPErrorBody(body) || !wkutil.IsRasterImageMagic(body) {
			u.Warn("头像对象非有效图片", zap.String("path", ph), zap.Int("len", len(body)))
			c.String(http.StatusBadGateway, "avatar payload is not a valid image")
			return false
		}
		return u.finishAvatarNormalizedPNG(c, body, ph, downloadFileName)
	}

	downloadURL := strings.TrimSpace(downloadURLPrebuilt)
	if downloadURL == "" {
		var err error
		downloadURL, err = u.fileService.DownloadURLForServerFetch(ph, downloadFileName)
		if err != nil {
			u.Error("获取文件下载地址失败", zap.Error(err))
			c.Writer.WriteHeader(http.StatusInternalServerError)
			return false
		}
	}
	if v != "" {
		sep := "?"
		if strings.Contains(downloadURL, "?") {
			sep = "&"
		}
		downloadURL = fmt.Sprintf("%s%sv=%s", downloadURL, sep, v)
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, downloadURL, nil)
	if err != nil {
		u.Error("创建头像代理请求失败", zap.Error(err))
		c.Writer.WriteHeader(http.StatusInternalServerError)
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		u.Error("拉取头像失败", zap.Error(err))
		c.Writer.WriteHeader(http.StatusBadGateway)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		c.Writer.WriteHeader(resp.StatusCode)
		_, _ = io.CopyN(c.Writer, resp.Body, 4096)
		return false
	}
	body, errRead := io.ReadAll(io.LimitReader(resp.Body, maxAvatarReadBytes+1))
	if errRead != nil {
		u.Error("读取头像代理响应失败", zap.Error(errRead))
		c.String(http.StatusBadGateway, "failed to read avatar upstream")
		return false
	}
	if len(body) > maxAvatarReadBytes {
		c.String(http.StatusBadGateway, "avatar too large")
		return false
	}
	if wkutil.LooksLikeS3OrHTTPErrorBody(body) || !wkutil.IsRasterImageMagic(body) {
		u.Warn("上游头像非有效图片", zap.String("path", ph), zap.Int("len", len(body)))
		c.String(http.StatusBadGateway, "upstream avatar is not a valid image")
		return false
	}
	return u.finishAvatarNormalizedPNG(c, body, ph, downloadFileName)
}

// UserAvatar 用户头像（访客/系统号/文件助手走本地资源 200，不依赖 MinIO；普通用户走存储）
func (u *User) UserAvatar(c *wkhttp.Context) {
	uid := c.Param("uid")
	v := c.Query("v")
	u.Info("UserAvatar request", zap.String("uid", uid), zap.String("v", v))
	if u.ctx.GetConfig().IsVisitor(uid) {
		avatarBytes, err := os.ReadFile("assets/assets/avatar.png")
		if err != nil {
			u.Error("访客头像读取失败！", zap.Error(err))
			c.Writer.WriteHeader(http.StatusNotFound)
			return
		}
		c.Header("Content-Length", strconv.Itoa(len(avatarBytes)))
		c.Data(http.StatusOK, "image/png", avatarBytes)
		return
	}
	if uid == u.ctx.GetConfig().Account.SystemUID {
		avatarBytes, err := os.ReadFile("assets/assets/logo.jpg")
		if err != nil {
			u.Error("系统用户头像读取失败！", zap.Error(err))
			c.Writer.WriteHeader(http.StatusNotFound)
			return
		}
		c.Header("Content-Length", strconv.Itoa(len(avatarBytes)))
		c.Data(http.StatusOK, "image/jpeg", avatarBytes)
		return
	}
	if uid == u.ctx.GetConfig().Account.FileHelperUID {
		avatarBytes, err := os.ReadFile("assets/assets/fileHelper.jpeg")
		if err != nil {
			u.Error("文件传输助手头像读取失败！", zap.Error(err))
			c.Writer.WriteHeader(http.StatusNotFound)
			return
		}
		c.Header("Content-Length", strconv.Itoa(len(avatarBytes)))
		c.Data(http.StatusOK, "image/jpeg", avatarBytes)
		return
	}

	userInfo, err := u.db.QueryByUID(uid)
	if err != nil {
		u.Error("查询用户信息错误", zap.Error(err))
		c.Writer.WriteHeader(http.StatusNotFound)
		return
	}
	if userInfo == nil {
		u.Error("用户不存在", zap.Error(err))
		c.Writer.WriteHeader(http.StatusNotFound)
		return
	}
	// 管理员未上传头像时用镜像内默认图，避免未种子化 MinIO 时头像 502
	adminUID := u.ctx.GetConfig().Account.AdminUID
	if adminUID != "" && uid == adminUID && userInfo.IsUploadAvatar != 1 {
		avatarBytes, err := os.ReadFile("assets/assets/logo.jpg")
		if err != nil {
			u.Error("管理员默认头像读取失败！", zap.Error(err))
			c.Writer.WriteHeader(http.StatusNotFound)
			return
		}
		c.Header("Cache-Control", "public, max-age=86400")
		c.Header("Content-Length", strconv.Itoa(len(avatarBytes)))
		c.Data(http.StatusOK, "image/jpeg", avatarBytes)
		return
	}
	ph := ""
	fileName := fmt.Sprintf("%s.png", uid)
	downloadURLPrebuilt := ""
	if userInfo.IsUploadAvatar == 1 {
		avatarID := crc32.ChecksumIEEE([]byte(uid)) % uint32(u.ctx.GetConfig().Avatar.Partition)
		ph = fmt.Sprintf("/avatar/%d/%s.png", avatarID, uid)
		u.Info("UserAvatar uploaded avatar path", zap.String("uid", uid), zap.String("path", ph), zap.Uint32("avatarID", avatarID))
	} else {
		if u.ctx.GetConfig().Avatar.Default != "" && strings.TrimSpace(u.ctx.GetConfig().Avatar.DefaultBaseURL) == "" {
			avatarPath := u.ctx.GetConfig().Avatar.Default
			imageData, err := os.ReadFile(avatarPath)
			if err != nil {
				u.Error("打开本地默认头像文件失败", zap.Error(err))
			} else {
				c.Header("Content-Disposition", "inline; filename=avatar.png")
				c.Header("Content-Length", strconv.Itoa(len(imageData)))
				c.Data(http.StatusOK, "image/png", imageData)
				return
			}
		}
		if ph == "" {
			avatarID := crc32.ChecksumIEEE([]byte(uid)) % uint32(u.ctx.GetConfig().Avatar.DefaultCount)
			// 与 seed-minio-default-avatars.sh 一致：MinIO 上为 PNG，避免极小/异常 JPEG 与 *.png 文件名混用
			ph = fmt.Sprintf("/avatar/default/test (%d).png", avatarID)
			fileName = fmt.Sprintf("test (%d).png", avatarID)
			if strings.TrimSpace(u.ctx.GetConfig().Avatar.DefaultBaseURL) != "" {
				downloadURLPrebuilt = strings.ReplaceAll(u.ctx.GetConfig().Avatar.DefaultBaseURL, "{avatar}", fmt.Sprintf("%d", avatarID))
			}
			u.Info("UserAvatar default avatar path", zap.String("uid", uid), zap.String("path", ph), zap.String("fileName", fileName), zap.String("defaultBaseURL", u.ctx.GetConfig().Avatar.DefaultBaseURL), zap.Uint32("defaultAvatarID", avatarID))
		}
	}
	u.redirectAvatarStorage(c, ph, fileName, v, downloadURLPrebuilt)
}

// uploadAvatar 上传用户头像
func (u *User) uploadAvatar(c *wkhttp.Context) {
	loginUID := c.GetLoginUID()
	if c.Request.MultipartForm == nil {
		err := c.Request.ParseMultipartForm(1024 * 1024 * 20) // 20M
		if err != nil {
			u.Error("数据格式不正确！", zap.Error(err))
			c.ResponseError(errors.New("数据格式不正确！"))
			return
		}
	}
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		u.Error("读取文件失败！", zap.Error(err))
		c.ResponseError(errors.New("读取文件失败！"))
		return
	}
	defer file.Close()

	pngBytes, err := normalizeAvatarToPNG(file)
	if err != nil {
		u.Error("头像不是有效图片或已损坏", zap.Error(err))
		c.ResponseError(errors.New("请上传有效的图片文件"))
		return
	}
	avatarID := crc32.ChecksumIEEE([]byte(loginUID)) % uint32(u.ctx.GetConfig().Avatar.Partition)
	_, err = u.fileService.UploadFile(fmt.Sprintf("avatar/%d/%s.png", avatarID, loginUID), "image/png", func(w io.Writer) error {
		_, err := w.Write(pngBytes)
		return err
	})
	if err != nil {
		u.Error("上传文件失败！", zap.Error(err))
		c.ResponseError(errors.New("上传文件失败！"))
		return
	}
	friends, err := u.friendDB.QueryFriends(loginUID)
	if err != nil {
		u.Error("查询用户好友失败")
		return
	}
	if len(friends) > 0 {
		uids := make([]string, 0)
		for _, friend := range friends {
			uids = append(uids, friend.ToUID)
		}
		// 发送头像更新命令
		err = u.ctx.SendCMD(config.MsgCMDReq{
			CMD:         common.CMDUserAvatarUpdate,
			Subscribers: uids,
			Param: map[string]interface{}{
				"uid": loginUID,
			},
		})
		if err != nil {
			u.Error("发送个人头像更新命令失败！")
			return
		}
	}
	//更改用户上传头像状态
	err = u.db.UpdateUsersWithField("is_upload_avatar", "1", loginUID)
	if err != nil {
		u.Error("修改用户是否修改头像错误！", zap.Error(err))
		c.ResponseError(errors.New("修改用户是否修改头像错误！"))
		return
	}
	c.ResponseOK()
}

// readRequestAPIToken 与 AuthMiddleware 取 token 规则一致，用于在已鉴权接口里拿到会话 token。
func readRequestAPIToken(c *wkhttp.Context) string {
	token := strings.TrimSpace(c.GetHeader("token"))
	if token == "" {
		authz := strings.TrimSpace(c.GetHeader("Authorization"))
		if strings.HasPrefix(strings.ToLower(authz), "bearer ") {
			token = strings.TrimSpace(authz[len("bearer "):])
		} else if authz != "" {
			token = authz
		}
	}
	if token == "" {
		token = strings.TrimSpace(c.GetHeader("X-Token"))
	}
	if token == "" {
		token = strings.TrimSpace(c.GetHeader("X-Access-Token"))
	}
	if token == "" {
		token = strings.TrimSpace(c.Query("token"))
	}
	if token == "" {
		token = strings.TrimSpace(c.Request.URL.Query().Get("token"))
	}
	if token == "" {
		raw := ""
		if c.Request != nil && c.Request.URL != nil {
			raw = c.Request.URL.RawQuery
		}
		if raw != "" {
			parts := strings.Split(raw, "&")
			for _, p := range parts {
				kv := strings.SplitN(p, "=", 2)
				if len(kv) == 2 && kv[0] == "token" {
					token = strings.TrimSpace(kv[1])
					break
				}
			}
		}
	}
	if token == "" {
		if ck, err := c.Request.Cookie("token"); err == nil {
			token = strings.TrimSpace(ck.Value)
		}
	}
	return token
}

// 获取用户的IM连接地址
func (u *User) userIM(c *wkhttp.Context) {
	uid := c.Param("uid")
	loginUID := c.GetLoginUID()
	ua := strings.TrimSpace(c.GetHeader("User-Agent"))
	if len(ua) > 160 {
		ua = ua[:160] + "…"
	}
	if uid != loginUID {
		u.Warn("【IM路由】拒绝：path uid 与登录 uid 不一致",
			zap.String("path_uid", uid),
			zap.String("login_uid", loginUID),
			zap.String("client_ip", c.ClientIP()),
			zap.String("ua", ua))
		c.ResponseError(errors.New("无权获取该用户IM信息"))
		return
	}
	sessionToken := readRequestAPIToken(c)
	if strings.TrimSpace(sessionToken) == "" {
		u.Warn("【IM路由】拒绝：未解析到会话 token（请检查 Header 是否带 token）",
			zap.String("uid", uid),
			zap.String("client_ip", c.ClientIP()),
			zap.String("ua", ua))
		c.ResponseError(errors.New("token无效"))
		return
	}
	u.Info("【IM路由】开始：拉 IM 节点并将当前会话 token 同步到悟空",
		zap.String("uid", uid),
		zap.String("client_ip", c.ClientIP()),
		zap.String("ua", ua),
		zap.Int("token_len", len(sessionToken)))
	// 悟空 IM 以「业务侧下发到 IM 的 token」校验长连接；若仅 HTTP 侧轮换/续期而未同步，会出现 expectToken≠actToken，全员表现为断连/连接失败。
	for _, df := range []config.DeviceFlag{config.APP, config.Web, config.PC} {
		imResp, err := u.ctx.UpdateIMToken(config.UpdateIMTokenReq{
			UID:         uid,
			Token:       sessionToken,
			DeviceFlag:  df,
			DeviceLevel: config.DeviceLevelSlave,
		})
		if err != nil {
			u.Error("同步IM token失败", zap.Error(err), zap.String("uid", uid), zap.Uint8("device_flag", uint8(df)))
			c.ResponseError(errors.New("同步IM连接凭证失败，请稍后重试"))
			return
		}
		if imResp != nil && imResp.Status == config.UpdateTokenStatusBan {
			c.ResponseError(errors.New("此账号已经被封禁！"))
			return
		}
	}

	resp, err := network.Get(fmt.Sprintf("%s/route?uid=%s", u.ctx.GetConfig().WuKongIM.APIURL, uid), nil, nil)
	if err != nil {
		u.Error("调用IM服务失败！", zap.Error(err))
		c.ResponseError(errors.New("调用IM服务失败！"))
		return
	}
	var resultMap map[string]interface{}
	err = util.ReadJsonByByte([]byte(resp.Body), &resultMap)
	if err != nil {
		u.Error("【IM路由】解析悟空 route 响应失败", zap.Error(err), zap.String("uid", uid))
		c.ResponseError(err)
		return
	}
	wssPreview := ""
	if v, ok := resultMap["wss_addr"].(string); ok && v != "" {
		wssPreview = v
	} else if v, ok := resultMap["ws_addr"].(string); ok && v != "" {
		wssPreview = v
	}
	if len(wssPreview) > 80 {
		wssPreview = wssPreview[:80] + "…"
	}
	u.Info("【IM路由】成功：已同步 token 并返回悟空 route",
		zap.String("uid", uid),
		zap.Int("im_http_status", resp.StatusCode),
		zap.String("addr_preview", wssPreview))
	c.JSON(resp.StatusCode, resultMap)
}

func (u *User) qrcodeMy(c *wkhttp.Context) {
	userModel, err := u.db.QueryByUID(c.GetLoginUID())
	if err != nil {
		c.ResponseErrorf("查询当前用户信息失败！", err)
		return
	}
	if userModel == nil {
		c.ResponseError(errors.New("登录用户不存在！"))
		return
	}
	if userModel.QRVercode == "" {
		c.ResponseError(errors.New("用户没有QRVercode，非法操作！"))
		return
	}
	path := strings.ReplaceAll(u.ctx.GetConfig().QRCodeInfoURL, ":code", fmt.Sprintf("vercode_%s", userModel.QRVercode))
	data := wkutil.FullBaseURL(u.ctx.GetConfig().External.BaseURL, path)
	c.Response(gin.H{
		"data": data,
	})
}

// 修改用户信息
func (u *User) userUpdateWithField(c *wkhttp.Context) {
	loginUID := c.GetLoginUID()

	var reqMap map[string]interface{}
	if err := c.BindJSON(&reqMap); err != nil {
		u.Error("数据格式有误！", zap.Error(err))
		c.ResponseError(errors.New("数据格式有误！"))
		return
	}
	// 查询用户信息
	users, err := u.db.QueryByUID(loginUID)
	if err != nil {
		c.ResponseError(errors.New("查询用户信息出错！"))
		return
	}
	if users == nil {
		c.ResponseError(errors.New("用户信息不存在！"))
		return
	}

	for key, value := range reqMap {
		//是否允许更新此field
		if !allowUpdateUserField(key) {
			c.ResponseError(errors.New("不允许更新【" + key + "】"))
			return
		}
		if key == "short_no" {
			if u.ctx.GetConfig().ShortNo.EditOff {
				c.ResponseError(errors.New("不允许编辑！"))
				return
			}
			if users.ShortStatus == 1 {
				c.ResponseError(errors.New("用户短编号只能修改一次"))
				return
			}
			if len(fmt.Sprintf("%s", value)) < 6 || len(fmt.Sprintf("%s", value)) > 20 {
				c.ResponseError(errors.New("短号须以字母开头，仅支持使用6～20个字母、数字、下划线、减号自由组合"))
				return
			}
			isLetter := true
			isIncludeNum := false
			for index, r := range fmt.Sprintf("%s", value) {
				if !unicode.IsLetter(r) && index == 0 {
					isLetter = false
					break
				}
				if unicode.Is(unicode.Han, r) {
					isLetter = false
					break
				}
				if unicode.IsDigit(r) {
					isIncludeNum = true
				}
				if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' {
					isLetter = false
					break
				}
			}
			if !isLetter || !isIncludeNum {
				c.ResponseError(errors.New("短号须以字母开头，仅支持使用6～20个字母、数字、下划线、减号自由组合"))
				return
			}
			users, err = u.db.QueryUserWithOnlyShortNo(fmt.Sprintf("%s", value))
			if err != nil {
				u.Error("通过short_no查询用户失败！", zap.Error(err), zap.String("shortNo", key))
				c.ResponseError(errors.New("通过short_no查询用户失败！"))
				return
			}
			if users != nil {
				c.ResponseError(errors.New("已存在，请换一个！"))
				return
			}

			tx, err := u.db.session.Begin()
			if err != nil {
				u.Error("创建事务失败！", zap.Error(err))
				c.ResponseError(errors.New("创建事务失败！"))
				return
			}
			defer func() {
				if err := recover(); err != nil {
					tx.Rollback()
					panic(err)
				}
			}()
			err = u.db.UpdateUsersWithField(key, fmt.Sprintf("%s", value), loginUID)
			if err != nil {
				c.ResponseError(errors.New("修改用户资料失败"))
				tx.Rollback()
				return
			}
			err = u.db.UpdateUsersWithField("short_status", "1", loginUID)
			if err != nil {
				u.Error("修改用户资料失败", zap.Error(err), zap.Any(key, value))
				c.ResponseError(errors.New("修改用户资料失败"))
				tx.Rollback()
				return
			}
			err = tx.Commit()
			if err != nil {
				u.Error("数据库事物提交失败", zap.Error(err))
				c.ResponseError(errors.New("数据库事物提交失败"))
				tx.Rollback()
				return
			}
			c.ResponseOK()
			return
		}
		//修改用户信息
		if key == "name" && value != nil && value.(string) == "" { // 修改名字
			c.ResponseError(errors.New("名字不能为空！"))
			return
		}

		err = u.db.UpdateUsersWithField(key, fmt.Sprintf("%s", value), loginUID)
		if err != nil {
			u.Error("修改用户资料失败", zap.Error(err))
			c.ResponseError(errors.New("修改用户资料失败"))
			return
		}
		if key == "name" {
			// 将重新设置token设置到缓存（这里主要是更新登录者的name）
			err = u.ctx.Cache().Set(u.ctx.GetConfig().Cache.TokenCachePrefix+c.GetHeader("token"), fmt.Sprintf("%s@%s@%s", loginUID, value, c.GetLoginRole()))
			if err != nil {
				u.Error("重新设置token缓存失败！", zap.Error(err))
				c.ResponseError(errors.New("重新设置token缓存失败！"))
				return
			}
		}
	}
	// 发送频道刚刚消息给登录好友
	friends, err := u.friendDB.QueryFriends(loginUID)
	if err != nil {
		u.Error("查询用户好友错误", zap.Error(err))
		c.ResponseError(errors.New("查询用户好友错误"))
		return
	}
	if len(friends) > 0 {
		uids := make([]string, 0)
		for _, friend := range friends {
			uids = append(uids, friend.ToUID)
		}
		err = u.ctx.SendCMD(config.MsgCMDReq{
			CMD:         common.CMDChannelUpdate,
			Subscribers: uids,
			Param: map[string]interface{}{
				"channel_id":   loginUID,
				"channel_type": common.ChannelTypePerson,
			},
		})
		if err != nil {
			u.Error("发送频道更改消息错误！", zap.Error(err))
			c.ResponseError(errors.New("发送频道更改消息错误！"))
			return
		}
	}

	c.ResponseOK()
}

func (u *User) userUpdateSetting(c *wkhttp.Context) {
	loginUID := c.GetLoginUID()

	var reqMap map[string]interface{}
	if err := c.BindJSON(&reqMap); err != nil {
		u.Error("数据格式有误！", zap.Error(err))
		c.ResponseError(errors.New("数据格式有误！"))
		return
	}
	// 查询用户信息
	users, err := u.db.QueryByUID(loginUID)
	if err != nil {
		c.ResponseError(errors.New("查询用户信息出错！"))
		return
	}
	if users == nil {
		c.ResponseError(errors.New("用户信息不存在！"))
		return
	}
	for key, value := range reqMap {
		if key == "device_lock" ||
			key == "search_by_phone" ||
			key == "search_by_short" ||
			key == "new_msg_notice" ||
			key == "msg_show_detail" ||
			key == "offline_protection" ||
			key == "voice_on" ||
			key == "shock_on" ||
			key == "mute_of_app" {
			if key == "device_lock" && fmt.Sprintf("%v", value) == "1" {
				if users.Phone == "15900000002" || users.Phone == "15900000003" || users.Phone == "15900000004" || users.Phone == "15900000005" || users.Phone == "15900000006" {
					c.ResponseError(errors.New("演示账号不支持开启设备锁"))
					return
				}

			}
			err = u.db.UpdateUsersWithField(key, fmt.Sprintf("%v", value), loginUID)
			if err != nil {
				u.Error("修改用户资料失败", zap.Error(err))
				c.ResponseError(errors.New("修改用户资料失败"))
				return
			}
		}
	}
	c.ResponseOK()
}

// 获取用户详情
func (u *User) get(c *wkhttp.Context) {
	uid := c.Param("uid")
	groupNo := c.Query("group_no")
	loginUID := c.MustGet("uid").(string)

	if u.ctx.GetConfig().IsVisitor(uid) { // 访客频道
		c.Request.URL.Path = fmt.Sprintf("/v1/hotline/visitors/%s/im", uid)
		u.ctx.GetHttpRoute().HandleContext(c)
		return
	}

	userDetailResp, err := u.userService.GetUserDetail(uid, loginUID)
	if err != nil {
		u.Error("获取用户详情失败！", zap.Error(err))
		c.ResponseError(errors.New("获取用户详情失败！"))
		return
	}
	if userDetailResp == nil {
		c.ResponseError(errors.New("用户不存在！"))
		return
	}
	isShowShortNo := false
	vercode := ""
	var groupMember *model.GroupMemberResp
	var loginGroupMember *model.GroupMemberResp
	canViewJoinMeta := false
	privilegeM, pErr := u.privilegeDB.queryByUID(loginUID)
	if pErr != nil {
		u.Warn("查询特权用户失败，按非特权处理用户资料字段裁剪", zap.Error(pErr), zap.String("uid", loginUID))
	} else if privilegeM != nil && privilegeM.UID != "" {
		canViewJoinMeta = true
	}
	if groupNo != "" {
		modules := register.GetModules(u.ctx)
		for _, m := range modules {
			if m.BussDataSource.IsShowShortNo != nil && vercode == "" {
				tempShowShortNo, tempVercode, _ := m.BussDataSource.IsShowShortNo(groupNo, uid, loginUID)
				if tempShowShortNo {
					isShowShortNo = tempShowShortNo
					vercode = tempVercode
				}
			}
			if m.BussDataSource.GetGroupMember != nil && groupMember == nil {
				groupMember, _ = m.BussDataSource.GetGroupMember(groupNo, uid)
			}
			if m.BussDataSource.GetGroupMember != nil && loginGroupMember == nil {
				loginGroupMember, _ = m.BussDataSource.GetGroupMember(groupNo, loginUID)
			}
		}
		if !canViewJoinMeta && loginGroupMember != nil && loginGroupMember.Role != int(common.GroupMemberRoleNormal) {
			canViewJoinMeta = true
		}
	}

	if groupMember != nil {
		if canViewJoinMeta && groupMember.InviteUID != "" && groupMember.IsDeleted == 0 {
			inviteJoinGroupUserInfo, err := u.userService.GetUserDetail(groupMember.InviteUID, uid)
			if err != nil {
				u.Error("获取加入群聊邀请用户详情失败！", zap.Error(err))
			}
			if inviteJoinGroupUserInfo != nil {
				var name = inviteJoinGroupUserInfo.Name
				if inviteJoinGroupUserInfo.Remark != "" {
					name = inviteJoinGroupUserInfo.Remark
				}
				userDetailResp.JoinGroupInviteUID = groupMember.InviteUID
				userDetailResp.JoinGroupTime = groupMember.CreatedAt
				userDetailResp.JoinGroupInviteName = name
			}
		}
		inviteUID := ""
		createdAt := ""
		if canViewJoinMeta {
			inviteUID = groupMember.InviteUID
			createdAt = groupMember.CreatedAt
		}
		userDetailResp.GroupMember = &GroupMemberResp{
			UID:                groupMember.UID,
			Name:               groupMember.Name,
			GroupNo:            groupMember.GroupNo,
			Remark:             groupMember.Remark,
			Role:               groupMember.Role,
			Status:             groupMember.Status,
			InviteUID:          inviteUID,
			Robot:              groupMember.Role,
			ForbiddenExpirTime: groupMember.ForbiddenExpirTime,
			CreatedAt:          createdAt,
		}
	}

	if userDetailResp.Follow == 1 || uid == loginUID {
		isShowShortNo = true
	}
	if !isShowShortNo {
		userDetailResp.ShortNo = ""
		userDetailResp.Vercode = ""
	} else {
		if groupNo != "" {
			userDetailResp.Vercode = vercode
		}
	}
	c.Response(userDetailResp)
}

//	获取用户详情
//
//	func (u *User) userConversationInfoGet(c *wkhttp.Context) {
//		uid := c.Param("uid")
//		loginUID := c.MustGet("uid").(string)
//		model, err := u.db.QueryDetailByUID(uid, loginUID)
//		if err != nil {
//			u.Error("查询用户信息失败！", zap.Error(err), zap.String("uid", uid))
//			c.ResponseError(errors.New("查询用户信息失败！"))
//			return
//		}
//		if model == nil {
//			c.ResponseError(errors.New("用户信息不存在！"))
//			return
//		}
//		userDetailResp := newUserDetailResp(model)
//		if uid == loginUID {
//			userDetailResp.Name = u.ctx.GetConfig().FileHelperName
//		}
//		c.Response(userDetailResp)
//	}
//

// 登录
func (u *User) login(c *wkhttp.Context) {

	var req loginReq
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	if err := req.Check(); err != nil {
		c.ResponseError(err)
		return
	}
	loginSpan := u.ctx.Tracer().StartSpan(
		"login",
		opentracing.ChildOf(c.GetSpanContext()),
	)
	loginSpanCtx := u.ctx.Tracer().ContextWithSpan(context.Background(), loginSpan)
	loginSpan.SetTag("username", req.Username)
	defer loginSpan.Finish()

	userInfo, err := u.db.QueryByUsernameCxt(loginSpanCtx, req.Username)
	if err != nil {
		u.Error("查询用户信息失败！", zap.String("username", req.Username))
		c.ResponseError(err)
		return
	}
	if userInfo == nil || userInfo.IsDestroy == 1 {
		c.ResponseError(errors.New("用户不存在"))
		return
	}
	if userInfo.Password == "" {
		c.ResponseError(errors.New("此账号不允许登录"))
		return
	}
	if util.MD5(util.MD5(req.Password)) != userInfo.Password {
		c.ResponseError(errors.New("密码不正确！"))
		return
	}
	u.execLoginAndRespose(userInfo, config.DeviceFlag(req.Flag), req.Device, loginSpanCtx, c)
}

// 验证登录用户信息
func (u *User) execLoginAndRespose(userInfo *Model, flag config.DeviceFlag, device *deviceReq, loginSpanCtx context.Context, c *wkhttp.Context) {

	result, err := u.execLogin(userInfo, flag, device, loginSpanCtx, strings.TrimSpace(c.Request.UserAgent()))
	if err != nil {
		if errors.Is(err, ErrUserNeedVerification) {
			phone := ""
			if len(userInfo.Phone) > 5 {
				phone = fmt.Sprintf("%s******%s", userInfo.Phone[0:3], userInfo.Phone[len(userInfo.Phone)-2:])
			}
			c.ResponseWithStatus(http.StatusBadRequest, map[string]interface{}{
				"status": 110,
				"msg":    "需要验证手机号码！",
				"uid":    userInfo.UID,
				"phone":  phone,
			})
			return
		}
		c.ResponseError(err)
		return
	}

	c.Response(result)

	publicIP := util.GetClientPublicIP(c.Request)
	go u.sentWelcomeMsg(publicIP, userInfo.UID)
}

func (u *User) execLogin(userInfo *Model, flag config.DeviceFlag, device *deviceReq, loginSpanCtx context.Context, userAgent string) (*loginUserDetailResp, error) {
	if userInfo.Status == int(common.UserDisable) {
		return nil, errors.New("该用户已被禁用")
	}
	deviceLevel := config.DeviceLevelSlave
	if flag == config.APP {
		// 允许移动端同账号多端同时在线：不再以 Master 级别踢掉旧端。
		deviceLevel = config.DeviceLevelSlave
	}
	//app登录验证设备锁
	if flag == 0 && userInfo.DeviceLock == 1 {
		if device == nil {
			return nil, errors.New("登录设备信息不能为空！")
		}
		var existDevice bool
		var err error
		existDevice, err = u.deviceDB.existDeviceWithDeviceIDAndUIDCtx(loginSpanCtx, device.DeviceID, userInfo.UID)
		if err != nil {
			u.Error("查询是否存在的设备失败", zap.Error(err))
			return nil, errors.New("查询是否存在的设备失败")
		}
		if existDevice {
			err = u.deviceDB.updateDeviceLastLoginCtx(loginSpanCtx, time.Now().Unix(), device.DeviceID, userInfo.UID)
			if err != nil {
				u.Error("更新用户登录设备失败", zap.Error(err))
				return nil, errors.New("更新用户登录设备失败")
			}
		}
		if !existDevice {
			err := u.ctx.GetRedisConn().SetAndExpire(fmt.Sprintf("%s%s", u.ctx.GetConfig().Cache.LoginDeviceCachePrefix, userInfo.UID), util.ToJson(device), u.ctx.GetConfig().Cache.LoginDeviceCacheExpire)
			if err != nil {
				u.Error("缓存登录设备失败！", zap.Error(err))
				return nil, errors.New("缓存登录设备失败！")
			}
			return nil, ErrUserNeedVerification
		}
	}
	//更新最后一次登录设备信息
	// flag == config.APP &&
	if device != nil {
		err := u.deviceDB.insertOrUpdateDeviceCtx(loginSpanCtx, &deviceModel{
			UID:         userInfo.UID,
			DeviceID:    device.DeviceID,
			DeviceName:  device.DeviceName,
			DeviceModel: device.DeviceModel,
			LastLogin:   time.Now().Unix(),
		})
		if err != nil {
			u.Error("更新用户登录设备失败", zap.Error(err))
			return nil, errors.New("更新用户登录设备失败")
		}

	}
	token := util.GenerUUID()
	// 将token设置到缓存
	tokenSpan, _ := u.ctx.Tracer().StartSpanFromContext(loginSpanCtx, "SetAndExpire")
	tokenSpan.SetTag("key", "token")
	// 获取老的token并清除老token数据
	oldToken, err := u.ctx.Cache().Get(fmt.Sprintf("%s%d%s", u.ctx.GetConfig().Cache.UIDTokenCachePrefix, flag, userInfo.UID))
	if err != nil {
		u.Error("获取旧token错误", zap.Error(err))
		tokenSpan.Finish()
		return nil, errors.New("获取旧token错误")
	}
	if flag != config.APP { // PC暂时不执行删除操作，因为PC可以同时登陆
		if strings.TrimSpace(oldToken) != "" { // 如果是web或pc类设备 因为支持多登所以这里依然使用老token
			token = oldToken
		}
	}

	err = u.ctx.Cache().SetAndExpire(u.ctx.GetConfig().Cache.TokenCachePrefix+token, fmt.Sprintf("%s@%s@%s", userInfo.UID, userInfo.Name, userInfo.Role), u.ctx.GetConfig().Cache.TokenExpire)
	if err != nil {
		u.Error("设置token缓存失败！", zap.Error(err))
		tokenSpan.Finish()
		return nil, errors.New("设置token缓存失败！")
	}
	err = u.ctx.Cache().SetAndExpire(fmt.Sprintf("%s%d%s", u.ctx.GetConfig().Cache.UIDTokenCachePrefix, flag, userInfo.UID), token, u.ctx.GetConfig().Cache.TokenExpire)
	if err != nil {
		u.Error("设置uidtoken缓存失败！", zap.Error(err))
		tokenSpan.Finish()
		return nil, errors.New("设置uidtoken缓存失败！")
	}
	// 桌面端实际连接时可能被识别为 WEB 或 PC；为避免二者标记不一致导致 IM token 校验失败，这里同步写入两种标记。
	if flag != config.APP {
		err = u.ctx.Cache().SetAndExpire(fmt.Sprintf("%s%d%s", u.ctx.GetConfig().Cache.UIDTokenCachePrefix, config.Web, userInfo.UID), token, u.ctx.GetConfig().Cache.TokenExpire)
		if err != nil {
			u.Error("设置web uidtoken缓存失败！", zap.Error(err))
			tokenSpan.Finish()
			return nil, errors.New("设置uidtoken缓存失败！")
		}
		err = u.ctx.Cache().SetAndExpire(fmt.Sprintf("%s%d%s", u.ctx.GetConfig().Cache.UIDTokenCachePrefix, config.PC, userInfo.UID), token, u.ctx.GetConfig().Cache.TokenExpire)
		if err != nil {
			u.Error("设置pc uidtoken缓存失败！", zap.Error(err))
			tokenSpan.Finish()
			return nil, errors.New("设置uidtoken缓存失败！")
		}
	}
	tokenSpan.Finish()

	updateTokenSpan, _ := u.ctx.Tracer().StartSpanFromContext(loginSpanCtx, "UpdateIMToken")

	imTokenReq := config.UpdateIMTokenReq{
		UID:         userInfo.UID,
		Token:       token,
		DeviceFlag:  config.DeviceFlag(flag),
		DeviceLevel: deviceLevel,
	}
	imResp, err := u.ctx.UpdateIMToken(imTokenReq)
	if err != nil {
		u.Error("更新IM的token失败！", zap.Error(err))
		updateTokenSpan.SetTag("err", err)
		updateTokenSpan.Finish()
		return nil, errors.New("更新IM的token失败！")
	}
	if flag != config.APP {
		// 与上面的缓存双写保持一致，确保 WEB/PC 任一标记下都能通过 IM token 校验。
		_, err = u.ctx.UpdateIMToken(config.UpdateIMTokenReq{
			UID:         userInfo.UID,
			Token:       token,
			DeviceFlag:  config.Web,
			DeviceLevel: deviceLevel,
		})
		if err == nil {
			_, err = u.ctx.UpdateIMToken(config.UpdateIMTokenReq{
				UID:         userInfo.UID,
				Token:       token,
				DeviceFlag:  config.PC,
				DeviceLevel: deviceLevel,
			})
		}
		if err != nil {
			u.Error("同步更新WEB/PC的IM token失败！", zap.Error(err))
			updateTokenSpan.SetTag("err", err)
			updateTokenSpan.Finish()
			return nil, errors.New("更新IM的token失败！")
		}
	}
	updateTokenSpan.Finish()

	if imResp.Status == config.UpdateTokenStatusBan {
		return nil, errors.New("此账号已经被封禁！")
	}

	// Web/PC 或手机浏览器 H5（即使误传 flag=APP）仅依赖长连接，不应沿用 App 写入的离线推送设备信息，
	// 否则离线 webhook 会按 IOS 等类型走 APNS/厂商通道并报「不支持的推送设备」。
	u.clearNativePushDeviceForWebOrBrowserH5(userInfo.UID, flag, userAgent)

	return newLoginUserDetailResp(userInfo, token, u.ctx), nil
}

// looksLikeBrowserH5UserAgent 根据 User-Agent 粗略识别桌面/移动浏览器或系统 WebView（H5），
// 用于客户端误将浏览器会话标为 flag=APP 时仍清除原生推送注册。
func looksLikeBrowserH5UserAgent(ua string) bool {
	if ua == "" {
		return false
	}
	l := strings.ToLower(ua)
	// 常见原生 / SDK HTTP 客户端，避免误判为浏览器
	if strings.Contains(l, "okhttp") ||
		strings.Contains(l, "alamofire") ||
		strings.Contains(l, "curl/") ||
		strings.Contains(l, "go-http-client") ||
		strings.Contains(l, "dart/") {
		return false
	}
	if !strings.Contains(l, "mozilla/5.0") {
		return false
	}
	return strings.Contains(l, "chrome/") ||
		strings.Contains(l, "chromium/") ||
		strings.Contains(l, "safari/") ||
		strings.Contains(l, "firefox/") ||
		strings.Contains(l, "edg/") ||
		strings.Contains(l, "crios/") ||
		strings.Contains(l, "fxios/") ||
		strings.Contains(l, "edgios/")
}

// clearNativePushDeviceForWebOrBrowserH5 清除 Redis 中的原生推送注册（与 quit 中一致）。
// 条件：会话为 Web/PC，或 User-Agent 表现为浏览器/H5（含误传 APP 的手机浏览器）。
func (u *User) clearNativePushDeviceForWebOrBrowserH5(uid string, flag config.DeviceFlag, userAgent string) {
	if flag != config.Web && flag != config.PC && !looksLikeBrowserH5UserAgent(userAgent) {
		return
	}
	err := u.ctx.GetRedisConn().Del(fmt.Sprintf("%s%s", u.userDeviceTokenPrefix, uid))
	if err != nil {
		u.Warn("清除用户推送设备缓存失败", zap.String("uid", uid), zap.Error(err))
	}
}

// sendWelcomeMsg 发送欢迎语
func (u *User) sentWelcomeMsg(publicIP, uid string) {
	appconfig, err := u.commonService.GetAppConfig()
	if err != nil {
		u.Error("获取应用配置错误", zap.Error(err))
	}
	if appconfig.SendWelcomeMessageOn == 0 {
		return
	}
	time.Sleep(time.Second * 2)
	//发送登录欢迎消息
	content := u.ctx.GetConfig().WelcomeMessage
	// 只发送欢迎消息，不推送IP信息
	var sentContent string

	if appconfig != nil && appconfig.WelcomeMessage != "" {
		content = appconfig.WelcomeMessage
	}
	if content == "" {
		content = "欢迎使用 AI私域课堂"
	}
	sentContent = content
	err = u.ctx.SendMessage(&config.MsgSendReq{
		FromUID:     u.ctx.GetConfig().Account.SystemUID,
		ChannelID:   uid,
		ChannelType: common.ChannelTypePerson.Uint8(),
		Payload: []byte(util.ToJson(map[string]interface{}{
			"content": sentContent,
			"type":    common.Text,
		})),
		Header: config.MsgHeader{
			RedDot: 1,
		},
	})
	if err != nil {
		u.Error("发送登录消息欢迎消息失败", zap.Error(err))
	}
	//保存登录日志
	u.loginLog.add(uid, publicIP)
}

// 注册
func (u *User) register(c *wkhttp.Context) {
	var req registerReq
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	if err := req.CheckRegister(); err != nil {
		c.ResponseError(err)
		return
	}
	registerInviteCode := req.GetInviteCode()
	u.Info("register request", zap.String("zone", req.Zone), zap.String("phone", req.Phone), zap.Uint8("flag", req.Flag), zap.String("invite_code", registerInviteCode))

	if u.ctx.GetConfig().Register.Off {
		c.ResponseError(errors.New("注册通道暂不开放，请长按标题使用官网上演示账号登录"))
		return
	}
	appConfig, err := u.commonService.GetAppConfig()
	if err != nil {
		u.Error("查询应用设置错误", zap.Error(err))
		c.ResponseError(err)
		return
	}
	var registerInviteOn = 0
	var inviteCodeSystemOn = 1
	if appConfig != nil {
		inviteCodeSystemOn = appConfig.InviteCodeSystemOn
		registerInviteOn = appConfig.RegisterInviteOn
	}
	var invite *model.Invite
	if inviteCodeSystemOn == 1 && registerInviteOn == 1 {
		if registerInviteCode == "" {
			c.ResponseError(errors.New("邀请码不能为空"))
			return
		}
		// 直接查 invite_code 表，避免模块未注册 GetInviteCode 时误判“邀请码不存在”。
		type inviteRow struct {
			UID string `db:"uid"`
		}
		var rows []*inviteRow
		_, err = u.ctx.DB().
			Select("uid").
			From("invite_code").
			Where("invite_code=? and status=1", registerInviteCode).
			Limit(1).
			Load(&rows)
		if err != nil {
			u.Error("查询邀请码失败", zap.String("invite_code", registerInviteCode), zap.Error(err))
			c.ResponseError(errors.New("查询邀请码失败"))
			return
		}
		if len(rows) == 0 || strings.TrimSpace(rows[0].UID) == "" {
			c.ResponseError(errors.New("邀请码不存在"))
			return
		}
		invite = &model.Invite{
			InviteCode: registerInviteCode,
			Uid:        strings.TrimSpace(rows[0].UID),
		}
	}
	registerSpan := u.ctx.Tracer().StartSpan(
		"user.register",
		opentracing.ChildOf(c.GetSpanContext()),
	)
	defer registerSpan.Finish()
	registerSpanCtx := u.ctx.Tracer().ContextWithSpan(context.Background(), registerSpan)

	registerSpan.SetTag("username", fmt.Sprintf("%s%s", req.Zone, req.Phone))
	//验证手机号是否注册
	userInfo, err := u.db.QueryByUsernameCxt(registerSpanCtx, fmt.Sprintf("%s%s", req.Zone, req.Phone))
	if err != nil {
		u.Error("查询用户信息失败！", zap.String("username", req.Phone))
		c.ResponseError(err)
		return
	}
	if userInfo != nil {
		c.ResponseError(errors.New("该用户已存在"))
		return
	}
	//测试模式
	if strings.TrimSpace(u.ctx.GetConfig().SMSCode) != "" {
		if strings.TrimSpace(u.ctx.GetConfig().SMSCode) != req.Code {
			c.ResponseError(errors.New("验证码错误"))
			return
		}
	} else {
		//线上验证短信验证码
		err = u.smsServie.Verify(registerSpanCtx, req.Zone, req.Phone, req.Code, commonapi.CodeTypeRegister)
		if err != nil {
			c.ResponseError(err)
			return
		}
	}
	var model = &createUserModel{
		Sex:        1,
		Name:       req.Name,
		Zone:       req.Zone,
		Phone:      req.Phone,
		Password:   req.Password,
		Flag:       int(req.Flag),
		Device:     req.Device,
		InviteCode: registerInviteCode,
	}
	u.createUser(registerSpanCtx, model, c, invite)
}

// 搜索用户
func (u *User) search(c *wkhttp.Context) {
	keyword := c.Query("keyword")
	useModel, err := u.db.QueryByKeyword(keyword)
	if err != nil {
		u.Error("查询用户信息失败！", zap.Error(err), zap.String("keyword", keyword))
		c.ResponseError(errors.New("查询用户信息失败！"))
		return
	}
	if useModel == nil {
		c.JSON(http.StatusOK, gin.H{
			"exist": 0,
		})
		return
	}
	appconfig, _ := u.commonService.GetAppConfig()

	if keyword == useModel.Phone {
		//关闭了手机号搜索
		if useModel.SearchByPhone == 0 || (appconfig != nil && appconfig.SearchByPhone == 0) || u.ctx.GetConfig().PhoneSearchOff {
			c.JSON(http.StatusOK, gin.H{
				"exist": 0,
			})
			return
		}
	}

	// 仅特权用户可搜索添加好友
	if appconfig != nil && appconfig.PrivilegeOnlyAddFriendOn == 1 {
		loginUID := c.GetLoginUID()
		if strings.TrimSpace(loginUID) == "" {
			c.JSON(http.StatusOK, gin.H{
				"exist": 0,
			})
			return
		}
		privilegeM, pErr := u.privilegeDB.queryByUID(loginUID)
		if pErr != nil {
			u.Error("查询特权用户失败", zap.Error(pErr), zap.String("uid", loginUID))
			c.ResponseError(errors.New("查询特权用户失败"))
			return
		}
		if privilegeM == nil || privilegeM.UID == "" {
			c.JSON(http.StatusOK, gin.H{
				"exist": 0,
			})
			return
		}
	}

	if useModel.SearchByShort == 0 {
		//关闭了短编号搜索
		if strings.EqualFold(keyword, useModel.ShortNo) {
			c.JSON(http.StatusOK, gin.H{
				"exist": 0,
			})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"exist": 1,
		"data":  newUserResp(useModel),
	})
}

// 注册用户设备token
func (u *User) registerUserDeviceToken(c *wkhttp.Context) {
	loginUID := c.MustGet("uid").(string)
	var req struct {
		DeviceToken string `json:"device_token"` // 设备token
		DeviceType  string `json:"device_type"`  // 设备类型 IOS，MI，HMS
		BundleID    string `json:"bundle_id"`    // app的唯一ID标示
	}
	if err := c.BindJSON(&req); err != nil {
		u.Error("数据格式有误！", zap.Error(err))
		c.ResponseError(errors.New("数据格式有误！"))
		return
	}
	if strings.TrimSpace(req.DeviceToken) == "" {
		c.ResponseError(errors.New("设备token不能为空！"))
		return
	}
	if strings.TrimSpace(req.DeviceType) == "" {
		c.ResponseError(errors.New("设备类型不能为空！"))
		return
	}
	if strings.TrimSpace(req.BundleID) == "" {
		c.ResponseError(errors.New("bundleID不能为空！"))
		return
	}
	err := u.ctx.GetRedisConn().Hmset(fmt.Sprintf("%s%s", u.userDeviceTokenPrefix, loginUID), "device_type", req.DeviceType, "device_token", req.DeviceToken, "bundle_id", req.BundleID)
	if err != nil {
		u.Error("存储用户设备token失败！", zap.Error(err))
		c.ResponseError(errors.New("存储用户设备token失败！"))
		return
	}
	c.ResponseOK()
}

// 注册用户设备红点数量
func (u *User) registerUserDeviceBadge(c *wkhttp.Context) {
	loginUID := c.MustGet("uid").(string)
	var req struct {
		Badge int `json:"badge"` // 设备红点数量
	}
	if err := c.BindJSON(&req); err != nil {
		u.Error("数据格式有误！", zap.Error(err))
		c.ResponseError(errors.New("数据格式有误！"))
		return
	}
	err := u.setUserBadge(loginUID, int64(req.Badge))
	if err != nil {
		u.Error("存储用户红点失败！", zap.Error(err))
		c.ResponseError(errors.New("存储用户红点失败！"))
		return
	}
	c.ResponseOK()
}

func (u *User) setUserBadge(uid string, badge int64) error {
	err := u.ctx.GetRedisConn().Hset(common.UserDeviceBadgePrefix, uid, fmt.Sprintf("%d", badge))
	if err != nil {
		return err
	}
	return nil
}

// 卸载注册设备token
func (u *User) unregisterUserDeviceToken(c *wkhttp.Context) {
	loginUID := c.MustGet("uid").(string)

	err := u.ctx.GetRedisConn().Del(fmt.Sprintf("%s%s", u.userDeviceTokenPrefix, loginUID))
	if err != nil {
		u.Error("删除设备token失败！", zap.Error(err))
		c.ResponseError(errors.New("删除设备token失败！"))
		return
	}
	c.ResponseOK()
}

// 获取登录的uuid（web登录）
func (u *User) getLoginUUID(c *wkhttp.Context) {
	uuid := util.GenerUUID()
	deviceId := c.Query("device_id")
	deviceName := c.Query("device_name")
	deviceModel := c.Query("device_model")
	err := u.ctx.GetRedisConn().SetAndExpire(fmt.Sprintf("%s%s", common.QRCodeCachePrefix, uuid), util.ToJson(common.NewQRCodeModel(common.QRCodeTypeScanLogin, map[string]interface{}{
		"app_id":  "wukongchat",
		"status":  common.ScanLoginStatusWaitScan,
		"pub_key": c.Query("pub_key"),
	})), time.Minute*1)
	if err != nil {
		u.Error("设置登录uuid失败！", zap.Error(err))
		c.ResponseError(errors.New("设置登录uuid失败！"))
		return
	}
	// 缓存设备信息
	if deviceId != "" && deviceName != "" && deviceModel != "" {
		err := u.ctx.GetRedisConn().SetAndExpire(fmt.Sprintf("%s%s", common.DeviceCacheUUIDPrefix, uuid), util.ToJson(map[string]interface{}{
			"device_id":    deviceId,
			"device_name":  deviceName,
			"device_model": deviceModel,
		}), time.Minute*2)
		if err != nil {
			u.Error("设置登录设备信息失败！", zap.Error(err))
			c.ResponseError(errors.New("设置登录设备信息失败！"))
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"uuid":   uuid,
		"qrcode": wkutil.FullBaseURL(u.ctx.GetConfig().External.BaseURL, strings.ReplaceAll(u.ctx.GetConfig().QRCodeInfoURL, ":code", uuid)),
	})
}

// 通过loginUUID获取登录状态
func (u *User) getloginStatus(c *wkhttp.Context) {
	uuid := c.Query("uuid")
	qrcodeInfo, err := u.ctx.GetRedisConn().GetString(fmt.Sprintf("%s%s", common.QRCodeCachePrefix, uuid))
	if err != nil {
		u.Error("获取uuid绑定的二维码信息失败！", zap.Error(err))
		c.ResponseError(errors.New("获取uuid绑定的二维码信息失败！"))
		return
	}
	if qrcodeInfo == "" {
		c.JSON(http.StatusOK, gin.H{
			"status": common.ScanLoginStatusExpired,
		})
		return
	}
	var qrcodeModel *common.QRCodeModel
	err = util.ReadJsonByByte([]byte(qrcodeInfo), &qrcodeModel)
	if err != nil {
		u.Error("解码二维码信息失败！", zap.Error(err))
		c.ResponseError(errors.New("解码二维码信息失败！"))
		return
	}
	if qrcodeModel == nil {
		c.JSON(http.StatusOK, gin.H{
			"status": common.ScanLoginStatusExpired,
		})
		return
	}
	qrcodeChan := u.getQRCodeModelChan(uuid)
	select {
	case qrcodeModel := <-qrcodeChan:
		u.removeQRCodeChan(uuid)
		if qrcodeModel == nil {
			break
		}
		c.JSON(http.StatusOK, qrcodeModel.Data)
		break
	case <-time.After(10 * time.Second):
		u.removeQRCodeChan(uuid)
		c.JSON(http.StatusOK, qrcodeModel.Data)
		break

	}
}

// 通过authCode登录
func (u *User) loginWithAuthCode(c *wkhttp.Context) {
	authCode := c.Param("auth_code")
	authCodeKey := fmt.Sprintf("%s%s", common.AuthCodeCachePrefix, authCode)
	flagI64, _ := strconv.ParseInt(c.Query("flag"), 10, 64)
	var flag config.DeviceFlag
	if flagI64 == 0 {
		flag = config.Web // loginWithAuthCode 默认为web登陆
	} else {
		flag = config.DeviceFlag(flag)
	}
	authInfo, err := u.ctx.GetRedisConn().GetString(authCodeKey)
	if err != nil {
		u.Error("获取授权信息失败！", zap.Error(err))
		c.ResponseError(errors.New("获取授权信息失败！"))
		return
	}
	if authInfo == "" {
		c.ResponseError(errors.New("授权码失效或不存在！"))
		return
	}
	var authInfoMap map[string]interface{}
	err = util.ReadJsonByByte([]byte(authInfo), &authInfoMap)
	if err != nil {
		u.Error("解码授权信息失败！", zap.Error(err))
		c.ResponseError(errors.New("解码授权信息失败！"))
		return
	}
	authType := authInfoMap["type"].(string)
	if authType != string(common.AuthCodeTypeScanLogin) {
		c.ResponseError(errors.New("授权码不是登录授权码！"))
		return
	}
	scaner := authInfoMap["scaner"].(string)
	// 获取老的token
	token, err := u.ctx.Cache().Get(fmt.Sprintf("%s%d%s", u.ctx.GetConfig().Cache.UIDTokenCachePrefix, flag, scaner))
	if err != nil {
		u.Error("获取旧token错误", zap.Error(err))
		c.ResponseError(errors.New("获取旧token错误"))
		return
	}
	if strings.TrimSpace(token) == "" {
		token = util.GenerUUID()
	}

	userModel, err := u.db.QueryByUID(scaner)
	if err != nil {
		u.Error("用户不存在！", zap.String("uid", scaner), zap.Error(err))
		c.ResponseError(errors.New("用户不存在！"))
		return
	}
	// 获取缓存设备
	uuid := authInfoMap["uuid"].(string)
	if uuid != "" {
		deviceCache, err := u.ctx.GetRedisConn().GetString(fmt.Sprintf("%s%s", common.DeviceCacheUUIDPrefix, uuid))
		if err != nil {
			u.Error("获取登录设备信息失败！", zap.Error(err))
			c.ResponseError(errors.New("获取登录设备信息失败！"))
			return
		}
		if deviceCache != "" {
			var deviceInfoMap map[string]interface{}
			err = util.ReadJsonByByte([]byte(deviceCache), &deviceInfoMap)
			if err != nil {
				u.Error("解码设备信息失败！", zap.Error(err))
				c.ResponseError(errors.New("解码设备信息失败！"))
				return
			}
			deviceId := deviceInfoMap["device_id"].(string)
			deviceName := deviceInfoMap["device_name"].(string)
			dmodel := deviceInfoMap["device_model"].(string)
			if deviceId != "" && deviceName != "" && dmodel != "" {
				span := u.ctx.Tracer().StartSpan(
					"user.authCodeLogin",
					opentracing.ChildOf(c.GetSpanContext()),
				)
				defer span.Finish()
				spanCtx := u.ctx.Tracer().ContextWithSpan(context.Background(), span)
				// 更新设备信息
				err := u.deviceDB.insertOrUpdateDeviceCtx(spanCtx, &deviceModel{
					UID:         userModel.UID,
					DeviceID:    deviceId,
					DeviceName:  deviceName,
					DeviceModel: dmodel,
					LastLogin:   time.Now().Unix(),
				})
				if err != nil {
					u.Error("更新用户登录设备失败", zap.Error(err))
					c.ResponseError(errors.New("更新用户登录设备失败"))
					return
				}
			}
		}
	}
	imResp, err := u.ctx.UpdateIMToken(config.UpdateIMTokenReq{
		UID:         scaner,
		Token:       token,
		DeviceFlag:  flag,
		DeviceLevel: config.DeviceLevelSlave,
	})
	if err != nil {
		u.Error("更新IM的token失败！", zap.Error(err))
		c.ResponseError(errors.New("更新IM的token失败！"))
		return
	}
	if imResp.Status == config.UpdateTokenStatusBan {
		c.ResponseError(errors.New("此账号已经被封禁！"))
		return
	}

	// 将token设置到缓存
	err = u.ctx.Cache().SetAndExpire(u.ctx.GetConfig().Cache.TokenCachePrefix+token, fmt.Sprintf("%s@%s", userModel.UID, userModel.Name), u.ctx.GetConfig().Cache.TokenExpire)
	if err != nil {
		u.Error("设置token缓存失败！", zap.Error(err))
		c.ResponseError(errors.New("设置token缓存失败！"))
		return
	}
	err = u.ctx.GetRedisConn().Del(authCodeKey)
	if err != nil {
		u.Error("删除授权码失败！", zap.Error(err))
		c.ResponseError(errors.New("删除授权码失败！"))
		return
	}

	err = u.ctx.Cache().SetAndExpire(fmt.Sprintf("%s%d%s", u.ctx.GetConfig().Cache.UIDTokenCachePrefix, flag, userModel.UID), token, u.ctx.GetConfig().Cache.TokenExpire)
	if err != nil {
		u.Error("设置uidtoken缓存失败！", zap.Error(err))
		c.ResponseError(errors.New("设置uidtoken缓存失败！"))
		return
	}

	u.clearNativePushDeviceForWebOrBrowserH5(userModel.UID, flag, strings.TrimSpace(c.Request.UserAgent()))

	c.Response(map[string]interface{}{
		"app_id":     userModel.AppID,
		"name":       userModel.Name,
		"username":   userModel.Username,
		"uid":        userModel.UID,
		"token":      token,
		"short_no":   userModel.ShortNo,
		"avatar":     wkutil.AvatarAPIRelativePath(userModel.UID),
		"im_pub_key": "",
	})
}

// 获取二维码数据的管道
func (u *User) getQRCodeModelChan(uuid string) <-chan *common.QRCodeModel {
	qrcodeModelChan := make(chan *common.QRCodeModel)
	qrcodeChanLock.Lock()
	qrcodeChanMap[uuid] = qrcodeModelChan
	qrcodeChanLock.Unlock()
	return qrcodeModelChan
}
func (u *User) removeQRCodeChan(uuid string) {
	qrcodeChanLock.Lock()
	defer qrcodeChanLock.Unlock()
	_, exist := qrcodeChanMap[uuid]
	if exist {
		delete(qrcodeChanMap, uuid)
	}
}

// SendQRCodeInfo 发送二维码数据
func SendQRCodeInfo(uuid string, qrcode *common.QRCodeModel) {
	qrcodeChanLock.Lock()
	qrcodeChan := qrcodeChanMap[uuid]
	qrcodeChanLock.Unlock()
	if qrcodeChan != nil {
		qrcodeChan <- qrcode
	}
}

// 授权登录
func (u *User) grantLogin(c *wkhttp.Context) {
	authCode := c.Query("auth_code")
	loginUID := c.MustGet("uid").(string)
	encrypt := c.Query("encrypt") // signal相关密钥
	if authCode == "" {
		c.ResponseError(errors.New("授权码不能为空！"))
		return
	}
	authInfo, err := u.ctx.GetRedisConn().GetString(fmt.Sprintf("%s%s", common.AuthCodeCachePrefix, authCode))
	if err != nil {
		u.Error("获取授权信息失败！", zap.Error(err))
		c.ResponseError(errors.New("获取授权信息失败！"))
		return
	}
	if authInfo == "" {
		c.ResponseError(errors.New("授权码失效或不存在！"))
		return
	}
	var authInfoMap map[string]interface{}
	err = util.ReadJsonByByte([]byte(authInfo), &authInfoMap)
	if err != nil {
		u.Error("解码授权信息失败！", zap.Error(err))
		c.ResponseError(errors.New("解码授权信息失败！"))
		return
	}
	authType := authInfoMap["type"].(string)
	if authType != string(common.AuthCodeTypeScanLogin) {
		c.ResponseError(errors.New("授权码不是登录授权码！"))
		return
	}
	scaner := authInfoMap["scaner"].(string)
	if scaner != loginUID {
		c.ResponseError(errors.New("扫描者与授权者不是同一个用户！"))
		return
	}
	uuid := authInfoMap["uuid"].(string)
	qrcodeInfo := common.NewQRCodeModel(common.QRCodeTypeScanLogin, map[string]interface{}{
		"app_id":    "wukongchat",
		"status":    common.ScanLoginStatusAuthed,
		"uid":       loginUID,
		"auth_code": authCode,
		"encrypt":   encrypt,
	})
	err = u.ctx.GetRedisConn().SetAndExpire(fmt.Sprintf("%s%s", common.QRCodeCachePrefix, uuid), util.ToJson(qrcodeInfo), time.Minute*5)
	if err != nil {
		u.Error("更新二维码信息失败！", zap.Error(err))
		c.ResponseError(errors.New("更新二维码信息失败！"))
		return
	}
	SendQRCodeInfo(uuid, qrcodeInfo)
	c.ResponseOK()
}

// addBlacklist 添加黑名单
func (u *User) addBlacklist(c *wkhttp.Context) {
	loginUID := c.MustGet("uid").(string)
	uid := c.Param("uid")
	if strings.TrimSpace(uid) == "" {
		c.ResponseError(errors.New("添加黑名单的用户ID不能空！"))
		return
	}
	model, err := u.settingDB.QueryUserSettingModel(uid, loginUID)
	if err != nil {
		u.Error("查询用户设置失败", zap.Error(err))
		c.ResponseError(errors.New("查询用户设置失败！"))
		return
	}
	//如果没有设置记录先添加一条记录
	if model == nil || strings.TrimSpace(model.UID) == "" {
		userSettingModel := &SettingModel{
			UID:   loginUID,
			ToUID: uid,
		}
		err = u.settingDB.InsertUserSettingModel(userSettingModel)
		if err != nil {
			u.Error("添加用户设置失败", zap.Error(err))
			c.ResponseError(errors.New("添加用户设置失败！"))
			return
		}
	}

	// 请求im服务器设置黑名单
	err = u.ctx.IMBlacklistAdd(config.ChannelBlacklistReq{
		ChannelReq: config.ChannelReq{
			ChannelID:   loginUID,
			ChannelType: common.ChannelTypePerson.Uint8(),
		},
		UIDs: []string{uid},
	})
	if err != nil {
		u.Error("设置黑名单失败！", zap.Error(err))
		c.ResponseError(errors.New("设置黑名单失败！"))
		return
	}
	//添加黑名单
	version := u.ctx.GenSeq(common.UserSettingSeqKey)
	friendVersion := u.ctx.GenSeq(common.FriendSeqKey)
	tx, err := u.ctx.DB().Begin()
	if err != nil {
		u.Error("开启事务失败！", zap.Error(err))
		c.ResponseError(errors.New("开启事务失败！"))
		return
	}
	defer func() {
		if err := recover(); err != nil {
			tx.Rollback()
			panic(err)
		}
	}()
	err = u.db.AddOrRemoveBlacklistTx(loginUID, uid, 1, version, tx)
	if err != nil {
		tx.Rollback()
		u.Error("添加黑名单失败！", zap.Error(err))
		c.ResponseError(errors.New("添加黑名单失败！"))
		return
	}
	err = u.friendDB.updateVersionTx(friendVersion, loginUID, uid, tx)
	if err != nil {
		tx.Rollback()
		u.Error("更新好友的版本号失败！", zap.Error(err))
		c.ResponseError(errors.New("更新好友的版本号失败！"))
		return
	}
	if err := tx.Commit(); err != nil {
		tx.Rollback()
		u.Error("提交数据库失败！", zap.Error(err))
		c.ResponseError(errors.New("提交数据库失败！"))
		return
	}

	// 发送给被拉黑的人去更新拉黑人的频道
	err = u.ctx.SendChannelUpdate(config.ChannelReq{
		ChannelID:   uid,
		ChannelType: common.ChannelTypePerson.Uint8(),
	}, config.ChannelReq{
		ChannelID:   loginUID,
		ChannelType: common.ChannelTypePerson.Uint8(),
	})
	if err != nil {
		u.Warn("发送频道更新命令失败！", zap.Error(err))
	}

	// 发送给操作者，去更新被拉黑的人的频道
	err = u.ctx.SendChannelUpdate(config.ChannelReq{
		ChannelID:   loginUID,
		ChannelType: common.ChannelTypePerson.Uint8(),
	}, config.ChannelReq{
		ChannelID:   uid,
		ChannelType: common.ChannelTypePerson.Uint8(),
	})
	if err != nil {
		u.Warn("发送频道更新命令失败！", zap.Error(err))
	}

	c.ResponseOK()
}

// removeBlacklist 移除黑名单
func (u *User) removeBlacklist(c *wkhttp.Context) {
	loginUID := c.MustGet("uid").(string)
	uid := c.Param("uid")
	if strings.TrimSpace(uid) == "" {
		c.ResponseError(errors.New("移除黑名单的用户ID不能空！"))
		return
	}

	version := u.ctx.GenSeq(common.UserSettingSeqKey)
	friendVersion := u.ctx.GenSeq(common.FriendSeqKey)

	tx, err := u.ctx.DB().Begin()
	if err != nil {
		u.Error("开启事务失败！", zap.Error(err))
		c.ResponseError(errors.New("开启事务失败！"))
		return
	}
	defer func() {
		if err := recover(); err != nil {
			tx.Rollback()
			panic(err)
		}
	}()
	err = u.db.AddOrRemoveBlacklistTx(loginUID, uid, 0, version, tx)
	if err != nil {
		tx.Rollback()
		u.Error("移除黑名单失败！", zap.Error(err))
		c.ResponseError(errors.New("移除黑名单失败！"))
		return
	}
	err = u.friendDB.updateVersionTx(friendVersion, loginUID, uid, tx)
	if err != nil {
		tx.Rollback()
		u.Error("更新好友的版本号失败！", zap.Error(err))
		c.ResponseError(errors.New("更新好友的版本号失败！"))
		return
	}
	if err := tx.Commit(); err != nil {
		tx.Rollback()
		u.Error("提交数据库失败！", zap.Error(err))
		c.ResponseError(errors.New("提交数据库失败！"))
		return
	}

	// 请求im服务器移除黑名单
	err = u.ctx.IMBlacklistRemove(config.ChannelBlacklistReq{
		ChannelReq: config.ChannelReq{
			ChannelID:   loginUID,
			ChannelType: common.ChannelTypePerson.Uint8(),
		},
		UIDs: []string{uid},
	})
	if err != nil {
		u.Error("设置黑名单失败！", zap.Error(err))
		c.ResponseError(errors.New("设置黑名单失败！"))
		return
	}

	// 发送给被拉黑的人去更新拉黑人的频道
	err = u.ctx.SendChannelUpdate(config.ChannelReq{
		ChannelID:   uid,
		ChannelType: common.ChannelTypePerson.Uint8(),
	}, config.ChannelReq{
		ChannelID:   loginUID,
		ChannelType: common.ChannelTypePerson.Uint8(),
	})
	if err != nil {
		u.Warn("发送频道更新命令失败！", zap.Error(err))
	}

	// 发送给操作者，去更新被拉黑的人的频道
	err = u.ctx.SendChannelUpdate(config.ChannelReq{
		ChannelID:   loginUID,
		ChannelType: common.ChannelTypePerson.Uint8(),
	}, config.ChannelReq{
		ChannelID:   uid,
		ChannelType: common.ChannelTypePerson.Uint8(),
	})
	if err != nil {
		u.Warn("发送频道更新命令失败！", zap.Error(err))
	}

	c.ResponseOK()
}

// blacklists 获取黑名单列表
func (u *User) blacklists(c *wkhttp.Context) {
	loginUID := c.MustGet("uid").(string)
	list, err := u.db.Blacklists(loginUID)
	if err != nil {
		u.Error("查询黑名单列表失败！", zap.Error(err))
		c.ResponseError(errors.New("查询黑名单列表失败！"))
		return
	}
	blacklists := []*blacklistResp{}
	for _, result := range list {
		blacklists = append(blacklists, &blacklistResp{
			UID:      result.UID,
			Name:     result.Name,
			Username: result.Username,
		})
	}
	c.Response(blacklists)
}

// sendRegisterCode 发送注册短信
func (u *User) sendRegisterCode(c *wkhttp.Context) {
	if u.ctx.GetConfig().Register.Off {
		c.ResponseError(errors.New("注册通道暂不开放，请长按标题使用官网上演示账号登录"))
		return
	}
	var req codeReq
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	if strings.TrimSpace(req.Zone) == "" {
		c.ResponseError(errors.New("区号不能为空！"))
		return
	}
	if strings.TrimSpace(req.Phone) == "" {
		c.ResponseError(errors.New("手机号不能为空！"))
		return
	}
	if u.ctx.GetConfig().Register.OnlyChina {
		if strings.TrimSpace(req.Zone) != "0086" {
			c.ResponseError(errors.New("仅仅支持中国大陆手机号注册！"))
			return
		}
	}

	span := u.ctx.Tracer().StartSpan(
		"user.sendRegisterCode",
		opentracing.ChildOf(c.GetSpanContext()),
	)
	defer span.Finish()
	spanCtx := u.ctx.Tracer().ContextWithSpan(context.Background(), span)

	model, err := u.db.QueryByPhone(req.Zone, req.Phone)
	if err != nil {
		u.Error("查询用户信息失败！", zap.Error(err))
		c.ResponseError(errors.New("查询用户信息失败！"))
		return
	}
	if model != nil {
		c.Response(map[string]interface{}{
			"exist": 1,
		})
		return
	}
	err = u.smsServie.SendVerifyCode(spanCtx, req.Zone, req.Phone, commonapi.CodeTypeRegister)
	if err != nil {
		u.Error("发送短信验证码失败", zap.Error(err))
		c.ResponseError(errors.New("发送短信验证码失败！"))
		return
	}
	c.Response(map[string]interface{}{
		"exist": 0,
	})
}

// setChatPwd 修改用户聊天密码
func (u *User) setChatPwd(c *wkhttp.Context) {
	var req chatPwdReq
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	if strings.TrimSpace(req.ChatPwd) == "" {
		c.ResponseError(errors.New("聊天密码不能为空"))
		return
	}
	if strings.TrimSpace(req.LoginPwd) == "" {
		c.ResponseError(errors.New("登录密码不能为空！"))
		return
	}
	loginUID := c.MustGet("uid").(string)
	user, err := u.db.QueryByUID(loginUID)
	if err != nil {
		u.Error("查询用户信息失败！", zap.Error(err))
		c.ResponseError(errors.New("查询用户信息失败"))
		return
	}
	if user.Password != util.MD5(util.MD5(req.LoginPwd)) {
		c.ResponseError(errors.New("登录密码错误"))
		return
	}
	//修改用户聊天密码
	err = u.db.UpdateUsersWithField("chat_pwd", req.ChatPwd, loginUID)
	if err != nil {
		u.Error("查询用户信息失败！", zap.Error(err))
		c.ResponseError(errors.New("修改聊天密码失败"))
		return
	}
	c.ResponseOK()
}

// 设置锁屏密码
func (u *User) lockScreenAfterMinuteSet(c *wkhttp.Context) {
	var req struct {
		LockAfterMinute int `json:"lock_after_minute"` // 在几分钟后锁屏
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	if req.LockAfterMinute < 0 {
		c.ResponseError(errors.New("锁屏时间不能小于0"))
		return
	}
	if req.LockAfterMinute > 60 {
		c.ResponseError(errors.New("锁屏时间不能大于60分钟"))
		return
	}
	loginUID := c.GetLoginUID()
	err := u.db.UpdateUsersWithField("lock_after_minute", strconv.FormatInt(int64(req.LockAfterMinute), 10), loginUID)
	if err != nil {
		u.Error("修改用户锁屏密码错误", zap.Error(err))
		c.ResponseError(errors.New("修改用户锁屏密码错误"))
		return
	}
	c.ResponseOK()
}

// 设置锁屏密码
func (u *User) setLockScreenPwd(c *wkhttp.Context) {
	var req struct {
		LockScreenPwd string `json:"lock_screen_pwd"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	if strings.TrimSpace(req.LockScreenPwd) == "" {
		c.ResponseError(errors.New("锁屏密码不能为空"))
		return
	}

	loginUID := c.GetLoginUID()
	err := u.db.UpdateUsersWithField("lock_screen_pwd", req.LockScreenPwd, loginUID)
	if err != nil {
		u.Error("修改用户锁屏密码错误", zap.Error(err))
		c.ResponseError(errors.New("修改用户锁屏密码错误"))
		return
	}
	c.ResponseOK()
}

// 关闭锁屏密码
func (u *User) closeLockScreenPwd(c *wkhttp.Context) {
	loginUID := c.GetLoginUID()
	err := u.db.UpdateUsersWithField("lock_screen_pwd", "", loginUID)
	if err != nil {
		u.Error("修改用户锁屏密码错误", zap.Error(err))
		c.ResponseError(errors.New("修改用户锁屏密码错误"))
		return
	}
	c.ResponseOK()
}

// sendLoginCheckPhoneCode 发送登录验证短信
func (u *User) sendLoginCheckPhoneCode(c *wkhttp.Context) {
	var req struct {
		UID string `json:"uid"`
	}
	if err := c.BindJSON(&req); err != nil {
		u.Error("数据格式有误！", zap.Error(err))
		c.ResponseError(errors.New("数据格式有误！"))
		return
	}
	if req.UID == "" {
		c.ResponseError(errors.New("uid不能为空！"))
		return
	}

	span := u.ctx.Tracer().StartSpan(
		"user.sendLoginCheckPhoneCode",
		opentracing.ChildOf(c.GetSpanContext()),
	)
	defer span.Finish()
	spanCtx := u.ctx.Tracer().ContextWithSpan(context.Background(), span)

	userinfo, err := u.db.QueryByUID(req.UID)
	if err != nil {
		u.Error("查询用户信息失败！", zap.Error(err))
		c.ResponseError(errors.New("修改聊天密码失败"))
		return
	}
	if userinfo == nil {
		u.Error("该用户不存在", zap.Error(err))
		c.ResponseError(errors.New("该用户不存在"))
		return
	}
	//发送短信
	// if u.ctx.GetConfig().Test {
	// 	c.ResponseOK()
	// 	return
	// }
	err = u.smsServie.SendVerifyCode(spanCtx, userinfo.Zone, userinfo.Phone, commonapi.CodeTypeCheckMobile)
	if err != nil {
		u.Error("发送短信失败", zap.Error(err))
		ext.LogError(span, err)
		c.ResponseError(errors.New("发送短信失败"))
		return
	}
	c.ResponseOK()
}

// loginCheckPhone 登录验证设备短信
func (u *User) loginCheckPhone(c *wkhttp.Context) {
	var req struct {
		UID  string `json:"uid"`
		Code string `json:"code"`
	}
	if err := c.BindJSON(&req); err != nil {
		u.Error("数据格式有误！", zap.Error(err))
		c.ResponseError(errors.New("数据格式有误！"))
		return
	}
	if req.UID == "" {
		c.ResponseError(errors.New("uid不能为空！"))
		return
	}
	if req.Code == "" {
		c.ResponseError(errors.New("验证码不能为空！"))
		return
	}
	span := u.ctx.Tracer().StartSpan(
		"user.loginCheckPhone",
		opentracing.ChildOf(c.GetSpanContext()),
	)
	defer span.Finish()
	spanCtx := u.ctx.Tracer().ContextWithSpan(context.Background(), span)

	userInfo, err := u.db.QueryByUID(req.UID)
	if err != nil {
		u.Error("查询用户信息失败！", zap.Error(err))
		c.ResponseError(errors.New("修改聊天密码失败"))
		return
	}
	if userInfo == nil {
		u.Error("该用户不存在", zap.Error(err))
		c.ResponseError(errors.New("该用户不存在"))
		return
	}
	err = u.smsServie.Verify(spanCtx, userInfo.Zone, userInfo.Phone, req.Code, commonapi.CodeTypeCheckMobile)
	if err != nil {
		u.Error("验证短信失败", zap.Error(err))
		c.ResponseError(err)
		return
	}

	loginDeviceJsonStr, err := u.ctx.GetRedisConn().GetString(fmt.Sprintf("%s%s", u.ctx.GetConfig().Cache.LoginDeviceCachePrefix, req.UID))
	if err != nil {
		u.Error("获取登录设备缓存失败！", zap.Error(err))
		c.ResponseError(errors.New("获取登录设备缓存失败！"))
		return
	}
	if loginDeviceJsonStr == "" {
		c.ResponseError(errors.New("登录设备已过期，请重新登录"))
		return
	}
	var loginDeivce *deviceReq
	err = util.ReadJsonByByte([]byte(loginDeviceJsonStr), &loginDeivce)
	if err != nil {
		u.Error("解码登录设备信息失败！", zap.Error(err), zap.String("uid", req.UID))
		c.ResponseError(errors.New("解码登录设备信息失败！"))
		return
	}
	err = u.deviceDB.insertOrUpdateDeviceCtx(spanCtx, &deviceModel{
		UID:         userInfo.UID,
		DeviceID:    loginDeivce.DeviceID,
		DeviceName:  loginDeivce.DeviceName,
		DeviceModel: loginDeivce.DeviceModel,
		LastLogin:   time.Now().Unix(),
	})
	if err != nil {
		u.Error("添加或更新登录设备信息失败！", zap.Error(err))
		c.ResponseError(errors.New("添加或更新登录设备信息失败！"))
		return
	}
	token := util.GenerUUID()
	// 将token设置到缓存
	err = u.ctx.Cache().SetAndExpire(u.ctx.GetConfig().Cache.TokenCachePrefix+token, fmt.Sprintf("%s@%s", userInfo.UID, userInfo.Name), u.ctx.GetConfig().Cache.TokenExpire)
	if err != nil {
		u.Error("设置token缓存失败！", zap.Error(err))
		c.ResponseError(errors.New("设置token缓存失败！"))
		return
	}
	// err = u.ctx.UpdateIMToken(userInfo.UID, token, config.DeviceFlag(0), config.DeviceLevelMaster)
	imResp, err := u.ctx.UpdateIMToken(config.UpdateIMTokenReq{
		UID:         userInfo.UID,
		Token:       token,
		DeviceFlag:  config.APP,
		DeviceLevel: config.DeviceLevelMaster,
	})
	if err != nil {
		u.Error("更新IM的token失败！", zap.Error(err))
		c.ResponseError(errors.New("更新IM的token失败！"))
		return
	}
	if imResp.Status == config.UpdateTokenStatusBan {
		c.ResponseError(errors.New("此账号已经被封禁！"))
		return
	}
	c.Response(newLoginUserDetailResp(userInfo, token, u.ctx))
}

// customerservices 客服列表：user.category='customerService' + 当前应用下 hotline_agent 坐席（与后台「客服配置管理」一致）
// customerservices 客服列表：系统客服 + 当前应用下 hotline_agent 坐席（与后台「客服配置管理」一致）
func (u *User) customerservices(c *wkhttp.Context) {
	results := []*customerservicesResp{}
	seen := map[string]bool{}

	// 1) 系统内置客服（用户分类=customerService）
	list, err := u.db.QueryByCategory(CategoryCustomerService)
	if err != nil {
		u.Error("查询客服列表失败", zap.Error(err))
	}
	for _, user := range list {
		if user != nil && user.UID != "" && !seen[user.UID] {
			results = append(results, &customerservicesResp{UID: user.UID, Name: user.Name})
			seen[user.UID] = true
		}
	}

	// 2) 当前 app 的客服坐席（hotline_agent） + 配置（hotline_config.logo/color/chat_bg）
	appID := c.GetAppID()
	if appID != "" {
		// 先取当前 app 的配置（logo 等）
		type hotlineCfg struct {
			Logo   string `db:"logo"`
			Color  string `db:"color"`
			ChatBg string `db:"chat_bg"`
		}
		var cfg hotlineCfg
		_, _ = u.ctx.DB().Select("logo", "color", "chat_bg").From("hotline_config").Where("app_id=?", appID).Load(&cfg)

		type hotlineCS struct {
			UID  string `db:"uid"`
			Name string `db:"name"`
		}
		var hlList []*hotlineCS
		_, qerr := u.ctx.DB().Select("uid", "name").From("hotline_agent").Where("app_id=? AND status=1", appID).Load(&hlList)
		if qerr != nil {
			u.Error("查询在线客服坐席失败", zap.Error(qerr))
		}
		for _, h := range hlList {
			if h != nil && h.UID != "" && !seen[h.UID] {
				results = append(results, &customerservicesResp{UID: h.UID, Name: h.Name, Logo: cfg.Logo, Color: cfg.Color, ChatBg: cfg.ChatBg})
				seen[h.UID] = true
			}
		}
	}

	c.Response(results)
}

// 发送注销账号验证吗
func (u *User) sendDestroyCode(c *wkhttp.Context) {
	loginUID := c.GetLoginUID()
	userInfo, err := u.db.QueryByUID(loginUID)
	if err != nil {
		u.Error("查询登录用户信息错误", zap.Error(err))
		c.ResponseError(errors.New("查询登录用户信息错误"))
		return
	}
	if userInfo == nil || userInfo.IsDestroy == 1 {
		c.ResponseError(errors.New("登录用户不存在"))
		return
	}
	err = u.smsServie.SendVerifyCode(c.Context, userInfo.Zone, userInfo.Phone, commonapi.CodeTypeDestroyAccount)
	if err != nil {
		c.ResponseError(err)
		return
	}
	c.ResponseOK()
}

// 注销账号
func (u *User) destroyAccount(c *wkhttp.Context) {
	code := c.Param("code")
	loginUID := c.GetLoginUID()
	if code == "" {
		c.ResponseError(errors.New("验证码不能为空"))
		return
	}
	userInfo, err := u.db.QueryByUID(loginUID)
	if err != nil {
		u.Error("查询登录用户信息错误", zap.Error(err))
		c.ResponseError(errors.New("查询登录用户信息错误"))
		return
	}
	if userInfo == nil || userInfo.IsDestroy == 1 {
		c.ResponseError(errors.New("登录用户不存在"))
		return
	}
	//测试模式
	if strings.TrimSpace(u.ctx.GetConfig().SMSCode) != "" {
		if strings.TrimSpace(u.ctx.GetConfig().SMSCode) != code {
			c.ResponseError(errors.New("验证码错误"))
			return
		}
	} else {
		//线上验证短信验证码
		// 校验验证码
		err = u.smsServie.Verify(c.Context, userInfo.Zone, userInfo.Phone, code, commonapi.CodeTypeDestroyAccount)
		if err != nil {
			c.ResponseError(err)
			return
		}
	}

	t := time.Now()
	time := fmt.Sprintf("%d%d%d%d%d", t.Year(), t.Month(), t.Day(), t.Minute(), t.Second())
	phone := fmt.Sprintf("%s@%s@delete", userInfo.Phone, time)
	username := fmt.Sprintf("%s%s", userInfo.Zone, phone)
	err = u.db.destroyAccount(loginUID, username, phone)
	if err != nil {
		u.Error("注销账号错误", zap.Error(err))
		c.ResponseError(errors.New("注销账号错误"))
		return
	}
	err = u.ctx.QuitUserDevice(c.GetLoginUID(), -1) // 退出全部登陆设备
	if err != nil {
		u.Error("退出登陆设备失败", zap.Error(err))
		c.ResponseError(errors.New("退出登陆设备失败"))
		return
	}

	c.ResponseOK()
}

// 处理注册用户和文件助手互为好友
func (u *User) addFileHelperFriend(uid string) error {
	if uid == "" {
		u.Error("用户ID不能为空")
		return errors.New("用户ID不能为空")
	}
	isFriend, err := u.friendDB.IsFriend(uid, u.ctx.GetConfig().Account.FileHelperUID)
	if err != nil {
		u.Error("查询用户关系失败")
		return err
	}
	if !isFriend {
		version := u.ctx.GenSeq(common.FriendSeqKey)
		err := u.friendDB.Insert(&FriendModel{
			UID:     uid,
			ToUID:   u.ctx.GetConfig().Account.FileHelperUID,
			Version: version,
		})
		if err != nil {
			u.Error("注册用户和文件助手成为好友失败")
			return err
		}
	}
	return nil
}

// addSystemFriend 处理注册用户和系统账号互为好友
func (u *User) addSystemFriend(uid string) error {

	if uid == "" {
		u.Error("用户ID不能为空")
		return errors.New("用户ID不能为空")
	}
	isFriend, err := u.friendDB.IsFriend(uid, u.ctx.GetConfig().Account.SystemUID)
	if err != nil {
		u.Error("查询用户关系失败")
		return err
	}
	tx, err := u.friendDB.session.Begin()
	if err != nil {
		u.Error("创建数据库事物失败")
		return errors.New("创建数据库事物失败")
	}
	defer func() {
		if err := recover(); err != nil {
			tx.Rollback()
			panic(err)
		}
	}()
	if !isFriend {
		version := u.ctx.GenSeq(common.FriendSeqKey)
		err := u.friendDB.InsertTx(&FriendModel{
			UID:     uid,
			ToUID:   u.ctx.GetConfig().Account.SystemUID,
			Version: version,
		}, tx)
		if err != nil {
			u.Error("注册用户和系统账号成为好友失败")
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
		u.Error("用户注册数据库事物提交失败", zap.Error(err))
		return err
	}
	return nil
}

// 重置登录密码
func (u *User) pwdforget(c *wkhttp.Context) {
	var req resetPwdReq
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	if strings.TrimSpace(req.Zone) == "" {
		c.ResponseError(errors.New("区号不能为空！"))
		return
	}
	if strings.TrimSpace(req.Phone) == "" {
		c.ResponseError(errors.New("手机号不能为空！"))
		return
	}
	if strings.TrimSpace(req.Code) == "" {
		c.ResponseError(errors.New("验证码不能为空！"))
		return
	}
	if strings.TrimSpace(req.Pwd) == "" {
		c.ResponseError(errors.New("密码不能为空！"))
		return
	}
	userInfo, err := u.db.QueryByPhone(req.Zone, req.Phone)
	if err != nil {
		u.Error("查询用户信息错误", zap.Error(err))
		c.ResponseError(errors.New("查询用户信息错误"))
		return
	}
	if userInfo == nil {
		c.ResponseError(errors.New("该账号不存在"))
		return
	}
	//测试模式
	if strings.TrimSpace(u.ctx.GetConfig().SMSCode) != "" {
		if strings.TrimSpace(u.ctx.GetConfig().SMSCode) != req.Code {
			c.ResponseError(errors.New("验证码错误"))
			return
		}
	} else {
		//线上验证短信验证码
		err = u.smsServie.Verify(context.Background(), req.Zone, req.Phone, req.Code, commonapi.CodeTypeForgetLoginPWD)
		if err != nil {
			c.ResponseError(err)
			return
		}
	}

	err = u.db.UpdateUsersWithField("password", util.MD5(util.MD5(req.Pwd)), userInfo.UID)
	if err != nil {
		u.Error("修改登录密码错误", zap.Error(err))
		c.ResponseError(errors.New("修改登录密码错误"))
		return
	}
	c.ResponseOK()
}

// 获取忘记密码验证码
func (u *User) getForgetPwdSMS(c *wkhttp.Context) {
	var req codeReq
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	if strings.TrimSpace(req.Zone) == "" {
		c.ResponseError(errors.New("区号不能为空！"))
		return
	}
	if strings.TrimSpace(req.Phone) == "" {
		c.ResponseError(errors.New("手机号不能为空！"))
		return
	}

	span := u.ctx.Tracer().StartSpan(
		"user.sendForgetPwdCode",
		opentracing.ChildOf(c.GetSpanContext()),
	)
	defer span.Finish()
	spanCtx := u.ctx.Tracer().ContextWithSpan(context.Background(), span)

	model, err := u.db.QueryByPhone(req.Zone, req.Phone)
	if err != nil {
		u.Error("查询用户信息失败！", zap.Error(err))
		c.ResponseError(errors.New("查询用户信息失败！"))
		return
	}
	if model == nil {
		c.ResponseError(errors.New("该手机号未注册"))
		return
	}
	err = u.smsServie.SendVerifyCode(spanCtx, req.Zone, req.Phone, commonapi.CodeTypeForgetLoginPWD)
	if err != nil {
		u.Error("发送短信验证码失败", zap.Error(err))
		c.ResponseError(errors.New("发送短信验证码失败！"))
		return
	}
	c.ResponseOK()
}

// 是否允许更新
func allowUpdateUserField(field string) bool {
	allowfields := []string{"sex", "short_no", "name", "search_by_phone", "search_by_short", "new_msg_notice", "msg_show_detail", "voice_on", "shock_on", "msg_expire_second"}
	for _, allowFiled := range allowfields {
		if field == allowFiled {
			return true
		}
	}
	return false
}

func (u *User) createUser(registerSpanCtx context.Context, createUser *createUserModel, c *wkhttp.Context, invite *model.Invite) {
	tx, err := u.db.session.Begin()
	if err != nil {
		u.Error("创建数据库事物失败", zap.Error(err))
		c.ResponseError(errors.New("创建数据库事物失败"))
		return
	}
	defer func() {
		if err := recover(); err != nil {
			tx.Rollback()
			panic(err)
		}
	}()
	publicIP := util.GetClientPublicIP(c.Request)
	resp, err := u.createUserWithRespAndTx(registerSpanCtx, createUser, publicIP, invite, tx, func() error {
		err := tx.Commit()
		if err != nil {
			tx.Rollback()
			u.Error("数据库事物提交失败", zap.Error(err))
			c.ResponseError(errors.New("数据库事物提交失败"))
			return nil
		}
		return nil
	})
	if err != nil {
		tx.Rollback()
		c.ResponseError(errors.New("注册失败！"))
		return
	}
	c.Response(resp)
}

func (u *User) defaultRegisterAvatarURL(uid string) string {
	if strings.TrimSpace(uid) == "" {
		return ""
	}
	defaultCount := u.ctx.GetConfig().Avatar.DefaultCount
	if defaultCount <= 0 {
		defaultCount = 900
	}
	avatarID := crc32.ChecksumIEEE([]byte(uid)) % uint32(defaultCount)
	defaultBaseURL := strings.TrimSpace(u.ctx.GetConfig().Avatar.DefaultBaseURL)
	if defaultBaseURL == "" {
		defaultBaseURL = "https://api.dicebear.com/8.x/avataaars/png?seed={avatar}&size=180"
	}
	return strings.ReplaceAll(defaultBaseURL, "{avatar}", fmt.Sprintf("%d", avatarID))
}

func (u *User) tryUploadRegisterAvatar(createUser *createUserModel) {
	if createUser.UID == "" {
		return
	}
	sourceURL := strings.TrimSpace(createUser.PendingAvatarURL)
	if sourceURL == "" {
		sourceURL = u.defaultRegisterAvatarURL(createUser.UID)
		createUser.PendingAvatarURL = sourceURL
	}
	if sourceURL == "" {
		return
	}
	u.Info("tryUploadRegisterAvatar start", zap.String("uid", createUser.UID), zap.String("url", sourceURL))
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	imgReader, _ := u.fileService.DownloadImage(sourceURL, timeoutCtx)
	if imgReader == nil {
		u.Warn("tryUploadRegisterAvatar download image empty", zap.String("uid", createUser.UID), zap.String("url", sourceURL))
		return
	}
	defer imgReader.Close()
	pngBytes, err := normalizeAvatarToPNG(imgReader)
	if err != nil {
		u.Warn("tryUploadRegisterAvatar normalize failed", zap.String("uid", createUser.UID), zap.Error(err))
		return
	}
	avatarID := crc32.ChecksumIEEE([]byte(createUser.UID)) % uint32(u.ctx.GetConfig().Avatar.Partition)
	_, err = u.fileService.UploadFile(fmt.Sprintf("avatar/%d/%s.png", avatarID, createUser.UID), "image/png", func(w io.Writer) error {
		_, werr := w.Write(pngBytes)
		return werr
	})
	if err == nil {
		createUser.IsUploadAvatar = 1
		u.Info("tryUploadRegisterAvatar success", zap.String("uid", createUser.UID), zap.Uint32("avatarID", avatarID))
	} else {
		u.Warn("tryUploadRegisterAvatar upload failed", zap.String("uid", createUser.UID), zap.Uint32("avatarID", avatarID), zap.Error(err))
	}
}

func (u *User) createUserWithRespAndTx(_ context.Context, createUser *createUserModel, publicIP string, invite *model.Invite, tx *dbr.Tx, commitCallback func() error) (*loginUserDetailResp, error) {
	uidStr, err := u.db.AllocateStagedNumericUID(tx)
	if err != nil {
		u.Error("分配用户UID失败", zap.Error(err))
		return nil, err
	}
	createUser.UID = uidStr
	u.Info("createUser allocated uid", zap.String("uid", createUser.UID), zap.String("phone", createUser.Phone), zap.Int("flag", createUser.Flag), zap.Int("pendingAvatarURLLen", len(strings.TrimSpace(createUser.PendingAvatarURL))))
	// 客户端「龙虾号/课堂号」展示的是 short_no，与数字 uid 保持一致
	shortNo := uidStr
	u.tryUploadRegisterAvatar(createUser)
	u.Info("createUser avatar upload status", zap.String("uid", createUser.UID), zap.Int("isUploadAvatar", createUser.IsUploadAvatar))

	userModel := &Model{}
	userModel.UID = createUser.UID
	if createUser.Name != "" {
		userModel.Name = createUser.Name
	} else {
		appconfig, err := u.commonService.GetAppConfig()
		if err != nil {
			u.Error("获取应用配置失败！", zap.Error(err))
			return nil, err
		}
		if appconfig != nil && appconfig.RegisterUserMustCompleteInfoOn == 1 {
			userModel.Name = ""
		} else {
			userModel.Name = Names[rand.Intn(len(Names)-1)]
		}
	}
	userModel.Sex = createUser.Sex
	userModel.Vercode = fmt.Sprintf("%s@%d", util.GenerUUID(), common.User)
	userModel.QRVercode = fmt.Sprintf("%s@%d", util.GenerUUID(), common.QRCode)
	userModel.Phone = createUser.Phone
	userModel.Zone = createUser.Zone
	if createUser.Phone != "" {
		userModel.Username = fmt.Sprintf("%s%s", createUser.Zone, createUser.Phone)
	}
	if createUser.Password != "" {
		userModel.Password = util.MD5(util.MD5(createUser.Password))
	}
	if createUser.Username != "" {
		userModel.Username = createUser.Username
	}

	userModel.ShortNo = shortNo
	userModel.OfflineProtection = 0
	userModel.NewMsgNotice = 1
	userModel.MsgShowDetail = 1
	userModel.SearchByPhone = 1
	userModel.SearchByShort = 1
	userModel.VoiceOn = 1
	userModel.ShockOn = 1
	userModel.IsUploadAvatar = createUser.IsUploadAvatar
	userModel.WXOpenid = createUser.WXOpenid
	userModel.WXUnionid = createUser.WXUnionid
	userModel.GiteeUID = createUser.GiteeUID
	userModel.GithubUID = createUser.GithubUID
	userModel.Status = int(common.UserAvailable)
	err = u.db.insertTx(userModel, tx)
	if err != nil {
		u.Error("注册用户失败", zap.Error(err))
		return nil, err
	}
	if createUser.Device != nil {
		err = u.deviceDB.insertOrUpdateDeviceTx(&deviceModel{
			UID:         createUser.UID,
			DeviceID:    createUser.Device.DeviceID,
			DeviceName:  createUser.Device.DeviceName,
			DeviceModel: createUser.Device.DeviceModel,
			LastLogin:   time.Now().Unix(),
		}, tx)
		if err != nil {
			u.Error("添加用户设备信息失败", zap.Error(err))
			return nil, err
		}
	}
	err = u.addSystemFriend(createUser.UID)
	if err != nil {
		u.Error("添加注册用户和系统账号为好友关系失败", zap.Error(err))
		return nil, err
	}
	err = u.addFileHelperFriend(createUser.UID)
	if err != nil {
		u.Error("添加注册用户和文件助手为好友关系失败", zap.Error(err))
		return nil, err
	}
	inviteCode := ""
	inviteUID := ""
	vercode := ""
	if invite != nil {
		inviteCode = invite.InviteCode
		inviteUID = invite.Uid
		vercode = invite.Vercode
	}
	if strings.TrimSpace(inviteCode) == "" {
		inviteCode = strings.TrimSpace(createUser.InviteCode)
	}
	//发送用户注册事件
	eventID, err := u.ctx.EventBegin(&wkevent.Data{
		Event: event.EventUserRegister,
		Type:  wkevent.Message,
		Data: map[string]interface{}{
			"uid":            createUser.UID,
			"invite_code":    inviteCode,
			"invite_uid":     inviteUID,
			"invite_vercode": vercode,
		},
	}, tx)
	if err != nil {
		u.Error("开启事件失败！", zap.Error(err))
		return nil, err
	}

	if commitCallback != nil {
		commitCallback()
	}
	u.ctx.EventCommit(eventID)
	token := util.GenerUUID()
	// 将token设置到缓存
	err = u.ctx.Cache().SetAndExpire(u.ctx.GetConfig().Cache.TokenCachePrefix+token, fmt.Sprintf("%s@%s@%s", userModel.UID, userModel.Name, userModel.Role), u.ctx.GetConfig().Cache.TokenExpire)
	if err != nil {
		u.Error("设置token缓存失败！", zap.Error(err))
		return nil, err
	}
	_, err = u.ctx.UpdateIMToken(config.UpdateIMTokenReq{
		UID:         createUser.UID,
		Token:       token,
		DeviceFlag:  config.DeviceFlag(createUser.Flag),
		DeviceLevel: config.DeviceLevelSlave,
	})
	if err != nil {
		u.Error("更新IM的token失败！", zap.Error(err))
		return nil, err
	}
	go u.sentWelcomeMsg(publicIP, createUser.UID)

	return newLoginUserDetailResp(userModel, token, u.ctx), nil
}

// ---------- vo ----------
type createUserModel struct {
	UID              string
	Name             string
	Zone             string
	Phone            string
	InviteCode       string
	Sex              int
	Password         string
	WXOpenid         string
	WXUnionid        string
	GiteeUID         string
	GithubUID        string
	Username         string
	Flag             int
	IsUploadAvatar   int
	Device           *deviceReq
	PendingAvatarURL string // 分配 uid 后再拉取头像（微信/GitHub/Gitee 注册）
}

// 重置登录密码
type resetPwdReq struct {
	Zone  string `json:"zone"`  //区号
	Phone string `json:"phone"` //手机号
	Code  string `json:"code"`  //验证码
	Pwd   string `json:"pwd"`   //密码
}
type customerservicesResp struct {
	UID    string `json:"uid"`
	Name   string `json:"name"`
	Logo   string `json:"logo"`    // 客服头像/图标（后台配置的图片地址，原样透传）
	Color  string `json:"color"`   // 客服主题色（可选）
	ChatBg string `json:"chat_bg"` // 客服聊天背景（可选）
}
type registerReq struct {
	Name             string     `json:"name"`
	Zone             string     `json:"zone"`
	Phone            string     `json:"phone"`
	Code             string     `json:"code"`
	Password         string     `json:"password"`
	Flag             uint8      `json:"flag"` // 注册设备的标记 0.APP 1.PC
	Device           *deviceReq `json:"device"`
	InviteCode       string     `json:"invite_code"`     // 邀请码（snake_case）
	InviteCodeCamel  string     `json:"inviteCode"`      // 邀请码（camelCase，兼容）
	InviteCodeSimple string     `json:"invite"`          // 邀请码（旧字段，兼容）
	InviteCodeAlt    string     `json:"invitation_code"` // 邀请码（第三方字段，兼容）
}

func (r registerReq) GetInviteCode() string {
	if v := strings.TrimSpace(r.InviteCode); v != "" {
		return v
	}
	if v := strings.TrimSpace(r.InviteCodeCamel); v != "" {
		return v
	}
	if v := strings.TrimSpace(r.InviteCodeSimple); v != "" {
		return v
	}
	return strings.TrimSpace(r.InviteCodeAlt)
}

func (r registerReq) CheckRegister() error {
	if strings.TrimSpace(r.Zone) == "" {
		return errors.New("区号不能为空！")
	}
	if strings.TrimSpace(r.Phone) == "" {
		return errors.New("手机号不能为空！")
	}
	if strings.TrimSpace(r.Code) == "" {
		return errors.New("验证码不能为空！")
	}
	if strings.TrimSpace(r.Password) == "" {
		return errors.New("密码不能为空！")
	}
	if len(r.Password) < 6 {
		return errors.New("密码长度必须大于6位！")
	}
	return nil
}

// 设置聊天密码请求
type chatPwdReq struct {
	ChatPwd  string `json:"chat_pwd"`  //聊天密码
	LoginPwd string `json:"login_pwd"` //登录密码
}

// 注册验证码请求
type codeReq struct {
	Zone  string `json:"zone"`
	Phone string `json:"phone"`
}
type loginReq struct {
	Username string     `json:"username"`
	Password string     `json:"password"`
	Flag     int        `json:"flag"`   // 与 config.DeviceFlag 一致：0=APP，1=Web，2=PC（手机浏览器 H5 建议 1；误传 0 时服务端会按 User-Agent 尽量识别浏览器并清理原生推送缓存）
	Device   *deviceReq `json:"device"` //登录设备信息
}

func (r loginReq) Check() error {
	if strings.TrimSpace(r.Username) == "" {
		return errors.New("用户名不能为空！")
	}
	if strings.TrimSpace(r.Password) == "" {
		return errors.New("密码不能为空！")
	}
	return nil
}

type userResp struct {
	UID     string `json:"uid"`
	Name    string `json:"name"`
	Vercode string `json:"vercode"`
}

func newUserResp(m *Model) userResp {
	return userResp{
		UID:     m.UID,
		Name:    m.Name,
		Vercode: m.Vercode,
	}
}

type deviceReq struct {
	DeviceID    string `json:"device_id"`    //设备唯一ID
	DeviceName  string `json:"device_name"`  //设备名称
	DeviceModel string `json:"device_model"` //设备model
}

type loginUserDetailResp struct {
	UID             string  `json:"uid"`
	AppID           string  `json:"app_id"`
	Name            string  `json:"name"`
	Username        string  `json:"username"`
	Sex             int     `json:"sex"`               //性别1:男
	Category        string  `json:"category"`          //用户分类 '客服'
	ShortNo         string  `json:"short_no"`          // 用户唯一短编号
	Zone            string  `json:"zone"`              //区号
	Phone           string  `json:"phone"`             //手机号
	Token           string  `json:"token"`             //token
	ChatPwd         string  `json:"chat_pwd"`          //聊天密码
	LockScreenPwd   string  `json:"lock_screen_pwd"`   // 锁屏密码
	LockAfterMinute int     `json:"lock_after_minute"` // 在N分钟后锁屏
	Setting         setting `json:"setting"`
	RSAPublicKey    string  `json:"rsa_public_key"` // 应用公钥做一些消息验证 base64编码
	ShortStatus     int     `json:"short_status"`
	MsgExpireSecond int64   `json:"msg_expire_second"` // 消息过期时长
}

type setting struct {
	SearchByPhone     int `json:"search_by_phone"`    //是否可以通过手机号搜索0.否1.是
	SearchByShort     int `json:"search_by_short"`    //是否可以通过短编号搜索0.否1.是
	NewMsgNotice      int `json:"new_msg_notice"`     //新消息通知0.否1.是
	MsgShowDetail     int `json:"msg_show_detail"`    //显示消息通知详情0.否1.是
	VoiceOn           int `json:"voice_on"`           //声音0.否1.是
	ShockOn           int `json:"shock_on"`           //震动0.否1.是
	OfflineProtection int `json:"offline_protection"` //离线保护，断网屏保
	DeviceLock        int `json:"device_lock"`        // 设备锁
	MuteOfApp         int `json:"mute_of_app"`        // web登录 app是否静音
}

type blacklistResp struct {
	UID      string `json:"uid"`
	Name     string `json:"name"`
	Username string `json:"usename"`
}

func newLoginUserDetailResp(m *Model, token string, ctx *config.Context) *loginUserDetailResp {

	return &loginUserDetailResp{
		UID:             m.UID,
		AppID:           m.AppID,
		Name:            m.Name,
		Username:        m.Username,
		Sex:             m.Sex,
		Category:        m.Category,
		ShortNo:         m.ShortNo,
		Zone:            m.Zone,
		Phone:           m.Phone,
		Token:           token,
		ChatPwd:         m.ChatPwd,
		LockScreenPwd:   m.LockScreenPwd,
		LockAfterMinute: m.LockAfterMinute,
		ShortStatus:     m.ShortStatus,
		RSAPublicKey:    base64.StdEncoding.EncodeToString([]byte(ctx.GetConfig().AppRSAPubKey)),
		MsgExpireSecond: m.MsgExpireSecond,
		Setting: setting{
			SearchByPhone:     m.SearchByPhone,
			SearchByShort:     m.SearchByShort,
			NewMsgNotice:      m.NewMsgNotice,
			MsgShowDetail:     m.MsgShowDetail,
			VoiceOn:           m.VoiceOn,
			ShockOn:           m.ShockOn,
			OfflineProtection: m.OfflineProtection,
			DeviceLock:        m.DeviceLock,
			MuteOfApp:         m.MuteOfApp,
		},
	}
}
