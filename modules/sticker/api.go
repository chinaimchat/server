package sticker

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/base/event"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/log"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"go.uber.org/zap"
)

// Sticker Sticker
type Sticker struct {
	ctx *config.Context
	log.Log
	db *DB
}

// stickerPathForFormatSuffix 从逻辑 path 取出用于判断后缀的小写路径（去 query；http(s) 则取 URL.Path）。
func stickerPathForFormatSuffix(path string) string {
	p := strings.TrimSpace(path)
	if p == "" {
		return ""
	}
	lo := strings.ToLower(p)
	if strings.HasPrefix(lo, "http://") || strings.HasPrefix(lo, "https://") {
		if u, err := url.Parse(p); err == nil && u.Path != "" {
			p = u.Path
		}
	}
	if i := strings.IndexByte(p, '?'); i >= 0 {
		p = p[:i]
	}
	return strings.ToLower(p)
}

// normalizeStickerFormat 供 JSON 返回与入库：安卓矢量贴需 format=="lim" 才走 RLottie；Telegram 系常见 .tgs。
// 历史数据常见 path 无后缀但 format 填 gzip（实为 RLottie 容器），或 path 为 .lim 而 format 误为 gzip。
func normalizeStickerFormat(path, format string) string {
	p := stickerPathForFormatSuffix(path)
	f := strings.ToLower(strings.TrimSpace(format))

	switch {
	case strings.HasSuffix(p, ".lim"):
		return "lim"
	case strings.HasSuffix(p, ".tgs"):
		return "tgs"
	case strings.HasSuffix(p, ".gif"):
		return "gif"
	case strings.HasSuffix(p, ".webp"):
		return "webp"
	case strings.HasSuffix(p, ".png"):
		return "png"
	case strings.HasSuffix(p, ".jpg"), strings.HasSuffix(p, ".jpeg"):
		return "jpg"
	}

	// 无明确位图/矢量后缀时，用 MIME/别名字段推断矢量类型
	switch f {
	case "gzip", "application/x-gzip", "application/gzip":
		// 贴纸表里 gzip 几乎均表示 RLottie .lim 字节流，勿让客户端当普通 gzip 文件处理
		return "lim"
	case "lottie", "rlottie", "vector", "application/lottie+json":
		return "lim"
	case "telegram-sticker", "tgsticker", "application/x-tgsticker":
		return "tgs"
	}

	return format
}

// normalizeStickerAPIPath 规范贴纸相关 JSON 中的相对路径。
// 客户端常见拼法为 apiURL + path，且 apiURL 常以 .../v1/ 结尾；若 path 以 / 开头会得到 .../v1//file/... 导致 404。
// 故 file 预览类路径统一为无前导斜杠的 file/preview/...，拼成 .../v1/file/preview/...。
// 兼容历史数据：preview/sticker/...、sticker/<category>/...；已是 http(s):// 则不改；非上述相对路径保持原样（如 avatar/...）。
func normalizeStickerAPIPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
		return p
	}
	trim := strings.TrimLeft(p, "/")
	loTrim := strings.ToLower(trim)
	var out string
	switch {
	case strings.HasPrefix(loTrim, "file/file/preview/"):
		out = strings.TrimPrefix(trim, "file/")
	case strings.HasPrefix(loTrim, "file/"):
		out = trim
	case strings.HasPrefix(loTrim, "preview/"):
		out = "file/" + trim
	case strings.HasPrefix(loTrim, "sticker/"):
		out = "file/preview/" + trim
	default:
		out = p
	}
	return out
}

// New 创建
func New(ctx *config.Context) *Sticker {
	s := &Sticker{ctx: ctx, db: newDB(ctx.DB()), Log: log.NewTLog("Sticker")}

	return s
}

// Route 路由配置
func (s *Sticker) Route(r *wkhttp.WKHttp) {
	v := r.Group("/v1/sticker", s.ctx.AuthMiddleware(r))
	{
		v.GET("", s.search)
		//用户添加表情
		v.POST("/user", s.userAdd)
		//用户删除表情
		v.DELETE("/user", s.userDelete)
		//用户自定义表情
		v.GET("/user", s.userCustomSticker)
		//用户移除表情分类
		v.DELETE("/remove", s.userDeleteByCategory)
		//通过category添加一批表情
		v.POST("/user/:category", s.userAddByCategory)
		//获取用户分类列表
		v.GET("/user/category", s.getCategorys)
		//通过分类获取表情
		v.GET("/user/sticker", s.getStickerWithCategory)
		//获取表情商店
		v.GET("/store", s.list)
		//将自定义表情移到最前
		v.PUT("/user/front", s.moveToFront)
		//将用户表情分类排序
		v.PUT("/user/category/reorder", s.reorderUserCategory)
	}

	if !s.ctx.GetConfig().Register.StickerAddOff {
		s.ctx.AddEventListener(event.EventUserRegister, s.handleRegisterUserEvent)
	}

}

