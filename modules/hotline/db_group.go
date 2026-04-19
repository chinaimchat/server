package hotline

import (
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/db"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/gocraft/dbr/v2"
)

type groupDB struct {
	ctx *config.Context
	db  *dbr.Session
}

func newGroupDB(ctx *config.Context) *groupDB {

	return &groupDB{
		ctx: ctx,
		db:  ctx.DB(),
	}
}

func (g *groupDB) queryDetailWithGroupTypeAndAppID(keyword string, groupType int, appID string) ([]*groupDetailModel, error) {
	var models []*groupDetailModel

	q := g.db.Select("g.*,(select count(*) from hotline_members m where m.group_no=g.group_no) member_count").From("hotline_group g").Where("group_type=? and app_id=?", groupType, appID)
	if keyword != "" {
		q = q.Where("name like ?", "%"+keyword+"%")
	}
	_, err := q.Load(&models)

	return models, err
}

func (g *groupDB) insertTx(m *groupModel, tx *dbr.Tx) error {
	_, err := tx.InsertInto("hotline_group").Columns(util.AttrToUnderscore(m)...).Record(m).Exec()
	return err
}

func (g *groupDB) insert(m *groupModel) error {
	_, err := g.db.InsertInto("hotline_group").Columns(util.AttrToUnderscore(m)...).Record(m).Exec()
	return err
}

func (g *groupDB) insertMember(m *memberModel) error {
	_, err := g.db.InsertInto("hotline_members").Columns(util.AttrToUnderscore(m)...).Record(m).Exec()
	return err
}

func (g *groupDB) insertMemberTx(m *memberModel, tx *dbr.Tx) error {
	_, err := tx.InsertInto("hotline_members").Columns(util.AttrToUnderscore(m)...).Record(m).Exec()
	return err
}

type groupDetailModel struct {
	groupModel
	MemberCount int64 // 成员数量
}

type groupModel struct {
	AppID              string
	GroupNo            string // 群组唯一编号
	GroupType          int    // 群类型 1.普通群 2.技能组
	CreaterUID         string // 创建者uid
	Name               string // 群组名字
	Remark             string // 群组备注
	IntelliAssign      int    // 是否开启智能分配
	SessionActiveLimit int    // 每名成员的活动对话限制
	Status             int
	db.BaseModel
}

type memberModel struct {
	AppID   string
	GroupNo string // 群唯一编号
	UID     string // 成员唯一编号
	db.BaseModel
}
