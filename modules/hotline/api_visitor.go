package hotline

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/common"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"go.uber.org/zap"
)

// 在线没有分配的访客
func (h *Hotline) visitorOnlineNoAgent(c *wkhttp.Context) {
	appID := c.GetAppID()
	visitorDetailAndChannelModels, err := h.visitorDB.queryDetailAndChannelWithOnlineAndNoAgent(appID)
	if err != nil {
		c.ResponseErrorf("查询实时访客失败！", err)
		return
	}
	results := make([]map[string]interface{}, 0, len(visitorDetailAndChannelModels))
	if len(visitorDetailAndChannelModels) > 0 {
		for _, visitorDetailAndChannelM := range visitorDetailAndChannelModels {
			results = append(results, map[string]interface{}{
				"vid":           visitorDetailAndChannelM.VID,
				"name":          visitorDetailAndChannelM.Name,
				"avatar":        visitorDetailAndChannelM.Avatar,
				"state":         visitorDetailAndChannelM.State,
				"city":          visitorDetailAndChannelM.City,
				"history_count": visitorDetailAndChannelM.HistoryCount,
				"channel_id":    visitorDetailAndChannelM.ChannelID,
				"channel_type":  visitorDetailAndChannelM.ChannelType,
				"last_online":   visitorDetailAndChannelM.LastOnline,
				"topic_name":    visitorDetailAndChannelM.TopicName,
				"source":        visitorDetailAndChannelM.Source,
			})
		}
	}
	c.Response(results)
}

func (h *Hotline) visitorGet(c *wkhttp.Context) {
	vid := c.Param("vid")
	appID := c.GetAppID()

	visitorM, err := h.visitorDB.queryDetailVIDAndAppID(vid, appID)
	if err != nil {
		h.Error("查询访客失败！", zap.Error(err))
		c.ResponseError(errors.New("查询访客失败！"))
		return
	}
	if visitorM == nil {
		c.ResponseError(errors.New("访客不存在！"))
		return
	}

	resp := newVisitorDetailResp(visitorM)
	firstHistory, err := h.historyDB.queryFirst(vid, appID)
	if err != nil {
		c.ResponseErrorf("查询第一条访问历史失败！", err)
		return
	}
	if firstHistory != nil {
		resp.FirstHistory = newHistoryResp(firstHistory)
	}
	lastHistory, err := h.historyDB.queryLast(vid, appID)
	if err != nil {
		c.ResponseErrorf("查询最新访问历史失败！", err)
		return
	}
	if lastHistory != nil {
		resp.LastHistory = newHistoryResp(lastHistory)
	}
	histories, err := h.historyDB.queryRecent(5, vid, appID)
	if err != nil {
		c.ResponseErrorf("查询最近访问历史失败！", err)
		return
	}
	if len(histories) > 0 {
		historyResps := make([]*historyResp, 0, len(histories))
		for _, history := range histories {
			historyResps = append(historyResps, newHistoryResp(history))
		}
		resp.Histories = historyResps
	}

	deviceM, err := h.deviceDB.queryWithVIDAndAppID(vid, appID)
	if err != nil {
		c.ResponseErrorf("查询设备数据失败！", err)
		return
	}
	if deviceM != nil {
		resp.Device = newDeviceResp(deviceM)
	}

	c.Response(resp)

}

// 访客自定义属性
func (h *Hotline) visitorPropsUpdate(c *wkhttp.Context) {
	vid := c.Param("vid")
	value := c.Param("value")
	appID := c.GetAppID()
	field := c.Param("field")
	if field == "" {
		c.ResponseError(errors.New("属性不能为空！"))
		return
	}
	visitorPropsM, err := h.visitorDB.queryPropsWithFieldAndVidAndAppID(field, vid, appID)
	if err != nil {
		c.ResponseErrorf("查询访客属性失败！", err)
		return
	}
	if visitorPropsM != nil {
		err = h.visitorDB.updatePropsWithVidAndAppID(field, value, vid, appID)
		if err != nil {
			c.ResponseErrorf("更新属性失败！", err)
			return
		}
	} else {
		err = h.visitorDB.insertPropsWithVidAndAppID(&visitorPropsModel{
			AppID: appID,
			VID:   vid,
			Field: field,
			Value: value,
		})
		if err != nil {
			c.ResponseErrorf("添加属性失败！", err)
			return
		}
	}
	c.ResponseOK()

}

func (h *Hotline) visitorFieldUpdate(c *wkhttp.Context, field string) {
	vid := c.Param("vid")
	value := c.Param("value")
	appID := c.GetAppID()

	var err error
	switch field {
	case "phone":
		err = h.visitorDB.updatePhone(value, vid, appID)
		if err != nil {
			c.ResponseErrorf("修改手机号失败！", err)
			return
		}
	case "email":
		err = h.visitorDB.updateEmail(value, vid, appID)
		if err != nil {
			c.ResponseErrorf("修噶邮箱失败！", err)
			return
		}
	}
	c.ResponseOK()
}

