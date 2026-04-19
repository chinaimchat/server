package wallet

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"go.uber.org/zap"
)

func defaultRedpacketConfig() *redpacketConfigModel {
	return &redpacketConfigModel{
		ExpireHours:            24,
		ExpireCheckIntervalMin: 5,
		MaxAmountPerRedpacket:  10000000,
		MaxDailyAmountPerUser:  100000000,
		MaxRedpacketsPerHour:   1000,
		MinAmountPerRedpacket:  100,
		BatchExpireLimit:       100,
		EnableRiskControl:      0,
		EnableAutoExpire:       1,
		RefundOnExpire:         1,
	}
}

func (s *service) rpLoadConfigOrDefault() *redpacketConfigModel {
	cfg, err := s.db.getRedpacketConfig()
	if err != nil || cfg == nil {
		if err != nil {
			s.Debug("redpacket_config 使用默认值", zap.Error(err))
		}
		return defaultRedpacketConfig()
	}
	return cfg
}

// rpValidateConfigOnSend 配置管理中的金额与频控（与后台「配置管理」一致）
func (s *service) rpValidateConfigOnSend(uid string, totalAmount float64) error {
	cfg := s.rpLoadConfigOrDefault()
	sendCents := totalAmount * 100
	if sendCents+1e-6 < float64(cfg.MinAmountPerRedpacket) {
		return fmt.Errorf("单笔红包不得低于 ¥%.2f", float64(cfg.MinAmountPerRedpacket)/100)
	}
	if sendCents-1e-6 > float64(cfg.MaxAmountPerRedpacket) {
		return fmt.Errorf("单笔红包不得超过 ¥%.2f", float64(cfg.MaxAmountPerRedpacket)/100)
	}
	today, err := s.db.rpUserTodaySendTotal(uid)
	if err != nil {
		s.Warn("rpUserTodaySendTotal failed", zap.Error(err))
	} else {
		maxDayYuan := float64(cfg.MaxDailyAmountPerUser) / 100
		if today+totalAmount > maxDayYuan+1e-6 {
			return fmt.Errorf("今日发红包总额不能超过 ¥%.2f", maxDayYuan)
		}
	}
	hourly, err := s.db.rpCountUserPacketsLastHour(uid)
	if err != nil {
		s.Warn("rpCountUserPacketsLastHour failed", zap.Error(err))
	} else if cfg.MaxRedpacketsPerHour > 0 && hourly >= cfg.MaxRedpacketsPerHour {
		return fmt.Errorf("一小时内最多发送 %d 个红包", cfg.MaxRedpacketsPerHour)
	}
	return nil
}

// rpEvaluateRiskOnSend 风控规则（与后台「风控管理」规则表一致，需开启 enable_risk_control）
func (s *service) rpEvaluateRiskOnSend(uid string, totalAmount float64) error {
	cfg := s.rpLoadConfigOrDefault()
	if cfg.EnableRiskControl == 0 {
		return nil
	}
	rules, err := s.db.rpListEnabledRiskRules()
	if err != nil {
		s.Warn("rpListEnabledRiskRules failed", zap.Error(err))
		return nil
	}
	sendCents := totalAmount * 100
	today, _ := s.db.rpUserTodaySendTotal(uid)
	th := func(r *rpRiskRuleModel) int { return int(math.Round(r.Threshold)) }

	for _, r := range rules {
		switch r.Type {
		case "amount":
			if sendCents > r.Threshold+1e-6 {
				remark := fmt.Sprintf("单笔¥%.2f超过规则阈值¥%.2f", totalAmount, r.Threshold/100)
				_ = s.db.rpInsertRiskEvent(uid, "amount", "high", remark)
				return errors.New("触发风控：单个红包金额超过规则限制")
			}
		case "daily_amount":
			maxYuan := r.Threshold / 100
			if today+totalAmount > maxYuan+1e-6 {
				remark := fmt.Sprintf("今日已发¥%.2f，本次¥%.2f，超过日限额¥%.2f", today, totalAmount, maxYuan)
				_ = s.db.rpInsertRiskEvent(uid, "daily_amount", "high", remark)
				return errors.New("触发风控：日发送金额超过规则限制")
			}
		case "frequency":
			limitN := th(r)
			if limitN <= 0 {
				continue
			}
			tw := r.TimeWindow
			if tw <= 0 {
				continue
			}
			cnt, e := s.db.rpCountUserPacketsSince(uid, tw)
			if e != nil {
				continue
			}
			if cnt >= limitN {
				remark := fmt.Sprintf("%d秒内已发%d个，规则上限%d", tw, cnt, limitN)
				_ = s.db.rpInsertRiskEvent(uid, "frequency", "medium", remark)
				return errors.New("触发风控：发红包过于频繁")
			}
		case "time_pattern":
			limitN := th(r)
			if limitN <= 0 {
				continue
			}
			h := time.Now().Hour()
			if h < 2 || h >= 6 {
				continue
			}
			tw := r.TimeWindow
			if tw <= 0 {
				tw = 86400
			}
			cnt, e := s.db.rpCountUserNightSendsSince(uid, tw)
			if e != nil {
				continue
			}
			if cnt >= limitN {
				remark := fmt.Sprintf("凌晨时段窗口内已发%d个，规则上限%d", cnt, limitN)
				_ = s.db.rpInsertRiskEvent(uid, "time_pattern", "high", remark)
				return errors.New("触发风控：异常时段发红包次数过多")
			}
		case "receive_frequency":
			// 仅在领取时校验
		}
	}
	return nil
}

