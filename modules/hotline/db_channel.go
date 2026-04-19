package hotline

import (
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/db"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/gocraft/dbr/v2"
)

type channelDB struct {
	ctx *config.Context
	db  *dbr.Session
}

func newChannelDB(ctx *config.Context) *channelDB {
	return &channelDB{
		ctx: ctx,
		db:  ctx.DB(),
	}
}

func (v *channelDB) queryWithIDsAndVid(topicIDs []int64, vid string) ([]*channelModel, error) {
	if len(topicIDs) <= 0 {
		return nil, nil
	}
	var models []*channelModel
	_, err := v.db.Select("*").From("hotline_channel").Where("vid=? and topic_id in ?", vid, topicIDs).Load(&models)
	return models, err
}

// 通过访客id查询最新的一条频道数据
func (v *channelDB) queryLastWithVid(vid string) (*channelModel, error) {
	var m *channelModel
	_, err := v.db.Select("*").From("hotline_channel").Where("vid=?", vid).OrderDesc("created_at").Load(&m)
	return m, err
}

func (v *channelDB) queryWithChannelID(channelID string, channelType uint8) (*channelModel, error) {
	var m *channelModel
	_, err := v.db.Select("*").From("hotline_channel").Where("channel_id=? and channel_type=?", channelID, channelType).Load(&m)
	return m, err
}

func (v *channelDB) queryDetailWithChannelID(channelID string, channelType uint8) (*channelDetailModel, error) {
	var m *channelDetailModel
	_, err := v.db.Select("hotline_channel.*,IFNULL(hotline_topic.title,'') topic_name,IFNULL(hotline_visitor.source,'') source,IFNULL(hotline_agent.name,'') agent_name").From("hotline_channel").LeftJoin("hotline_visitor", "hotline_visitor.vid=hotline_channel.vid").LeftJoin("hotline_topic", "hotline_channel.topic_id=hotline_topic.id").LeftJoin("hotline_agent", "hotline_channel.agent_uid=hotline_agent.uid and hotline_channel.app_id=hotline_agent.app_id").Where("channel_id=? and channel_type=?", channelID, channelType).Load(&m)
	return m, err
}

func (v *channelDB) queryPage(busType string, appID string, pIndex, pSize int64) ([]*channelDetailModel, error) {
	builder := v.db.Select("hotline_channel.*,IFNULL(hotline_topic.title,'') topic_name,IFNULL(hotline_agent.name,'') agent_name,(select count(*) from hotline_subscribers where hotline_subscribers.channel_id=hotline_channel.channel_id and hotline_subscribers.channel_type=hotline_channel.channel_type) member_count").From("hotline_channel").LeftJoin("hotline_topic", "hotline_channel.topic_id=hotline_topic.id").LeftJoin("hotline_agent", "hotline_channel.agent_uid=hotline_agent.uid and hotline_channel.app_id=hotline_agent.app_id").Where("hotline_channel.app_id=?", appID)
	if busType != "" {
		builder = builder.Where("hotline_channel.bus_type=?", busType)
	}
	var models []*channelDetailModel
	_, err := builder.Offset(uint64((pIndex-1)*pSize)).OrderDir("hotline_channel.created_at", false).Limit(uint64(pSize)).Load(&models)
	return models, err
}

func (v *channelDB) queryCount(busType string, appID string) (int64, error) {
	builder := v.db.Select("count(*)").From("hotline_channel").LeftJoin("hotline_topic", "hotline_channel.topic_id=hotline_topic.id").LeftJoin("hotline_agent", "hotline_channel.agent_uid=hotline_agent.uid and hotline_channel.app_id=hotline_agent.app_id").Where("hotline_channel.app_id=?", appID)
	if busType != "" {
		builder = builder.Where("hotline_channel.bus_type=?", busType)
	}
	var cn int64
	_, err := builder.Load(&cn)
	return cn, err
}

func (v *channelDB) existSubscriber(subscriber string, channelID string, channelType uint8) (bool, error) {
	var cn int
	_, err := v.db.Select("count(*)").From("hotline_subscribers").Where("channel_id=? and channel_type=? and subscriber=?", channelID, channelType, subscriber).Load(&cn)
	return cn > 0, err
}

func (v *channelDB) insertTx(m *channelModel, tx *dbr.Tx) error {
	_, err := tx.InsertInto("hotline_channel").Columns(util.AttrToUnderscore(m)...).Record(m).Exec()
	return err
}

func (v *channelDB) deleteWithChannelID(channelID string) error {
	_, err := v.db.DeleteFrom("hotline_channel").Where("channel_id=?", channelID).Exec()
	return err
}

func (v *channelDB) deleteSubscribersWithChannelID(channelID string) error {
	_, err := v.db.DeleteFrom("hotline_subscribers").Where("channel_id=?", channelID).Exec()
	return err
}