func (h *Hotline) visitorPage(c *wkhttp.Context) {
	pIndex, pSize := c.GetPage()
	keyword := c.Query("keyword")
	appID := c.GetAppID()

	visitors, err := h.visitorDB.queryWithPage(keyword, appID, pIndex, pSize)
	if err != nil {
		c.ResponseErrorf("查询访客数据失败！", err)
		return
	}
	total, err := h.visitorDB.queryTotalWith(keyword, appID)
	if err != nil {
		c.ResponseErrorf("查询访客总数量失败！", err)
		return
	}

	resps := make([]*visitorDetailResp, 0, len(visitors))
	if len(visitors) > 0 {
		for _, visitor := range visitors {
			resp := newVisitorDetailResp(visitor)

			onlineMap, err := h.ctx.GetRedisConn().Hgetall(h.getVisitorOnlineCacheKey(visitor.VID))
			if err != nil {
				h.Warn("从缓存查询访客在线状态失败！", zap.Error(err))
			}
			if onlineMap != nil {
				onlineStr := onlineMap["online"]
				if onlineStr == "1" {
					resp.Online = 1
				}
				lastOnlineStr := onlineMap["last_online"]
				lastOfflineStr := onlineMap["last_offline"]

				if lastOnlineStr != "" {
					lastOnline, _ := strconv.ParseInt(lastOnlineStr, 10, 64)
					resp.LastOnline = util.ToyyyyMMddHHmm(time.Unix(lastOnline, 0))
				}
				if lastOfflineStr != "" {
					lastOffline, _ := strconv.ParseInt(lastOfflineStr, 10, 64)
					resp.LastOffline = util.ToyyyyMMddHHmm(time.Unix(lastOffline, 0))
				}
			}
			resps = append(resps, resp)
		}
	}

	c.Response(common.NewPageResult(pIndex, pSize, total, resps))

}

// 获取访客信息
func (h *Hotline) visitor(c *wkhttp.Context) {
	appID := c.Param("app_id")
	var req visitorReq
	if err := c.BindJSON(&req); err != nil {
		c.ResponseErrorf("数据格式有误！", err)
		return
	}
	vid := req.VID
	hasVID := len(vid) > 0
	if hasVID {
		visitorM, err := h.visitorDB.queryWithVIDAndAppID(vid, appID)
		if err != nil {
			c.ResponseErrorf("查询是否存在访客信息失败！", err)
			return
		}
		if visitorM != nil {
			err = h.historyDB.insert(&historyModel{
				AppID:     appID,
				VID:       vid,
				SiteTitle: req.SiteTitle,
				SiteURL:   req.SiteURL,
				Referrer:  req.ReferrerURL,
			})
			if err != nil {
				c.ResponseErrorf("添加访问历史失败！", err)
				return
			}
			token := util.GenerUUID()
			if req.NotRegisterToken != 1 {
				err = h.setVisitorToken(visitorM.VID, visitorM.Name, token)
				if err != nil {
					c.ResponseErrorf("token设置失败！", err)
					return
				}
			}

			c.Response(&visitorLoginResp{
				VID:   visitorM.VID,
				Name:  visitorM.Name,
				Token: token,
			})
			return
		}
	} else {
		vid = fmt.Sprintf("%s%s", h.ctx.GetConfig().VisitorUIDPrefix, util.GenerUUID())
	}

	var err error
	publicIP := util.GetClientPublicIP(c.Request)
	var province, city string
	var source, searchKeyword string // 来源
	if strings.TrimSpace(publicIP) != "" {
		province, city, err = util.GetIPAddress(publicIP)
		if err != nil {
			h.Warn("获取IP归属地址失败！", zap.String("IP", publicIP), zap.Error(err))
		}
	}
	if req.ReferrerURL != "" {
		source, searchKeyword = h.GetSourceAndSearchKeyword(req.ReferrerURL)
	}

	tx, _ := h.ctx.DB().Begin()
	defer func() {
		if err := recover(); err != nil {
			tx.RollbackUnlessCommitted()
			panic(err)
		}
	}()
	visitorM := &visitorModel{
		AppID:         appID,
		VID:           vid,
		Name:          util.GetRandomName(),
		IPAddress:     publicIP,
		State:         province,
		City:          city,
		Timezone:      req.Timezone,
		Source:        source,
		Local:         req.Local,
		SearchKeyword: searchKeyword,
	}
	err = h.visitorDB.insertTx(visitorM, tx)
	if err != nil {
		tx.Rollback()
		c.ResponseErrorf("添加访客数据失败！", err)
		return
	}

	if req.Device != nil {
		deviceM := &deviceModel{
			AppID:   appID,
			VID:     vid,
			Device:  req.Device.Device,
			OS:      req.Device.OS,
			Model:   req.Device.Model,
			Version: req.Device.Version,
		}
		err = h.deviceDB.insertTx(deviceM, tx)
		if err != nil {
			tx.Rollback()
			c.ResponseErrorf("添加访客设备信息失败！", err)
			return
		}
	}
	err = h.historyDB.insertTx(&historyModel{
		AppID:     appID,
		VID:       vid,
		SiteTitle: req.SiteTitle,
		SiteURL:   req.SiteURL,
		Referrer:  req.ReferrerURL,
	}, tx)
	if err != nil {
		tx.Rollback()
		c.ResponseErrorf("新增访问历史失败！", err)
		return
	}

	if err := tx.Commit(); err != nil {
		tx.Rollback()
		c.ResponseErrorf("提交事务失败！", err)
		return
	}
	var token string
	if req.NotRegisterToken != 1 {
		token = util.GenerUUID()
		err = h.setVisitorToken(visitorM.VID, visitorM.Name, token)
		if err != nil {
			c.ResponseErrorf("token设置失败！", err)
			return
		}
	}

	c.Response(&visitorLoginResp{
		VID:   visitorM.VID,
		Name:  visitorM.Name,
		Token: token,
	})

}

