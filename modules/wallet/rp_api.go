package wallet

import (
	"net/http"
	"sync"
	"time"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/common"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func (w *Wallet) RouteRedpacket(r *wkhttp.WKHttp) {
	group := r.Group("/v1/redpacket", w.ctx.AuthMiddleware(r))
	{
		group.POST("/send", w.rpSend)
		group.POST("/open", w.rpOpenAPI)
		group.GET("/:packet_no", w.rpDetail)
	}
}

func (w *Wallet) rpSend(c *wkhttp.Context) {
	var req struct {
		Type        int     `json:"type"`
		ChannelID   string  `json:"channel_id"`
		ChannelType int     `json:"channel_type"`
		TotalAmount float64 `json:"total_amount"`
		TotalCount  int     `json:"total_count"`
		ToUID       string  `json:"to_uid"`
		Remark      string  `json:"remark"`
		Password    string  `json:"password"`
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
	if req.TotalAmount <= 0 || req.TotalCount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "msg": "金额和个数必须大于0"})
		return
	}
	packetNo, err := w.service.rpCreateWithDeduction(uid, req.ChannelID, req.ChannelType, req.Type, req.TotalAmount, req.TotalCount, req.ToUID, req.Remark)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "msg": err.Error()})
		return
	}
	if err = w.rpSendCreateMsg(packetNo); err != nil {
		w.Error("redpacket initial message send failed",
			zap.Error(err),
			zap.String("packet_no", packetNo),
			zap.String("channel_id", req.ChannelID),
			zap.Int("channel_type", req.ChannelType),
		)
	}
	c.JSON(http.StatusOK, gin.H{"status": 200, "packet_no": packetNo})
}

func (w *Wallet) rpSendCreateMsg(packetNo string) error {
	packet, err := w.service.db.rpGet(packetNo)
	if err != nil {
		return err
	}
	if packet == nil {
		return nil
	}

	channelID := packet.ChannelID
	if packet.ChannelType == int(common.ChannelTypePerson) && channelID == "" {
		channelID = packet.ToUID
	}
	if channelID == "" {
		return nil
	}

	sendName := w.service.db.rpUserDisplayName(packet.UID)
	if sendName == "" {
		sendName = "好友"
	}

	payload := map[string]interface{}{
		"type":        9,
		"packet_no":   packet.PacketNo,
		"packet_type": packet.Type,
		"remark":      packet.Remark,
		"sender_name": sendName,
		"status":      RPStatusPending,
	}
	ts := time.Now().Unix()
	payload["server_ts"] = ts
	payload["server_sign"] = signRedpacketIM(packet.PacketNo, packet.Type, packet.Remark, packet.UID, channelID, packet.ChannelType, ts)

	return w.ctx.SendMessage(&config.MsgSendReq{
		Header: config.MsgHeader{
			NoPersist: 0,
			RedDot:    1,
			SyncOnce:  0,
		},
		FromUID:     packet.UID,
		ChannelID:   channelID,
		ChannelType: uint8(packet.ChannelType),
		Payload:     []byte(util.ToJson(payload)),
	})
}

