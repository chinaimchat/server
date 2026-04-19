package hotline

import "github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"

// 角色列表
func (h *Hotline) roleList(c *wkhttp.Context) {
	roleModels, err := h.roleDB.queryRolesWithAppID(c.GetAppID())
	if err != nil {
		c.ResponseErrorf("查询角色列表失败！", err)
		return
	}
	if len(roleModels) <= 0 {
		roleModels, err = h.roleDB.queryRolesWithAppID(DefaultAppID)
		if err != nil {
			c.ResponseErrorf("查询默认角色列表失败！", err)
			return
		}
	}

	roleResps := make([]*roleResp, 0)
	for _, roleModel := range roleModels {
		roleResps = append(roleResps, newRoleResp(roleModel))
	}
	c.Response(roleResps)
}

type roleResp struct {
	Role   string `json:"role"`
	Remark string `json:"remark"`
}

func newRoleResp(m *roleModel) *roleResp {
	return &roleResp{
		Role:   m.Role,
		Remark: m.Remark,
	}
}