func (h *Hotline) setVisitorToken(vid string, visitorName string, token string) error {

	// 将token设置到缓存
	err := h.ctx.Cache().SetAndExpire(h.ctx.GetConfig().Cache.TokenCachePrefix+token, fmt.Sprintf("%s@%s", vid, visitorName), h.ctx.GetConfig().Cache.TokenExpire)
	if err != nil {
		h.Error("设置token缓存失败！", zap.Error(err))
		return errors.New("设置token缓存失败！")
	}
	// 更新IM的token
	_, err = h.ctx.UpdateIMToken(config.UpdateIMTokenReq{
		UID:         vid,
		Token:       token,
		DeviceFlag:  config.Web,
		DeviceLevel: config.DeviceLevelSlave,
	})
	if err != nil {
		h.Error("更新IM token失败！", zap.Error(err))
		return errors.New("更新IM token失败！")
	}
	return nil
}

// 获取访客的IM信息
func (h *Hotline) visitorIMGet(c *wkhttp.Context) {
	vid := c.Param("vid")
	visitorM, err := h.visitorDB.queryWithVID(vid)
	if err != nil {
		h.Error("查询访客信息失败！", zap.Error(err))
		c.ResponseErrorf("查询访客信息失败！", err)
		return
	}
	if visitorM == nil {
		h.Error("访客信息不存在！", zap.String("vid", vid))
		c.ResponseError(errors.New("访客不存在！"))
		return
	}

	c.Response(userDetailResp{
		UID:  vid,
		Name: visitorM.Name,
	})
}

// 访客请求
type visitorReq struct {
	VID              string            `json:"vid"`
	ReferrerURL      string            `json:"referrer_url"`       // 上级页面url
	SiteURL          string            `json:"site_url"`           // 网站url
	SiteTitle        string            `json:"site_title"`         // 网站标题
	Device           *visitorDeviceReq `json:"device"`             // 设备
	Timezone         string            `json:"timezone"`           // 访客时区
	Local            int               `json:"local"`              // 是否是本系统用户
	NotRegisterToken int               `json:"not_register_token"` // 不换成token，为true的情况只有在自己系统才有，第三方系统不存在true
}

type visitorDeviceReq struct {
	Device  string `json:"device"`  // 设备：例如: desktop,phone
	OS      string `json:"os"`      // 设备系统 例如 Web,iOS,Android
	Model   string `json:"model"`   // 备型号 例如 Chrome，iPhone X
	Version string `json:"version"` // 系统版本：例如：13.0
}

type visitorLoginResp struct {
	VID    string `json:"vid"`
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
	Token  string `json:"token"`
}

