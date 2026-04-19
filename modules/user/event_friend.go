package user

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/source"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/common"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

// 处理通过好友
func (f *Friend) handleFriendSure(data []byte, commit config.EventCommit) {
	var req map[string]interface{}
	err := util.ReadJsonByByte(data, &req)
	if err != nil {
		f.Error("好友关系处理通过好友申请参数有误")
		commit(err)
		return
	}
	uid := req["uid"].(string)
	toUID := req["to_uid"].(string)
	if uid == "" || toUID == "" {
		commit(errors.New("好友ID不能为空"))
		return
	}
	loginUidList := make([]string, 0, 1)
	loginUidList = append(loginUidList, toUID)
	err = f.ctx.IMWhitelistAdd(config.ChannelWhitelistReq{
		ChannelReq: config.ChannelReq{
			ChannelID:   uid,
			ChannelType: common.ChannelTypePerson.Uint8(),
		},
		UIDs: loginUidList,
	})
	if err != nil {
		commit(errors.New("添加IM白名单错误"))
		return
	}
	applyUIDList := make([]string, 0, 1)
	applyUIDList = append(applyUIDList, uid)
	err = f.ctx.IMWhitelistAdd(config.ChannelWhitelistReq{
		ChannelReq: config.ChannelReq{
			ChannelID:   toUID,
			ChannelType: common.ChannelTypePerson.Uint8(),
		},
		UIDs: applyUIDList,
	})
	if err != nil {
		commit(errors.New("添加IM白名单错误"))
		return
	}
	commit(nil)
}

// 处理删除好友
func (f *Friend) handleDeleteFriend(data []byte, commit config.EventCommit) {
	var req map[string]interface{}
	err := util.ReadJsonByByte(data, &req)
	if err != nil {
		f.Error("处理删除好友参数错误")
		commit(err)
		return
	}
	uid := req["uid"].(string)
	toUID := req["to_uid"].(string)
	if uid == "" || toUID == "" {
		commit(errors.New("好友ID不能为空"))
		return
	}

	err = f.ctx.IMWhitelistRemove(config.ChannelWhitelistReq{
		ChannelReq: config.ChannelReq{
			ChannelID:   toUID,
			ChannelType: common.ChannelTypePerson.Uint8(),
		},
		UIDs: []string{uid},
	})
	if err != nil {
		commit(errors.New("移除IM白名单错误"))
		return
	}
	err = f.ctx.IMWhitelistRemove(config.ChannelWhitelistReq{
		ChannelReq: config.ChannelReq{
			ChannelID:   uid,
			ChannelType: common.ChannelTypePerson.Uint8(),
		},
		UIDs: []string{toUID},
	})
	if err != nil {
		commit(errors.New("移除IM白名单错误"))
		return
	}
	commit(nil)
}