// rpEvaluateRiskOnOpen 领取频率类规则
func (s *service) rpEvaluateRiskOnOpen(uid string) error {
	cfg := s.rpLoadConfigOrDefault()
	if cfg.EnableRiskControl == 0 {
		return nil
	}
	rules, err := s.db.rpListEnabledRiskRules()
	if err != nil {
		return nil
	}
	for _, r := range rules {
		if r.Type != "receive_frequency" {
			continue
		}
		tw := r.TimeWindow
		if tw <= 0 {
			continue
		}
		cnt, e := s.db.rpCountUserReceivesSince(uid, tw)
		if e != nil {
			continue
		}
		limit := int(math.Round(r.Threshold))
		if cnt >= limit {
			remark := fmt.Sprintf("%d秒内已领%d次，规则上限%d", tw, cnt, limit)
			_ = s.db.rpInsertRiskEvent(uid, "receive_frequency", "medium", remark)
			return errors.New("触发风控：领取红包过于频繁")
		}
	}
	return nil
}

const (
	RPTypeIndividual  = 1
	RPTypeGroupRandom = 2
	RPTypeGroupNormal = 3
	RPTypeExclusive   = 4

	RPStatusPending  = 0
	RPStatusFinished = 1
	RPStatusExpired  = 2
)

func rpRandomAmount(remainingAmount float64, remainingCount int) float64 {
	if remainingCount == 1 {
		return math.Round(remainingAmount*100) / 100
	}
	minVal := 0.01
	maxVal := (remainingAmount / float64(remainingCount)) * 2
	if maxVal < minVal {
		maxVal = minVal
	}
	amount := minVal + rand.Float64()*(maxVal-minVal)
	amount = math.Floor(amount*100) / 100
	if amount < minVal {
		amount = minVal
	}
	if amount > remainingAmount-float64(remainingCount-1)*minVal {
		amount = remainingAmount - float64(remainingCount-1)*minVal
	}
	return math.Round(amount*100) / 100
}

// rpCreateWithDeduction 发红包：扣款、流水、写 redpacket 同一事务
func (s *service) rpCreateWithDeduction(uid, channelID string, channelType, packetType int,
	totalAmount float64, totalCount int, toUID, remark string) (string, error) {

	uid = strings.TrimSpace(uid)
	if uid == "" {
		return "", errors.New("无效用户")
	}
	var err error
	totalAmount, err = normalizeSpendAmount(totalAmount)
	if err != nil {
		return "", err
	}
	if totalCount <= 0 {
		return "", errors.New("红包个数无效")
	}

	if err = s.rpValidateConfigOnSend(uid, totalAmount); err != nil {
		return "", err
	}
	if err = s.rpEvaluateRiskOnSend(uid, totalAmount); err != nil {
		return "", err
	}

	w, err := s.ensureWallet(uid)
	if err != nil {
		return "", err
	}
	if w.Status == 2 {
		return "", errors.New("钱包已冻结")
	}

	tx, err := s.ctx.DB().Begin()
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	affected, err := s.db.deductBalanceTx(uid, totalAmount, tx)
	if err != nil {
		return "", err
	}
	if affected == 0 {
		err = errors.New("余额不足")
		return "", err
	}

	balAfter, err := s.db.walletSelectBalanceTx(uid, tx)
	if err != nil {
		return "", err
	}

	packetNo := util.GenerUUID()
	err = s.db.insertTransactionTx(&transactionModel{
		UID:          uid,
		Type:         "redpacket_send",
		Amount:       -totalAmount,
		RelatedID:    packetNo,
		Remark:       "发红包",
		BalanceAfter: balAfter,
	}, tx)
	if err != nil {
		return "", err
	}

	cfg := s.rpLoadConfigOrDefault()
	expH := cfg.ExpireHours
	if expH <= 0 {
		expH = 24
	}
	expiredAt := time.Now().Add(time.Duration(expH) * time.Hour)
	m := &rpModel{
		PacketNo: packetNo, UID: uid, ChannelID: channelID, ChannelType: channelType,
		Type: packetType, TotalAmount: totalAmount, TotalCount: totalCount,
		RemainingAmount: totalAmount, RemainingCount: totalCount,
		ToUID: toUID, Remark: remark, Status: RPStatusPending, ExpiredAt: expiredAt,
	}
	if err = s.db.rpInsertTx(m, tx); err != nil {
		return "", err
	}

	if err = tx.Commit(); err != nil {
		return "", err
	}
	s.scheduleWalletBalanceIMNotify(uid, "redpacket_send", packetNo)
	return packetNo, nil
}

