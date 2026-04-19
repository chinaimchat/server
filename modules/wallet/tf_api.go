package wallet

import (
	"net/http"
	"strings"
	"time"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/common"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func (w *Wallet) RouteTransfer(r *wkhttp.WKHttp) {
	group := r.Group("/v1/transfer", w.ctx.AuthMiddleware(r))
	{
		group.POST("/send", w.tfSend)
		group.POST("/:transfer_no/accept", w.tfAcceptAPI)
		group.GET("/:transfer_no", w.tfDetailAPI)
	}
}

func (w *Wallet) tfSend(c *wkhttp.Context) {
	var req struct {
		ToUID       string  `json:"to_uid"`
		Amount      float64 `json:"amount"`
		Remark      string  `json:"remark"`
		Password    string  `json:"password"`
		ChannelID   string  `json:"channel_id"`
		ChannelType int     `json:"channel_type"`
		PayScene    string  `json:"pay_scene"` // receive_qr means direct credit from a receive-QR flow.
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(err)
		return
	}
	uid := c.GetLoginUID()
	if err := w.service.VerifyPayPassword(uid, req.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "msg": err.Error()})
		return
	}
	if req.Amount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "msg": "金额必须大于0"})
		return
	}
	transferNo, tr, err := w.service.tfSendWithDeduction(uid, req.ToUID, req.Amount, req.Remark, req.ChannelID, req.ChannelType, req.PayScene)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "msg": err.Error()})
		return
	}
	if err = w.tfSendCreateMsg(transferNo); err != nil {
		w.Error("transfer initial message send failed",
			zap.Error(err),
			zap.String("transfer_no", transferNo),
			zap.String("channel_id", req.ChannelID),
			zap.Int("channel_type", req.ChannelType),
		)
	}
	if tr != nil && tr.Status == TFStatusAccepted {
		w.tfSendNotify(tr, tr.ToUID)
	}

	// Keep send response fields under data so Android can update the local bubble state.
	data := gin.H{"transfer_no": transferNo}
	if tr != nil {
		data["status_code"] = tr.Status
		data["transfer_status"] = tr.Status
	}
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": data})
}

func (w *Wallet) tfSendCreateMsg(transferNo string) error {
	tr, err := w.service.db.tfGet(transferNo)
	if err != nil {
		return err
	}
	if tr == nil {
		return nil
	}

	channelID := strings.TrimSpace(tr.ChannelID)
	if tr.ChannelType != int(common.ChannelTypeGroup) && channelID == "" {
		channelID = tr.ToUID
	}
	if channelID == "" {
		return nil
	}
	payload := map[string]interface{}{
		"type":        10,
		"transfer_no": tr.TransferNo,
		"amount":      tr.Amount,
		"remark":      tr.Remark,
		"from_uid":    tr.FromUID,
		"to_uid":      tr.ToUID,
		"status":      tr.Status,
	}
	ts := time.Now().Unix()
	payload["server_ts"] = ts
	payload["server_sign"] = signTransferIM(tr.TransferNo, tr.Amount, tr.Remark, tr.FromUID, tr.ToUID, channelID, tr.ChannelType, ts)

	return w.ctx.SendMessage(&config.MsgSendReq{
		Header: config.MsgHeader{
			NoPersist: 0,
			RedDot:    1,
			SyncOnce:  0,
		},
		FromUID:     tr.FromUID,
		ChannelID:   channelID,
		ChannelType: uint8(tr.ChannelType),
		Payload:     []byte(util.ToJson(payload)),
	})
}

func (w *Wallet) tfAcceptAPI(c *wkhttp.Context) {
	transferNo := c.Param("transfer_no")
	uid := c.GetLoginUID()
	tr, err := w.service.db.tfGet(transferNo)
	if err != nil {
		c.ResponseError(err)
		return
	}
	if tr == nil {
		c.JSON(http.StatusNotFound, gin.H{"status": 404, "msg": "转账不存在"})
		return
	}
	if err = w.service.tfAccept(transferNo, uid); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "msg": err.Error()})
		return
	}
	if err = w.service.AddBalance(uid, tr.Amount, "transfer_in", transferNo, "收到转账"); err != nil {
		c.ResponseError(err)
		return
	}
	w.tfSendNotify(tr, uid)
	trAfter, _ := w.service.db.tfGet(transferNo)
	data := gin.H{"transfer_no": transferNo}
	if trAfter != nil {
		data["status_code"] = trAfter.Status
		data["transfer_status"] = trAfter.Status
	}
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": data})
}