// 用户添加表情
func (s *Sticker) userAdd(c *wkhttp.Context) {
	loginUID := c.GetLoginUID()
	var req struct {
		Path        string `json:"path"`
		Width       int    `json:"width"`
		Height      int    `json:"height"`
		Format      string `json:"format"`
		Placeholder string `json:"placeholder"`
		Category    string `json:"category"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	if req.Path == "" {
		c.ResponseError(errors.New("文件地址不能为空"))
		return
	}
	if req.Width == 0 || req.Height == 0 {
		c.ResponseError(errors.New("表情高宽不能为空"))
		return
	}
	normFormat := normalizeStickerFormat(req.Path, req.Format)
	tempSticker, err := s.db.queryUserCustomStickerWithPath(loginUID, req.Path)
	if err != nil {
		s.Error("查询用户自定义表情错误", zap.Error(err))
		c.ResponseError(errors.New("查询用户自定义表情错误"))
		return
	}
	if tempSticker != nil && tempSticker.Path != "" {
		c.ResponseOK()
		return
	}
	cmodel, err := s.db.queryUserStickerWithMaxSortNum(loginUID)
	if err != nil {
		s.Error("删除表情错误", zap.Error(err))
		c.ResponseError(errors.New("查询用户表情最大编号错误"))
		return
	}
	var sortNum int = 1
	if cmodel != nil {
		sortNum = cmodel.SortNum + 1
	}
	tx, _ := s.ctx.DB().Begin()
	defer func() {
		if err := recover(); err != nil {
			tx.Rollback()
			panic(err)
		}
	}()
	//将表情添加到用户表
	err = s.db.insertUserStickerTx(&customModel{
		Path:        req.Path,
		UID:         loginUID,
		SortNum:     sortNum,
		Width:       req.Width,
		Height:      req.Height,
		Format:      normFormat,
		Placeholder: req.Placeholder,
		Category:    req.Category,
	}, tx)
	if err != nil {
		tx.Rollback()
		s.Error("添加用户表情错误", zap.Error(err))
		c.ResponseError(errors.New("添加用户表情错误"))
		return
	}
	//将表情添加到所有表情中
	err = s.db.insertStickerTx(&model{
		Path:           req.Path,
		Category:       loginUID,
		UserCustom:     1,
		Width:          req.Width,
		Height:         req.Height,
		SearchableWord: "",
		Format:         normFormat,
		Title:          "",
		Placeholder:    req.Placeholder,
	}, tx)
	if err != nil {
		tx.Rollback()
		s.Error("将用户自定义表情添加到表情中错误", zap.Error(err))
		c.ResponseError(errors.New("将用户自定义表情添加到表情中错误"))
		return
	}

	err = tx.Commit()
	if err != nil {
		s.Error("数据库事物提交失败", zap.Error(err))
		c.ResponseError(errors.New("数据库事物提交失败"))
		tx.Rollback()
		return
	}
	c.ResponseOK()
}

// 用户删除表情
func (s *Sticker) userDelete(c *wkhttp.Context) {
	loginUID := c.GetLoginUID()
	var req struct {
		Paths []string `json:"paths"` //删除表情ID集合
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	if len(req.Paths) == 0 {
		c.ResponseError(errors.New("数据不能为空"))
		return
	}
	err := s.db.deleteUserStickerWithPaths(req.Paths, loginUID)
	if err != nil {
		s.Error("删除表情错误", zap.Error(err))
		c.ResponseError(errors.New("删除表情错误"))
		return
	}
	c.ResponseOK()
}

// 获取用户自定义表情
func (s *Sticker) userCustomSticker(c *wkhttp.Context) {
	loginUID := c.GetLoginUID()
	list, err := s.db.queryUserCustomSticker(loginUID)
	if err != nil {
		s.Error("查询用户自定义表情错误", zap.Error(err))
		c.ResponseError(errors.New("查询用户自定义表情错误"))
		return
	}
	resps := make([]*categoryStickerResp, 0)
	if len(list) > 0 {
		for _, model := range list {
			resps = append(resps, &categoryStickerResp{
				Path:        normalizeStickerAPIPath(model.Path),
				Category:    model.Category,
				Width:       model.Width,
				Height:      model.Height,
				SortNum:     model.SortNum,
				Format:      normalizeStickerFormat(model.Path, model.Format),
				Placeholder: model.Placeholder,
			})
		}
	}
	c.Response(resps)
}

// 通过category删除用户表情
func (s *Sticker) userDeleteByCategory(c *wkhttp.Context) {
	loginUID := c.GetLoginUID()
	category := c.Query("category")
	if category == "" {
		c.ResponseError(errors.New("分类不能为空"))
		return
	}
	err := s.db.deleteUserStickerWithCategory(category, loginUID)
	if err != nil {
		s.Error("移除表情分类错误", zap.Error(err))
		c.ResponseError(errors.New("移除表情分类错误"))
		return
	}
	c.ResponseOK()
}

// 通过分类添加表情
func (s *Sticker) userAddByCategory(c *wkhttp.Context) {
	loginUID := c.GetLoginUID()
	category := c.Param("category")
	if category == "" {
		c.ResponseError(errors.New("分类名不能为空"))
		return
	}
	list, err := s.db.queryStickersByCategory(category)
	if err != nil {
		s.Error("通过category查询表情错误", zap.Error(err))
		c.ResponseError(errors.New("通过category查询表情错误"))
		return
	}
	if len(list) == 0 {
		c.ResponseError(errors.New("该分类下表情为空"))
		return
	}
	model, err := s.db.queryUserCategoryWithCategory(loginUID, category)
	if err != nil {
		s.Error("查询用户分类表情错误", zap.Error(err))
		c.ResponseError(errors.New("查询用户分类表情错误"))
		return
	}
	if model != nil {
		c.ResponseOK()
		return
	}
	cmodel, err := s.db.queryUserCategoryWithMaxSortNum(loginUID)
	if err != nil {
		s.Error("查询最大用户表情分类错误", zap.Error(err))
		c.ResponseError(errors.New("查询最大用户表情分类错误"))
		return
	}
	var sortNum int = 1
	if cmodel != nil {
		sortNum = cmodel.SortNum + 1
	}
	err = s.db.insertUserCategory(&categoryModel{
		UID:      loginUID,
		Category: category,
		SortNum:  sortNum,
	})
	if err != nil {
		s.Error("添加表情分类错误", zap.Error(err))
		c.ResponseError(errors.New("添加表情分类错误"))
		return
	}
	c.ResponseOK()
}

// 获取用户表情分类
func (s *Sticker) getCategorys(c *wkhttp.Context) {
	loginUID := c.GetLoginUID()
	list, err := s.db.queryUserCategorys(loginUID)
	if err != nil {
		s.Error("查询用户表情分类错误", zap.Error(err))
		c.ResponseError(errors.New("查询用户表情分类错误"))
		return
	}
	result := make([]*stickerCategoryResp, 0)
	if len(list) > 0 {
		for _, model := range list {
			if model.Category != loginUID {
				result = append(result, &stickerCategoryResp{
					Category: model.Category,
					Cover:    normalizeStickerAPIPath(model.Cover),
					CoverLim: normalizeStickerAPIPath(model.CoverLim),
					SortNum:  model.SortNum,
					Title:    model.Title,
					Desc:     model.Desc,
				})
			}
		}
	}
	c.Response(result)
}

// 获取分类下的表情
func (s *Sticker) getStickerWithCategory(c *wkhttp.Context) {
	category := c.Query("category")
	if category == "" {
		c.ResponseError(errors.New("参数错误"))
		return
	}
	// 获取表情详情
	m, err := s.db.queryStoreWithCategory(category)
	if err != nil {
		s.Error("查询分类表情错误", zap.Error(err))
		c.ResponseError(errors.New("查询分类表情错误"))
		return
	}
	if m == nil {
		c.ResponseError(errors.New("该分类无表情"))
		return
	}
	isAdded, err := s.db.isAddedCategory(category, c.GetLoginUID())
	if err != nil {
		s.Error("查询登录用户是否添加该分类表情错误", zap.Error(err))
		c.ResponseError(errors.New("查询登录用户是否添加该分类表情错误"))
		return
	}
	list, err := s.db.queryStickersByCategory(category)
	if err != nil {
		s.Error("查询分类下的表情错误", zap.Error(err))
		c.ResponseError(errors.New("查询分类下的表情错误"))
		return
	}
	stickerList := make([]*categoryStickerResp, 0)
	if len(list) > 0 {
		for _, model := range list {
			stickerList = append(stickerList, &categoryStickerResp{
				Path:           normalizeStickerAPIPath(model.Path),
				Category:       model.Category,
				Title:          model.Title,
				Width:          model.Width,
				Height:         model.Height,
				Placeholder:    model.Placeholder,
				Format:         normalizeStickerFormat(model.Path, model.Format),
				SearchableWord: model.SearchableWord,
			})
		}
	}
	var resp = &stickerDetialResp{
		List:     stickerList,
		Title:    m.Title,
		Cover:    normalizeStickerAPIPath(m.Cover),
		CoverLim: normalizeStickerAPIPath(m.CoverLim),
		Category: category,
		Desc:     m.Desc,
		Added:    isAdded,
	}
	c.Response(resp)
}

// 表情商店
func (s *Sticker) list(c *wkhttp.Context) {
	loginUID := c.GetLoginUID()
	pageIndex, pageSize := c.GetPage()
	list, err := s.db.queryStickerStroeWithPage(uint64(pageIndex), uint64(pageSize), loginUID)
	if err != nil {
		s.Error("查询表情商店错误", zap.Error(err))
		c.ResponseError(errors.New("查询表情商店错误"))
		return
	}
	resps := make([]*stickerStoreResp, 0)
	if len(list) > 0 {
		for _, model := range list {
			resps = append(resps, &stickerStoreResp{
				Status:   model.Status,
				Title:    model.Title,
				Desc:     model.Desc,
				// 列表缩略图：相对路径（无前导 /），由客户端用 apiURL + path 拼完整地址。
				Cover:    normalizeStickerAPIPath(model.Cover),
				CoverLim: normalizeStickerAPIPath(model.CoverLim),
				Category: model.Category,
			})
		}
	}
	c.Response(resps)
}

// 将某些表情移到最前
func (s *Sticker) moveToFront(c *wkhttp.Context) {
	loginUID := c.GetLoginUID()
	var req struct {
		Paths []string `json:"paths"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	if len(req.Paths) == 0 {
		c.ResponseError(errors.New("数据不能为空"))
		return
	}
	cmodel, err := s.db.queryUserStickerWithMaxSortNum(loginUID)
	if err != nil {
		s.Error("查询用户表情最大编号错误", zap.Error(err))
		c.ResponseError(errors.New("查询用户表情最大编号错误"))
		return
	}
	var maxNum = 0
	if cmodel != nil {
		maxNum = cmodel.SortNum
	}
	tx, _ := s.ctx.DB().Begin()
	defer func() {
		if err := recover(); err != nil {
			tx.Rollback()
			panic(err)
		}
	}()
	var tempSortNum = len(req.Paths)
	for _, path := range req.Paths {
		err = s.db.updateCustomStickerSortNumTx(path, loginUID, (tempSortNum + maxNum), tx)
		if err != nil {
			tx.Rollback()
			s.Error("修改用户自定义表情顺序错误", zap.Error(err))
			c.ResponseError(errors.New("修改用户自定义表情顺序错误"))
			return
		}
		tempSortNum--
	}
	err = tx.Commit()
	if err != nil {
		s.Error("数据库事物提交失败", zap.Error(err))
		c.ResponseError(errors.New("数据库事物提交失败"))
		tx.Rollback()
		return
	}
	c.ResponseOK()
}

// 将用户表情分类排序
func (s *Sticker) reorderUserCategory(c *wkhttp.Context) {
	loginUID := c.GetLoginUID()
	var req struct {
		Categorys []string `json:"categorys"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	if len(req.Categorys) == 0 {
		c.ResponseError(errors.New("数据不能为空"))
		return
	}
	tx, _ := s.ctx.DB().Begin()
	defer func() {
		if err := recover(); err != nil {
			tx.Rollback()
			panic(err)
		}
	}()
	var sortNum = 0
	for _, category := range req.Categorys {
		err := s.db.updateCategorySortNumTx(category, loginUID, (len(req.Categorys) - sortNum), tx)
		if err != nil {
			tx.Rollback()
			s.Error("修改用户表情分类顺序错误", zap.Error(err))
			c.ResponseError(errors.New("修改用户表情分类顺序错误"))
			return
		}
		sortNum++
	}
	err := tx.Commit()
	if err != nil {
		s.Error("数据库事物提交失败", zap.Error(err))
		c.ResponseError(errors.New("数据库事物提交失败"))
		tx.Rollback()
		return
	}
	c.ResponseOK()
}

// 搜索表情
func (s *Sticker) search(c *wkhttp.Context) {
	keyword := c.Query("keyword")
	page := c.Query("page")
	pageSize := c.Query("page_size")
	if page == "" {
		page = "1"
	}
	if pageSize == "" {
		pageSize = "20"
	}
	pageI, _ := strconv.ParseInt(page, 10, 64)
	pageSizeI, _ := strconv.ParseInt(pageSize, 10, 64)
	list, err := s.db.search(keyword, uint64(pageI), uint64(pageSizeI))
	if err != nil {
		s.Error("查询表情失败！", zap.Error(err))
		c.ResponseError(errors.New("查询表情失败！"))
		return
	}
	resps := make([]*stickerResp, 0)
	if len(list) == 0 {
		c.JSON(http.StatusOK, resps)
		return
	}
	for _, m := range list {
		resps = append(resps, &stickerResp{
			Title:          m.Title,
			Category:       m.Category,
			Height:         m.Height,
			Width:          m.Width,
			Format:         normalizeStickerFormat(m.Path, m.Format),
			Path:           normalizeStickerAPIPath(m.Path),
			Placeholder:    m.Placeholder,
			SearchableWord: m.SearchableWord,
		})
	}
	c.JSON(http.StatusOK, resps)
}

// stickerResp stickerResp
type stickerResp struct {
	Path           string `json:"path"` // 表情地址
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	Title          string `json:"title"`           // 表情名字
	Category       string `json:"category"`        // 分类
	Placeholder    string `json:"placeholder"`     //占位图
	Format         string `json:"format"`          // lim|tgs|gif|webp|png|jpg 等；矢量需 lim/tgs 客户端才走对应解码器
	SearchableWord string `json:"searchable_word"` // 搜索关键字
}

// 表情分类
type stickerCategoryResp struct {
	Category string `json:"category"` // 分类
	// Cover：位图封面相对路径（如 file/preview/sticker/.../cover.png，无前导 /），客户端用 apiURL + cover。
	Cover string `json:"cover"`
	// CoverLim：RLottie 矢量（.lim），仅应用 RLottieDrawable 等矢量通道加载；用 Glide 当位图加载会失败→蓝块。
	CoverLim string `json:"cover_lim"`
	SortNum  int    `json:"sort_num"` //排序号
	Title    string `json:"title"`    // 标题
	Desc     string `json:"desc"`     // 描述
}

// 表情商店
type stickerStoreResp struct {
	Status   int    `json:"status"`    // 1:用户已添加
	Category string `json:"category"`  // 分类
	Cover    string `json:"cover"`     // 位图封面相对路径，列表缩略图
	CoverLim string `json:"cover_lim"` // RLottie .lim，勿用 Glide 直接作缩略图
	Title    string `json:"title"`     // 标题
	Desc     string `json:"desc"`      // 封面
}

type categoryStickerResp struct {
	// Path：一般为 file/preview/sticker/.../xxx.lim（RLottie）；GIF 为 .gif。列表格子若用 Glide 加载 .lim 会蓝块，需 RLottie 或改用同路径 .png 静帧。
	Path           string `json:"path"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	Title          string `json:"title"`           // 表情名字
	Category       string `json:"category"`        // 分类
	Placeholder    string `json:"placeholder"`     //占位图
	Format         string `json:"format"`          // lim|tgs|gif|webp|png|jpg 等
	SortNum        int    `json:"sort_num"`        //排序编号
	SearchableWord string `json:"searchable_word"` // 搜索关键字
}

// 表情详情
type stickerDetialResp struct {
	List     []*categoryStickerResp `json:"list"`  // 表情列表
	Desc     string                 `json:"desc"`  //说明
	Cover    string                 `json:"cover"` // 封面相对路径（位图 .png）
	CoverLim string                 `json:"cover_lim"`
	Title    string                 `json:"title"`    //标题
	Category string                 `json:"category"` //分类
	Added    bool                   `json:"added"`    // 是否添加
}
