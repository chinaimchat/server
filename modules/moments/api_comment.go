package moments

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	wkutil "github.com/TangSengDaoDao/TangSengDaoDaoServer/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/common"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
)

// like 点赞
func (m *Moments) like(c *wkhttp.Context) {
	loginUID := c.MustGet("uid").(string)
	momentNo := c.Param("moment_no")
	if momentNo == "" {
		m.Error("点赞朋友圈编号不能为空")
		c.ResponseError(errors.New("朋友圈编号不能为空"))
		return
	}
	userInfo, err := m.userService.GetUser(loginUID)
	if err != nil {
		m.Error("查询点赞用户信息错误")
		c.ResponseError(errors.New("查询点赞用户信息错误"))
		return
	}
	if userInfo == nil {
		m.Error("操作用户不存在")
		c.ResponseError(errors.New("操作用户不存在"))
		return
	}
	_, err = m.commentDB.insert(&commentModel{
		MomentNo:   momentNo,
		UID:        loginUID,
		Name:       userInfo.Name,
		HandleType: 0,
	})
	if err != nil {
		m.Error("点赞朋友圈报错")
		c.ResponseError(errors.New("点赞朋友圈报错"))
		return
	}
	isLike, err := m.ctx.Cache().Get(loginUID + momentNo)
	if err != nil {
		m.Error("获取点赞缓存失败", zap.Error(err))
		c.ResponseError(errors.New("获取点赞缓存失败"))
		return
	}
	if isLike == "" {
		//发送cmd消息
		err = m.sendMSG(momentNo, loginUID, userInfo.Name, "", "like", 0)
		if err != nil {
			c.ResponseError(err)
			return
		}
		err = m.ctx.Cache().SetAndExpire(loginUID+momentNo, "1", time.Hour*24)
		if err != nil {
			m.Error("设置点赞缓存失败！", zap.Error(err))
			c.ResponseError(errors.New("设置点赞缓存失败！"))
			return
		}
	}
	c.ResponseOK()
}

// unlike 取消点赞
func (m *Moments) unlike(c *wkhttp.Context) {
	loginUID := c.MustGet("uid").(string)
	loginName := c.GetLoginName()
	momentNo := c.Param("moment_no")
	if momentNo == "" {
		m.Error("取消点赞动态编号不能为空")
		c.ResponseError(errors.New("取消点赞动态编号不能为空"))
		return
	}
	err := m.commentDB.delete(loginUID, momentNo, 0)
	if err != nil {
		m.Error("取消点赞错误")
		c.ResponseError(errors.New("取消点赞失败"))
		return
	}
	err = m.sendMSG(momentNo, loginUID, loginName, "", "unlike", 0)
	if err != nil {
		c.ResponseError(err)
		return
	}
	c.ResponseOK()
}

// comments 评论动态
func (m *Moments) comments(c *wkhttp.Context) {
	loginUID := c.MustGet("uid").(string)
	momentNo := c.Param("moment_no")
	var req commentReq
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	if err := m.checkCommentReq(req); err != nil {
		c.ResponseError(err)
		return
	}
	if len(req.Content) > 100 {
		m.Error("评论内容太长！")
		c.ResponseError(errors.New("评论内容太长！"))
		return
	}
	userInfo, err := m.userService.GetUser(loginUID)
	if err != nil {
		m.Error("查询评论用户信息错误")
		c.ResponseError(errors.New("查询评论用户信息错误"))
		return
	}
	if userInfo == nil {
		m.Error("操作用户不存在")
		c.ResponseError(errors.New("操作用户不存在"))
		return
	}
	comment := &commentModel{
		UID:            loginUID,
		Name:           userInfo.Name,
		MomentNo:       momentNo,
		Content:        req.Content,
		ReplyCommentID: req.ReplyCommentID,
		ReplyUID:       req.ReplyUID,
		ReplyName:      req.ReplyName,
		HandleType:     1,
	}
	id, err := m.commentDB.insert(comment)
	if err != nil {
		m.Error("添加评论错误", zap.Error(err))
		c.ResponseError(err)
		return
	}
	// 发送消息

	err = m.sendMSG(momentNo, loginUID, userInfo.Name, req.Content, "comment", id)
	if err != nil {
		c.ResponseError(err)
		return
	}
	c.Response(map[string]interface{}{
		"id": strconv.FormatInt(id, 10),
	})
}

