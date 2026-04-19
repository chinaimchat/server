package message

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/TangSengDaoDao/TangSengDaoDaoServer/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"go.uber.org/zap"
)

const (
	contentTypeRedpacket = 9
	contentTypeTransfer  = 10
)

func messageIMSignSecret() string {
	secret := strings.TrimSpace(os.Getenv("WK_JWT_SECRET"))
	if secret == "" {
		secret = "wallet-im-sign-fallback"
	}
	return secret
}

func messageSignRedpacket(packetNo string, packetType int, remark, fromUID, channelID string, channelType int, ts int64) string {
	raw := fmt.Sprintf("rp|%s|%d|%s|%s|%s|%d|%d",
		strings.TrimSpace(packetNo),
		packetType,
		strings.TrimSpace(remark),
		strings.TrimSpace(fromUID),
		strings.TrimSpace(channelID),
		channelType,
		ts,
	)
	return util.HmacSha256(raw, messageIMSignSecret())
}

func messageSignTransfer(transferNo string, amount float64, remark, fromUID, toUID, channelID string, channelType int, ts int64) string {
	raw := fmt.Sprintf("tf|%s|%.2f|%s|%s|%s|%s|%d|%d",
		strings.TrimSpace(transferNo),
		amount,
		strings.TrimSpace(remark),
		strings.TrimSpace(fromUID),
		strings.TrimSpace(toUID),
		strings.TrimSpace(channelID),
		channelType,
		ts,
	)
	return util.HmacSha256(raw, messageIMSignSecret())
}

func asInt(v interface{}) int {
	switch t := v.(type) {
	case int:
		return t
	case int32:
		return int(t)
	case int64:
		return int(t)
	case float64:
		return int(t)
	case float32:
		return int(t)
	case json.Number:
		i, _ := t.Int64()
		return int(i)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(t))
		return i
	default:
		return 0
	}
}

func asInt64(v interface{}) int64 {
	switch t := v.(type) {
	case int:
		return int64(t)
	case int32:
		return int64(t)
	case int64:
		return t
	case float64:
		return int64(t)
	case float32:
		return int64(t)
	case json.Number:
		i, _ := t.Int64()
		return i
	case string:
		i, _ := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
		return i
	default:
		return 0
	}
}

func asFloat64(v interface{}) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int32:
		return float64(t)
	case int64:
		return float64(t)
	case json.Number:
		f, _ := t.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f
	default:
		return 0
	}
}

func asString(v interface{}) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", v))
}

func (m *Message) validateWalletMessagePayload(fromUID, channelID string, channelType uint8, payload map[string]interface{}) error {
	if payload == nil {
		return nil
	}
	contentType := m.contentType(payload)
	if contentType != contentTypeRedpacket && contentType != contentTypeTransfer {
		return nil
	}
	message := &config.MessageResp{
		FromUID:     strings.TrimSpace(fromUID),
		ChannelID:   strings.TrimSpace(channelID),
		ChannelType: channelType,
	}
	if !m.isSignedWalletMessage(message, payload) {
		return fmt.Errorf("wallet message must be signed by server")
	}
	return nil
}

func (m *Message) isSignedWalletMessage(message *config.MessageResp, payload map[string]interface{}) bool {
	contentType := m.contentType(payload)
	ts := asInt64(payload["server_ts"])
	sign := asString(payload["server_sign"])
	if ts <= 0 || sign == "" {
		return false
	}

	switch contentType {
	case contentTypeRedpacket:
		expected := messageSignRedpacket(
			asString(payload["packet_no"]),
			asInt(payload["packet_type"]),
			asString(payload["remark"]),
			message.FromUID,
			message.ChannelID,
			int(message.ChannelType),
			ts,
		)
		return sign == expected
	case contentTypeTransfer:
		expected := messageSignTransfer(
			asString(payload["transfer_no"]),
			asFloat64(payload["amount"]),
			asString(payload["remark"]),
			message.FromUID,
			asString(payload["to_uid"]),
			message.ChannelID,
			int(message.ChannelType),
			ts,
		)
		return sign == expected
	default:
		return true
	}
}

func (m *Message) revokeForgedWalletMessage(message *config.MessageResp, contentType int) {
	systemUID := strings.TrimSpace(m.ctx.GetConfig().Account.SystemUID)
	if systemUID == "" || message == nil || message.MessageID == 0 {
		return
	}
	if err := m.ctx.SendRevoke(&config.MsgRevokeReq{
		Operator:     systemUID,
		OperatorName: "system",
		FromUID:      systemUID,
		ChannelID:    message.ChannelID,
		ChannelType:  message.ChannelType,
		MessageID:    message.MessageID,
	}); err != nil {
		m.Error("revoke forged wallet message failed",
			zap.Error(err),
			zap.Int("content_type", contentType),
			zap.Int64("message_id", message.MessageID),
			zap.String("from_uid", message.FromUID),
			zap.String("channel_id", message.ChannelID),
			zap.Uint8("channel_type", message.ChannelType),
		)
	}
}
