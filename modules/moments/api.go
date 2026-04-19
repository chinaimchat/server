package moments

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkevent"

	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/base/event"
	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/file"
	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/user"
	wkutil "github.com/TangSengDaoDao/TangSengDaoDaoServer/pkg/util"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/log"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"go.uber.org/zap"
)

// normalizeV1V1 把 URL/路径里的常见重复前缀收敛掉，避免移动端 404/播放失败。
func normalizeV1V1(s string) string {
	for strings.Contains(s, "/v1/v1") {
		s = strings.ReplaceAll(s, "/v1/v1", "/v1")
	}
	// 兼容客户端把 file/preview 前缀重复拼接导致的路径异常
	for strings.Contains(s, "file/preview/file/preview/") {
		s = strings.ReplaceAll(s, "file/preview/file/preview/", "file/preview/")
	}
	s = strings.ReplaceAll(s, "file/preview/file/preview", "file/preview")
	for strings.Contains(s, "/preview/preview/") {
		s = strings.ReplaceAll(s, "/preview/preview/", "/preview/")
	}
	s = strings.ReplaceAll(s, "preview/preview", "preview")
	return s
}

// Moments 朋友圈
type Moments struct {
	ctx *config.Context
	log.Log
	db           *db
	momentUserDB *momentUserDB
	commentDB    *commentDB
	settingDB    *settingDB
	userService  user.IService
	fileService  file.IService
}

// New  New
func New(ctx *config.Context) *Moments {
	m := &Moments{
		ctx:          ctx,
		db:           newDB(ctx),
		settingDB:    newSettingDB(ctx),
		momentUserDB: newMomentUserDB(ctx),
		commentDB:    newCommentDB(ctx),
		Log:          log.NewTLog("moments"),
		userService:  user.NewService(ctx),
		fileService:  file.NewService(ctx),
	}
	m.ctx.AddEventListener(event.EventUserPublishMoment, m.handlePublishMoment)
	m.ctx.AddEventListener(event.EventUserDeleteMoment, m.handleDeleteMoment)
	m.ctx.AddEventListener(event.FriendSure, m.handleFriendSure)
	m.ctx.AddEventListener(event.FriendDelete, m.handlerFriendDelete)
	return m
}

// Route 路由配置
func (m *Moments) Route(r *wkhttp.WKHttp) {
	moments := r.Group("/v1/moments", m.ctx.AuthMiddleware(r))
	{
		moments.POST("", m.add)                                     // 发布朋友圈
		moments.GET("", m.list)                                     //朋友圈列表
		moments.GET("/:moment_no", m.detail)                        //动态详情
		moments.DELETE("/:moment_no", m.delete)                     //删除朋友圈
		moments.PUT("/:moment_no/like", m.like)                     //点赞
		moments.PUT("/:moment_no/unlike", m.unlike)                 //取消点赞
		moments.POST("/:moment_no/comments", m.comments)            //评论
		moments.DELETE("/:moment_no/comments/:id", m.deleteComment) //删除评论

	}
	moment := r.Group("/v1/moment")
	{
		moment.GET("/cover", m.getMomentCover)                                  // 封面
		moment.GET("/attachment", m.ctx.AuthMiddleware(r), m.attachmentMoments) // 查询某个用户只含有图片和视频的动态
	}
}

