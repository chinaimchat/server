package hotline

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/user"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/common"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/log"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/gocraft/dbr/v2"
	"go.uber.org/zap"
)

// IRouteService 路由规则服务
type IRouteService interface {
	Route(ctx *RouteContext) (*RouteResult, error)
	// 分配客服到访客频道内
	AssignToUser(agentUID string, operatorUID string, channelID string, channelType uint8) error
}

// RouteResult 路由结果
type RouteResult struct {
	GroupNo  string // 群编号
	AgentUID string // 客服UID
}

// RouteContext 路由上下文
type RouteContext struct {
	AppID   string
	Message *config.MessageResp // 消息（可空）
	Visitor *VisitorDetail      // 访客详情
	Channel *ChannelDetail      // 频道
	Session *SessionDetail      // 会话数据
}

// RouteService 路由服务
type RouteService struct {
	ctx             *config.Context
	ruleDB          *ruleDB
	groupDB         *groupDB
	intelliAssignDB *intelliAssignDB
	visitorDB       *visitorDB
	channelDB       *channelDB
	agentDB         *agentDB
	sessionDB       *sessionDB
	log.Log
	onlineService user.IOnlineService
}

// NewRouteService NewRouteService
func NewRouteService(ctx *config.Context) *RouteService {
	return &RouteService{
		ctx:             ctx,
		ruleDB:          newRuleDB(ctx),
		groupDB:         newGroupDB(ctx),
		intelliAssignDB: newIntelliAssignDB(ctx),
		visitorDB:       newVisitorDB(ctx),
		channelDB:       newChannelDB(ctx),
		agentDB:         newAgentDB(ctx),
		sessionDB:       newSessionDB(ctx),
		Log:             log.NewTLog("RouteService"),
		onlineService:   user.NewOnlineService(ctx),
	}
}