// 处理用户注册
func (f *Friend) handleUserRegister(data []byte, commit config.EventCommit) {
	var req map[string]interface{}
	err := util.ReadJsonByByte(data, &req)
	if err != nil {
		f.Error("好友处理用户注册加入群聊参数有误")
		commit(err)
		return
	}
	uid := strings.TrimSpace(fmt.Sprint(req["uid"]))
	if uid == "" {
		f.Error("好友处理用户注册uid不能为空")
		commit(errors.New("好友处理用户注册uid不能为空"))
		return
	}
	inviteVercode := strings.TrimSpace(fmt.Sprint(req["invite_vercode"]))
	if strings.EqualFold(inviteVercode, "<nil>") || strings.EqualFold(inviteVercode, "nil") {
		inviteVercode = ""
	}

	// 通过邀请码注册：按邀请码配置自动加好友（免验证，不阻断注册主流程）
	inviteCode := strings.TrimSpace(fmt.Sprint(req["invite_code"]))
	f.Info("邀请码自动加好友-事件入参", zap.String("uid", uid), zap.String("invite_code", inviteCode), zap.String("invite_vercode", inviteVercode))
	if inviteCode != "" {
		type row struct {
			FriendsJSON string `db:"friends_json"`
		}
		var rows []*row
		_, _ = f.ctx.DB().Select("friends_json").From("invite_code").Where("invite_code=? and status=1", inviteCode).Limit(1).Load(&rows)
		if len(rows) == 0 {
			f.Warn("邀请码自动加好友-未匹配到邀请码配置", zap.String("uid", uid), zap.String("invite_code", inviteCode))
		}
		if len(rows) > 0 && strings.TrimSpace(rows[0].FriendsJSON) != "" {
			f.Info("邀请码自动加好友-读取配置", zap.String("uid", uid), zap.String("invite_code", inviteCode), zap.String("friends_json", rows[0].FriendsJSON))
			type friendCfg struct {
				FriendUID      string `json:"friend_uid"`
				WelcomeMessage string `json:"welcome_message"`
			}
			list := make([]friendCfg, 0)
			if err := json.Unmarshal([]byte(rows[0].FriendsJSON), &list); err == nil {
				srcVercode := inviteVercode
				if srcVercode == "" {
					// 兜底来源，避免因 invite_vercode 缺失导致自动加好友整段被跳过。
					srcVercode = fmt.Sprintf("%s@%d", util.GenerUUID(), common.InvitationCode)
				}
				for _, item := range list {
					targetUID := strings.TrimSpace(item.FriendUID)
					if targetUID == "" || targetUID == uid {
						continue
					}
					f.Info("邀请码自动加好友-开始执行", zap.String("uid", uid), zap.String("to_uid", targetUID))
					if e := f.autoMakeFriend(uid, targetUID, srcVercode, strings.TrimSpace(item.WelcomeMessage)); e != nil {
						f.Warn("邀请码自动加好友失败", zap.String("uid", uid), zap.String("to_uid", targetUID), zap.Error(e))
					}
				}
			} else {
				f.Warn("邀请码自动加好友-配置解析失败", zap.String("uid", uid), zap.String("invite_code", inviteCode), zap.Error(err))
			}
		}
	}

	// 旧逻辑：邀请人（invite_uid）自动成为好友。若缺少 invite_vercode/invite_uid，则仅跳过这段，不影响上面的邀请码自动加好友。
	if inviteVercode == "" {
		commit(nil)
		return
	}
	inviteUid := strings.TrimSpace(fmt.Sprint(req["invite_uid"]))
	if inviteUid == "" {
		commit(nil)
		return
	}
	// 是否是好友
	applyFriendModel, err := f.db.queryWithUID(uid, inviteUid)
	if err != nil {
		f.Error("查询是否是好友失败！", zap.Error(err), zap.String("uid", uid), zap.String("toUid", inviteUid))
		commit(errors.New("查询是否是好友失败！"))
		return
	}
	// 添加好友到数据库
	tx, err := f.ctx.DB().Begin()
	if err != nil {
		f.Error("开启事务失败！", zap.Error(err))
		commit(errors.New("开启事务失败！"))
		return
	}
	defer func() {
		if err := recover(); err != nil {
			tx.Rollback()
			panic(err)
		}
	}()
	version := f.ctx.GenSeq(common.FriendSeqKey)
	if applyFriendModel == nil {
		// 验证code
		err = source.CheckSource(inviteVercode)
		if err != nil {
			commit(err)
			return
		}

		util.CheckErr(err)
		err = f.db.InsertTx(&FriendModel{
			UID:           uid,
			ToUID:         inviteUid,
			Version:       version,
			Initiator:     0,
			IsAlone:       0,
			Vercode:       fmt.Sprintf("%s@%d", util.GenerUUID(), common.Friend),
			SourceVercode: inviteVercode,
		}, tx)
		if err != nil {
			tx.Rollback()
			commit(errors.New("添加好友失败！"))
			return
		}
	} else {
		err = f.db.updateRelationshipTx(uid, inviteUid, 0, 0, inviteVercode, version, tx)
		if err != nil {
			tx.Rollback()
			commit(errors.New("修改好友关系失败"))
			return
		}
	}
	// 是否是好友
	loginFriendModel, err := f.db.queryWithUID(inviteUid, uid)
	//loginIsFriend, err := f.db.IsFriend(applyUID, loginUID)
	if err != nil {
		tx.Rollback()
		f.Error("查询被添加者是否是好友失败！", zap.Error(err), zap.String("uid", uid), zap.String("toUid", inviteUid))
		commit(errors.New("查询被添加者是否是好友失败！"))
		return
	}
	if loginFriendModel == nil {
		err = f.db.InsertTx(&FriendModel{
			UID:           inviteUid,
			ToUID:         uid,
			Version:       version,
			Initiator:     1,
			IsAlone:       0,
			Vercode:       fmt.Sprintf("%s@%d", util.GenerUUID(), common.Friend),
			SourceVercode: inviteVercode,
		}, tx)
		if err != nil {
			tx.Rollback()
			commit(errors.New("添加好友失败！"))
			return
		}
	} else {
		err = f.db.updateRelationshipTx(inviteUid, uid, 0, 0, inviteVercode, version, tx)
		if err != nil {
			tx.Rollback()
			commit(errors.New("修改好友关系失败"))
			return
		}
	}
	if err := tx.Commit(); err != nil {
		f.Error("提交事务失败！", zap.Error(err))
		commit(errors.New("提交事务失败！"))
		return
	}
	// 添加白名单
	loginUidList := make([]string, 0, 1)
	loginUidList = append(loginUidList, inviteUid)
	err = f.ctx.IMWhitelistAdd(config.ChannelWhitelistReq{
		ChannelReq: config.ChannelReq{
			ChannelID:   uid,
			ChannelType: common.ChannelTypePerson.Uint8(),
		},
		UIDs: loginUidList,
	})
	if err != nil {
		commit(errors.New("添加IM白名单错误"))
		return
	}
	applyUIDList := make([]string, 0, 1)
	applyUIDList = append(applyUIDList, uid)
	err = f.ctx.IMWhitelistAdd(config.ChannelWhitelistReq{
		ChannelReq: config.ChannelReq{
			ChannelID:   inviteUid,
			ChannelType: common.ChannelTypePerson.Uint8(),
		},
		UIDs: applyUIDList,
	})
	if err != nil {
		commit(errors.New("添加IM白名单错误"))
		return
	}
	userInfo, err := f.userDB.QueryByUID(uid)
	if err != nil {
		commit(errors.New("查询用户资料错误"))
		return
	}
	if userInfo == nil {
		commit(errors.New("用户不存在"))
		return
	}
	// 发送确认消息给对方
	err = f.ctx.SendCMD(config.MsgCMDReq{
		CMD:         common.CMDFriendAccept,
		Subscribers: []string{uid, inviteUid},
		Param: map[string]interface{}{
			"to_uid":    inviteUid,
			"from_uid":  uid,
			"from_name": userInfo.Name,
		},
	})
	if err != nil {
		f.Error("发送消息失败！", zap.Error(err))
		commit(errors.New("发送消息失败！"))
		return
	}
	content := "我们已经是好友了，可以愉快的聊天了！"
	if f.ctx.GetConfig().Friend.AddedTipsText != "" {
		content = f.ctx.GetConfig().Friend.AddedTipsText
	}
	// 发送消息
	payload := []byte(util.ToJson(map[string]interface{}{
		"content": content,
		"type":    common.Tip,
	}))

	err = f.ctx.SendMessage(&config.MsgSendReq{
		FromUID:     uid,
		ChannelID:   inviteUid,
		ChannelType: common.ChannelTypePerson.Uint8(),
		Payload:     payload,
		Header: config.MsgHeader{
			RedDot: 1,
		},
	})
	if err != nil {
		f.Error("发送通过好友请求消息失败！", zap.Error(err))
		commit(errors.New("发送通过好友请求消息失败！"))
		return
	}

	commit(nil)
}

