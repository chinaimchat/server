package hotline

import (
	"errors"
	"net/http"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"github.com/gin-gonic/gin"
)

func (h *Hotline) widgetConfigUpdate(c *wkhttp.Context) {
	var req struct {
		AppName string `json:"app_name"`
		Logo    string `json:"logo"`
		Color   string `json:"color"`
		ChatBg  string `json:"chat_bg"`
	}

	if err := c.BindJSON(&req); err != nil {
		c.ResponseErrorf("数据格式错误！", err)
		return
	}
	configM, err := h.configDB.queryWithAppID(c.GetAppID())
	if err != nil {
		c.ResponseErrorf("获取配置信息失败！", err)
		return
	}
	if configM == nil {
		c.ResponseError(errors.New("配置不存在！"))
		return
	}

	err = h.configDB.update(&configModel{
		AppID:   c.GetAppID(),
		AppName: req.AppName,
		Logo:    req.Logo,
		Color:   req.Color,
		ChatBg:  req.ChatBg,
	})
	if err != nil {
		c.ResponseErrorf("更新配置失败！", err)
		return
	}
	c.ResponseOK()
}

// 获取咨询web端的配置
func (h *Hotline) widgetConfig(c *wkhttp.Context) {
	appID := c.Param("app_id")

	configM, err := h.configDB.queryWithAppID(appID)
	if err != nil {
		c.ResponseErrorf("appID不存在！", err)
		return
	}
	if configM == nil {
		c.ResponseError(errors.New("没有获取到配置信息！"))
		return
	}
	c.Response(newConfigResp(configM))
}

// 获取widget
func (h *Hotline) widget(c *wkhttp.Context) {
	// appID := c.Query("app_id")
	// domain := c.Query("domain")
	// referer := c.Query("referer")

	c.HTML(http.StatusOK, "hotline/index.html", gin.H{
		"base_url": h.ctx.GetConfig().External.BaseURL + "/hotline/",
	})

}

// 获取widget信息
func (h *Hotline) widgetInfoGet(c *wkhttp.Context) {
	appID := c.Param("app_id")
	topicModels, err := h.topicDB.queryWithAppID(appID)
	if err != nil {
		c.ResponseErrorf("查询topic失败！", err)
		return
	}
	var widgetInfo = widgetInfoResp{}
	topicResps := make([]*topicResp, 0)
	if len(topicModels) > 0 {
		topicIds := make([]int64, 0, len(topicModels))
		for _, topicM := range topicModels {
			topicIds = append(topicIds, topicM.Id)
			topicResps = append(topicResps, newTopicResp(topicM))
		}
		channels, err := h.channelDB.queryWithIDsAndVid(topicIds, c.GetLoginUID())
		if err != nil {
			c.ResponseErrorf("查询信息失败！", err)
			return
		}
		channelMap := map[int64]*channelModel{}
		if len(channels) > 0 {
			for _, channel := range channels {
				channelMap[channel.TopicID] = channel
			}

			for _, topicResp := range topicResps {
				channel := channelMap[topicResp.ID]
				if channel != nil {
					topicResp.ChannelID = channel.ChannelID
					topicResp.ChannelType = channel.ChannelType
				}
			}
		}

	}
	widgetInfo.Topics = topicResps

	c.Response(widgetInfo)

}

type configResp struct {
	Logo    string `json:"logo"`
	Color   string `json:"color"`
	ChatBg  string `json:"chat_bg"`
	AppName string `json:"app_name"`
}

func newConfigResp(m *configModel) *configResp {
	return &configResp{
		Logo:    m.Logo,
		Color:   m.Color,
		ChatBg:  m.ChatBg,
		AppName: m.AppName,
	}
}

type widgetInfoResp struct {
	Topics []*topicResp `json:"topics"`
}

type topicResp struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Welcome     string `json:"welcome"`
	ChannelID   string `json:"channel_id"`
	ChannelType uint8  `json:"channel_type"`
}

func newTopicResp(m *topicModel) *topicResp {

	return &topicResp{
		ID:      m.Id,
		Title:   m.Title,
		Welcome: m.Welcome,
	}
}