func (v *channelDB) updateAgentUIDAndBindTx(agentUID string, bind string, channelID string, channelType uint8, tx *dbr.Tx) error {
	_, err := tx.Update("hotline_channel").Set("agent_uid", agentUID).Set("bind", bind).Where("channel_id=? and channel_type=?", channelID, channelType).Exec()
	return err
}

func (v *channelDB) updateCategory(category, channelID string, channelType uint8) error {
	_, err := v.db.Update("hotline_channel").Set("category", category).Where("channel_id=? and channel_type=?", channelID, channelType).Exec()
	return err
}

func (v *channelDB) updateCategoryAndAgentUID(category, agentUID string, channelID string, channelType uint8) error {
	_, err := v.db.Update("hotline_channel").Set("agent_uid", agentUID).Set("category", category).Where("channel_id=? and channel_type=?", channelID, channelType).Exec()
	return err
}

func (v *channelDB) insertSubscriberTx(subscriber *subscribersModel, tx *dbr.Tx) error {
	_, err := tx.InsertInto("hotline_subscribers").Columns(util.AttrToUnderscore(subscriber)...).Record(subscriber).Exec()
	return err
}

func (v *channelDB) deleteSubscriberWithBusType(subscriber string, subscriberType int, busType int, appID string) error {
	_, err := v.db.DeleteFrom("hotline_subscribers").Where("subscriber=? and subscriber_type=? and bus_type=? and app_id=?", subscriber, subscriberType, busType, appID).Exec()
	return err
}

func (v *channelDB) querySubscribers(channelID string, channelType uint8) ([]*subscribersModel, error) {
	var models []*subscribersModel
	_, err := v.db.Select("*").From("hotline_subscribers").Where("channel_id=? and channel_type=?", channelID, channelType).Load(&models)
	return models, err
}

func (v *channelDB) queryBindSubscribers(channelID string, channelType uint8) ([]*subscribersModel, *channelModel, error) {
	channel, err := v.queryWithChannelID(channelID, channelType)
	if err != nil {
		return nil, nil, err
	}
	if channel == nil {
		return nil, nil, nil
	}
	newSubscribers := make([]*subscribersModel, 0)

	subscribers, err := v.querySubscribers(channelID, channelType)
	if err != nil {
		return nil, nil, err
	}
	if len(subscribers) > 0 {
		newSubscribers = append(newSubscribers, subscribers...)
	}

	if len(channel.Bind) > 0 {
		subscribers, err = v.querySubscribers(channel.Bind, channelType)
		if err != nil {
			return nil, nil, err
		}
		if len(subscribers) > 0 {
			newSubscribers = append(newSubscribers, subscribers...)
		}
	}
	return newSubscribers, channel, nil

}

func (v *channelDB) querySessionActiveLimitWithAppID(appID string) ([]*channelAgentSessionActiveLimit, error) {
	var models []*channelAgentSessionActiveLimit
	_, err := v.db.Select("hotline_subscribers.subscriber uid,hotline_channel.session_active_limit").From("hotline_channel").LeftJoin("hotline_subscribers", "hotline_channel.channel_id=hotline_subscribers.channel_id and hotline_channel.channel_type=hotline_subscribers.channel_type").GroupBy("hotline_subscribers.subscriber", "hotline_channel.session_active_limit", "hotline_channel.app_id").Where("hotline_channel.app_id=? and hotline_channel.bus_type=? and hotline_channel.disable=0", appID, ChannelBusTypeSkill.Int()).Load(&models)
	return models, err
}

func (v *channelDB) updateDisable(disable int, channelID string, channelType uint8) error {
	_, err := v.db.Update("hotline_channel").Set("disable", disable).Where("channel_id=? and channel_type=?", channelID, channelType).Exec()
	return err
}

// 是否存在指定访客和topic的频道
func (v *channelDB) queryVisitorChannel(vid string, topicID int64, appID string) (*channelModel, error) {
	var model *channelModel
	_, err := v.db.Select("*").From("hotline_channel").Where("topic_id=? and vid=? and app_id=?", topicID, vid, appID).Load(&model)
	return model, err
}

type channelAgentSessionActiveLimit struct {
	UID                string
	SessionActiveLimit int
}

type channelDetailModel struct {
	channelModel
	TopicName   string
	AgentName   string
	MemberCount int
	Source      string // 来源
}

type channelModel struct {
	AppID              string
	VID                string
	TopicID            int64
	Title              string
	ChannelID          string
	ChannelType        uint8
	AgentUID           string
	Category           string
	Bind               string
	Disable            int
	BusType            int // 频道业务类型 0.访客频道 1.技能组 2.普通群组
	SessionActiveLimit int // 最大会话限制
	db.BaseModel
}

type subscribersModel struct {
	AppID          string
	ChannelID      string
	ChannelType    uint8
	SubscriberType int    // 0.访客 1.客服
	Subscriber     string // 订阅者ID 0.位vid 1.为uid
	db.BaseModel
}
