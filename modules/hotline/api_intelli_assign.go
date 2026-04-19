package hotline

import (
	"errors"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"go.uber.org/zap"
)

// 获取智能分配详情
func (h *Hotline) intelliAssignGet(c *wkhttp.Context) {
	appID := c.GetAppID()

	intelliAssignM, err := h.intelliAssignDB.queryWithAppID(appID)
	if err != nil {
		c.ResponseErrorf("获取智能分配数据失败！", err)
		return
	}
	resp := intelliAssignResp{}
	if intelliAssignM != nil {
		resp.Strategy = intelliAssignM.Strategy
		resp.UserMaxIdle = intelliAssignM.UserMaxIdle
		resp.SessionActiveLimit = intelliAssignM.SessionActiveLimit
		resp.SessionRememberMin = intelliAssignM.SessionRememberMin
		resp.SessionRememberEnable = intelliAssignM.SessionRememberEnable
		resp.VisitorMaxIdle = intelliAssignM.VisitorMaxIdle
		resp.Status = intelliAssignM.Status
	} else {
		resp.Strategy = IntelliStrategyTurn.String()
		resp.UserMaxIdle = 15
		resp.VisitorMaxIdle = 10
		resp.SessionRememberEnable = 0
		resp.SessionRememberMin = 30
		resp.SessionActiveLimit = 8
		resp.Status = 0
	}
	c.Response(resp)
}

func (h *Hotline) intelliAssignEnable(c *wkhttp.Context) {
	var req struct {
		Enable int `json:"enable"`
	}
	if err := c.BindJSON(&req); err != nil {
		h.Error("数据格式有误！", zap.Error(err))
		c.ResponseError(errors.New("数据格式有误！"))
		return
	}
	intelliAssignM, err := h.intelliAssignDB.queryWithAppID(c.GetAppID())
	if err != nil {
		c.ResponseErrorf("获取智能分配数据失败！", err)
		return
	}
	if intelliAssignM == nil {
		intelliAssignM = &intelliAssignModel{
			AppID:              c.GetAppID(),
			Strategy:           IntelliStrategyBalance.String(),
			UserMaxIdle:        15,
			VisitorMaxIdle:     10,
			SessionRememberMin: 8,
			SessionActiveLimit: 8,
			Status:             1,
		}
		err = h.intelliAssignDB.insert(intelliAssignM)
		if err != nil {
			c.ResponseErrorf("添加智能分配失败！", err)
			return
		}
	} else {
		err = h.intelliAssignDB.updateStatus(req.Enable, intelliAssignM.Id)
		if err != nil {
			c.ResponseErrorf("更新智能分配失败！", err)
			return
		}
	}
	c.ResponseOK()
}

func (h *Hotline) intelliAssignSet(c *wkhttp.Context) {
	var req intelliAssignReq
	if err := c.BindJSON(&req); err != nil {
		h.Error("数据格式有误！", zap.Error(err))
		c.ResponseError(errors.New("数据格式有误！"))
		return
	}
	intelliAssignM, err := h.intelliAssignDB.queryWithAppID(c.GetAppID())
	if err != nil {
		c.ResponseErrorf("获取智能分配数据失败！", err)
		return
	}
	var exist bool = true
	if intelliAssignM == nil {
		exist = false
	}
	var id int64 = 0
	if intelliAssignM != nil {
		id = intelliAssignM.Id
	}
	intelliAssignM = &intelliAssignModel{
		AppID:                 c.GetAppID(),
		Strategy:              req.Strategy,
		UserMaxIdle:           req.UserMaxIdle,
		VisitorMaxIdle:        req.VisitorMaxIdle,
		SessionRememberMin:    req.SessionRememberMin,
		SessionRememberEnable: req.SessionRememberEnable,
		SessionActiveLimit:    req.SessionActiveLimit,
		Status:                1,
	}
	intelliAssignM.Id = id

	if exist {
		err = h.intelliAssignDB.update(intelliAssignM)
		if err != nil {
			c.ResponseErrorf("更新智能规则失败！", err)
			return
		}
	} else {
		err = h.intelliAssignDB.insert(intelliAssignM)
		if err != nil {
			c.ResponseErrorf("添加智能规则失败！", err)
			return
		}
	}
	c.ResponseOK()
}

// ---------- model ----------

type intelliAssignReq struct {
	Strategy              string `json:"strategy"` // 策略
	UserMaxIdle           int    `json:"user_max_idle"`
	VisitorMaxIdle        int    `json:"visitor_max_idle"`
	SessionRememberEnable int    `json:"session_remember_enable"` // 是否启用会话记忆
	SessionRememberMin    int    `json:"session_remember_min"`    // 对话忘记时间(单位分钟) 如果对话在此时间内重新打开，则将其重新指派给同一成员 如果是 0 则表示不忘记
	SessionActiveLimit    int    `json:"session_active_limit"`
}

type intelliAssignResp struct {
	Strategy              string `json:"strategy"`                // 策略
	UserMaxIdle           int    `json:"user_max_idle"`           // 成员最大空闲(单位分钟),如果成员达到最大空闲，则将其设为非活动状态'
	VisitorMaxIdle        int    `json:"visitor_max_idle"`        // 访客最大空闲(单位分钟),如果访客达到最大空闲，则将其设为非活动状态
	SessionRememberEnable int    `json:"session_remember_enable"` // 是否启用会话记忆
	SessionRememberMin    int    `json:"session_remember_min"`    // 对话忘记时间(单位分钟) 如果对话在此时间内重新打开，则将其重新指派给同一成员 如果是 0 则表示不忘记
	SessionActiveLimit    int    `json:"session_active_limit"`    // 每名成员的活动对话限制
	Status                int    `json:"status"`                  // 0.禁用 1.启用
}
