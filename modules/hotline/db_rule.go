package hotline

import (
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/db"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/gocraft/dbr/v2"
)

type ruleDB struct {
	ctx *config.Context
	db  *dbr.Session
}

func newRuleDB(ctx *config.Context) *ruleDB {
	return &ruleDB{
		ctx: ctx,
		db:  ctx.DB(),
	}
}

// 查询有效的规则
func (r *ruleDB) queryVailDetailWithAppID(appID string) ([]*ruleDetailModel, error) {
	var models []*ruleModel
	_, err := r.db.Select("*").From("hotline_rule").Where("app_id=? and status=1", appID).OrderDir("weight", false).Load(&models)

	var details = make([]*ruleDetailModel, 0)
	if len(models) > 0 {
		ruleNos := make([]string, 0, len(models))
		for _, model := range models {
			ruleNos = append(ruleNos, model.RuleNo)
		}
		conditions, err := r.queryConditionWithRuleNos(appID, ruleNos)
		if err != nil {
			return nil, err
		}
		var conditionMap = map[string][]*conditionModel{}
		if len(conditions) > 0 {
			for _, condition := range conditions {
				ruleConditions := conditionMap[condition.RuleNo]
				if ruleConditions == nil {
					ruleConditions = make([]*conditionModel, 0)
				}
				ruleConditions = append(ruleConditions, condition)
				conditionMap[condition.RuleNo] = ruleConditions
			}
		}
		for _, model := range models {
			detail := &ruleDetailModel{}
			detail.ruleModel = *model
			detail.Conditions = conditionMap[model.RuleNo]
			details = append(details, detail)
		}
	}

	return details, err
}

func (r *ruleDB) queryConditionWithRuleNos(appID string, ruleNos []string) ([]*conditionModel, error) {
	if len(ruleNos) <= 0 {
		return nil, nil
	}
	var models []*conditionModel
	_, err := r.db.Select("*").From("hotline_condition").Where("app_id=? and rule_no in ?", appID, ruleNos).Load(&models)
	return models, err
}

func (r *ruleDB) queryRuleResultWithRuleNo(ruleNo string) (*ruleResultModel, error) {
	var model *ruleResultModel
	_, err := r.db.Select("*").From("hotline_rule_result").Where("rule_no=?", ruleNo).Load(&model)
	return model, err
}

func (r *ruleDB) insertTx(m *ruleModel, tx *dbr.Tx) error {
	_, err := tx.InsertInto("hotline_rule").Columns(util.AttrToUnderscore(m)...).Record(m).Exec()
	return err
}

func (r *ruleDB) insertConditionTx(m *conditionModel, tx *dbr.Tx) error {
	_, err := tx.InsertInto("hotline_condition").Columns(util.AttrToUnderscore(m)...).Record(m).Exec()
	return err
}

func (r *ruleDB) insertResultTx(m *ruleResultModel, tx *dbr.Tx) error {
	_, err := tx.InsertInto("hotline_rule_result").Columns(util.AttrToUnderscore(m)...).Record(m).Exec()
	return err
}

func (r *ruleDB) querySimpleWithAppID(appID string) ([]*ruleSimpleModel, error) {
	var models []*ruleSimpleModel
	_, err := r.db.Select("hotline_rule.*,IFNULL(hotline_rule_result.assign_no,'') agent_uid,IFNULL(hotline_agent.name,'') agent_name").From("hotline_rule").
		LeftJoin("hotline_rule_result", "hotline_rule.rule_no=hotline_rule_result.rule_no and hotline_rule.app_id=hotline_rule_result.app_id").
		LeftJoin("hotline_agent", "hotline_agent.uid=hotline_rule_result.assign_no and hotline_rule_result.assign_type=1  and hotline_agent.app_id=hotline_rule.app_id").OrderDir("hotline_rule.created_at", false).Where("hotline_rule.app_id=?", appID).Load(&models)
	return models, err
}

func (r *ruleDB) deleteRuleTx(ruleNo, appID string, tx *dbr.Tx) error {
	_, err := tx.DeleteFrom("hotline_rule").Where("rule_no=? and app_id=?", ruleNo, appID).Exec()
	return err
}

func (r *ruleDB) deleteConditionTx(ruleNo, appID string, tx *dbr.Tx) error {
	_, err := tx.DeleteFrom("hotline_condition").Where("rule_no=? and app_id=?", ruleNo, appID).Exec()
	return err
}

func (r *ruleDB) deleteResultTx(ruleNo, appID string, tx *dbr.Tx) error {
	_, err := tx.DeleteFrom("hotline_rule_result").Where("rule_no=? and app_id=?", ruleNo, appID).Exec()
	return err
}

func (r *ruleDB) updateStatus(status int, ruleNo, appID string) error {
	_, err := r.db.Update("hotline_rule").Set("status", status).Where("rule_no=? and app_id=?", ruleNo, appID).Exec()
	return err
}

type ruleModel struct {
	AppID      string
	RuleNo     string
	Name       string
	Expression string
	Status     int
	Weight     int
	db.BaseModel
}

type ruleSimpleModel struct {
	ruleModel
	AgentUID  string
	AgentName string
}

type ruleDetailModel struct {
	ruleModel
	Conditions []*conditionModel
}

type conditionModel struct {
	AppID     string
	RuleNo    string
	Tag       string // 条件标示
	Field     string
	Value     string
	Condition string
	db.BaseModel
}

// 规则对应的结果
type ruleResultModel struct {
	AppID      string
	RuleNo     string
	AssignType int    // 指派类型 1.分配给人 2.分配给群组
	AssignNo   string // 指派编号，如果是1.则为个人uid，如果是2.则为群组编号
	db.BaseModel
}