func (s *service) rpOpen(packetNo, uid string) (float64, error) {
	rp, err := s.db.rpGet(packetNo)
	if err != nil {
		return 0, err
	}
	if rp == nil {
		return 0, errors.New("红包不存在")
	}
	if err = s.rpEvaluateRiskOnOpen(uid); err != nil {
		return 0, err
	}
	if rp.Status != RPStatusPending {
		if rp.Status == RPStatusExpired {
			return 0, errors.New("红包已过期")
		}
		return 0, errors.New("红包已领完")
	}
	if rp.RemainingCount <= 0 {
		return 0, errors.New("红包已领完")
	}
	if rp.Type == RPTypeExclusive && rp.ToUID != uid {
		return 0, errors.New("这是专属红包，不是给你的哦")
	}
	existing, _ := s.db.rpGetRecordByUID(packetNo, uid)
	if existing != nil {
		return 0, errors.New("你已经领取过了")
	}

	var amount float64
	switch rp.Type {
	case RPTypeGroupNormal:
		amount = math.Round((rp.TotalAmount/float64(rp.TotalCount))*100) / 100
	case RPTypeIndividual, RPTypeExclusive:
		amount = rp.TotalAmount
	default:
		amount = rpRandomAmount(rp.RemainingAmount, rp.RemainingCount)
	}

	tx, err := s.ctx.DB().Begin()
	if err != nil {
		return 0, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	affected, err := s.db.rpDeductTx(packetNo, amount, tx)
	if err != nil {
		return 0, err
	}
	if affected == 0 {
		err = errors.New("红包已领完")
		return 0, err
	}
	err = s.db.rpInsertRecordTx(&rpRecordModel{PacketNo: packetNo, UID: uid, Amount: amount}, tx)
	if err != nil {
		return 0, err
	}
	if rp.RemainingCount-1 <= 0 {
		err = s.db.rpUpdateStatusTx(packetNo, RPStatusFinished, tx)
		if err != nil {
			return 0, err
		}
	}
	err = tx.Commit()
	return amount, err
}

func (s *service) rpProcessExpired() {
	cfg := s.rpLoadConfigOrDefault()
	if cfg.EnableAutoExpire == 0 {
		return
	}
	interval := cfg.ExpireCheckIntervalMin
	if interval <= 0 {
		interval = 5
	}
	s.rpExpireMu.Lock()
	if !s.lastRpExpireRun.IsZero() && time.Since(s.lastRpExpireRun) < time.Duration(interval)*time.Minute {
		s.rpExpireMu.Unlock()
		return
	}
	s.lastRpExpireRun = time.Now()
	s.rpExpireMu.Unlock()

	list, err := s.db.rpGetExpired()
	if err != nil {
		s.Warn("get expired redpackets failed", zap.Error(err))
		return
	}
	limit := cfg.BatchExpireLimit
	if limit <= 0 {
		limit = 100
	}
	for i, rp := range list {
		if i >= limit {
			break
		}
		if rp.RemainingAmount > 0 {
			if cfg.RefundOnExpire != 0 {
				if refundErr := s.AddBalance(rp.UID, rp.RemainingAmount, "redpacket_refund", rp.PacketNo, "红包过期退回"); refundErr != nil {
					s.Warn("refund expired redpacket failed", zap.Error(refundErr), zap.String("packet_no", rp.PacketNo))
					continue
				}
			} else {
				s.Info("红包过期未退款（配置 refund_on_expire=0）", zap.String("packet_no", rp.PacketNo), zap.Float64("remaining", rp.RemainingAmount))
			}
		}
		_ = s.db.rpUpdateStatus(rp.PacketNo, RPStatusExpired)
	}
}

func (s *service) rpMarkBestLuck(packetNo string) {
	records, err := s.db.rpGetRecords(packetNo)
	if err != nil || len(records) == 0 {
		return
	}
	var best *rpRecordModel
	for _, r := range records {
		if best == nil || r.Amount > best.Amount {
			best = r
		}
	}
	if best != nil {
		_ = s.db.rpUpdateRecordBest(packetNo, best.UID)
	}
}
