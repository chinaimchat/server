package hotline

import (
	"errors"
	"strconv"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/common"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/db"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
)

func (h *Hotline) topicAdd(c *wkhttp.Context) {
	var req topicAddReq
	if err := c.BindJSON(&req); err != nil {
		c.ResponseErrorf("数据格式有误！", err)
		return
	}
	if req.Title == "" {
		c.ResponseError(errors.New("标题不能为空！"))
		return
	}
	if req.Welcome == "" {
		c.ResponseError(errors.New("欢迎语不能为空！"))
		return
	}
	err := h.topicDB.insert(&topicModel{
		AppID:   c.GetAppID(),
		Title:   req.Title,
		Welcome: req.Welcome,
	})
	if err != nil {
		c.ResponseErrorf("添加话题失败！", err)
		return
	}
	c.ResponseOK()
}

func (h *Hotline) topicDel(c *wkhttp.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)

	topicM, err := h.topicDB.queryWithIDAndAppID(id, c.GetAppID())
	if err != nil {
		c.ResponseErrorf("查询话题失败！", err)
		return
	}
	if topicM != nil && topicM.IsDefault == 1 {
		c.ResponseError(errors.New("默认话题不能删除！"))
		return
	}
	err = h.topicDB.delete(id, c.GetAppID())
	if err != nil {
		c.ResponseErrorf("删除话题失败！", err)
		return
	}
	c.ResponseOK()

}

func (h *Hotline) topicPage(c *wkhttp.Context) {
	pIndex, pSize := c.GetPage()
	topicModels, err := h.topicDB.queryPageWithAppID(c.GetAppID(), uint64(pIndex), uint64(pSize))
	if err != nil {
		c.ResponseErrorf("查询话题失败！", err)
		return
	}

	topicCount, err := h.topicDB.queryCountWithAppID(c.GetAppID())
	if err != nil {
		c.ResponseErrorf("查询话题数量失败！", err)
		return
	}

	resps := make([]*topicDataResp, 0, len(topicModels))

	if len(topicModels) > 0 {
		for _, topicM := range topicModels {
			resps = append(resps, newTopicDataResp(topicM))
		}
	}
	c.Response(common.NewPageResult(pIndex, pSize, topicCount, resps))
}

func (h *Hotline) topicUpdate(c *wkhttp.Context) {
	var req topicUpdateReq
	if err := c.BindJSON(&req); err != nil {
		c.ResponseErrorf("数据格式有误！", err)
		return
	}

	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	err := h.topicDB.update(&topicModel{
		AppID:   c.GetAppID(),
		Title:   req.Title,
		Welcome: req.Welcome,
		BaseModel: db.BaseModel{
			Id: id,
		},
	})
	if err != nil {
		c.ResponseErrorf("更新话题失败！", err)
		return
	}
	c.ResponseOK()
}

type topicAddReq struct {
	Title   string `json:"title"`
	Welcome string `json:"welcome"`
}
type topicUpdateReq struct {
	Title   string `json:"title"`
	Welcome string `json:"welcome"`
}

type topicDataResp struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Welcome   string `json:"welcome"`
	IsDefault int    `json:"is_default"`
	CreatedAt string `json:"created_at"`
}

func newTopicDataResp(m *topicModel) *topicDataResp {

	return &topicDataResp{
		ID:        m.Id,
		Title:     m.Title,
		Welcome:   m.Welcome,
		IsDefault: m.IsDefault,
		CreatedAt: m.CreatedAt.String(),
	}
}
