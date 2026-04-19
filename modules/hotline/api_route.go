package hotline

import (
	"errors"
	"fmt"
	"time"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"go.uber.org/zap"
)

// // 分配客服-系统最核心的接口）
// func (h *Hotline) route(c *wkhttp.Context) {
// 	vid := c.GetLoginUID() // 当前访客vid
// 	appID := c.Param("app_id")

// 	visitorM, err := h.visitorDB.queryWithVID(vid, appID)
// 	if err != nil {
// 		return
// 	}
// }

func (h *Hotline) assignTo(c *wkhttp.Context) {
	var req struct {
		UID         string `json:"uid"`
		ChannelID   string `json:"channel_id"`
		ChannelType uint8  `json:"channel_type"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseErrorf("数据格式有误！", err)
		return
	}
	if req.UID == "" {
		c.ResponseError(errors.New("uid不能为空！"))
		return
	}
	if req.ChannelID == "" {
		c.ResponseError(errors.New("频道ID不能为空！"))
		return
	}
	err := h.routeService.AssignToUser(req.UID, c.GetLoginUID(), req.ChannelID, req.ChannelType)
	if err != nil {
		c.ResponseError(err)
		return
	}
	c.ResponseOK()
}

func (h *Hotline) getVisitorDetailWithMessage(message *config.MessageResp) *VisitorDetail {
	// ---------- 访客信息 ----------
	visitorM, err := h.visitorDB.queryWithVID(message.FromUID)
	if err != nil {
		h.Error("查询访客数据失败！", zap.Error(err), zap.String("vid", message.FromUID))
		return nil
	}
	if visitorM == nil {
		h.Error("访客不存在。不能进行路由逻辑！", zap.String("vid", message.FromUID))
		return nil
	}

	visitorDetail := &VisitorDetail{
		AppID:        visitorM.AppID,
		VID:          visitorM.VID,
		Name:         visitorM.Name,
		Avatar:       visitorM.Avatar,
		IPAddress:    visitorM.IPAddress,
		State:        visitorM.State,
		City:         visitorM.City,
		Timezone:     visitorM.Timezone,
		SessionCount: visitorM.SessionCount,
	}

	deviceM, err := h.deviceDB.queryWithVID(visitorM.VID)
	if err != nil {
		h.Error("查询访客设备失败！", zap.Error(err), zap.String("vid", visitorM.VID))
		return nil
	}
	if deviceM != nil {
		visitorDetail.Device = deviceM.Device
		visitorDetail.OS = deviceM.OS
		visitorDetail.Model = deviceM.Model
		visitorDetail.Version = deviceM.Version
	}
	return visitorDetail
}

func (h *Hotline) getChannelDetailWithMessage(message *config.MessageResp) *ChannelDetail {

	channelM, err := h.channelDB.queryWithChannelID(message.ChannelID, message.ChannelType)
	if err != nil {
		h.Error("查询频道失败！", zap.Error(err))
		return nil
	}
	if channelM == nil {
		h.Error("频道不存在！", zap.String("channelID", message.ChannelID), zap.Uint8("channelType", message.ChannelType))
		return nil
	}

	return &ChannelDetail{
		VID:         channelM.VID,
		TopicID:     channelM.TopicID,
		Title:       channelM.Title,
		ChannelID:   channelM.ChannelID,
		ChannelType: channelM.ChannelType,
		AgentUID:    channelM.AgentUID,
	}
}

func (h *Hotline) getSessionDetailWithMessage(message *config.MessageResp) *SessionDetail {
	sessionM, err := h.sessionDB.queryWithChannel(message.ChannelID, message.ChannelType)
	if err != nil {
		h.Error("查询会话信息失败！", zap.Error(err))
		return nil
	}
	if sessionM == nil {
		h.Warn("没有查询到频道的会话数据！", zap.String("channelID", message.ChannelID), zap.Uint8("channelType", message.ChannelType))
		return nil
	}
	return &SessionDetail{
		VID:         sessionM.VID,
		ChannelType: sessionM.ChannelType,
		ChannelID:   sessionM.ChannelID,
		SendCount:   sessionM.SendCount,
		RecvCount:   sessionM.RecvCount,
		LastRecv:    sessionM.LastRecv,
		LastSend:    sessionM.LastSend,
		LastSession: sessionM.LastSessionTimestamp,
	}
}

func (h *Hotline) route(message *config.MessageResp) error {

	visitorDetail := h.getVisitorDetailWithMessage(message)
	if visitorDetail == nil {
		h.Error("路由失败！没有获取到访客信息！")
		return errors.New("路由失败！没有获取到访客信息！")
	}
	channelDetail := h.getChannelDetailWithMessage(message)
	if channelDetail == nil {
		h.Error("路由失败！没有获取到频道信息！")
		return errors.New("路由失败！没有获取到频道信息！")
	}
	sessionDetail := h.getSessionDetailWithMessage(message)

	routeResult, err := h.routeService.Route(&RouteContext{
		AppID:   visitorDetail.AppID,
		Message: message,
		Visitor: visitorDetail,
		Channel: channelDetail,
		Session: sessionDetail,
	})
	if err != nil {
		h.Error("[严重]路由客服失败！", zap.Error(err), zap.String("vid", visitorDetail.VID))
		return fmt.Errorf("[严重]路由客服失败！")
	}
	if routeResult == nil {
		return errors.New("没有分配到客服，请检查系统配置。")
	}

	h.Debug("路由结果", zap.Any("routeResult", routeResult))

	if channelDetail.AgentUID != "" && routeResult.AgentUID == channelDetail.AgentUID { // 如果被分配的客服还是原来已经分配的那个客服则不执行分配逻辑
		h.Info("路由到的客服跟已经分配的客服是同一个，不执行分配！")
		return errors.New("路由到的客服跟已经分配的客服是同一个，不执行分配！")
	}

	err = h.routeService.AssignToUser(routeResult.AgentUID, "", message.ChannelID, message.ChannelType)
	if err != nil {
		h.Error("将客服分配给指定的频道失败！", zap.Error(err), zap.String("agentUID", routeResult.AgentUID), zap.String("channelID", message.ChannelID), zap.Uint8("channelType", message.ChannelType))
		return fmt.Errorf("将客服分配给指定的频道失败！")
	}
	return nil

}

// 是否已经分配客服
func (h *Hotline) hasRoute(message *config.MessageResp) bool {

	value, err := h.ctx.GetRedisConn().GetString(fmt.Sprintf("%s%s", h.hotlineVisitorCachePrefix, message.FromUID))
	if err != nil {
		h.Error("获取访问的访客失败！", zap.Error(err))
		return false
	}
	return value == "1"
}

func (h *Hotline) setRouted(fromUID string) error {
	err := h.ctx.GetRedisConn().SetAndExpire(fmt.Sprintf("%s%s", h.hotlineVisitorCachePrefix, fromUID), "1", time.Hour*24)
	if err != nil {
		h.Error("查询活跃访客失败！", zap.Error(err), zap.String("fromUID", fromUID))
		return err
	}
	return nil
}