// 查询某个用户只包含图片/视频的动态
func (m *Moments) attachmentMoments(c *wkhttp.Context) {
	loginUID := c.GetLoginUID()
	uid := c.Query("uid")
	if uid == "" {
		c.ResponseError(errors.New("查看的用户ID不能为空"))
		return
	}
	momentResps := make([]*momentResp, 0)
	isFriend, err := m.userService.IsFriend(loginUID, uid)
	if err != nil {
		c.ResponseError(errors.New("查询用户好友关系错误"))
		return
	}
	if !isFriend && uid != loginUID {
		c.Response(momentResps)
		return
	}
	setting, err := m.settingDB.queryWithUIDAndToUID(uid, loginUID)
	if err != nil {
		m.Error("查询用户朋友圈设置错误", zap.Error(err))
		c.ResponseError(errors.New("查询用户朋友圈设置错误"))
	}

	if setting != nil && setting.IsHideMy == 1 {
		c.Response(momentResps)
		return
	}
	list := make([]*model, 0)
	// 查询所有表里面的前4条数据
	models, err := m.db.listWithUIDAndImgs("moments1", uid, loginUID)
	if err != nil {
		m.Error("查询只包含图片动态错误", zap.Error(err))
		c.ResponseError(errors.New("查询只包含图片动态错误"))
		return
	}
	if len(models) > 0 {
		list = append(list, models...)
	}
	models, err = m.db.listWithUIDAndImgs("moments2", uid, loginUID)
	if err != nil {
		m.Error("查询只包含图片动态错误", zap.Error(err))
		c.ResponseError(errors.New("查询只包含图片动态错误"))
		return
	}
	if len(models) > 0 {
		list = append(list, models...)
	}
	models, err = m.db.listWithUIDAndImgs("moments3", uid, loginUID)
	if err != nil {
		m.Error("查询只包含图片动态错误", zap.Error(err))
		c.ResponseError(errors.New("查询只包含图片动态错误"))
		return
	}
	if len(models) > 0 {
		list = append(list, models...)
	}
	if len(list) > 0 {
		// 排序
		var flag bool
		for i := 0; i < len(list)-1; i++ {
			flag = true
			for j := 0; j < len(list)-i-1; j++ {
				time1 := time.Time(list[j+1].CreatedAt)
				time2 := time.Time(list[j].CreatedAt)
				if time1.Unix() > time2.Unix() {
					flag = false
					list[j], list[j+1] = list[j+1], list[j]
				}
			}
			if flag {
				break
			}
		}

		for _, model := range list {
			momentResps = append(momentResps, m.getMomentResp(loginUID, model, make([]*likeResp, 0), make([]*commentResp, 0)))
		}
	}

	c.Response(momentResps)
}

// add 发布朋友圈
func (m *Moments) add(c *wkhttp.Context) {
	loginUID := c.MustGet("uid").(string)
	var req momentReq
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	err := m.checkReq(req)
	if err != nil {
		c.ResponseError(err)
		return
	}
	var imgs = ""
	if len(req.Imgs) > 0 {
		imgs = strings.Join(req.Imgs, ",")
	}
	var privacyUids = ""
	if len(req.PrivacyUIDs) > 0 {
		privacyUids = strings.Join(req.PrivacyUIDs, ",")
	}
	var remindUids = ""
	if len(req.RemindUIDs) > 0 {
		remindUids = strings.Join(req.RemindUIDs, ",")
	}
	// 避免客户端传入的 /v1/v1 写入后导致小视频播放 404。
	req.VideoPath = normalizeV1V1(req.VideoPath)
	req.VideoCoverPath = normalizeV1V1(req.VideoCoverPath)
	userInfo, err := m.userService.GetUser(loginUID)
	if err != nil {
		m.Error("查询登录用户信息失败", zap.Error(err))
		c.ResponseError(errors.New("查询登录用户信息失败！"))
	}
	tx, err := m.momentUserDB.session.Begin()
	if err != nil {
		m.Error("开启事物错误", zap.Error(err))
		c.ResponseError(errors.New("开启事物错误"))
		return
	}
	defer func() {
		if err := recover(); err != nil {
			tx.Rollback()
			panic(err)
		}
	}()
	momentNo := util.GenerUUID()
	err = m.db.insertTx(&model{
		MomentNo:       momentNo,
		Publisher:      loginUID,
		PublisherName:  userInfo.Name,
		Imgs:           imgs,
		VideoPath:      req.VideoPath,
		VideoCoverPath: req.VideoCoverPath,
		Content:        req.Text,
		PrivacyType:    req.PrivacyType,
		PrivacyUids:    privacyUids,
		Address:        req.Address,
		Longitude:      req.Longitude,
		Latitude:       req.Latitude,
		RemindUids:     remindUids,
	}, tx)
	if err != nil {
		m.Error("发布朋友圈错误！", zap.Error(err))
		tx.Rollback()
		c.ResponseError(errors.New("发布朋友圈错误！"))
		return
	}

	//发送用户发布动态事件
	eventID, err := m.ctx.EventBegin(&wkevent.Data{
		Event: event.EventUserPublishMoment,
		Type:  wkevent.Message,
		Data: map[string]interface{}{
			"publisher":    loginUID,
			"moment_no":    momentNo,
			"privacy_type": req.PrivacyType,
			"privacy_uids": privacyUids,
			"remind_uids":  remindUids,
		},
	}, tx)

	if err != nil {
		m.Error("开启事件失败！", zap.Error(err))
		tx.Rollback()
		c.ResponseError(errors.New("开启事件失败！"))
		return
	}
	err = tx.Commit()
	if err != nil {
		tx.Rollback()
		m.Error("数据库事物提交失败", zap.Error(err))
		c.ResponseError(errors.New("数据库事物提交失败"))
		return
	}
	m.ctx.EventCommit(eventID)
	c.ResponseOK()
}

