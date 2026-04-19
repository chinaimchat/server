package hotline

import (
	"errors"
	"strconv"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/common"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
)

// 添加快捷回复
func (h *Hotline) quickReplyAdd(c *wkhttp.Context) {
	var req quickReplyAddReq
	if err := c.BindJSON(&req); err != nil {
		c.ResponseErrorf("数据格式有误！", err)
		return
	}
	if err := req.check(); err != nil {
		c.ResponseError(err)
		return
	}

	err := h.quickReplyDB.insert(&quickReplyModel{
		AppID:      c.GetAppID(),
		Title:      req.Title,
		Content:    req.Content,
		CategoryNo: req.CategoryNo,
		Shortcode:  req.Shortcode,
		Creater:    c.GetLoginUID(),
	})
	if err != nil {
		c.ResponseErrorf("添加快捷回复失败！", err)
		return
	}
	c.ResponseOK()

}

func (h *Hotline) quickReplySync(c *wkhttp.Context) {
	quickReplyModels, err := h.quickReplyDB.queryMyWithAppID(c.GetLoginUID(), c.GetAppID())
	if err != nil {
		c.ResponseErrorf("查询快捷回复失败！", err)
		return
	}
	resps := make([]*quickReplyResp, 0, len(quickReplyModels))
	if len(quickReplyModels) > 0 {
		for _, quickReplyM := range quickReplyModels {
			resps = append(resps, newQuickReplyResp2(quickReplyM))
		}
	}
	c.Response(resps)
}

func (h *Hotline) quickReplyPage(c *wkhttp.Context) {
	pageIndex, pageSize := c.GetPage()

	categoryNo := c.Query("category_no")

	quickReplyModels, err := h.quickReplyDB.queryPageWithCategoryNo(categoryNo, c.GetAppID(), uint64(pageIndex), uint64(pageSize))
	if err != nil {
		c.ResponseErrorf("查询快捷回复失败！", err)
		return
	}
	resps := make([]*quickReplyResp, 0, len(quickReplyModels))
	if len(quickReplyModels) > 0 {
		for _, quickReplyM := range quickReplyModels {
			resps = append(resps, newQuickReplyResp(quickReplyM))
		}
	}
	c.Response(common.NewPageResult(pageIndex, pageSize, 0, resps))
}

func (h *Hotline) quickReplyDel(c *wkhttp.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)

	err := h.quickReplyDB.deleteWithIDAndAppID(id, c.GetAppID())
	if err != nil {
		c.ResponseErrorf("删除快捷回复失败！", err)
		return
	}
	c.ResponseOK()
}

type quickReplyAddReq struct {
	Title      string `json:"title"`
	Content    string `json:"content"`
	CategoryNo string `json:"category_no"`
	Shortcode  string `json:"shortcode"`
}

func (q quickReplyAddReq) check() error {
	if q.Title == "" {
		return errors.New("标题不能为空！")
	}
	if q.Content == "" {
		return errors.New("内容不能为空！")
	}
	if q.CategoryNo == "" {
		return errors.New("类别编号不能为空！")
	}
	return nil
}

type quickReplyResp struct {
	ID           int64  `json:"id"`
	Title        string `json:"title"`
	Content      string `json:"content"`
	CategoryNo   string `json:"category_no"`
	CategoryName string `json:"category_name"`
	Shortcode    string `json:"shortcode"`
}

func newQuickReplyResp(m *quickReplyModel) *quickReplyResp {
	return &quickReplyResp{
		ID:         m.Id,
		Title:      m.Title,
		Content:    m.Content,
		CategoryNo: m.CategoryNo,
		Shortcode:  m.Shortcode,
	}

}
func newQuickReplyResp2(m *quickReplyDetailModel) *quickReplyResp {
	q := newQuickReplyResp(&m.quickReplyModel)
	q.CategoryName = m.CategoryName
	return q
}
