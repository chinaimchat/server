package moments

import (
	"errors"
	"strings"
	"time"

	wkutil "github.com/TangSengDaoDao/TangSengDaoDaoServer/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/common"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"go.uber.org/zap"
)

// handlePublishMoment 处理发布朋友圈
func (m *Moments) handlePublishMoment(data []byte, commit config.EventCommit) {

	var req map[string]interface{}
	err := util.ReadJsonByByte(data, &req)
	if err != nil {
		m.Error("处理发布朋友圈参数有误")
		commit(err)
		return
	}
	publisher := req["publisher"].(string)
	momentNo := req["moment_no"].(string)
	privacyType := req["privacy_type"].(string)
	privacyUIDs := req["privacy_uids"].(string)
	remindUIDs := req["remind_uids"].(string)
	var uids []string
	if privacyType == "internal" {
		//部分可见
		uids = strings.Split(privacyUIDs, ",")
	} else if privacyType == "public" || privacyType == "prohibit" {
		//公开 || 不给谁看
		//查询发布者的所有好友
		friends, err := m.userService.GetFriends(publisher)
		if err != nil {
			commit(errors.New("查询发布动态好友失败"))
			return
		}

		var friendsMomentSettings []*settingModel
		if len(friends) > 0 {
			friendUIDs := make([]string, 0)
			for _, friend := range friends {
				if friend.IsAlone == 0 { // 单项好友不添加
					friendUIDs = append(friendUIDs, friend.UID)
				}
			}
			friendsMomentSettings, err = m.settingDB.queryWithUIDsAndToUID(friendUIDs, publisher)
			if err != nil {
				commit(errors.New("查询用户好友设置错误"))
				return
			}
		}
		publisherMomentSettings, err := m.settingDB.queryWithUID(publisher)
		if err != nil {
			commit(errors.New("查询用户设置信息错误"))
			return
		}

		if len(friends) > 0 {
			for _, friend := range friends {
				if friend.IsAlone == 1 { // 单项好友不添加
					continue
				}
				var isAdd bool
				if privacyType == "prohibit" {
					isAdd = !strings.Contains(privacyUIDs, friend.UID)
				} else {
					isAdd = true
				}
				if isAdd {
					// 判断发布者是否对好友隐藏了自己的动态
					if len(publisherMomentSettings) > 0 {
						for _, setting := range publisherMomentSettings {
							if setting.ToUID == friend.UID && setting.IsHideMy == 1 {
								isAdd = false
								break
							}
						}
					}
				}

				if isAdd {
					// 判断发布者好友是否不看发布者动态
					if len(friendsMomentSettings) > 0 {
						for _, setting := range friendsMomentSettings {
							if setting.UID == friend.UID && setting.ToUID == publisher && setting.IsHideHis == 1 {
								isAdd = false
								break
							}
						}
					}
				}

				if isAdd {
					uids = append(uids, friend.UID)
				}
			}
		}
	}
	//将自己添加到关系表中
	uids = append(uids, publisher)
	uids = util.RemoveRepeatedElement(uids)
	tx, err := m.momentUserDB.session.Begin()
	if err != nil {
		m.Error("开启事物错误", zap.Error(err))
		commit(errors.New("开启事物错误"))
		return
	}
	defer func() {
		if err := recover(); err != nil {
			tx.Rollback()
			commit(err.(error))
			panic(err)
		}
	}()
	//将发布者的所有好友中添加一条动态信息
	for _, uid := range uids {
		_, err = m.momentUserDB.insertTx(&momentUserModel{
			UID:       uid,
			Publisher: publisher,
			MomentNo:  momentNo,
			SortNum:   time.Now().Unix(),
		}, tx)
		if err != nil {
			m.Error("将发布者动态添加到好友失败", zap.Error(err))
			tx.Rollback()
			commit(err)
			return
		}
	}
	err = tx.Commit()
	if err != nil {
		m.Error("数据库事物提交失败", zap.Error(err))
		tx.Rollback()
		commit(err)
		return
	}
	err = m.sendMsg(momentNo, publisher, uids, "publish")
	if err != nil {
		commit(err)
		return
	}
	//如果是提及到用户就发送提醒消息
	if remindUIDs != "" {
		remindList := strings.Split(remindUIDs, ",")
		err = m.sendMsg(momentNo, publisher, remindList, "remind")
		if err != nil {
			commit(err)
			return
		}
	}
	commit(nil)
}

