package hotline

import (
	"errors"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"go.uber.org/zap"
)

// 技能组
func (h *Hotline) skillGroupList(c *wkhttp.Context) {
	groupModels, err := h.getOrInsertSkillGroups(c.GetAppID())
	if err != nil {
		c.ResponseErrorf("获取技能组失败！", err)
		return
	}
	resps := make([]*groupKillResp, 0, len(groupModels))
	for _, groupModel := range groupModels {
		resps = append(resps, newGroupKillResp(groupModel))
	}
	c.Response(resps)
}

// 群列表
func (h *Hotline) groupList(c *wkhttp.Context) {
	keyword := c.Query("keyword")
	groupModels, err := h.groupDB.queryDetailWithGroupTypeAndAppID(keyword, GroupTypeCommon.Int(), c.GetAppID())
	if err != nil {
		c.ResponseErrorf("查询群组失败！", err)
		return
	}
	resps := make([]*hGroupResp, 0, len(groupModels))
	for _, groupModel := range groupModels {
		resps = append(resps, newHGroupResp(groupModel))
	}
	c.Response(resps)

}

func (h *Hotline) groupAdd(c *wkhttp.Context) {
	var req struct {
		Name   string   `json:"name"`
		UIDs   []string `json:"uids"`
		Remark string   `json:"remark"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseErrorf("数据格式有误！", err)
		return
	}
	if req.Name == "" {
		c.ResponseError(errors.New("群名不能为空！"))
		return
	}
	if len(req.UIDs) <= 0 {
		c.ResponseError(errors.New("成员不能为空！"))
		return
	}
	tx, _ := h.ctx.DB().Begin()
	defer func() {
		if err := recover(); err != nil {
			tx.RollbackUnlessCommitted()
			panic(err)
		}
	}()
	groupNo := util.GenerUUID()
	err := h.groupDB.insertTx(&groupModel{
		AppID:     c.GetAppID(),
		GroupNo:   groupNo,
		GroupType: GroupTypeCommon.Int(),
		Name:      req.Name,
		Remark:    req.Remark,
		Status:    1,
	}, tx)
	if err != nil {
		tx.Rollback()
		c.ResponseError(errors.New("添加群失败！"))
		return
	}
	for _, uid := range req.UIDs {
		err = h.groupDB.insertMemberTx(&memberModel{
			AppID:   c.GetAppID(),
			GroupNo: groupNo,
			UID:     uid,
		}, tx)
		if err != nil {
			tx.Rollback()
			c.ResponseError(errors.New("添加群成员！"))
			return
		}
	}
	if err := tx.Commit(); err != nil {
		tx.Rollback()
		c.ResponseError(errors.New("提交事务失败！"))
		return
	}
	c.ResponseOK()
}

func (h *Hotline) getOrInsertSkillGroups(appID string) ([]*groupDetailModel, error) {

	groupModels, err := h.groupDB.queryDetailWithGroupTypeAndAppID("", GroupTypeKill.Int(), appID)
	if err != nil {
		return nil, err
	}
	if len(groupModels) <= 0 {
		groupModels, err = h.groupDB.queryDetailWithGroupTypeAndAppID("", GroupTypeKill.Int(), DefaultAppID)
		if err != nil {
			return nil, err
		}
		if len(groupModels) <= 0 {
			return nil, nil
		}
		tx, _ := h.ctx.DB().Begin()
		defer func() {
			if err := recover(); err != nil {
				tx.RollbackUnlessCommitted()
				panic(err)
			}
		}()
		for _, groupM := range groupModels {
			groupM.AppID = appID
			groupM.GroupNo = util.GenerUUID()
			groupM.Status = 1
			err = h.groupDB.insertTx(&groupM.groupModel, tx)
			if err != nil {
				tx.Rollback()
				return nil, err
			}
		}
		if err := tx.Commit(); err != nil {
			h.Error("提交事务失败！", zap.Error(err))
			tx.RollbackUnlessCommitted()
			return nil, err
		}

	}
	groupModels, err = h.groupDB.queryDetailWithGroupTypeAndAppID("", GroupTypeKill.Int(), appID)
	if err != nil {
		return nil, err
	}
	return groupModels, nil
}

type hGroupResp struct {
	GroupNo     string `json:"group_no"`     // 群组唯一编号
	Name        string `json:"name"`         // 群组名字
	Remark      string `json:"remark"`       // 群组备注
	MemberCount int64  `json:"member_count"` // 成员数量
}

func newHGroupResp(m *groupDetailModel) *hGroupResp {
	return &hGroupResp{
		GroupNo:     m.GroupNo,
		Name:        m.Name,
		Remark:      m.Remark,
		MemberCount: m.MemberCount,
	}
}

type groupKillResp struct {
	GroupNo            string `json:"group_no"`             // 群组唯一编号
	Name               string `json:"name"`                 // 群组名字
	Remark             string `json:"remark"`               // 群组备注
	IntelliAssign      int    `json:"intelli_assign"`       // 是否开启智能分配
	SessionActiveLimit int    `json:"session_active_limit"` // 每名成员的活动对话限制
	MemberCount        int64  `json:"member_count"`         // 成员数量
}

func newGroupKillResp(m *groupDetailModel) *groupKillResp {
	return &groupKillResp{
		GroupNo:            m.GroupNo,
		Name:               m.Name,
		Remark:             m.Remark,
		IntelliAssign:      m.IntelliAssign,
		SessionActiveLimit: m.SessionActiveLimit,
		MemberCount:        m.MemberCount,
	}
}
