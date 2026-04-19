package wallet

import (
	"strings"
	"time"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"go.uber.org/zap"
)

// WalletIMSyncPayloadType 钱包余额同步（IM 下行）。与红包提示 1011、转账提示 1012 同族，前端按 content.type 识别；
// 建议不渲染为聊天气泡，仅用于刷新本地余额 / 弹 toast。
const WalletIMSyncPayloadType = 1020

// scheduleWalletBalanceIMNotify 异步向指定用户下发当前钱包快照（可用/冻结/总览/冻结状态）。
// reason：业务原因，如流水 type 或 withdrawal_apply；related_id：关联单号等，可为空。
func (s *service) scheduleWalletBalanceIMNotify(uid, reason, relatedID string) {
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return
	}
	go s.sendWalletBalanceIM(uid, strings.TrimSpace(reason), strings.TrimSpace(relatedID))
}

func (s *service) sendWalletBalanceIM(uid, reason, relatedID string) {
	sysUID := strings.TrimSpace(s.ctx.GetConfig().Account.SystemUID)
	if sysUID == "" {
		s.Debug("wallet im sync skipped: empty system uid")
		return
	}
	w, err := s.ensureWallet(uid)
	if err != nil || w == nil {
		s.Debug("wallet im sync skipped: no wallet", zap.String("uid", uid), zap.Error(err))
		return
	}
	avail := w.Balance
	frozen := w.FrozenBalance
	total := avail + frozen
	payload := map[string]interface{}{
		"type":                 WalletIMSyncPayloadType,
		"wallet_sync_version":  1,
		"usdt_available":       avail,
		"usdt_frozen":          frozen,
		"usdt_balance":         total,
		"balance":              avail,
		"wallet_status":        w.Status,
		"reason":               reason,
		"related_id":           relatedID,
		"ts":                   time.Now().UnixMilli(),
	}
	err = s.ctx.SendMessageBatch(&config.MsgSendBatch{
		Header:      config.MsgHeader{RedDot: 0},
		FromUID:     sysUID,
		Payload:     []byte(util.ToJson(payload)),
		Subscribers: []string{uid},
	})
	if err != nil {
		s.Warn("wallet im sync send failed", zap.String("uid", uid), zap.Error(err))
	}
}