func (w *Wallet) rpOpenAPI(c *wkhttp.Context) {
	var req struct {
		PacketNo string `json:"packet_no"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(err)
		return
	}
	uid := c.GetLoginUID()
	amount, err := w.service.rpOpen(req.PacketNo, uid)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "msg": err.Error()})
		return
	}
	if err = w.service.AddBalance(uid, amount, "redpacket_receive", req.PacketNo, "领取红包"); err != nil {
		c.ResponseError(err)
		return
	}
	w.rpSendOpenMsg(req.PacketNo, uid, amount)
	packet, _ := w.service.db.rpGet(req.PacketNo)
	if packet != nil && packet.RemainingCount <= 0 {
		go w.service.rpMarkBestLuck(req.PacketNo)
	}

	// Keep fields under data so Android can update the local bubble state immediately.
	data := gin.H{
		"amount":    amount,
		"packet_no": req.PacketNo,
		"my_amount": amount,
	}
	if packet != nil {
		data["remaining_count"] = packet.RemainingCount
		data["remaining_amount"] = packet.RemainingAmount
		st := packet.Status
		data["redpacket_status"] = st
		data["packet_status"] = st
		data["status"] = st
	}
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": data})
}

func (w *Wallet) rpSendOpenMsg(packetNo, receiverUID string, amount float64) {
	packet, err := w.service.db.rpGet(packetNo)
	if err != nil || packet == nil {
		return
	}

	// Serialize claim-tip IM sends per packet so claim order stays stable.
	unlock := acquireRPTipSendLock(packetNo)
	defer unlock()

	senderUID := packet.UID
	recvName := w.service.db.rpUserDisplayName(receiverUID)
	if recvName == "" {
		recvName = "好友"
	}
	sendName := w.service.db.rpUserDisplayName(senderUID)
	if sendName == "" {
		sendName = "好友"
	}

	claimOrder, _ := w.service.db.rpRecordCount(packetNo)
	var claimedAt string
	if rec, _ := w.service.db.rpGetRecordByUID(packetNo, receiverUID); rec != nil {
		claimedAt = rec.CreatedAt.Format("2006-01-02 15:04:05")
	}

	forbiddenFriend := 0
	if packet.ChannelType == int(common.ChannelTypeGroup) {
		forbiddenFriend = w.service.db.groupForbiddenAddFriend(packet.ChannelID)
	}

	// type=1011 is a tappable system tip shown in chat after a claim succeeds.
	rpExtra := func(name, uid string) map[string]string {
		return map[string]string{"name": name, "uid": uid}
	}
	buildPayload := func(content string, extra []map[string]string) []byte {
		if extra == nil {
			extra = []map[string]string{}
		}
		return []byte(util.ToJson(map[string]interface{}{
			"type":                  1011,
			"packet_no":             packetNo,
			"uid":                   receiverUID,
			"receiver_uid":          receiverUID,
			"receiver_name":         recvName,
			"sender_uid":            senderUID,
			"sender_name":           sendName,
			"amount":                amount,
			"content":               content,
			"extra":                 extra,
			"tappable_suffix": map[string]string{
				"text":       "红包",
				"packet_no":  packetNo,
				"action":     "redpacket_detail",
				"color_hint": "red",
			},
			"channel_id":            packet.ChannelID,
			"channel_type":          packet.ChannelType,
			"forbidden_add_friend":  forbiddenFriend,
			"show_below_redpacket":  false,
			"redpacket_tip_version": 2,
			"claim_order":           claimOrder,
			"claimed_at":            claimedAt,
		}))
	}

	// Keep this as a normal persisted chat message instead of a stream fragment.
	send := func(subscribers []string, content string, extra []map[string]string) {
		req := &config.MsgSendReq{
			Header: config.MsgHeader{
				NoPersist: 0,
				RedDot:    1,
				SyncOnce:  0,
			},
			FromUID:     w.ctx.GetConfig().Account.SystemUID,
			ChannelID:   packet.ChannelID,
			ChannelType: uint8(packet.ChannelType),
			StreamNo:    "",
			Subscribers: subscribers,
			Payload:     buildPayload(content, extra),
		}
		if err := w.ctx.SendMessage(req); err != nil {
			w.Error("redpacket claim tip send failed",
				zap.Error(err),
				zap.String("packet_no", packetNo),
				zap.String("receiver_uid", receiverUID),
				zap.String("channel_id", packet.ChannelID),
				zap.Int("channel_type", packet.ChannelType),
				zap.Strings("subscribers", subscribers),
				zap.String("content_template", content),
			)
		}
	}

	ex := func(name, uid string) []map[string]string {
		return []map[string]string{rpExtra(name, uid)}
	}
	ex2 := func(n1, u1, n2, u2 string) []map[string]string {
		return []map[string]string{rpExtra(n1, u1), rpExtra(n2, u2)}
	}

	// In 1:1 chat, keep channel_id aligned with the original redpacket message.
	if packet.ChannelType == int(common.ChannelTypePerson) {
		if receiverUID == senderUID {
			send([]string{senderUID}, "你领取了自己的红包", []map[string]string{})
			peer := packet.ChannelID
			if peer != "" && peer != senderUID {
				send([]string{peer}, "{0}领取了自己发送的红包", ex(sendName, senderUID))
			}
			return
		}
		send([]string{senderUID}, "{0}领取了你的红包", ex(recvName, receiverUID))
		send([]string{receiverUID}, "你领取了{0}的红包", ex(sendName, senderUID))
		return
	}

	// Group chat uses a single third-person tip visible to the whole conversation.
	if packet.ChannelType == int(common.ChannelTypeGroup) {
		if receiverUID == senderUID {
			send(nil, "{0}领取了自己发送的红包", ex(sendName, senderUID))
			return
		}
		send(nil, "{0}领取了{1}的红包", ex2(recvName, receiverUID, sendName, senderUID))
		return
	}

	// Fallback for other channel types: send one third-person tip.
	if receiverUID == senderUID {
		send(nil, "{0}领取了自己发送的红包", ex(sendName, senderUID))
	} else {
		send(nil, "{0}领取了{1}的红包", ex2(recvName, receiverUID, sendName, senderUID))
	}
}

const rpTipShardN = 256

var rpTipShardMu [rpTipShardN]sync.Mutex

// acquireRPTipSendLock shards by packet_no so claim tips for one packet remain ordered.
func acquireRPTipSendLock(packetNo string) func() {
	var h uint32 = 2166136261
	for i := 0; i < len(packetNo); i++ {
		h ^= uint32(packetNo[i])
		h *= 16777619
	}
	i := int(h % uint32(rpTipShardN))
	rpTipShardMu[i].Lock()
	return func() { rpTipShardMu[i].Unlock() }
}

func (w *Wallet) rpDetail(c *wkhttp.Context) {
	packetNo := c.Param("packet_no")
	uid := c.GetLoginUID()
	packet, err := w.service.db.rpGet(packetNo)
	if err != nil {
		c.ResponseError(err)
		return
	}
	if packet == nil {
		c.JSON(http.StatusNotFound, gin.H{"status": 404, "msg": "红包不存在"})
		return
	}
	records, _ := w.service.db.rpGetRecords(packetNo)
	var myAmount float64
	recordList := make([]gin.H, 0, len(records))
	for _, r := range records {
		recordList = append(recordList, gin.H{
			"uid": r.UID, "amount": r.Amount, "is_best": r.IsBest,
			"created_at": r.CreatedAt.Format("2006-01-02 15:04:05"),
		})
		if r.UID == uid {
			myAmount = r.Amount
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"status": 200, "packet_no": packet.PacketNo, "sender_uid": packet.UID,
		"type": packet.Type, "total_amount": packet.TotalAmount, "total_count": packet.TotalCount,
		"remaining_amount": packet.RemainingAmount, "remaining_count": packet.RemainingCount,
		"remark": packet.Remark, "redpacket_status": packet.Status, "my_amount": myAmount,
		"records": recordList, "created_at": packet.CreatedAt.Format("2006-01-02 15:04:05"),
	})
}
