package hotline

import (
	"errors"
	"fmt"
	"strings"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
)

func (h *Hotline) ruleList(c *wkhttp.Context) {
	ruleSimpleModels, err := h.ruleDB.querySimpleWithAppID(c.GetAppID())
	if err != nil {
		c.ResponseErrorf("查询规则列表失败！", err)
		return
	}
	resps := make([]*ruleSimpleResp, 0)
	if len(ruleSimpleModels) > 0 {
		for _, ruleSimpleM := range ruleSimpleModels {
			resps = append(resps, newRuleSimpleResp(ruleSimpleM))
		}
	}
	c.Response(resps)
}

func (h *Hotline) ruleStatus(c *wkhttp.Context) {
	appID := c.GetAppID()
	ruleNo := c.Param("rule_no")
	statusStr := c.Param("status")

	status := 0
	if strings.ToLower(statusStr) == "enable" {
		status = 1
	}
	err := h.ruleDB.updateStatus(status, ruleNo, appID)
	if err != nil {
		c.ResponseErrorf("更新规则状态失败！", err)
		return
	}
	c.ResponseOK()
}

func (h *Hotline) ruleDelete(c *wkhttp.Context) {
	appID := c.GetAppID()
	ruleNo := c.Param("rule_no")

	tx, _ := h.ctx.DB().Begin()
	defer func() {
		if err := recover(); err != nil {
			tx.RollbackUnlessCommitted()
			panic(err)
		}
	}()
	err := h.ruleDB.deleteRuleTx(ruleNo, appID, tx)
	if err != nil {
		tx.Rollback()
		c.ResponseError(errors.New("删除规则失败！"))
		return
	}
	err = h.ruleDB.deleteConditionTx(ruleNo, appID, tx)
	if err != nil {
		tx.Rollback()
		c.ResponseError(errors.New("删除条件失败！"))
		return
	}
	err = h.ruleDB.deleteResultTx(ruleNo, appID, tx)
	if err != nil {
		tx.Rollback()
		c.ResponseError(errors.New("删除结果失败！"))
		return
	}
	if err := tx.Commit(); err != nil {
		tx.Rollback()
		c.ResponseError(errors.New("提交事务失败！"))
		return
	}
	c.ResponseOK()
}

func (h *Hotline) ruleAdd(c *wkhttp.Context) {
	var req ruleReq
	if err := c.BindJSON(&req); err != nil {
		c.ResponseErrorf("数据格式有误！", err)
		return
	}
	ruleM, conditions := req.toModel(c.GetAppID())

	tx, _ := h.ctx.DB().Begin()
	defer func() {
		if err := recover(); err != nil {
			tx.RollbackUnlessCommitted()
			panic(err)
		}
	}()
	err := h.ruleDB.insertTx(ruleM, tx)
	if err != nil {
		tx.Rollback()
		c.ResponseErrorf("添加规则失败！", err)
		return
	}

	if len(conditions) > 0 {
		for _, condition := range conditions {
			err = h.ruleDB.insertConditionTx(condition, tx)
			if err != nil {
				tx.Rollback()
				c.ResponseErrorf("添加条件失败！", err)
				return
			}
		}
	}
	err = h.ruleDB.insertResultTx(&ruleResultModel{
		AppID:      c.GetAppID(),
		RuleNo:     ruleM.RuleNo,
		AssignType: AssignTypePerson.Int(),
		AssignNo:   req.AgentUID,
	}, tx)
	if err != nil {
		tx.Rollback()
		c.ResponseErrorf("添加结果失败！", err)
		return
	}
	if err := tx.Commit(); err != nil {
		tx.Rollback()
		c.ResponseErrorf("提交事务失败！", err)
	}
	c.ResponseOK()

}

type ruleSimpleResp struct {
	RuleNo     string `json:"rule_no"`
	Name       string `json:"name"`
	Expression string `json:"expression"`
	Status     int    `json:"status"`
	AgentUID   string `json:"agent_uid"`
	AgentName  string `json:"agent_name"`
}

func newRuleSimpleResp(m *ruleSimpleModel) *ruleSimpleResp {
	return &ruleSimpleResp{
		RuleNo:     m.RuleNo,
		Name:       m.Name,
		Expression: m.Expression,
		Status:     m.Status,
		AgentUID:   m.AgentUID,
		AgentName:  m.AgentName,
	}
}

type termType int

const termTypeAnd termType = 0 // 与 JSON term_type 一致：0=AND；OR 分支值为 1，当前解析逻辑未单独引用常量名

func (t termType) int() int {
	return int(t)
}