// 朋友圈列表
func (m *Moments) list(c *wkhttp.Context) {
	loginUID := c.MustGet("uid").(string)
	pageIndex, pageSize := c.GetPage()
	uid := c.Query("uid")
	var momentUsers []*momentUserModel
	momentResps := make([]*momentResp, 0)
	if uid == "" {
		// 查询朋友圈设置的权限
		//我对好友的设置
		settings, err := m.settingDB.queryWithUID(loginUID)
		if err != nil {
			m.Error("查询用户朋友圈设置错误", zap.Error(err))
			c.ResponseError(errors.New("查询用户朋友圈设置错误"))
			return
		}
		uids := make([]string, 0)
		if len(settings) > 0 {
			for _, setting := range settings {
				if setting.IsHideHis == 1 {
					uids = append(uids, setting.ToUID)
				}
			}
		}
		// 好友对我的设置
		settings, err = m.settingDB.queryWithToUID(loginUID)
		if err != nil {
			m.Error("查询好友对登录用户的朋友圈设置错误", zap.Error(err))
			c.ResponseError(errors.New("查询好友对登录用户的朋友圈设置错误"))
			return
		}
		if len(settings) > 0 {
			for _, setting := range settings {
				if setting.IsHideMy == 1 {
					uids = append(uids, setting.UID)
				}
			}
		}
		if len(uids) == 0 {
			momentUsers, err = m.momentUserDB.queryWithUIDAndPage(loginUID, uint64(pageIndex), uint64(pageSize))
		} else {
			momentUsers, err = m.momentUserDB.queryWithUIDAndExcludeUIDs(loginUID, uids, uint64(pageIndex), uint64(pageSize))
		}
		if err != nil {
			c.ResponseError(errors.New("查询朋友圈错误"))
			return
		}
	} else {
		// 查询登录用户与查看用户是否为好友关系
		isFriend, err := m.userService.IsFriend(loginUID, uid)
		if err != nil {
			c.ResponseError(errors.New("查询用户好友关系错误"))
			return
		}
		if !isFriend && uid != loginUID {
			c.Response(momentResps)
			return
		}
		// 查询朋友圈权限
		// setting, err := m.settingDB.queryWithUIDAndToUID(loginUID, uid)
		// if err != nil {
		// 	c.ResponseError(errors.New("查询用户朋友圈设置信息错误"))
		// 	return
		// }
		// if setting != nil && setting.IsHideHis == 1 {
		// 	c.Response(momentResps)
		// 	return
		// }
		setting, err := m.settingDB.queryWithUIDAndToUID(uid, loginUID)
		if err != nil {
			c.ResponseError(errors.New("查询好友对登录用户朋友圈设置错误"))
			return
		}
		if setting != nil && setting.IsHideMy == 1 {
			c.Response(momentResps)
			return
		}
		momentUsers, err = m.momentUserDB.queryWithPublisherAndPage(uid, uint64(pageIndex), uint64(pageSize))
		if err != nil {
			c.ResponseError(errors.New("查询用户朋友圈错误"))
			return
		}
	}

	if momentUsers == nil {
		c.Response(momentResps)
		return
	}

	//将动态编号分组
	momentNoArr := make([]*momentUserTable, 0)
	for _, momentUser := range momentUsers {
		tableName := m.db.getTableName(momentUser.MomentNo)
		isAdd := true
		for index := range momentNoArr {
			if tableName == momentNoArr[index].TableName {
				momentNoArr[index].momentNos = append(momentNoArr[index].momentNos, momentUser.MomentNo)
				isAdd = false
				break
			}
		}
		if isAdd {
			momentNos := make([]string, 0)
			momentNos = append(momentNos, momentUser.MomentNo)
			momentNoArr = append(momentNoArr, &momentUserTable{
				TableName: tableName,
				momentNos: momentNos,
			})
		}
	}

	momentList := make([]*model, 0)
	for index := range momentNoArr {
		if uid != "" && uid != loginUID {
			// 过滤发布者私有朋友圈和部分可见朋友圈
			list, err := m.db.listWithUID(momentNoArr[index].TableName, loginUID, momentNoArr[index].momentNos)
			if err != nil {
				m.Error("查询某个用户朋友圈错误", zap.Error(err))
				c.ResponseError(errors.New("查询某个用户朋友圈错误"))
				return
			}
			if len(list) > 0 {
				momentList = append(momentList, list...)
			}
		} else {
			list, err := m.db.list(momentNoArr[index].TableName, momentNoArr[index].momentNos)
			if err != nil {
				m.Error("查询朋友圈列表错误", zap.Error(err))
				c.ResponseError(errors.New("查询朋友圈列表错误"))
				return
			}
			if len(list) > 0 {
				momentList = append(momentList, list...)
			}
		}

	}
	momentMap := make(map[string]*model)
	for _, moment := range momentList {
		momentMap[moment.MomentNo] = moment
	}
	for _, momentUser := range momentUsers {
		likeResps, commentResps, err := m.getComments(momentUser.MomentNo, loginUID)
		if err != nil {
			c.ResponseError(err)
			return
		}
		if momentMap[momentUser.MomentNo] != nil {
			momentResps = append(momentResps, m.getMomentResp(loginUID, momentMap[momentUser.MomentNo], likeResps, commentResps))
		}
	}
	c.Response(momentResps)
}

