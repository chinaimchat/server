package hotline

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/user"
	wkutil "github.com/TangSengDaoDao/TangSengDaoDaoServer/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/log"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"go.uber.org/zap"
)

type IService interface {
	// 获取频道详情
	ChannelDetailGet(channelID string, channelType uint8, loginUID string) (*ChannelResp, error)

	// 获取订阅者
	SubscribersGet(channelID string, channelType uint8) ([]*SubscriberResp, error)
}

type Service struct {
	ctx         *config.Context
	channelDB   *channelDB
	onlineDB    *onlineDB
	configDB    *configDB
	visitorDB   *visitorDB
	deviceDB    *deviceDB
	userService user.IService
	log.Log
}

func NewService(ctx *config.Context) IService {

	return &Service{
		ctx:         ctx,
		Log:         log.NewTLog("hotlineService"),
		channelDB:   newChannelDB(ctx),
		onlineDB:    newOnlineDB(ctx),
		configDB:    newConfigDB(ctx),
		visitorDB:   newVisitorDB(ctx),
		deviceDB:    newDeviceDB(ctx),
		userService: user.NewService(ctx),
	}
}

func (s *Service) SubscribersGet(channelID string, channelType uint8) ([]*SubscriberResp, error) {
	subscriberModels, err := s.channelDB.querySubscribers(channelID, channelType)
	if err != nil {
		s.Error("查询订阅者失败！", zap.Error(err))
		return nil, err
	}
	if len(subscriberModels) == 0 {
		return nil, nil
	}
	subscribers := make([]*SubscriberResp, 0, len(subscriberModels))
	for _, subscriberM := range subscriberModels {
		subscribers = append(subscribers, newSubscriberResp(subscriberM))
	}
	return subscribers, nil
}

func (s *Service) ChannelDetailGet(channelID string, channelType uint8, loginUID string) (*ChannelResp, error) {

	channelM, err := s.channelDB.queryDetailWithChannelID(channelID, uint8(channelType))
	if err != nil {
		s.Error("查询频道详情失败！", zap.Error(err))
		return nil, fmt.Errorf("查询频道详情失败！")
	}
	if channelM == nil {
		s.Error("频道不存在！", zap.String("channelID", channelID))
		return nil, nil
	}

	vid := channelM.VID
	appID := channelM.AppID

	onlineM, err := s.onlineDB.queryWithUserIDAndDeviceFlag(channelM.VID, config.Web.Uint8())
	if err != nil {
		s.Error("查询在线数据失败！", zap.Error(err))
		return nil, fmt.Errorf("查询在线数据失败！")
	}
	var online int
	var lastOffline int
	if onlineM != nil {
		online = onlineM.Online
		if online != 1 {
			lastOffline = onlineM.LastOffline
		}
	}

	extraMap := map[string]interface{}{
		"topic_id":   channelM.TopicID,
		"topic_name": channelM.TopicName,
		"agent_uid":  channelM.AgentUID,
		"agent_name": channelM.AgentName,
		"category":   channelM.Category,
		"vid":        channelM.VID,
	}

	isVisitor := vid == loginUID
	logo := ""
	name := ""
	var category UserCategory
	if isVisitor { // 访客看到的是客服的资料
		logo = fmt.Sprintf("hotline/apps/%s/avatar", channelM.AppID)
		configM, err := s.configDB.queryWithAppID(channelM.AppID)
		if err != nil {
			return nil, errors.New("获取客服配置失败！")
		}
		if configM == nil {
			// hotline_config 未预置时，旧客户端/部署会直接 400（影响聊天列表加载）。
			// 这里降级处理：返回可用的默认 name/logo，避免接口硬失败。
			s.Warn("hotline_config missing for app_id", zap.String("app_id", channelM.AppID))
			appName := strings.TrimSpace(s.ctx.GetConfig().AppName)
			if appName == "" {
				appName = channelM.AppID
			}
			name = appName
			category = UserCategoryCustomerService
		} else {
			name = configM.AppName
			category = UserCategoryCustomerService
		}

	} else { // 客服看到的是访客的资料
		visitorM, err := s.visitorDB.queryWithVID(vid)
		if err != nil {
			s.Error("查询访客数据失败！", zap.Error(err))
			return nil, err
		}
		if visitorM == nil {
			s.Error("访客数据不存在！", zap.String("vid", vid))
			return nil, fmt.Errorf("访客数据不存在！")
		}
		phone := visitorM.Phone
		email := visitorM.Email
		if visitorM.Local == 1 {
			userResp, err := s.userService.GetUserDetail(vid, loginUID)
			if err != nil {
				s.Error("获取本地用户数据失败！", zap.Error(err), zap.String("uid", vid))
				return nil, err
			}
			if userResp == nil {
				s.Error("本地用户不存在！", zap.String("uid", vid))
				return nil, errors.New("本地用户不存在！")
			}
			online = userResp.Online
			lastOffline = userResp.LastOffline
			name = userResp.Name
			logo = wkutil.AvatarAPIRelativePath(channelM.VID)
			if phone == "" {
				phone = userResp.Phone
			}
			if email == "" {
				email = userResp.Email
			}

		} else {
			name = channelM.Title
			logo = fmt.Sprintf("hotline/visitors/%s/avatar", channelM.VID)
		}
		category = UserCategoryVisitor

		deviceM, err := s.deviceDB.queryWithVIDAndAppID(vid, appID)
		if err != nil {
			s.Error("获取设备信息失败！", zap.Error(err))
			return nil, err
		}
		extraMap["phone"] = phone
		extraMap["email"] = email
		extraMap["source"] = channelM.Source
		extraMap["ip_address"] = visitorM.IPAddress
		extraMap["city"] = visitorM.City
		extraMap["state"] = visitorM.State
		extraMap["created_at"] = util.ToyyyyMMddHHmm(time.Time(visitorM.CreatedAt))
		if visitorM.City == visitorM.State {
			extraMap["address"] = visitorM.City
		} else {
			extraMap["address"] = fmt.Sprintf("%s%s", visitorM.City, visitorM.State)
		}

		if deviceM != nil {
			extraMap["device"] = map[string]interface{}{
				"os":      deviceM.OS,
				"model":   deviceM.Model,
				"device":  deviceM.Device,
				"version": deviceM.Version,
			}

		}
	}

	return &ChannelResp{
		ChannelID:   channelID,
		ChannelType: channelType,
		Name:        name,
		Online:      online,
		Logo:        logo,
		Category:    string(category),
		LastOffline: int64(lastOffline),
		Receipt:     1,
		Extra:       extraMap,
	}, nil
}

type ChannelResp struct {
	ChannelID   string                 `json:"channel_id"`
	ChannelType uint8                  `json:"channel_type"`
	Name        string                 `json:"name"`
	Logo        string                 `json:"logo"`
	Category    string                 `json:"category"`
	Online      int                    `json:"online"`
	LastOffline int64                  `json:"last_offline"`
	Receipt     int                    `json:"receipt"`
	Extra       map[string]interface{} `json:"extra"`
}

type SubscriberResp struct {
	AppID          string         // appid
	Subscriber     string         // 订阅者ID
	SubscriberType SubscriberType // 订阅者类型
}

func newSubscriberResp(m *subscribersModel) *SubscriberResp {

	return &SubscriberResp{
		AppID:          m.AppID,
		Subscriber:     m.Subscriber,
		SubscriberType: SubscriberType(m.SubscriberType),
	}
}