type term struct {
	tag     string
	realTag string
	Field   string `json:"field"`
	Symbol  string `json:"symbol"`
	Value   string `json:"value"`
}

func (t term) toModel(ruleNo string, appID string) *conditionModel {
	return &conditionModel{
		AppID:     appID,
		RuleNo:    ruleNo,
		Field:     t.Field,
		Condition: t.Symbol,
		Value:     t.Value,
		Tag:       t.realTag,
	}
}

type ruleReq struct {
	Name     string     `json:"name"`
	AgentUID string     `json:"agent_uid"`
	TermType int        `json:"term_type"`       // 0. and 1. or
	Terms    []*term    `json:"terms,omitempty"` // term or ruleReq
	Rules    []*ruleReq `json:"rules,omitempty"`
}

func getRuleExpression(r *ruleReq) string {
	setTag(r, 0)
	return getExpress(r, true)
}

func setTag(r *ruleReq, tagIndex int) {
	if len(r.Terms) > 0 {
		for i := 0; i < len(r.Terms); i++ {
			term := r.Terms[i]
			term.tag = getTag(tagIndex)
		}
	}
	if len(r.Rules) > 0 {
		for _, rule := range r.Rules {
			tagIndex++
			setTag(rule, tagIndex)
		}
	}
}

var tags = []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M", "N", "O", "P"}

func getTag(i int) string {
	return tags[i]
}

func getExpress(r *ruleReq, root bool) string {
	express := make([]string, 0)
	if len(r.Terms) > 0 {
		for i := 0; i < len(r.Terms); i++ {
			term := r.Terms[i]
			term.realTag = fmt.Sprintf("%s%d", term.tag, i)
			express = append(express, term.realTag)
		}
	}
	expStr := ""
	if len(express) > 0 {
		if root {
			if r.TermType == termTypeAnd.int() {
				expStr = strings.Join(express, "&&")
			} else {
				expStr = strings.Join(express, "||")
			}
		} else {
			if r.TermType == termTypeAnd.int() {
				expStr = strings.Join(express, "&")
			} else {
				expStr = strings.Join(express, "|")
			}
			expStr = fmt.Sprintf("(%s)", expStr)
		}
	}

	subExpStr := ""
	if len(r.Rules) > 0 {
		for _, rs := range r.Rules {
			subExpList := make([]string, 0)
			subExpre := getExpress(rs, false)
			if len(subExpre) > 0 {
				subExpList = append(subExpList, subExpre)
			}
			if root {
				if r.TermType == termTypeAnd.int() {
					subExpStr = strings.Join(subExpList, "&&")
				} else {
					subExpStr = strings.Join(subExpList, "||")
				}
			} else {
				if r.TermType == termTypeAnd.int() {
					subExpStr = strings.Join(subExpList, "&")
				} else {
					subExpStr = strings.Join(subExpList, "|")
				}
				subExpStr = fmt.Sprintf("(%s)", subExpStr)
			}
		}
	}
	if expStr == "" && subExpStr == "" {
		return ""
	}
	if expStr == "" {
		return subExpStr
	}
	if subExpStr == "" {
		return expStr
	}
	resultExpStr := ""
	expList := []string{expStr, subExpStr}
	if r.TermType == termTypeAnd.int() {
		if root {
			resultExpStr = strings.Join(expList, "&&")
		} else {
			resultExpStr = strings.Join(expList, "&")
		}
	} else {
		if root {
			resultExpStr = strings.Join(expList, "||")
		} else {
			resultExpStr = strings.Join(expList, "|")
		}
		resultExpStr = fmt.Sprintf("(%s)", resultExpStr)
	}
	return resultExpStr

}

func getConditionModels(r *ruleReq, models *[]*conditionModel, appID string, ruleNo string) {
	if len(r.Terms) > 0 {
		for _, term := range r.Terms {
			*models = append(*models, term.toModel(ruleNo, appID))
		}
	}
	if len(r.Rules) > 0 {
		for _, rule := range r.Rules {
			getConditionModels(rule, models, appID, ruleNo)
		}
	}
}

func (r *ruleReq) toModel(appID string) (*ruleModel, []*conditionModel) {
	ruleM := &ruleModel{}
	ruleM.AppID = appID
	ruleM.Name = r.Name
	ruleM.RuleNo = util.GenerUUID()
	ruleM.Expression = getRuleExpression(r)
	ruleM.Status = 1

	conditionModels := make([]*conditionModel, 0)
	getConditionModels(r, &conditionModels, appID, ruleM.RuleNo)

	return ruleM, conditionModels
}
