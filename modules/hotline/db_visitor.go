package hotline

import (
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/db"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/gocraft/dbr/v2"
)

type visitorDB struct {
	ctx *config.Context
	db  *dbr.Session
}

func newVisitorDB(ctx *config.Context) *visitorDB {
	return &visitorDB{
		ctx: ctx,
		db:  ctx.DB(),
	}
}

func (v *visitorDB) queryWithVIDAndAppID(vid string, appID string) (*visitorModel, error) {
	var model *visitorModel
	_, err := v.db.Select("*").From("hotline_visitor").Where("app_id=? and vid=?", appID, vid).Load(&model)
	return model, err
}

func (v *visitorDB) queryWithVID(vid string) (*visitorModel, error) {
	var model *visitorModel
	_, err := v.db.Select("*").From("hotline_visitor").Where("vid=?", vid).Load(&model)
	return model, err
}

func (v *visitorDB) insertTx(m *visitorModel, tx *dbr.Tx) error {
	_, err := tx.InsertInto("hotline_visitor").Columns(util.AttrToUnderscore(m)...).Record(m).Exec()
	return err
}

func (v *visitorDB) queryWithPage(_ string, appID string, pageIndex, pageSize int64) ([]*visitorDetailModel, error) {
	var models []*visitorDetailModel
	q := v.db.Select("hotline_visitor.*,IFNULL(hotline_online.online,0) online,IFNULL(hotline_online.last_online,0) last_online,IFNULL(hotline_online.last_offline,0) last_offline").From("hotline_visitor").LeftJoin("hotline_online", "hotline_visitor.vid=hotline_online.user_id and hotline_online.user_type=0").OrderDir("created_at", false).Where("hotline_visitor.app_id=?", appID)
	_, err := q.Offset(uint64((pageIndex - 1) * pageSize)).Limit(uint64(pageSize)).Load(&models)
	return models, err
}

func (v *visitorDB) queryDetailVIDAndAppID(vid string, appID string) (*visitorDetailModel, error) {
	var m *visitorDetailModel
	_, err := v.db.Select("hotline_visitor.*,IFNULL(hotline_online.online,0) online,IFNULL(hotline_online.last_online,0) last_online,IFNULL(hotline_online.last_offline,0) last_offline").From("hotline_visitor").LeftJoin("hotline_online", "hotline_visitor.vid=hotline_online.user_id and hotline_online.user_type=0").OrderDir("created_at", false).Where("hotline_visitor.vid=? and hotline_visitor.app_id=?", vid, appID).Load(&m)
	return m, err
}

func (v *visitorDB) queryTotalWith(_ string, appID string) (int64, error) {
	var count int64
	_, err := v.db.Select("count(*)").From("hotline_visitor").Where("app_id=?", appID).Load(&count)
	return count, err
}

func (v *visitorDB) updatePhone(phone string, vid, appID string) error {
	_, err := v.db.Update("hotline_visitor").Set("phone", phone).Where("vid=? and app_id=?", vid, appID).Exec()
	return err
}

func (v *visitorDB) updateEmail(email string, vid, appID string) error {
	_, err := v.db.Update("hotline_visitor").Set("email", email).Where("vid=? and app_id=?", vid, appID).Exec()
	return err
}

func (v *visitorDB) updatePropsWithVidAndAppID(field, value string, vid, appID string) error {
	_, err := v.db.Update("hotline_visitor_props").Set("field", field).Set("value", value).Where("vid=? and app_id=?", vid, appID).Exec()
	return err
}

func (v *visitorDB) insertPropsWithVidAndAppID(m *visitorPropsModel) error {
	_, err := v.db.InsertInto("hotline_visitor_props").Columns(util.AttrToUnderscore(m)...).Record(m).Exec()
	return err
}

func (v *visitorDB) queryPropsWithFieldAndVidAndAppID(field, vid, appID string) (*visitorPropsModel, error) {
	var m *visitorPropsModel
	_, err := v.db.Select("*").From("hotline_visitor_props").Where("field=? and vid=? and app_id=?", field, vid, appID).Load(&m)
	return m, err
}

// 查询在线访客带频道的访客详情
func (v *visitorDB) queryDetailAndChannelWithOnlineAndNoAgent(appID string) ([]*visitorDetailAndChannelModel, error) {
	var models []*visitorDetailAndChannelModel
	builder := v.db.Select("hotline_visitor.*,hotline_channel.channel_id,hotline_channel.channel_type,hotline_online.last_online,IFNULL(hotline_topic.title,'') topic_name,(select count(*) from hotline_history where hotline_history.vid=hotline_visitor.vid) history_count").From("hotline_visitor").Join("hotline_channel", "hotline_visitor.app_id=hotline_channel.app_id and hotline_channel.vid=hotline_visitor.vid").Join("hotline_online", "hotline_online.user_type=0 and hotline_online.user_id=hotline_visitor.vid")
	_, err := builder.LeftJoin("hotline_topic", "hotline_channel.topic_id=hotline_topic.id").Where("hotline_online.online=? and hotline_channel.agent_uid='' and hotline_visitor.app_id=?", 1, appID).Load(&models)
	return models, err
}

type visitorModel struct {
	AppID         string
	VID           string
	Name          string
	Avatar        string
	Phone         string
	Email         string
	IPAddress     string
	State         string
	City          string
	Source        string
	SearchKeyword string
	LastSession   int64
	LastReply     int64
	Timezone      string
	SessionCount  int
	Local         int
	db.BaseModel
}

type visitorPropsModel struct {
	AppID string
	VID   string
	Field string
	Value string
	db.BaseModel
}

type visitorDetailModel struct {
	visitorModel
	Online      int
	LastOnline  int64
	LastOffline int64
}

type visitorDetailAndChannelModel struct {
	visitorModel
	ChannelID    string
	ChannelType  uint8
	HistoryCount int // 访客历史数量
	LastOnline   int
	TopicName    string
}