// 删除朋友圈
func (m *Moments) delete(c *wkhttp.Context) {
	loginUID := c.MustGet("uid").(string)
	momentNo := c.Param("moment_no")
	moment, err := m.db.queryWithMomentNo(momentNo)
	if err != nil {
		m.Error("删除朋友圈查询动态失败", zap.Error(err))
		c.ResponseError(errors.New("删除朋友圈查询动态失败"))
		return
	}
	if moment == nil {
		m.Error("删除朋友圈该动态不存在", zap.Error(err))
		c.ResponseError(errors.New("该动态不存在"))
		return
	}
	if moment.Publisher != loginUID {
		c.ResponseError(errors.New("操作用户无权删除本条动态"))
		return
	}
	tx, err := m.momentUserDB.session.Begin()
	if err != nil {
		m.Error("开启事物错误", zap.Error(err))
		c.ResponseError(errors.New("开启事物错误"))
		return
	}
	defer func() {
		if err := recover(); err != nil {
			tx.Rollback()
			panic(err)
		}
	}()
	err = m.db.deleteTx(loginUID, momentNo, tx)
	if err != nil {
		tx.Rollback()
		m.Error("删除朋友圈错误", zap.Error(err))
		c.ResponseError(errors.New("删除朋友圈错误"))
		return
	}
	//发布删除动态事件
	eventID, err := m.ctx.EventBegin(&wkevent.Data{
		Event: event.EventUserDeleteMoment,
		Type:  wkevent.Message,
		Data: map[string]interface{}{
			"publisher": loginUID,
			"moment_no": momentNo,
		},
	}, tx)

	if err != nil {
		m.Error("开启事件失败！", zap.Error(err))
		tx.Rollback()
		c.ResponseError(errors.New("开启事件失败！"))
		return
	}
	err = tx.Commit()
	if err != nil {
		tx.Rollback()
		m.Error("数据库事物提交失败", zap.Error(err))
		c.ResponseError(errors.New("数据库事物提交失败"))
		return
	}
	m.ctx.EventCommit(eventID)
	c.ResponseOK()
}

