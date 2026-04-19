package wallet

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/common"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"go.uber.org/zap"
)

const (
	TFStatusPending  = 0
	TFStatusAccepted = 1
	TFStatusRefunded = 2
)

func defaultTransferConfigParsed() *transferConfigParsed {
	return &transferConfigParsed{
		DailyLimitFen: 1000000, ExpireHours: 24,
		MaxAmountFen: 500000, MinAmountFen: 100, RiskThresholdFen: 100000,
	}
}

func (s *service) tfLoadTransferConfigOrDefault() *transferConfigParsed {
	cfg, err := s.db.getTransferConfigParsed()
	if err != nil || cfg == nil {
		if err != nil {
			s.Debug("transfer_config 使用默认值", zap.Error(err))
		}
		return defaultTransferConfigParsed()
	}
	return cfg
}

// tfValidateTransferRules 系统配置：单笔最小/最大、日累计（与后台「系统配置」一致）
func (s *service) tfValidateTransferRules(fromUID string, amount float64) error {
	cfg := s.tfLoadTransferConfigOrDefault()
	sendFen := int64(math.Round(amount * 100))
	if cfg.MinAmountFen > 0 && sendFen < cfg.MinAmountFen {
		return fmt.Errorf("转账金额不得低于 ¥%.2f", float64(cfg.MinAmountFen)/100)
	}
	if cfg.MaxAmountFen > 0 && sendFen > cfg.MaxAmountFen {
		return fmt.Errorf("转账金额不得超过 ¥%.2f", float64(cfg.MaxAmountFen)/100)
	}
	today, err := s.db.tfUserTodaySendTotal(fromUID)
	if err != nil {
		s.Warn("tfUserTodaySendTotal failed", zap.Error(err))
		return nil
	}
	todayFen := int64(math.Round(today * 100))
	if cfg.DailyLimitFen > 0 && todayFen+sendFen > cfg.DailyLimitFen {
		return fmt.Errorf("今日转账累计不能超过 ¥%.2f", float64(cfg.DailyLimitFen)/100)
	}
	return nil
}