type visitorDetailResp struct {
	VID          string `json:"vid"`            // 访客唯一ID
	Name         string `json:"name"`           // 访客名字
	Phone        string `json:"phone"`          // 手机号
	Email        string `json:"email"`          // email
	IPAddress    string `json:"ip_address"`     // IP地址
	State        string `json:"state"`          // 省份
	City         string `json:"city"`           // 城市
	Timezone     string `json:"timezone"`       // 时区
	Source       string `json:"source"`         // 来源
	SeachKeyword string `json:"search_keyword"` // 搜索关键字
	SessionCount int    `json:"session_count"`  // 会话次数

	Online      int    `json:"online"`       // 是否在线 1.在线 0.不在线
	LastOnline  string `json:"last_online"`  // 最后一次在线时间
	LastOffline string `json:"last_offline"` // 最后一次离线时间

	CreatedAt string `json:"created_at"` // 创建时间

	Device *deviceResp `json:"device"` // 访客设备

	FirstHistory *historyResp `json:"first_history"` // 第一次聊天的历史页面
	LastHistory  *historyResp `json:"last_history"`  // 最后一次聊天的历史页面

	Histories []*historyResp `json:"histories"` // 访客历史
}

func newVisitorDetailResp(m *visitorDetailModel) *visitorDetailResp {
	lastOnlineStr := ""
	lastOfflineStr := ""
	if m.LastOnline != 0 {
		lastOnlineStr = util.ToyyyyMMddHHmm(time.Unix(m.LastOnline, 0))
	}
	if m.LastOffline != 0 {
		lastOfflineStr = util.ToyyyyMMddHHmm(time.Unix(m.LastOffline, 0))
	}
	return &visitorDetailResp{
		VID:          m.VID,
		Name:         m.Name,
		Email:        m.Email,
		Phone:        m.Phone,
		IPAddress:    m.IPAddress,
		State:        m.State,
		City:         m.City,
		Timezone:     m.Timezone,
		Source:       m.Source,
		SeachKeyword: m.SearchKeyword,
		Online:       m.Online,
		LastOnline:   lastOnlineStr,
		LastOffline:  lastOfflineStr,
		CreatedAt:    util.ToyyyyMMddHHmm(time.Time(m.CreatedAt)),
		SessionCount: m.SessionCount,
	}
}

type deviceResp struct {
	Device  string `json:"device"`  // 设备：例如: desktop,phone
	OS      string `json:"os"`      // 设备系统 例如 Web,iOS,Android
	Model   string `json:"model"`   // 备型号 例如 Chrome，iPhone X
	Version string `json:"version"` // 系统版本：例如：13.0
}

func newDeviceResp(m *deviceModel) *deviceResp {
	return &deviceResp{
		Device:  m.Device,
		OS:      m.OS,
		Model:   m.Model,
		Version: m.Version,
	}
}

type historyResp struct {
	SiteURL   string `json:"site_url"`
	SiteTitle string `json:"site_title"`
	Referrer  string `json:"referrer"`
	CreatedAt string `json:"created_at"`
}

func newHistoryResp(m *historyModel) *historyResp {
	return &historyResp{
		SiteURL:   m.SiteURL,
		SiteTitle: m.SiteTitle,
		Referrer:  m.Referrer,
		CreatedAt: m.CreatedAt.String(),
	}
}

// VisitorDetail 访客详情
type VisitorDetail struct {
	AppID        string `json:"app_id"`
	VID          string `json:"vid"`           // 访客唯一ID
	Name         string `json:"name"`          // 访客名字
	Avatar       string `json:"avatar"`        // 访客头像
	IPAddress    string `json:"ip_address"`    // IP地址
	State        string `json:"state"`         // 省份
	City         string `json:"city"`          // 城市
	Timezone     string `json:"timezone"`      // 时区
	SessionCount int    `json:"session_count"` // 会话次数

	GroupNo string `json:"group_no"` // 分配的群组

	Device  string `json:"device"`  // 设备：例如: desktop,phone
	OS      string `json:"os"`      // 设备系统 例如 Web,iOS,Android
	Model   string `json:"model"`   // 备型号 例如 Chrome，iPhone X
	Version string `json:"version"` // 系统版本：例如：13.0
}

type userDetailResp struct {
	UID          string `json:"uid"`
	Name         string `json:"name"`
	Mute         int    `json:"mute"`          // 免打扰
	Top          int    `json:"top"`           // 置顶
	Sex          int    `json:"sex"`           //性别1:男
	Category     string `json:"category"`      //用户分类 '客服'
	ShortNo      string `json:"short_no"`      // 用户唯一短编号
	ChatPwdOn    int    `json:"chat_pwd_on"`   //是否开启聊天密码
	Screenshot   int    `json:"screenshot"`    //截屏通知
	RevokeRemind int    `json:"revoke_remind"` //撤回提醒
	Online       int    `json:"online"`        //是否在线
	LastOffline  int    `json:"last_offline"`  //最后一次离线时间
	Follow       int    `json:"follow"`        //是否是好友
	Code         string `json:"code"`          //加好友所需vercode
	SourceDesc   string `json:"source_desc"`   // 好友来源
	Remark       string `json:"remark"`        //好友备注
	Status       int    `json:"status"`        //用户状态 1 正常 2:黑名单
}
