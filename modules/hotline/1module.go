package hotline

import (
	"embed"
	"errors"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/common"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/model"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/register"
	"go.uber.org/zap"
)

//go:embed sql
var sqlFS embed.FS

//go:embed swagger/api.yaml
var swaggerContent string

func init() {
	register.AddModule(func(ctx interface{}) register.Module {

		api := New(ctx.(*config.Context))
		return register.Module{
			Name: "hotline",
			SetupAPI: func() register.APIRouter {
				return api
			},
			SQLDir:  register.NewSQLFS(sqlFS),
			Swagger: swaggerContent,
			IMDatasource: register.IMDatasource{
				HasData: func(channelID string, channelType uint8) register.IMDatasourceType {
					if channelType == common.ChannelTypeCustomerService.Uint8() {
						return register.IMDatasourceTypeSubscribers
					}
					return register.IMDatasourceTypeNone
				},
				Subscribers: func(channelID string, channelType uint8) ([]string, error) {
					if channelType != common.ChannelTypeCustomerService.Uint8() { // 一定要返回 register.ErrDatasourceNotProcess
						return nil, nil
					}
					subscriberResps, err := api.service.SubscribersGet(channelID, channelType)
					if err != nil {
						return nil, err
					}
					subscribers := make([]string, 0)
					if len(subscriberResps) > 0 {
						for _, subscriberResp := range subscriberResps {
							subscribers = append(subscribers, subscriberResp.Subscriber)
						}
					}
					return subscribers, nil
				},
			},
			BussDataSource: register.BussDataSource{
				ChannelGet: func(channelID string, channelType uint8, loginUID string) (*model.ChannelResp, error) {
					if channelType != common.ChannelTypeCustomerService.Uint8() {
						return nil, register.ErrDatasourceNotProcess
					}
					channelR, err := api.service.ChannelDetailGet(channelID, channelType, loginUID)
					if err != nil {
						api.Error("查询客服频道失败！", zap.Error(err))
						return nil, err
					}
					if channelR == nil {
						api.Error("客服频道不存在！", zap.String("channel_id", channelID))
						return nil, errors.New("客服频道不存在！")
					}
					return newChannelRespWithHotlineChannelResp(channelR), nil
				},
			},
		}
	})
}

func newChannelRespWithHotlineChannelResp(hotlineResp *ChannelResp) *model.ChannelResp {
	resp := &model.ChannelResp{}
	resp.Channel.ChannelID = hotlineResp.ChannelID
	resp.Channel.ChannelType = hotlineResp.ChannelType
	resp.Online = hotlineResp.Online
	resp.LastOffline = hotlineResp.LastOffline
	resp.Receipt = hotlineResp.Receipt
	resp.Name = hotlineResp.Name
	resp.Category = hotlineResp.Category
	resp.Logo = hotlineResp.Logo
	resp.Status = 1
	resp.Extra = hotlineResp.Extra

	return resp
}