func (f *Friend) autoMakeFriend(uid, toUID, sourceVercode, welcomeMessage string) error {
	if uid == "" || toUID == "" || uid == toUID {
		return nil
	}
	tx, err := f.ctx.DB().Begin()
	if err != nil {
		return err
	}
	version := f.ctx.GenSeq(common.FriendSeqKey)
	m1, err := f.db.queryWithUID(uid, toUID)
	if err != nil {
		tx.Rollback()
		return err
	}
	if m1 == nil {
		if err = f.db.InsertTx(&FriendModel{
			UID:           uid,
			ToUID:         toUID,
			Version:       version,
			Initiator:     0,
			IsAlone:       0,
			Vercode:       fmt.Sprintf("%s@%d", util.GenerUUID(), common.Friend),
			SourceVercode: sourceVercode,
		}, tx); err != nil {
			tx.Rollback()
			return err
		}
	} else {
		if err = f.db.updateRelationshipTx(uid, toUID, 0, 0, sourceVercode, version, tx); err != nil {
			tx.Rollback()
			return err
		}
	}
	m2, err := f.db.queryWithUID(toUID, uid)
	if err != nil {
		tx.Rollback()
		return err
	}
	if m2 == nil {
		if err = f.db.InsertTx(&FriendModel{
			UID:           toUID,
			ToUID:         uid,
			Version:       version,
			Initiator:     1,
			IsAlone:       0,
			Vercode:       fmt.Sprintf("%s@%d", util.GenerUUID(), common.Friend),
			SourceVercode: sourceVercode,
		}, tx); err != nil {
			tx.Rollback()
			return err
		}
	} else {
		if err = f.db.updateRelationshipTx(toUID, uid, 0, 0, sourceVercode, version, tx); err != nil {
			tx.Rollback()
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		tx.Rollback()
		return err
	}
	if err = f.ctx.IMWhitelistAdd(config.ChannelWhitelistReq{
		ChannelReq: config.ChannelReq{ChannelID: uid, ChannelType: common.ChannelTypePerson.Uint8()},
		UIDs:       []string{toUID},
	}); err != nil {
		return err
	}
	if err = f.ctx.IMWhitelistAdd(config.ChannelWhitelistReq{
		ChannelReq: config.ChannelReq{ChannelID: toUID, ChannelType: common.ChannelTypePerson.Uint8()},
		UIDs:       []string{uid},
	}); err != nil {
		return err
	}
	userInfo, _ := f.userDB.QueryByUID(uid)
	fromName := uid
	if userInfo != nil && strings.TrimSpace(userInfo.Name) != "" {
		fromName = strings.TrimSpace(userInfo.Name)
	}
	// 严格复用手动通过好友后的同一推送逻辑（同一函数、同一默认文案来源）。
	_ = f.sendFriendAcceptedNotice(uid, toUID, fromName)
	if welcomeMessage != "" {
		_ = f.ctx.SendMessage(&config.MsgSendReq{
			FromUID:     toUID,
			ChannelID:   uid,
			ChannelType: common.ChannelTypePerson.Uint8(),
			Payload: []byte(util.ToJson(map[string]interface{}{
				"content": welcomeMessage,
				"type":    common.Tip,
			})),
			Header: config.MsgHeader{RedDot: 1},
		})
	}
	return nil
}

// sendFriendAcceptedNotice 复用“手动通过好友”后的推送逻辑：
// 1) CMDFriendAccept 刷新会话/联系人
// 2) 发送 AddedTipsText（或默认文案）提示消息
func (f *Friend) sendFriendAcceptedNotice(fromUID, toUID, fromName string) error {
	if strings.TrimSpace(fromUID) == "" || strings.TrimSpace(toUID) == "" {
		return errors.New("好友ID不能为空")
	}
	if strings.TrimSpace(fromName) == "" {
		fromName = fromUID
	}
	err := f.ctx.SendCMD(config.MsgCMDReq{
		CMD:         common.CMDFriendAccept,
		Subscribers: []string{toUID, fromUID},
		Param: map[string]interface{}{
			"to_uid":    toUID,
			"from_uid":  fromUID,
			"from_name": fromName,
		},
	})
	if err != nil {
		return err
	}
	content := "我们已经是好友了，可以愉快的聊天了！"
	if f.ctx.GetConfig().Friend.AddedTipsText != "" {
		content = f.ctx.GetConfig().Friend.AddedTipsText
	}
	return f.ctx.SendMessage(&config.MsgSendReq{
		FromUID:     fromUID,
		ChannelID:   toUID,
		ChannelType: common.ChannelTypePerson.Uint8(),
		Payload: []byte(util.ToJson(map[string]interface{}{
			"content": content,
			"type":    common.Tip,
		})),
		Header: config.MsgHeader{
			RedDot: 1,
		},
	})
}