// tfSendWithDeduction 发起转账：扣款、流水、写 transfer 同一事务。
// payScene 为 receive_qr 时：同一事务内给收款方加余额并记 transfer_in，transfer.status 直接为已收款，无需再调 accept。
// channelType：0 视为单聊；群聊须为 ChannelTypeGroup 且 channelID 为群编号（领取提示会发到该群）。
func (s *service) tfSendWithDeduction(fromUID, toUID string, amount float64, remark, channelID string, channelType int, payScene string) (string, *tfModel, error) {
	fromUID = strings.TrimSpace(fromUID)
	toUID = strings.TrimSpace(toUID)
	if fromUID == "" || toUID == "" {
		return "", nil, errors.New("无效用户")
	}
	channelID = strings.TrimSpace(channelID)
	if channelType == 0 {
		channelType = int(common.ChannelTypePerson)
	}
	if channelType == int(common.ChannelTypeGroup) && channelID == "" {
		return "", nil, errors.New("群聊转账请传入 channel_id（群编号）")
	}
	var err error
	amount, err = normalizeSpendAmount(amount)
	if err != nil {
		return "", nil, err
	}

	instantReceive := strings.TrimSpace(payScene) == "receive_qr"
	if instantReceive && fromUID == toUID {
		return "", nil, errors.New("不能向自己转账")
	}

	w, err := s.ensureWallet(fromUID)
	if err != nil {
		return "", nil, err
	}
	if w.Status == 2 {
		return "", nil, errors.New("钱包已冻结")
	}

	if instantReceive {
		tw, werr := s.ensureWallet(toUID)
		if werr != nil {
			return "", nil, werr
		}
		if tw.Status == 2 {
			return "", nil, errors.New("对方钱包已冻结")
		}
	}

	if err = s.tfValidateTransferRules(fromUID, amount); err != nil {
		return "", nil, err
	}

	tx, err := s.ctx.DB().Begin()
	if err != nil {
		return "", nil, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	affected, err := s.db.deductBalanceTx(fromUID, amount, tx)
	if err != nil {
		return "", nil, err
	}
	if affected == 0 {
		err = errors.New("余额不足")
		return "", nil, err
	}

	balAfter, err := s.db.walletSelectBalanceTx(fromUID, tx)
	if err != nil {
		return "", nil, err
	}

	transferNo := util.GenerUUID()
	err = s.db.insertTransactionTx(&transactionModel{
		UID:          fromUID,
		Type:         "transfer_out",
		Amount:       -amount,
		RelatedID:    transferNo,
		Remark:       "转账给" + toUID,
		BalanceAfter: balAfter,
	}, tx)
	if err != nil {
		return "", nil, err
	}

	tCfg := s.tfLoadTransferConfigOrDefault()
	expH := tCfg.ExpireHours
	if expH <= 0 {
		expH = 24
	}
	expiredAt := time.Now().Add(time.Duration(expH) * time.Hour)
	tfStatus := TFStatusPending
	if instantReceive {
		tfStatus = TFStatusAccepted
	}
	m := &tfModel{
		TransferNo: transferNo, FromUID: fromUID, ToUID: toUID,
		ChannelID: channelID, ChannelType: channelType,
		Amount: amount, Remark: remark, Status: tfStatus, ExpiredAt: expiredAt,
	}
	if err = s.db.tfInsertTx(m, tx); err != nil {
		return "", nil, err
	}

	if instantReceive {
		if err = s.db.addBalanceTx(toUID, amount, tx); err != nil {
			return "", nil, err
		}
		toBalAfter, berr := s.db.walletSelectBalanceTx(toUID, tx)
		if berr != nil {
			return "", nil, berr
		}
		if err = s.db.insertTransactionTx(&transactionModel{
			UID:          toUID,
			Type:         "transfer_in",
			Amount:       amount,
			RelatedID:    transferNo,
			Remark:       "收到转账",
			BalanceAfter: toBalAfter,
		}, tx); err != nil {
			return "", nil, err
		}
	}

	if err = tx.Commit(); err != nil {
		return "", nil, err
	}
	s.scheduleWalletBalanceIMNotify(fromUID, "transfer_out", transferNo)
	if instantReceive {
		s.scheduleWalletBalanceIMNotify(toUID, "transfer_in", transferNo)
		return transferNo, m, nil
	}
	return transferNo, nil, nil
}

func (s *service) tfAccept(transferNo, uid string) error {
	t, err := s.db.tfGet(transferNo)
	if err != nil {
		return err
	}
	if t == nil {
		return errors.New("转账不存在")
	}
	if t.ToUID != uid {
		return errors.New("无权操作")
	}
	if t.Status != TFStatusPending {
		if t.Status == TFStatusAccepted {
			return errors.New("已收款")
		}
		return errors.New("转账已退回")
	}
	return s.db.tfUpdateStatus(transferNo, TFStatusAccepted)
}

func (s *service) tfProcessExpired() {
	list, err := s.db.tfGetExpired()
	if err != nil {
		s.Warn("get expired transfers failed", zap.Error(err))
		return
	}
	for _, t := range list {
		if refundErr := s.AddBalance(t.FromUID, t.Amount, "transfer_refund", t.TransferNo, "转账过期退回"); refundErr != nil {
			s.Warn("refund expired transfer failed", zap.Error(refundErr), zap.String("transfer_no", t.TransferNo))
			continue
		}
		_ = s.db.tfUpdateStatus(t.TransferNo, TFStatusRefunded)
	}
}

// tfManagerRefund 管理端退回：与 TFStatus 一致，仅 0=待确认、1=已完成 可退，退回后 status=2。
// 待确认：发起方已扣款，直接退回发起方；已完成：从收款方扣回再退发起方（余额不足则失败）。
func (s *service) tfManagerRefund(transferNo, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "管理员退回"
	}
	t, err := s.db.tfGet(transferNo)
	if err != nil {
		return err
	}
	if t == nil {
		return errors.New("转账不存在")
	}
	if t.Status == TFStatusRefunded {
		return errors.New("该转账已退回")
	}

	tx, err := s.ctx.DB().Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	switch t.Status {
	case TFStatusPending:
		if err = s.db.addBalanceTx(t.FromUID, t.Amount, tx); err != nil {
			return err
		}
		balAfter, err2 := s.db.walletSelectBalanceTx(t.FromUID, tx)
		if err2 != nil {
			err = err2
			return err
		}
		err = s.db.insertTransactionTx(&transactionModel{
			UID: t.FromUID, Type: "transfer_refund", Amount: t.Amount,
			RelatedID: transferNo, Remark: "管理员退回: " + reason, BalanceAfter: balAfter,
		}, tx)
		if err != nil {
			return err
		}
	case TFStatusAccepted:
		var affected int64
		affected, err = s.db.deductBalanceTx(t.ToUID, t.Amount, tx)
		if err != nil {
			return err
		}
		if affected == 0 {
			err = errors.New("收款方余额不足，无法完成原路退回")
			return err
		}
		toBalAfter, err2 := s.db.walletSelectBalanceTx(t.ToUID, tx)
		if err2 != nil {
			err = err2
			return err
		}
		err = s.db.insertTransactionTx(&transactionModel{
			UID: t.ToUID, Type: "transfer_refund", Amount: -t.Amount,
			RelatedID: transferNo, Remark: "管理员退回扣回: " + reason, BalanceAfter: toBalAfter,
		}, tx)
		if err != nil {
			return err
		}
		if err = s.db.addBalanceTx(t.FromUID, t.Amount, tx); err != nil {
			return err
		}
		fromBalAfter, err2 := s.db.walletSelectBalanceTx(t.FromUID, tx)
		if err2 != nil {
			err = err2
			return err
		}
		err = s.db.insertTransactionTx(&transactionModel{
			UID: t.FromUID, Type: "transfer_refund", Amount: t.Amount,
			RelatedID: transferNo, Remark: "管理员退回: " + reason, BalanceAfter: fromBalAfter,
		}, tx)
		if err != nil {
			return err
		}
	default:
		return errors.New("当前状态不可退回")
	}

	if err = s.db.tfUpdateStatusTx(transferNo, TFStatusRefunded, tx); err != nil {
		return err
	}
	return tx.Commit()
}
