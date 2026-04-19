package hotline

import (
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/db"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/gocraft/dbr/v2"
)

// 智能分配db
type intelliAssignDB struct {
	ctx *config.Context
	db  *dbr.Session
}

func newIntelliAssignDB(ctx *config.Context) *intelliAssignDB {
	return &intelliAssignDB{
		ctx: ctx,
		db:  ctx.DB(),
	}
}

// 查询有效的智能配置
func (i *intelliAssignDB) queryVaildWithAppID(appID string) (*intelliAssignModel, error) {
	var model *intelliAssignModel
	_, err := i.db.Select("*").From("hotline_intelli_assign").Where("app_id=? and status=1", appID).Load(&model)
	return model, err
}

func (i *intelliAssignDB) queryWithAppID(appID string) (*intelliAssignModel, error) {
	var model *intelliAssignModel
	_, err := i.db.Select("*").From("hotline_intelli_assign").Where("app_id=?", appID).Load(&model)
	return model, err
}

func (i *intelliAssignDB) insert(m *intelliAssignModel) error {
	_, err := i.db.InsertInto("hotline_intelli_assign").Columns(util.AttrToUnderscore(m)...).Record(m).Exec()
	return err
}
func (i *intelliAssignDB) update(m *intelliAssignModel) error {
	_, err := i.db.Update("hotline_intelli_assign").SetMap(map[string]interface{}{
		"user_max_idle":           m.UserMaxIdle,
		"visitor_max_idle":        m.VisitorMaxIdle,
		"session_remember_min":    m.SessionRememberMin,
		"session_remember_enable": m.SessionRememberEnable,
		"session_active_limit":    m.SessionActiveLimit,
		"status":                  m.Status,
	}).Where("id=?", m.Id).Exec()
	return err
}

func (i *intelliAssignDB) updateStatus(status int, id int64) error {
	_, err := i.db.Update("hotline_intelli_assign").Set("status", status).Where("id=?", id).Exec()
	return err
}

type intelliAssignModel struct {
	AppID                 string
	Strategy              string // 策略
	UserMaxIdle           int    // 成员最大空闲(单位分钟),如果成员达到最大空闲，则将其设为非活动状态'
	VisitorMaxIdle        int    // 访客最大空闲(单位分钟),如果访客达到最大空闲，则将其设为非活动状态
	SessionRememberEnable int    // 会话记住是否开启
	SessionRememberMin    int    // 对话记忆时间(单位分钟) 如果对话在此时间内重新打开，则将其重新指派给同一成员
	SessionActiveLimit    int    // 每名成员的活动对话限制  不能为0 为0将不会分配
	Status                int    // 0.禁用 1.启用
	db.BaseModel
}
