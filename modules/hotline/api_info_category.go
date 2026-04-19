package hotline

import (
	"errors"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"github.com/gin-gonic/gin"
)

// 添加类别
func (h *Hotline) infoCategoryAdd(c *wkhttp.Context) {
	var req infoCategoryReq
	if err := c.BindJSON(&req); err != nil {
		c.ResponseErrorf("数据格式有误！", err)
		return
	}
	if err := req.check(); err != nil {
		c.ResponseError(err)
		return
	}
	categoryNo := util.GenerUUID()
	err := h.infoCategoryDB.insert(&infoCategoryModel{
		AppID:        c.GetAppID(),
		CategoryNo:   categoryNo,
		CategoryName: req.CategoryName,
		Creater:      c.GetLoginUID(),
		Share:        req.Share,
	})
	if err != nil {
		c.ResponseErrorf("添加类别失败！", err)
		return
	}
	c.Response(gin.H{
		"category_no": categoryNo,
	})

}

// 查询类别列表
func (h *Hotline) infoCategoryList(c *wkhttp.Context) {
	infoCategoryModels, err := h.infoCategoryDB.queryMyWithAppID(c.GetLoginUID(), c.GetAppID())
	if err != nil {
		c.ResponseErrorf("查询类别失败！", err)
		return
	}
	infoCategoryResps := make([]*infoCategoryResp, 0, len(infoCategoryModels))

	for _, infoCategoryM := range infoCategoryModels {
		infoCategoryResps = append(infoCategoryResps, newInfoCategoryResp(infoCategoryM))
	}
	c.Response(infoCategoryResps)
}

type infoCategoryReq struct {
	CategoryName string `json:"category_name"` // 类别名称
	Share        int    `json:"share"`         // 是否分享给所有人
}

func (i infoCategoryReq) check() error {
	if i.CategoryName == "" {
		return errors.New("类别名称不能为空！")
	}
	return nil
}

type infoCategoryResp struct {
	CategoryNo   string `json:"category_no"`
	UID          string `json:"uid"`
	CategoryName string `json:"category_name"` // 类别名称
	Share        int    `json:"share"`         // 是否分享给所有人
}

func newInfoCategoryResp(m *infoCategoryModel) *infoCategoryResp {
	return &infoCategoryResp{
		CategoryNo:   m.CategoryNo,
		CategoryName: m.CategoryName,
		UID:          m.Creater,
		Share:        m.Share,
	}
}