// handleDeleteMoment 处理删除朋友圈动态
func (m *Moments) handleDeleteMoment(data []byte, commit config.EventCommit) {
	var req map[string]interface{}
	err := util.ReadJsonByByte(data, &req)
	if err != nil {
		m.Error("处理发布朋友圈参数有误")
		commit(err)
		return
	}
	publisher := req["publisher"].(string)
	momentNo := req["moment_no"].(string)

	if publisher == "" {
		commit(errors.New("发布者UID不能为空！"))
		return
	}
	if momentNo == "" {
		commit(errors.New("动态不能为空！"))
		return
	}
	tx, err := m.momentUserDB.session.Begin()
	if err != nil {
		m.Error("开启事物错误", zap.Error(err))
		commit(errors.New("开启事物错误"))
		return
	}
	defer func() {
		if err := recover(); err != nil {
			tx.Rollback()
			commit(err.(error))
			panic(err)
		}
	}()
	//删除朋友圈关系链
	err = m.momentUserDB.deleteTx("moment_user1", publisher, momentNo, tx)
	if err != nil {
		tx.Rollback()
		commit(errors.New("删除发布者好友动态失败"))
		return
	}
	err = m.momentUserDB.deleteTx("moment_user2", publisher, momentNo, tx)
	if err != nil {
		tx.Rollback()
		commit(errors.New("删除发布者好友动态失败"))
		return
	}
	err = m.momentUserDB.deleteTx("moment_user3", publisher, momentNo, tx)
	if err != nil {
		tx.Rollback()
		commit(err)
		return
	}
	//删除朋友圈评论
	err = m.commentDB.deleteWidthMomentNo(momentNo, tx)
	if err != nil {
		m.Error("删除动态评论错误", zap.Error(err))
		tx.Rollback()
		commit(err)
		return
	}
	err = tx.Commit()
	if err != nil {
		m.Error("数据库事物提交失败", zap.Error(err))
		tx.Rollback()
		commit(err)
		return
	}
	commit(nil)
}

// 发送cmd消息
func (m *Moments) sendMsg(momentNo string, publisher string, uids []string, action string) error {
	// 将发布者移除
	var sendMsgUids []string
	for _, uid := range uids {
		if uid != publisher {
			sendMsgUids = append(sendMsgUids, uid)
		}
	}
	if len(sendMsgUids) == 0 {
		return nil
	}
	userInfo, err := m.userService.GetUser(publisher)
	if err != nil {
		m.Error("查询发布者信息失败", zap.Error(err))
		return errors.New("查询发布者信息失败")
	}
	if userInfo == nil {
		return errors.New("发布者不能存在")
	}
	moment, err := m.db.queryWithMomentNo(momentNo)
	if err != nil {
		return errors.New("查询动态失败")
	}
	if moment == nil {
		return errors.New("动态不存在")
	}
	apiBaseURL := m.ctx.GetConfig().External.APIBaseURL
	imgs := make([]string, 0)
	if moment.Imgs != "" {
		for _, img := range strings.Split(moment.Imgs, ",") {
			imgs = append(imgs, wkutil.FullAPIURL(apiBaseURL, strings.TrimSpace(img)))
		}
	}
	err = m.ctx.SendCMD(config.MsgCMDReq{
		CMD:         common.CMDMomentMsg,
		Subscribers: sendMsgUids,
		Param: map[string]interface{}{
			"action":    action,
			"uid":       publisher,
			"name":      userInfo.Name,
			"action_at": time.Now().Unix(),
			"moment_no": momentNo,
			"content": map[string]interface{}{
				"moment_content":    moment.Content,
				"video_conver_path": wkutil.FullAPIURL(apiBaseURL, moment.VideoCoverPath),
				"imgs":              imgs,
			},
		},
	})
	if err != nil {
		m.Error("发送发布动态命令失败！")
		return errors.New("发送发布动态命令失败！")
	}
	return nil
}

