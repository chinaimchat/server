package hotline

import (
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/db"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/gocraft/dbr/v2"
)

type agentDB struct {
	ctx *config.Context
	db  *dbr.Session
}

func newAgentDB(ctx *config.Context) *agentDB {
	return &agentDB{
		ctx: ctx,
		db:  ctx.DB(),
	}
}

// 查询工作中的客服列表
func (a *agentDB) queryWorkingWithAppID(appID string) ([]*agentModel, error) {
	var models []*agentModel
	_, err := a.db.Select("*").From("hotline_agent").Where("app_id=? and is_work=1", appID).Load(&models)
	return models, err
}

func (a *agentDB) queryWithUIDAndAppID(uid string, appID string) (*agentModel, error) {
	var m *agentModel
	_, err := a.db.Select("*").From("hotline_agent").Where("uid=? and app_id=?", uid, appID).Load(&m)
	return m, err
}

func (a *agentDB) insert(m *agentModel) error {
	_, err := a.db.InsertInto("hotline_agent").Columns(util.AttrToUnderscore(m)...).Record(m).Exec()
	return err
}

func (a *agentDB) queryWithUID(uid string) ([]*agentModel, error) {
	var models []*agentModel
	_, err := a.db.Select("*").From("hotline_agent").Where("uid=? and status=1", uid).OrderDir("created_at", false).Load(&models)
	return models, err
}

func (a *agentDB) insertTx(m *agentModel, tx *dbr.Tx) error {
	_, err := tx.InsertInto("hotline_agent").Columns(util.AttrToUnderscore(m)...).Record(m).Exec()
	return err
}

func (a *agentDB) queryWith(role string, name string, skillGroupNo string, groupNo string, status string, working string, pIndex int64, pSize int64, appID string) ([]*agentDetailModel, error) {
	sql := `
	select e.* from (select d.app_id,d.uid,d.name,d.role,d.status,d.is_work,d.skill_no,d.skill_name ,GROUP_CONCAT(d.group_no) group_nos,GROUP_CONCAT(d.group_name) group_names from 
	(select ag.*,IFNULL(b.group_no,'') skill_no,IFNULL(b.name,'') skill_name,IFNULL(c.group_no,'') group_no,IFNULL(c.name,'') group_name   from hotline_agent ag 
	left join (select a.* from (select g.app_id,g.group_no,g.name,m.uid from hotline_members m  join hotline_group g on m.group_no=g.group_no and g.group_type=2) a) b on ag.app_id=b.app_id and ag.uid=b.uid
	left join (select a.* from (select g.app_id,g.group_no,g.name,m.uid from hotline_members m  join hotline_group g on m.group_no=g.group_no and g.group_type=1) a) c on ag.app_id=c.app_id and ag.uid=c.uid ) d  
	group by d.app_id,d.uid,d.role,d.status,d.is_work,d.skill_no,d.skill_name) e where 1=1 `

	wheres := make([]string, 0)
	args := make([]interface{}, 0)

	wheres = append(wheres, "e.app_id=?")
	args = append(args, appID)
	if role != "" {
		wheres = append(wheres, "e.role=?")
		args = append(args, role)
	}
	if skillGroupNo != "" {
		wheres = append(wheres, "e.skill_no=?")
		args = append(args, skillGroupNo)
	}

	if groupNo != "" {
		wheres = append(wheres, "e.group_no=?")
		args = append(args, groupNo)
	}
	if status != "" {
		wheres = append(wheres, "e.status=?")
		args = append(args, status)
	}
	if name != "" {
		wheres = append(wheres, "e.name like ?")
		args = append(args, "%"+name+"%")
	}
	if working != "" {
		wheres = append(wheres, "e.is_work=?")
		args = append(args, working)
	}

	for _, where := range wheres {
		sql += "and " + where
	}
	args = append(args, uint64((pIndex-1)*pSize), uint64(pSize))

	sql += " limit ?,?"

	q := a.db.SelectBySql(sql, args...)

	var models []*agentDetailModel
	_, err := q.Load(&models)
	return models, err
}
func (a *agentDB) queryTotalWith(role string, name string, skillGroupNo string, groupNo string, status string, working string, appID string) (int64, error) {
	sql := `
	select count(e.uid) from (select d.app_id,d.uid,d.name,d.role,d.status,d.is_work,d.skill_no,d.skill_name ,GROUP_CONCAT(d.group_no) group_nos,GROUP_CONCAT(d.group_name) group_names from 
	(select ag.*,IFNULL(b.group_no,'') skill_no,IFNULL(b.name,'') skill_name,IFNULL(c.group_no,'') group_no,IFNULL(c.name,'') group_name  from hotline_agent ag 
	left join (select a.* from (select g.app_id,g.group_no,g.name,m.uid from hotline_members m  join hotline_group g on m.group_no=g.group_no and g.group_type=2) a) b on ag.app_id=b.app_id and ag.uid=b.uid
	left join (select a.* from (select g.app_id,g.group_no,g.name,m.uid from hotline_members m  join hotline_group g on m.group_no=g.group_no and g.group_type=1) a) c on ag.app_id=c.app_id and ag.uid=c.uid ) d  
	group by d.app_id,d.uid,d.role,d.status,d.is_work,d.skill_no,d.skill_name) e where 1=1 `

	wheres := make([]string, 0)
	args := make([]interface{}, 0)

	wheres = append(wheres, "e.app_id=?")
	args = append(args, appID)
	if role != "" {
		wheres = append(wheres, "e.role=?")
		args = append(args, role)
	}
	if skillGroupNo != "" {
		wheres = append(wheres, "e.skill_no=?")
		args = append(args, skillGroupNo)
	}

	if groupNo != "" {
		wheres = append(wheres, "e.group_no=?")
		args = append(args, groupNo)
	}
	if status != "" {
		wheres = append(wheres, "e.status=?")
		args = append(args, status)
	}
	if name != "" {
		wheres = append(wheres, "e.name like ?")
		args = append(args, "%"+name+"%")
	}
	if working != "" {
		wheres = append(wheres, "e.is_work=?")
		args = append(args, working)
	}

	for _, where := range wheres {
		sql += "and " + where
	}

	q := a.db.SelectBySql(sql, args...)

	var total int64
	_, err := q.Load(&total)
	return total, err
}

func (a *agentDB) updateIsWork(isWork int, uid, appID string) error {
	_, err := a.db.Update("hotline_agent").Set("is_work", isWork).Where("uid=? and app_id=?", uid, appID).Exec()
	return err
}

func (a *agentDB) updatePosition(position string, uid, appID string) error {
	_, err := a.db.Update("hotline_agent").Set("position", position).Where("uid=? and app_id=?", uid, appID).Exec()
	return err
}

type agentModel struct {
	AppID      string
	UID        string // 客服uid
	Name       string // 客服名称
	LastActive int    // 最后一次活动时间 10位时间戳（单位秒）
	Role       string
	Position   string // 职位
	IsWork     int    // 是否是工作状态
	Status     int
	db.BaseModel
}

type agentDetailModel struct {
	agentModel
	SkillNo    string // 技能组编号
	SkillName  string // 技能组名称
	GroupNos   string // 群组编号
	GroupNames string // 群组名称
}