func (r *RouteService) AssignToUser(agentUID string, operatorUID string, channelID string, channelType uint8) error {

	visitorChannelM, err := r.channelDB.queryWithChannelID(channelID, channelType)
	if err != nil {
		r.Error("获取访客频道失败！", zap.Error(err), zap.String("channelID", channelID), zap.Uint8("channelType", channelType))
		return err
	}
	if visitorChannelM == nil {
		return errors.New("访客频道不存在！")
	}
	if visitorChannelM.VID == "" {
		return errors.New("非访客频道！")
	}
	agentM, err := r.agentDB.queryWithUIDAndAppID(agentUID, visitorChannelM.AppID)
	if err != nil {
		return err
	}
	if agentM == nil {
		return errors.New("客服不存在！")
	}

	tx, _ := r.ctx.DB().Begin()
	defer func() {
		if err := recover(); err != nil {
			tx.RollbackUnlessCommitted()
			panic(err)
		}
	}()
	// ########## 将客服添加到指定的访客频道 ##########
	err = r.addAgentToChannel(agentUID, visitorChannelM, tx)
	if err != nil {
		tx.Rollback()
		return err
	}

	// ########## 更新访客的分配数据 ##########
	err = r.channelDB.updateAgentUIDAndBindTx(agentUID, "", visitorChannelM.ChannelID, visitorChannelM.ChannelType, tx)
	if err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		tx.Rollback()
		return err
	}

	// 创建或更新IM的频道
	err = r.ctx.IMCreateOrUpdateChannel(&config.ChannelCreateReq{
		ChannelID:   visitorChannelM.ChannelID,
		ChannelType: visitorChannelM.ChannelType,
		Subscribers: []string{agentUID},
	})
	if err != nil {
		return err
	}

	// ########## 发送分配的消息至频道 ##########
	subscribers, _, err := r.channelDB.queryBindSubscribers(channelID, channelType)
	if err != nil {
		return err
	}
	subscriberUIDs := make([]string, 0, len(subscribers))
	visitorSubscriberUID := ""
	if len(subscribers) > 0 {
		for _, subscriber := range subscribers {
			if subscriber.SubscriberType == SubscriberTypeVisitor.Int() { // 此消息不发送给访客
				visitorSubscriberUID = subscriber.Subscriber
			} else {
				subscriberUIDs = append(subscriberUIDs, subscriber.Subscriber)
			}
		}
	}
	if len(subscriberUIDs) > 0 {
		var operatorAgentM *agentModel
		if operatorUID != "" {
			var err error
			operatorAgentM, err = r.agentDB.queryWithUIDAndAppID(operatorUID, visitorChannelM.AppID)
			if err != nil {
				return err
			}
		}
		extra := make([]config.UserBaseVo, 0)
		var content string
		if operatorAgentM == nil {
			content = "会话已指派至{0}"
			extra = append(extra, config.UserBaseVo{
				UID:  agentM.UID,
				Name: agentM.Name,
			})
		} else {
			content = "会话已被{0}指派至{1}"
			extra = append(extra, config.UserBaseVo{
				UID:  operatorAgentM.UID,
				Name: operatorAgentM.Name,
			}, config.UserBaseVo{
				UID:  agentM.UID,
				Name: agentM.Name,
			})
		}

		err = r.ctx.SendMessage(&config.MsgSendReq{
			Header: config.MsgHeader{
				RedDot: 1,
			},
			ChannelID:   channelID,
			ChannelType: channelType,
			Subscribers: subscriberUIDs,
			Payload: []byte(util.ToJson(map[string]interface{}{
				"type":       common.HotlineAssignTo,
				"content":    content,
				"extra":      extra,
				"invisibles": []string{visitorSubscriberUID},
			})),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *RouteService) addAgentToChannel(agentUID string, visitorChannelM *channelModel, tx *dbr.Tx) error {
	exist, err := r.channelDB.existSubscriber(agentUID, visitorChannelM.ChannelID, visitorChannelM.ChannelType)
	if err != nil {
		return err
	}
	if !exist {
		err = r.channelDB.insertSubscriberTx(&subscribersModel{
			AppID:          visitorChannelM.AppID,
			ChannelID:      visitorChannelM.ChannelID,
			ChannelType:    visitorChannelM.ChannelType,
			SubscriberType: SubscriberTypeAgent.Int(),
			Subscriber:     agentUID,
		}, tx)
		if err != nil {
			return err
		}
	}

	return nil
}

// Route Route
func (r *RouteService) Route(ctx *RouteContext) (*RouteResult, error) {

	// ########## 根据规则分配客服 ##########
	r.Debug("开始执行规则逻辑...", zap.String("appID", ctx.AppID))
	result, err := r.execRule(ctx)
	if err != nil {
		return nil, err
	}
	if result != nil {
		return result, nil
	}
	r.Debug("规则逻辑没有匹配到客服...", zap.String("appID", ctx.AppID))

	// ########## 根据智能配置分配客服 ##########
	// 查询有效的智能配置
	r.Debug("开始执行智能分配逻辑...")
	intelliAssignM, err := r.intelliAssignDB.queryVaildWithAppID(ctx.AppID)
	if err != nil {
		r.Error("查询智能配置失败！", zap.Error(err), zap.String("appID", ctx.AppID))
		return nil, err
	}
	if intelliAssignM != nil {
		result, err = r.execIntelliAssign(intelliAssignM, ctx)
		if err != nil {
			r.Error("执行智能分配失败！", zap.Error(err), zap.String("appID", ctx.AppID))
			return nil, err
		}
		if result != nil {
			r.Debug("已路由到客服...", zap.String("agentUID", result.AgentUID), zap.String("appID", ctx.AppID))
			return result, nil
		} else {
			r.Info("没有理由到客服！", zap.String("vid", ctx.Visitor.VID), zap.String("appID", ctx.AppID))
		}
	} else {
		r.Info("没有获取到智能配置", zap.String("appID", ctx.AppID))
	}
	r.Debug("结束执行路由逻辑...", zap.String("appID", ctx.AppID))
	return nil, nil
}

// 执行智能分配
func (r *RouteService) execIntelliAssign(intelliAssignM *intelliAssignModel, ctx *RouteContext) (*RouteResult, error) {
	channel := ctx.Channel
	if channel.AgentUID != "" { // 如果访客原来有做分配
		if ctx.Session != nil {
			session := ctx.Session
			if intelliAssignM.SessionRememberEnable == 1 {
				if time.Now().Unix()-session.LastSession < int64(intelliAssignM.SessionRememberMin) { //如果最后一次会话时间在记忆时间内 则分配原来分配过的客服

					// TODO: 这里应该还需要做下客服是否有效
					return &RouteResult{
						AgentUID: channel.AgentUID,
					}, nil
				}
			}

		}

	}

	agentModel, err := r.getMaxIdleAgent(intelliAssignM) // 获取最闲的客服
	if err != nil {
		return nil, err
	}
	if agentModel != nil {
		return &RouteResult{
			AgentUID: agentModel.UID,
		}, nil
	}
	return nil, nil

}

type agentIdle struct {
	*agentModel
	idle          float32 // 空闲能力
	recentSession int64   // 最近一次会话时间
}

type agentIdleSlice []*agentIdle

func (s agentIdleSlice) Len() int { return len(s) }

func (s agentIdleSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

func (s agentIdleSlice) Less(i, j int) bool {
	if s[i].idle == s[j].idle {
		return s[i].recentSession > s[j].recentSession
	}
	return s[i].idle > s[j].idle
}

// 获取最闲的客服
func (r *RouteService) getMaxIdleAgent(intelliAssignM *intelliAssignModel) (*agentModel, error) {

	// 最闲客服获取逻辑 例下：
	// 第一步：获取在工作时间内并且在线的客服
	// 第二步：获取每个客服正在处理访客的数量
	// 第三步：获取每个客服获取技能组限制的对话数量
	// 第四步： (对话限制数量 - 当前正在处理会话数量)/对话限制数量 = 空闲率

	// ---------- 第一步：获取在工作时间内并且在线的客服 ----------
	appID := intelliAssignM.AppID
	// 查询所有在上班的客服
	agents, err := r.agentDB.queryWorkingWithAppID(appID)
	if err != nil {
		return nil, err
	}

	uids := make([]string, 0, len(agents))
	for _, agent := range agents {
		uids = append(uids, agent.UID)
	}

	// 过滤掉不在线的
	// onlinestatusResps, err := r.onlineService.GetUserOnlineStatus(uids)
	// if err != nil {
	// 	return nil, err
	// }
	onlineAgents := make([]*agentModel, 0, len(agents)) // 在线的客服
	if len(uids) > 0 {
		for _, agent := range agents {
			for _, uid := range uids {
				if agent.UID == uid {
					onlineAgents = append(onlineAgents, agent)
				}
			}
		}

	}

	// ---------- 第二步：获取每个客服正在处理访客的数量----------
	// 获取客服正在处理访客的数量列表
	sessionAgentTotals, err := r.sessionDB.queryAgentSessionTotalWithAppID(appID)
	if err != nil {
		return nil, err
	}
	sessionAgentTotalMap := map[string]*sessionAgentTotal{}
	if len(sessionAgentTotals) > 0 {
		for _, sessionAgentTotal := range sessionAgentTotals {
			sessionAgentTotalMap[sessionAgentTotal.AgentUID] = sessionAgentTotal
		}
	}

	// ---------- 第三步：获取每个客服获取技能组限制的对话数量----------
	// 获取技能组设置的客服的对话限制
	channelAgentSessionActiveLimits, err := r.channelDB.querySessionActiveLimitWithAppID(appID)
	if err != nil {
		return nil, err
	}
	channelAgentSessionActiveLimitMap := map[string]*channelAgentSessionActiveLimit{}
	if len(channelAgentSessionActiveLimits) > 0 {
		for _, channelAgentSessionActiveLimitM := range channelAgentSessionActiveLimits {
			channelAgentSessionActiveLimitMap[channelAgentSessionActiveLimitM.UID] = channelAgentSessionActiveLimitM
		}
	}

	// ---------- 第四步： (对话限制数量 - 当前处理会话数量)/对话限制数量 = 空闲率 ----------
	var agentIdleArray agentIdleSlice = make(agentIdleSlice, 0)
	for _, agent := range onlineAgents {
		sessionAgentTotalM := sessionAgentTotalMap[agent.UID]
		channelAgentSessionActiveLimitM := channelAgentSessionActiveLimitMap[agent.UID]
		sessionActiveLimit := intelliAssignM.SessionActiveLimit
		if channelAgentSessionActiveLimitM != nil {
			sessionActiveLimit = channelAgentSessionActiveLimitM.SessionActiveLimit
		}

		var sessionCount int // 正在处理的会话数量
		var recentSession int64
		if sessionAgentTotalM != nil {
			sessionCount = sessionAgentTotalM.SessionCount
			recentSession = sessionAgentTotalM.RecentSession
		}
		if sessionCount < sessionActiveLimit {
			agentIdleArray = append(agentIdleArray, &agentIdle{
				agentModel:    agent,
				recentSession: recentSession,
				idle:          float32(sessionActiveLimit-sessionCount) / float32(sessionActiveLimit),
			})
		}

	}
	sort.Sort(agentIdleArray)

	if len(agentIdleArray) > 0 {
		return agentIdleArray[0].agentModel, nil
	}
	return nil, errors.New("没有获取到空闲的客服")
}

// 执行规则
func (r *RouteService) execRule(ctx *RouteContext) (*RouteResult, error) {
	// 查询所有有效的规则
	rules, err := r.ruleDB.queryVailDetailWithAppID(ctx.AppID)
	if err != nil {
		r.Error("查询规则失败！", zap.Error(err))
		return nil, err
	}
	if len(rules) <= 0 {
		return nil, nil
	}
	rule := r.getTriggerRule(rules, ctx)
	if rule == nil {
		return nil, nil
	}

	// 获取规则触发的结果
	ruleResult, err := r.ruleDB.queryRuleResultWithRuleNo(rule.RuleNo)
	if err != nil {
		r.Error("查询规则结果失败！", zap.Error(err), zap.String("ruleNo", rule.RuleNo))
		return nil, err
	}
	if ruleResult == nil {
		r.Warn("没有查询到规则的结果！", zap.String("ruleNo", rule.RuleNo))
		return nil, nil
	}

	result := &RouteResult{}
	if ruleResult.AssignType == int(AssignTypePerson) {
		result.AgentUID = ruleResult.AssignNo
	} else {
		result.GroupNo = ruleResult.AssignNo
	}
	return result, nil
}

// 获取触发的规则
func (r *RouteService) getTriggerRule(rules []*ruleDetailModel, ctx *RouteContext) *ruleDetailModel {
	for _, rule := range rules {
		expression := r.parseExpression(rule.Expression) // 解析表达式
		r.calcExpression(expression, rule, ctx)          // 计算表达式

		if r.assertExpression(expression) { // 断言表达式
			return rule
		}
	}
	return nil
}

func (r *RouteService) calcExpression(expression *Expression, rule *ruleDetailModel, ctx *RouteContext) {
	if len(rule.Conditions) > 0 {
		for _, condition := range rule.Conditions {
			if expression.Tag != "" && expression.Tag == condition.Tag {
				actValueObj := r.getValueByField(ctx, condition.Field) // 获取field对应的值
				actValue := fmt.Sprintf("%v", actValueObj)
				expectValue := condition.Value // 期望值

				switch Action(condition.Condition) {
				case ActionEqual:
					if actValue == expectValue {
						expression.Result = true
					}
				case ActionNoContain:
					expression.Result = !strings.Contains(actValue, expectValue)
				case ActionContain:
					expression.Result = strings.Contains(actValue, expectValue)
				case ActionNotEqual:
					if actValue != expectValue {
						expression.Result = true
					}
				}
				break
			} else if len(expression.Children) > 0 {
				for _, childExpress := range expression.Children {
					r.calcExpression(childExpress, rule, ctx)
				}
			}
		}
	}
}

func (r *RouteService) assertChildrenExpression(expType ExpType, expressions []*Expression) bool {

	switch expType {
	case ExpTypeAnd:
		for _, expression := range expressions {
			result := true
			if len(expression.Children) > 0 {
				result = r.assertChildrenExpression(expression.ExpType, expression.Children)
			} else {
				result = expression.Result
			}
			if !result {
				return false
			}
		}
		return true
	case ExpTypeOr:
		for _, expression := range expressions {
			result := true
			if len(expression.Children) > 0 {
				result = r.assertChildrenExpression(expression.ExpType, expression.Children)
			} else {
				result = expression.Result
			}
			if result {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (r *RouteService) assertExpression(expression *Expression) bool {

	if len(expression.Children) > 0 {
		return r.assertChildrenExpression(expression.ExpType, expression.Children)

	}
	return expression.Result
}

func (r *RouteService) parseExpression(expression string) *Expression {
	orSymbol := "||"
	andSymbol := "&&"
	exp := &Expression{}
	var subExps []string
	if strings.Contains(expression, orSymbol) { // or
		exp.ExpType = ExpTypeOr
		subExps = strings.Split(expression, orSymbol)

	} else { // and
		exp.ExpType = ExpTypeAnd
		subExps = strings.Split(expression, andSymbol)
	}
	children := make([]*Expression, 0)
	for _, subExp := range subExps {
		subExpObj := &Expression{}
		var ssubExps = make([]string, 0)
		if strings.Contains(subExp, "|") { // or
			subExpObj.ExpType = ExpTypeOr
			ssubExps = strings.Split(subExp, "|")
		} else { // and
			subExpObj.ExpType = ExpTypeAnd
			ssubExps = strings.Split(subExp, "&")
		}
		children = append(children, subExpObj)
		cchildren := make([]*Expression, 0)
		for _, ssubExp := range ssubExps {
			ssubExpObj := &Expression{}
			ssubExpObj.Tag = strings.ReplaceAll(strings.ReplaceAll(ssubExp, "(", ""), ")", "")
			cchildren = append(cchildren, ssubExpObj)
		}
		subExpObj.Children = cchildren
	}
	exp.Children = children
	return exp
}

func (r *RouteService) getValueByField(ctx *RouteContext, field string) interface{} {
	fields := strings.Split(field, ".")
	if len(fields) <= 1 {
		return nil
	}
	first := fields[0]
	if first == "message" && ctx.Message != nil {
		return r.getObjValue(*ctx.Message, strings.Join(fields[1:], "."), true)
	} else if first == "visitor" && ctx.Visitor != nil {
		return r.getObjValue(*ctx.Visitor, strings.Join(fields[1:], "."), false)
	}
	return nil
}

func (r *RouteService) getObjValue(obj interface{}, field string, isMessage bool) interface{} {

	keys := strings.Split(field, ".")
	objV := reflect.ValueOf(obj)
	objRef := reflect.TypeOf(obj)
	for i := 0; i < len(keys); i++ {
		key := strings.ToLower(keys[i])
		for j := 0; j < objRef.NumField(); j++ {
			fd := objRef.Field(j)
			name := strings.ToLower(fd.Name)
			if name == key {
				newObj := objV.Field(j).Interface()
				if isMessage && name == "payload" && len(keys) > 1 {
					payloadKey := keys[1]
					payloadBytes := newObj.([]byte)
					var payloadMap map[string]interface{}
					util.ReadJsonByByte(payloadBytes, &payloadMap)
					if payloadMap != nil {
						return payloadMap[payloadKey]
					}

					return nil

				}
				if i == len(keys)-1 {
					return newObj
				}
				return r.getObjValue(newObj, strings.Join(keys[1:], "."), isMessage)
			}

		}
	}
	return nil
}

// Action Action
type Action string

const (
	// ActionEqual 等于
	ActionEqual Action = "="
	// ActionNotEqual 不等于
	ActionNotEqual Action = "<>"
	// ActionContain 包含
	ActionContain Action = "in"
	// ActionNoContain 不包含
	ActionNoContain Action = "not"
)

// ExpType ExpType
type ExpType int

const (
	// ExpTypeAnd 和
	ExpTypeAnd ExpType = 1
	// ExpTypeOr 或
	ExpTypeOr ExpType = 2
)

// Expression 表达式
type Expression struct {
	ExpType  ExpType
	Tag      string
	Result   bool // 结果
	Children []*Expression
}

type ChannelDetail struct {
	VID         string `json:"vid"`
	TopicID     int64  `json:"topic_id"`
	Title       string `json:"title"`
	ChannelID   string `json:"channel_id"`
	ChannelType uint8  `json:"channel_type"`
	AgentUID    string `json:"agent_uid"`
}

type SessionDetail struct {
	VID         string `json:"vid"`
	ChannelType uint8  `json:"channel_type"`
	ChannelID   string `json:"channel_id"`
	SendCount   int    `json:"send_count"`
	RecvCount   int    `json:"recv_count"`
	UnreadCount int    `json:"unread_count"`
	LastRecv    int64  `json:"last_recv"`    // 客服最后一次收消息时间（访客最后一次发消息时间）
	LastSend    int64  `json:"last_send"`    // 客服最后一次发消息时间
	LastSession int64  `json:"last_session"` // 最后一次会话时间
}