// delete 删除评论
func (m *Moments) deleteComment(c *wkhttp.Context) {
	loginUID := c.MustGet("uid").(string)
	momentNo := c.Param("moment_no")
	loginName := c.GetLoginName()
	id := c.Param("id")
	if strings.TrimSpace(momentNo) == "" {
		m.Error("删除评论的动态编号不能为空")
		c.ResponseError(errors.New("删除评论的动态编号不能为空"))
		return
	}
	commentID, _ := strconv.Atoi(id)
	err := m.commentDB.deleteComment(loginUID, momentNo, int64(commentID))
	if err != nil {
		m.Error("删除动态评论错误", zap.Error(err))
		c.ResponseError(errors.New("删除动态评论错误"))
		return
	}
	err = m.sendMSG(momentNo, loginUID, loginName, "", "delete_comment", int64(commentID))
	if err != nil {
		c.ResponseError(err)
		return
	}
	c.ResponseOK()
}

// sendMSG 发送操作消息
func (m *Moments) sendMSG(momentNo string, loginUID string, loginName string, comment string, action string, commentID int64) error {
	var uids []string
	model, err := m.db.queryWithMomentNo(momentNo)
	if err != nil {
		m.Error("查询动态详情失败", zap.Error(err))
		return errors.New("查询动态详情失败")
	}
	if model == nil {
		return errors.New("动态不存在")
	}
	//查询本条动态的所有评论或点赞数据
	list, err := m.commentDB.queryWithMomentNo(momentNo)
	if err != nil {
		return errors.New("查询动态评论失败")
	}
	isAddPublisher := true
	if len(list) > 0 {
		commentUids := make([]string, 0)
		for _, comment := range list {
			commentUids = append(commentUids, comment.UID)
		}
		friends, err := m.userService.GetFriendsWithToUIDs(loginUID, commentUids)
		if err != nil {
			return errors.New("查询评论或点赞用户的好友列表错误")
		}
		for _, comment := range list {
			if model.Publisher == loginUID {
				if comment.UID != loginUID {
					uids = append(uids, comment.UID)
				}
			} else {
				isAdd := false
				if len(friends) > 0 {
					for _, friend := range friends {
						if friend.UID == comment.UID && friend.IsAlone == 0 {
							isAdd = true
							break
						}
					}
				}
				if comment.UID != loginUID && isAdd {
					uids = append(uids, comment.UID)
				}
			}
			if comment.UID == model.Publisher {
				isAddPublisher = false
			}
		}
	}
	//自己操作自己发布的动态不需要发送消息提醒
	if model.Publisher == loginUID {
		isAddPublisher = false
	}
	if isAddPublisher {
		uids = append(uids, model.Publisher)
	}
	uids = util.RemoveRepeatedElement(uids)
	//发送点赞评论消息
	moment, err := m.db.queryWithMomentNo(momentNo)
	if err != nil {
		return errors.New("查询动态失败")
	}
	if moment == nil {
		return errors.New("动态不存在")
	}
	if len(uids) > 0 {
		apiBaseURL := m.ctx.GetConfig().External.APIBaseURL
		imgs := make([]string, 0)
		if moment.Imgs != "" {
			for _, img := range strings.Split(moment.Imgs, ",") {
				imgs = append(imgs, wkutil.FullAPIURL(apiBaseURL, strings.TrimSpace(img)))
			}
		}
		err = m.ctx.SendCMD(config.MsgCMDReq{
			CMD:         common.CMDMomentMsg,
			Subscribers: uids,
			Param: map[string]interface{}{
				"action":     action,
				"comment_id": commentID,
				"uid":        loginUID,
				"name":       loginName,
				"moment_no":  momentNo,
				"comment":    comment,
				"action_at":  time.Now().Unix(),
				"content": map[string]interface{}{
					"moment_content":    moment.Content,
					"video_conver_path": wkutil.FullAPIURL(apiBaseURL, moment.VideoCoverPath),
					"imgs":              imgs,
				},
			},
		})
		if err != nil {
			m.Error("发送点赞动态命令失败！", zap.Error(err))
			return errors.New("发送点赞动态命令失败！")
		}
	}
	return nil
}

// checkCommentReq  检查评论请求内容
func (m *Moments) checkCommentReq(req commentReq) error {
	if strings.TrimSpace(req.Content) == "" {
		return errors.New("评论内容不能为空")
	}
	return nil
}

// commentReq 评论model
type commentReq struct {
	Content        string `json:"content"`          //评论内容
	ReplyCommentID string `json:"reply_comment_id"` //回复评论的id
	ReplyUID       string `json:"reply_uid"`        //回复某人的uid
	ReplyName      string `json:"reply_name"`       //回复某人的名字
}
