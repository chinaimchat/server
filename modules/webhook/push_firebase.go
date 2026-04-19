package webhook

import (
	"context"
	"strconv"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/user"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/log"
	"google.golang.org/api/option"
)

// FIREBASEPush 参考代码 https://github.com/firebase/firebase-admin-go/blob/61c6c041bf807c045f6ff3fd0d02fc480f806c9a/snippets/messaging.go#L29-L55
// FIREBASEPush GOOGLE推送
type FIREBASEPush struct {
	jsonPath    string //
	packageName string // android包名
	projectId   string // serviceAccountJson中的project_id值
	channelID   string // 频道id 如果有则填写
	client      messaging.Client
	log.Log
}

// NewFIREBASEPush NewFIREBASEPush
func NewFIREBASEPush(jsonPath string, packageName string, projectID string, channelID string) *FIREBASEPush {
	// Initialize another app with a different config
	ctx := context.Background()

	c := &firebase.Config{ProjectID: projectID}

	opt := option.WithCredentialsFile(jsonPath)
	app, err := firebase.NewApp(ctx, c, opt)
	if err != nil {
		log.Error("无法初始化firebase: 通过json创建firebase客户端时 ")
		return nil
	}
	// Obtain a messaging.Client from the App.
	client, err := app.Messaging(ctx)
	if err != nil {
		log.Error("通过APP client 创建 message client时:" + err.Error())
		return nil
	}

	return &FIREBASEPush{
		jsonPath:    jsonPath,
		packageName: packageName,
		channelID:   channelID,
		client:      *client,
		projectId:   projectID,
		Log:         log.NewTLog("FIREBASEPush"),
	}
}

// FIREBASEPayload Google Firebase负载
type FIREBASEPayload struct {
	Payload
	fcmData map[string]string
}

// buildFCMDataMap 构造 FCM data（值均为字符串，满足 Admin SDK 要求）。
// 约定：action=im_wake 用于唤醒客户端/重连；type=im|rtc 区分普通消息与信令来电等。
func buildFCMDataMap(msg msgOfflineNotify, payloadInfo *PayloadInfo) map[string]string {
	data := map[string]string{
		"action":        "im_wake",
		"message_seq":   strconv.FormatUint(uint64(msg.MessageSeq), 10),
		"channel_id":    msg.ChannelID,
		"channel_type":  strconv.FormatUint(uint64(msg.ChannelType), 10),
		"from_uid":      msg.FromUID,
		"client_msg_no": msg.ClientMsgNo,
		"message_id":    strconv.FormatInt(msg.MessageID, 10),
		"badge":         strconv.Itoa(payloadInfo.Badge),
	}
	setting := config.SettingFromUint8(msg.Setting)
	if (msg.PayloadMap != nil && msg.PayloadMap["cmd"] != nil) || setting.Signal {
		data["type"] = "rtc"
	} else {
		data["type"] = "im"
	}
	return data
}

// NewFIREBASEPayload NewFIREBASEPayload
func NewFIREBASEPayload(msg msgOfflineNotify, payloadInfo *PayloadInfo) *FIREBASEPayload {
	return &FIREBASEPayload{
		Payload: payloadInfo.toPayload(),
		fcmData: buildFCMDataMap(msg, payloadInfo),
	}
}

// GetPayload 获取推送负载
func (m *FIREBASEPush) GetPayload(msg msgOfflineNotify, ctx *config.Context, toUser *user.Resp) (Payload, error) {
	payloadInfo, err := ParsePushInfo(msg, ctx, toUser)
	if err != nil {
		return nil, err
	}
	return NewFIREBASEPayload(msg, payloadInfo), nil
}

// Push 离线唤醒推送（联调说明）：
//   - Android：FCM HTTP v1 / Admin SDK 对应 android.priority = high，减轻后台投递被过度延后（仍受系统与厂商策略影响）。
//   - iOS：经 APNs 投递，Headers 中 apns-priority=10 表示高优先级即时投递；aps 含 content-available=1 以便 application:didReceiveRemoteNotification:fetchCompletionHandler:。
// content-available 会提高系统侧调度与省电策略的关注度，请在产品层控制推送频率并仅用于确有同步/重连必要的场景。
func (m *FIREBASEPush) Push(deviceToken string, payload Payload) error {
	miPayload := payload.(*FIREBASEPayload)
	ctx := context.Background()

	android := &messaging.AndroidConfig{
		Priority: "high",
	}
	if m.packageName != "" {
		android.RestrictedPackageName = m.packageName
	}
	if m.channelID != "" {
		android.Notification = &messaging.AndroidNotification{
			ChannelID: m.channelID,
		}
	}

	badge := miPayload.GetBadge()
	apns := &messaging.APNSConfig{
		// 与 FCM v1 文档中 APNs 的 apns-priority 一致：10 为高优先级（含 alert 的即时通知类投递）。
		Headers: map[string]string{
			"apns-priority": "10",
		},
		Payload: &messaging.APNSPayload{
			Aps: &messaging.Aps{
				ContentAvailable: true,
				Alert: &messaging.ApsAlert{
					Title: miPayload.GetTitle(),
					Body:  miPayload.GetContent(),
				},
				Badge: &badge,
			},
		},
	}

	fcmMsg := &messaging.Message{
		Data: miPayload.fcmData,
		Notification: &messaging.Notification{
			Title: miPayload.GetTitle(),
			Body:  miPayload.GetContent(),
		},
		Android: android,
		APNS:    apns,
		Token:   deviceToken,
	}

	response, err := m.client.Send(ctx, fcmMsg)
	m.Debug("Successfully sent firebase message:" + response)
	if err != nil {
		return err
	}
	return nil
}