// 动态详情
func (m *Moments) detail(c *wkhttp.Context) {
	loginUID := c.MustGet("uid").(string)
	momentNo := c.Param("moment_no")
	if strings.TrimSpace(momentNo) == "" {
		m.Error("动态编号不能为空！")
		c.ResponseError(errors.New("动态编号不能为空！"))
		return
	}
	model, err := m.db.queryWithMomentNo(momentNo)
	if err != nil {
		m.Error("查询动态详细错误", zap.Error(err))
		c.ResponseError(errors.New("查询动态详细错误"))
		return
	}
	if model == nil {
		c.ResponseError(errors.New("查询动态不存在"))
		return
	}
	//查询动态下面的评论
	likeResps, commentResps, err := m.getComments(momentNo, loginUID)
	if err != nil {
		c.ResponseError(err)
		return
	}
	c.Response(m.getMomentResp(loginUID, model, likeResps, commentResps))
}

// 返回数据
func (m *Moments) getMomentResp(loginUID string, model *model, likeResps []*likeResp, commentResps []*commentResp) *momentResp {
	apiBaseURL := m.ctx.GetConfig().External.APIBaseURL

	imgs := make([]string, 0)
	if model.Imgs != "" {
		for _, img := range strings.Split(model.Imgs, ",") {
			imgs = append(imgs, wkutil.FullAPIURL(apiBaseURL, strings.TrimSpace(img)))
		}
	}
	privacyUids := make([]string, 0)
	if model.PrivacyUids != "" {
		privacyUids = strings.Split(model.PrivacyUids, ",")
	}

	remindUids := make([]string, 0)
	if model.RemindUids != "" {
		if model.Publisher == loginUID {
			remindUids = strings.Split(model.RemindUids, ",")
		} else {
			if strings.Contains(model.RemindUids, loginUID) {
				remindUids = append(remindUids, loginUID)
			}
		}
	}
	return &momentResp{
		MomentNo:       model.MomentNo,
		PrivacyType:    model.PrivacyType,
		Publisher:      model.Publisher,
		PublisherName:  model.PublisherName,
		Text:           model.Content,
		VideoPath:      wkutil.FullAPIURL(apiBaseURL, model.VideoPath),
		VideoCoverPath: wkutil.FullAPIURL(apiBaseURL, model.VideoCoverPath),
		Imgs:           imgs,
		PrivacyUIDs:    privacyUids,
		RemindUIDs:     remindUids,
		Address:        model.Address,
		Latitude:       model.Latitude,
		Longitude:      model.Longitude,
		CreatedAt:      model.CreatedAt.String(),
		Likes:          likeResps,
		Comments:       commentResps,
	}
}

// 获取动态评论
func (m *Moments) getComments(momentNo, loginUID string) ([]*likeResp, []*commentResp, error) {
	likeResps := make([]*likeResp, 0)
	commentResps := make([]*commentResp, 0)
	comments, err := m.commentDB.queryWithMomentNo(momentNo)
	if err != nil {
		m.Error("查询朋友圈评论错误", zap.Error(err))
		return likeResps, commentResps, err
	}
	if len(comments) > 0 {
		friends, err := m.userService.GetFriends(loginUID)
		if err != nil {
			m.Error("查询登录用户好友错误", zap.Error(err))
			return likeResps, commentResps, err
		}
		//将自己添加到好友中
		friends = append(friends, &user.FriendResp{UID: loginUID})
		for _, comment := range comments {
			isAdd := false
			for _, friend := range friends {
				if friend.UID == comment.UID {
					isAdd = true
					break
				}
			}
			// 判断被回复者是不是好友关系
			if isAdd && comment.ReplyUID != "" {
				tempAdd := false
				for _, friend := range friends {
					if comment.ReplyUID == friend.UID {
						tempAdd = true
						break
					}
				}
				isAdd = tempAdd
			}

			if isAdd {
				if comment.HandleType == 0 {
					//点赞
					likeResps = append(likeResps, &likeResp{
						UID:  comment.UID,
						Name: comment.Name,
					})
				} else {
					//评论
					commentResps = append(commentResps, &commentResp{
						SID:       strconv.FormatInt(comment.Id, 10),
						UID:       comment.UID,
						Name:      comment.Name,
						Content:   comment.Content,
						ReplyUID:  comment.ReplyUID,
						ReplyName: comment.ReplyName,
						CommentAt: comment.CreatedAt.String(),
					})
				}
			}
		}
	}
	return likeResps, commentResps, nil
}