func (w *Wallet) tfSendNotify(tr *tfModel, acceptUID string) {
	recvName := w.service.db.rpUserDisplayName(acceptUID)
	if recvName == "" {
		recvName = "好友"
	}
	sendName := w.service.db.rpUserDisplayName(tr.FromUID)
	if sendName == "" {
		sendName = "好友"
	}

	chType := tr.ChannelType
	if chType == 0 {
		chType = int(common.ChannelTypePerson)
	}

	var imChannelID string
	var imChannelType uint8
	switch chType {
	case int(common.ChannelTypeGroup):
		imChannelID = strings.TrimSpace(tr.ChannelID)
		if imChannelID == "" {
			return
		}
		imChannelType = common.ChannelTypeGroup.Uint8()
	default:
		// In 1:1 chat, use the same fake channel strategy as other wallet tips.
		imChannelID = common.GetFakeChannelIDWith(tr.FromUID, tr.ToUID)
		imChannelType = common.ChannelTypePerson.Uint8()
	}

	forbiddenFriend := 0
	if chType == int(common.ChannelTypeGroup) && imChannelID != "" {
		forbiddenFriend = w.service.db.groupForbiddenAddFriend(imChannelID)
	}

	// type=1012 is a tappable transfer tip shown in chat after funds are accepted.
	tfExtra := func(name, uid string) map[string]string {
		return map[string]string{"name": name, "uid": uid}
	}
	buildPayload := func(content string, extra []map[string]string) []byte {
		if extra == nil {
			extra = []map[string]string{}
		}
		return []byte(util.ToJson(map[string]interface{}{
			"type":                 1012,
			"transfer_no":          tr.TransferNo,
			"from_uid":             tr.FromUID,
			"to_uid":               tr.ToUID,
			"channel_id":           imChannelID,
			"channel_type":         int(imChannelType),
			"forbidden_add_friend": forbiddenFriend,
			"sender_name":          sendName,
			"receiver_name":        recvName,
			"amount":               tr.Amount,
			"content":              content,
			"extra":                extra,
			"tappable_suffix": map[string]string{
				"text":        "转账",
				"transfer_no": tr.TransferNo,
				"action":      "transfer_detail",
				"color_hint":  "blue",
			},
			"transfer_tip_version": 2,
		}))
	}

	send := func(subscribers []string, content string, extra []map[string]string) {
		if err := w.ctx.SendMessage(&config.MsgSendReq{
			Header: config.MsgHeader{
				NoPersist: 0,
				RedDot:    1,
				SyncOnce:  0,
			},
			FromUID:     w.ctx.GetConfig().Account.SystemUID,
			ChannelID:   imChannelID,
			ChannelType: imChannelType,
			StreamNo:    "",
			Subscribers: subscribers,
			Payload:     buildPayload(content, extra),
		}); err != nil {
			w.Error("transfer notify IM send failed",
				zap.Error(err),
				zap.String("transfer_no", tr.TransferNo),
				zap.String("accept_uid", acceptUID),
				zap.String("channel_id", imChannelID),
				zap.Uint8("channel_type", imChannelType),
				zap.Strings("subscribers", subscribers),
				zap.String("content_template", content),
			)
		}
	}

	ex := func(name, uid string) []map[string]string {
		return []map[string]string{tfExtra(name, uid)}
	}
	ex2 := func(n1, u1, n2, u2 string) []map[string]string {
		return []map[string]string{tfExtra(n1, u1), tfExtra(n2, u2)}
	}

	// Group chat uses a single third-person tip visible to the whole conversation.
	if chType == int(common.ChannelTypeGroup) {
		if acceptUID == tr.FromUID {
			send(nil, "{0}领取了自己发起的转账", ex(sendName, tr.FromUID))
			return
		}
		send(nil, "{0}领取了{1}的转账", ex2(recvName, acceptUID, sendName, tr.FromUID))
		return
	}

	if acceptUID == tr.FromUID {
		send([]string{tr.FromUID}, "你领取了自己的转账", []map[string]string{})
		peer := strings.TrimSpace(tr.ChannelID)
		if peer != "" && peer != tr.FromUID {
			send([]string{peer}, "{0}领取了自己发起的转账", ex(sendName, tr.FromUID))
		}
		return
	}
	send([]string{tr.FromUID}, "{0}领取了你的转账", ex(recvName, acceptUID))
	send([]string{acceptUID}, "你领取了{0}的转账", ex(sendName, tr.FromUID))
}

func (w *Wallet) tfDetailAPI(c *wkhttp.Context) {
	transferNo := c.Param("transfer_no")
	tr, err := w.service.db.tfGet(transferNo)
	if err != nil {
		c.ResponseError(err)
		return
	}
	if tr == nil {
		c.JSON(http.StatusNotFound, gin.H{"status": 404, "msg": "转账不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status": 200, "transfer_no": tr.TransferNo,
		"from_uid": tr.FromUID, "to_uid": tr.ToUID, "amount": tr.Amount,
		"remark": tr.Remark, "status_code": tr.Status,
		"channel_id": tr.ChannelID, "channel_type": tr.ChannelType,
		"created_at": tr.CreatedAt.Format("2006-01-02 15:04:05"),
	})
}