// 处理通过好友申请建立朋友圈好友关系
func (m *Moments) handleFriendSure(data []byte, commit config.EventCommit) {
	var req map[string]interface{}
	err := util.ReadJsonByByte(data, &req)
	if err != nil {
		m.Error("朋友圈处理通过好友申请参数有误")
		commit(err)
		return
	}
	uid := req["uid"].(string)
	toUID := req["to_uid"].(string)
	if uid == "" || toUID == "" {
		commit(errors.New("好友ID不能为空"))
		return
	}
	list, err := m.momentUserDB.queryWithUIDAndSize(uid, 50)
	if err != nil {
		m.Error("查询用户动态数据错误", zap.Error(err))
		commit(errors.New("查询用户动态数据错误"))
		return
	}
	tx, err := m.momentUserDB.session.Begin()
	if err != nil {
		m.Error("开启事物错误", zap.Error(err))
		commit(errors.New("开启事物错误"))
		return
	}
	defer func() {
		if err := recover(); err != nil {
			tx.Rollback()
			commit(err.(error))
			panic(err)
		}
	}()
	if len(list) > 0 {
		// 添加数据
		for _, model := range list {
			_, err = m.momentUserDB.insertTx(&momentUserModel{
				UID:       toUID,
				MomentNo:  model.MomentNo,
				Publisher: uid,
				SortNum:   model.SortNum,
			}, tx)
			if err != nil {
				tx.Rollback()
				m.Error("添加朋友圈关系错误", zap.Error(err))
				commit(errors.New("添加朋友圈关系错误"))
				return
			}
		}
	}
	list, err = m.momentUserDB.queryWithUIDAndSize(toUID, 50)
	if err != nil {
		tx.Rollback()
		m.Error("查询用户动态数据错误", zap.Error(err))
		commit(errors.New("查询用户动态数据错误"))
		return
	}
	if len(list) > 0 {
		// 添加数据
		for _, model := range list {
			_, err = m.momentUserDB.insertTx(&momentUserModel{
				UID:       uid,
				MomentNo:  model.MomentNo,
				Publisher: toUID,
				SortNum:   model.SortNum,
			}, tx)
			if err != nil {
				tx.Rollback()
				m.Error("添加朋友圈关系错误", zap.Error(err))
				commit(errors.New("添加朋友圈关系错误"))
				return
			}
		}
	}
	if err := tx.Commit(); err != nil {
		tx.RollbackUnlessCommitted()
		m.Error("提交事务失败！", zap.Error(err))
		commit(errors.New("提交事务失败！"))
		return
	}
	commit(nil)
}

// 处理删除好友时对应删除好友关系链
func (m *Moments) handlerFriendDelete(data []byte, commit config.EventCommit) {
	var req map[string]interface{}
	err := util.ReadJsonByByte(data, &req)
	if err != nil {
		m.Error("处理删除好友参数错误")
		commit(err)
		return
	}
	uid := req["uid"].(string)
	toUID := req["to_uid"].(string)
	if uid == "" || toUID == "" {
		commit(errors.New("好友ID不能为空"))
		return
	}
	tx, err := m.momentUserDB.session.Begin()
	if err != nil {
		m.Error("开启事物错误", zap.Error(err))
		commit(errors.New("开启事物错误"))
		return
	}
	defer func() {
		if err := recover(); err != nil {
			tx.Rollback()
			commit(err.(error))
			panic(err)
		}
	}()
	err = m.momentUserDB.deleteWithUIDAndPublisherTx(uid, toUID, tx)
	if err != nil {
		tx.Rollback()
		m.Error("删除用户朋友圈关系链错误", zap.Error(err))
		commit(errors.New("删除用户朋友圈关系链错误"))
		return
	}
	err = m.momentUserDB.deleteWithUIDAndPublisherTx(toUID, uid, tx)
	if err != nil {
		tx.Rollback()
		m.Error("删除用户朋友圈关系链错误", zap.Error(err))
		commit(errors.New("删除用户朋友圈关系链错误"))
		return
	}
	if err := tx.Commit(); err != nil {
		tx.RollbackUnlessCommitted()
		m.Error("提交事务失败！", zap.Error(err))
		commit(errors.New("提交事务失败！"))
		return
	}
	commit(nil)
}