// 获取动态封面
func (m *Moments) getMomentCover(c *wkhttp.Context) {
	uid := c.Query("uid")
	if uid == "" {
		c.ResponseError(errors.New("用户ID不能为空"))
		return
	}
	downloadUrl, _ := m.fileService.DownloadURLForServerFetch(fmt.Sprintf("%s/%s.png", file.TypeMomentCover, uid), uid)
	// Android 等客户端对 302/301 跳转兼容性差（尤其涉及 cleartext），这里直接代理返回二进制流。
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, downloadUrl, nil)
	if err != nil {
		c.ResponseError(err)
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.ResponseError(err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		c.Writer.WriteHeader(resp.StatusCode)
		_, _ = io.CopyN(c.Writer, resp.Body, 1024)
		return
	}

	for _, h := range []string{
		"Content-Type",
		"Content-Length",
		"Content-Disposition",
		"Cache-Control",
		"ETag",
		"Last-Modified",
	} {
		if v := resp.Header.Get(h); v != "" {
			c.Writer.Header().Set(h, v)
		}
	}
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = io.Copy(c.Writer, resp.Body)
}

// checkReq 检测请求参数是否合法
func (m *Moments) checkReq(req momentReq) error {
	if strings.TrimSpace(req.VideoPath) == "" && strings.TrimSpace(req.Text) == "" && len(req.Imgs) == 0 {
		return errors.New("发布内容不能为空！")
	}
	if strings.TrimSpace(req.PrivacyType) == "" {
		return errors.New("隐私类型不能为空！")
	}
	if (req.PrivacyType == "internal" || req.PrivacyType == "prohibit") && len(req.PrivacyUIDs) == 0 {
		return errors.New("部分可见和prohibit不给谁看的用户成员不能为空")
	}
	return nil
}

// momentReq 请求
type momentReq struct {
	VideoPath      string   `json:"video_path"`       //视频地址
	VideoCoverPath string   `json:"video_cover_path"` //视频封面
	Text           string   `json:"text"`             //发布内容
	Imgs           []string `json:"imgs"`             //图片集合
	PrivacyType    string   `json:"privacy_type"`     //隐私类型 【public：公开】【private：私有】【internal：部分可见】【prohibit：不给谁看】
	PrivacyUIDs    []string `json:"privacy_uids"`     //隐私类型对应的用户UID
	Address        string   `json:"address"`          //地址
	Longitude      string   `json:"longitude"`        //经度
	Latitude       string   `json:"latitude"`         //纬度
	RemindUIDs     []string `json:"remind_uids"`      //提醒谁看的用户集合
}

// 返回数据
type momentResp struct {
	MomentNo       string         `json:"moment_no"`        //动态编号
	Publisher      string         `json:"publisher"`        //发布者uid
	PublisherName  string         `json:"publisher_name"`   //发布者名称
	PrivacyType    string         `json:"privacy_type"`     //隐私类型 【public：公开】【private：私有】【internal：部分可见】【prohibit：不给谁看】
	PrivacyUIDs    []string       `json:"privacy_uids"`     //隐私类型对应的用户UID
	VideoPath      string         `json:"video_path"`       //视频地址
	VideoCoverPath string         `json:"video_cover_path"` //视频封面
	Text           string         `json:"text"`             //内容
	CreatedAt      string         `json:"created_at"`       //发布时间
	Imgs           []string       `json:"imgs"`             //图片
	Address        string         `json:"address"`          //动态地址
	Longitude      string         `json:"longitude"`        //经度
	Latitude       string         `json:"latitude"`         //纬度
	RemindUIDs     []string       `json:"remind_uids"`      //提醒谁看的用户集合
	Likes          []*likeResp    `json:"likes"`            // 点赞列表
	Comments       []*commentResp `json:"comments"`         //评论列表
}

// 点赞
type likeResp struct {
	UID  string `json:"uid"`
	Name string `json:"name"`
}

// 评论
type commentResp struct {
	SID       string `json:"sid"`
	UID       string `json:"uid"`
	Name      string `json:"name"`
	Content   string `json:"content"`
	ReplyUID  string `json:"reply_uid"`  // 回复某个人的uid
	ReplyName string `json:"reply_name"` // 回复某个的名字
	CommentAt string `json:"comment_at"` // 评论时间
}
type momentUserTable struct {
	TableName string
	momentNos []string
}
