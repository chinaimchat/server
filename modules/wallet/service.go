package wallet

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/log"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"go.uber.org/zap"
)

type service struct {
	ctx *config.Context
	db  *walletDB
	log.Log
	rpExpireMu      sync.Mutex
	lastRpExpireRun time.Time
}

func newService(ctx *config.Context) *service {
	return &service{
		ctx: ctx,
		db:  newWalletDB(ctx),
		Log: log.NewTLog("WalletService"),
	}
}

func hashPassword(password string) string {
	h := sha256.New()
	h.Write([]byte(password))
	return hex.EncodeToString(h.Sum(nil))
}

func (s *service) ensureWallet(uid string) (*walletModel, error) {
	w, err := s.db.getWallet(uid)
	if err != nil {
		return nil, err
	}
	if w == nil || w.UID == "" {
		err = s.db.insertWallet(uid)
		if err != nil {
			return nil, err
		}
		w, err = s.db.getWallet(uid)
		if err != nil {
			return nil, err
		}
	}
	return w, nil
}

// getWalletBalances 可用、冻结、账面总资产（可用+冻结）
func (s *service) getWalletBalances(uid string) (available, frozen, total float64, err error) {
	w, err := s.ensureWallet(uid)
	if err != nil {
		return 0, 0, 0, err
	}
	return w.Balance, w.FrozenBalance, w.Balance + w.FrozenBalance, nil
}

func (s *service) hasPayPassword(uid string) (bool, error) {
	w, err := s.ensureWallet(uid)
	if err != nil {
		return false, err
	}
	return w.PayPassword != "", nil
}

func (s *service) setPayPassword(uid string, password string) error {
	_, err := s.ensureWallet(uid)
	if err != nil {
		return err
	}
	return s.db.updatePayPassword(uid, hashPassword(password))
}

func (s *service) changePayPassword(uid string, oldPwd, newPwd string) error {
	w, err := s.ensureWallet(uid)
	if err != nil {
		return err
	}
	if w.PayPassword != hashPassword(oldPwd) {
		return errors.New("原密码错误")
	}
	return s.db.updatePayPassword(uid, hashPassword(newPwd))
}

func (s *service) VerifyPayPassword(uid string, password string) error {
	w, err := s.ensureWallet(uid)
	if err != nil {
		return err
	}
	if w.PayPassword == "" {
		return errors.New("请先设置支付密码")
	}
	if w.PayPassword != hashPassword(password) {
		return errors.New("支付密码错误")
	}
	return nil
}

func normalizeSpendAmount(amount float64) (float64, error) {
	if amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return 0, errors.New("金额无效")
	}
	return math.Round(amount*100) / 100, nil
}

func (s *service) DeductBalance(uid string, amount float64, txType, relatedID, remark string) error {
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return errors.New("无效用户")
	}
	var err error
	amount, err = normalizeSpendAmount(amount)
	if err != nil {
		return err
	}
	w, err := s.ensureWallet(uid)
	if err != nil {
		return err
	}
	if w.Status == 2 {
		return errors.New("钱包已冻结")
	}

	tx, err := s.ctx.DB().Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				s.Warn("rollback failed", zap.Error(rbErr))
			}
		}
	}()

	affected, err := s.db.deductBalanceTx(uid, amount, tx)
	if err != nil {
		return err
	}
	if affected == 0 {
		err = errors.New("余额不足")
		return err
	}

	balAfter, err := s.db.walletSelectBalanceTx(uid, tx)
	if err != nil {
		return err
	}

	err = s.db.insertTransactionTx(&transactionModel{
		UID:          uid,
		Type:         txType,
		Amount:       -amount,
		RelatedID:    relatedID,
		Remark:       remark,
		BalanceAfter: balAfter,
	}, tx)
	if err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return err
	}
	s.scheduleWalletBalanceIMNotify(uid, txType, relatedID)
	return nil
}

func (s *service) AddBalance(uid string, amount float64, txType, relatedID, remark string) error {
	_, err := s.ensureWallet(uid)
	if err != nil {
		return err
	}

	tx, err := s.ctx.DB().Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				s.Warn("rollback failed", zap.Error(rbErr))
			}
		}
	}()

	err = s.db.addBalanceTx(uid, amount, tx)
	if err != nil {
		return err
	}

	balAfter, err := s.db.walletSelectBalanceTx(uid, tx)
	if err != nil {
		return err
	}

	err = s.db.insertTransactionTx(&transactionModel{
		UID:          uid,
		Type:         txType,
		Amount:       amount,
		RelatedID:    relatedID,
		Remark:       remark,
		BalanceAfter: balAfter,
	}, tx)
	if err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return err
	}
	s.scheduleWalletBalanceIMNotify(uid, txType, relatedID)
	return nil
}

const minWithdrawalAmount = 0.01

// computeWithdrawalFee fee_rate 为占提现金额的百分比（如 1 表示 1%），fee_fixed 为每笔另加的固定金额。
func computeWithdrawalFee(amount float64, feeRatePctStr, feeFixedStr string) float64 {
	rateStr := strings.TrimSpace(feeRatePctStr)
	fixedStr := strings.TrimSpace(feeFixedStr)
	ratePct, _ := strconv.ParseFloat(rateStr, 64)
	if ratePct < 0 || math.IsNaN(ratePct) || math.IsInf(ratePct, 0) {
		ratePct = 0
	}
	if ratePct > 100 {
		ratePct = 100
	}
	fixedPart, _ := strconv.ParseFloat(fixedStr, 64)
	if fixedPart < 0 || math.IsNaN(fixedPart) || math.IsInf(fixedPart, 0) {
		fixedPart = 0
	}
	fee := amount*ratePct/100.0 + fixedPart
	if fee < 0 {
		fee = 0
	}
	return math.Round(fee*100) / 100
}

// ComputeWithdrawalFeeForAmount 根据后台配置的费率计算手续费（供试算接口与申请共用）
func (s *service) ComputeWithdrawalFeeForAmount(amount float64) (float64, error) {
	cfg, err := s.db.getWithdrawalConfig()
	if err != nil {
		return 0, err
	}
	return computeWithdrawalFee(amount, cfg.FeeRate, cfg.FeeFixed), nil
}

// applyWithdrawal 用户发起提现：将 amount+fee 从可用划入冻结，写入待审核；审核通过再真实划出冻结，拒绝/超时解冻退回。
func (s *service) applyWithdrawal(uid string, amount float64, address, remark, password string) (withdrawalNo string, fee float64, err error) {
	if err = s.VerifyPayPassword(uid, password); err != nil {
		return "", 0, err
	}
	if amount < minWithdrawalAmount {
		return "", 0, errors.New("提现金额过低")
	}
	address = strings.TrimSpace(address)
	if address == "" {
		return "", 0, errors.New("请填写收款账户或地址")
	}
	w, err := s.ensureWallet(uid)
	if err != nil {
		return "", 0, err
	}
	if w.Status == 2 {
		return "", 0, errors.New("钱包已冻结，无法提现")
	}
	cfg, err := s.db.getWithdrawalConfig()
	if err != nil {
		return "", 0, err
	}
	fee = computeWithdrawalFee(amount, cfg.FeeRate, cfg.FeeFixed)
	if fee >= amount {
		return "", fee, errors.New("手续费不能大于或等于提现金额")
	}

	withdrawalNo = util.GenerUUID()
	tx, err := s.ctx.DB().Begin()
	if err != nil {
		return "", fee, err
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				s.Warn("rollback failed", zap.Error(rbErr))
			}
		}
	}()

	affected, err := s.db.freezeBalanceForWithdrawalTx(uid, amount, tx)
	if err != nil {
		return "", fee, err
	}
	if affected == 0 {
		err = errors.New("余额不足")
		return "", fee, err
	}

	balAfter, err := s.db.walletSelectBalanceTx(uid, tx)
	if err != nil {
		return "", fee, err
	}

	err = s.db.insertTransactionTx(&transactionModel{
		UID:          uid,
		Type:         "withdrawal_freeze",
		Amount:       -amount,
		RelatedID:    withdrawalNo,
		Remark:       fmt.Sprintf("提现冻结¥%.2f(含手续费¥%.2f，到账¥%.2f)", amount, fee, amount-fee),
		BalanceAfter: balAfter,
	}, tx)
	if err != nil {
		return "", fee, err
	}

	err = s.db.insertWithdrawalTx(&withdrawalModel{
		WithdrawalNo: withdrawalNo,
		UID:          uid,
		Amount:       amount,
		Fee:          fee,
		Status:       0,
		Address:      address,
		Remark:       strings.TrimSpace(remark),
	}, tx)
	if err != nil {
		return "", fee, err
	}

	if err = tx.Commit(); err != nil {
		return "", fee, err
	}
	s.scheduleWalletBalanceIMNotify(uid, "withdrawal_apply", withdrawalNo)
	return withdrawalNo, fee, nil
}

// ApproveWithdrawal 管理端通过：从冻结划出（真实扣减），状态改为已批准。冻结的是 amount，实际到账 amount-fee。
func (s *service) ApproveWithdrawal(id int64, adminRemark string) error {
	wd, err := s.db.getWithdrawal(id)
	if err != nil || wd == nil || wd.WithdrawalNo == "" {
		return errors.New("提现记录不存在")
	}
	if wd.Status != 0 {
		return errors.New("该提现已处理，无法重复审核")
	}

	tx, err := s.ctx.DB().Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				s.Warn("rollback failed", zap.Error(rbErr))
			}
		}
	}()

	affected, err := s.db.finalizeFrozenWithdrawalTx(wd.UID, wd.Amount, tx)
	if err != nil {
		return err
	}
	if affected == 0 {
		err = errors.New("冻结余额不足，无法完成审核，请核对数据")
		return err
	}

	balAfter, err := s.db.walletSelectBalanceTx(wd.UID, tx)
	if err != nil {
		return err
	}
	actualAmount := wd.Amount - wd.Fee
	err = s.db.insertTransactionTx(&transactionModel{
		UID:          wd.UID,
		Type:         "withdrawal",
		Amount:       -wd.Amount,
		RelatedID:    wd.WithdrawalNo,
		Remark:       fmt.Sprintf("提现完成(到账¥%.2f，手续费¥%.2f)", actualAmount, wd.Fee),
		BalanceAfter: balAfter,
	}, tx)
	if err != nil {
		return err
	}
	if err = s.db.updateWithdrawalStatusTx(id, 1, adminRemark, tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	s.scheduleWalletBalanceIMNotify(wd.UID, "withdrawal_approve", wd.WithdrawalNo)
	return nil
}

// RejectWithdrawal 管理端拒绝或超时任务：解冻并退回可用（退回 amount），状态改为已拒绝
func (s *service) RejectWithdrawal(id int64, adminRemark string) error {
	wd, err := s.db.getWithdrawal(id)
	if err != nil || wd == nil || wd.WithdrawalNo == "" {
		return errors.New("提现记录不存在")
	}
	if wd.Status != 0 {
		return errors.New("该提现已处理，无法重复审核")
	}

	tx, err := s.ctx.DB().Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				s.Warn("rollback failed", zap.Error(rbErr))
			}
		}
	}()

	affected, err := s.db.unfreezeBalanceForWithdrawalTx(wd.UID, wd.Amount, tx)
	if err != nil {
		return err
	}
	if affected == 0 {
		err = errors.New("冻结余额不足，无法退款，请核对数据")
		return err
	}

	balAfter, err := s.db.walletSelectBalanceTx(wd.UID, tx)
	if err != nil {
		return err
	}
	err = s.db.insertTransactionTx(&transactionModel{
		UID:          wd.UID,
		Type:         "withdrawal_refund",
		Amount:       wd.Amount,
		RelatedID:    wd.WithdrawalNo,
		Remark:       "提现退回",
		BalanceAfter: balAfter,
	}, tx)
	if err != nil {
		return err
	}
	if err = s.db.updateWithdrawalStatusTx(id, 2, adminRemark, tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	s.scheduleWalletBalanceIMNotify(wd.UID, "withdrawal_reject", wd.WithdrawalNo)
	return nil
}

const pendingWithdrawalTimeoutHours = 72

func (s *service) withdrawalProcessExpired() {
	cutoff := time.Now().Add(-time.Duration(pendingWithdrawalTimeoutHours) * time.Hour)
	list, err := s.db.listPendingWithdrawalsExpired(cutoff)
	if err != nil {
		s.Warn("list pending withdrawals for timeout failed", zap.Error(err))
		return
	}
	for _, wd := range list {
		if err := s.RejectWithdrawal(wd.ID, "超时未审核，系统自动退回"); err != nil {
			s.Warn("withdrawal auto refund failed", zap.String("withdrawal_no", wd.WithdrawalNo), zap.Error(err))
		}
	}
}

const minRechargeApplyAmount = 0.01
const maxPendingRechargeApplications = 20

func parseExchangeRateFromInstallKey(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("渠道未配置汇率")
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, errors.New("汇率无效，请在支付配置中填写大于0的数字")
	}
	return v, nil
}

// SubmitRechargeApplication 用户提交充值申请（不直接入账，待管理员审核）。
// USDT-TRC20(pay_type=4)：若 amount_u>0 且已选渠道，则 入账金额 = amount_u × 渠道 install_key（汇率）；否则 amount 视为已折算后的入账金额。
func (s *service) SubmitRechargeApplication(uid string, amount, amountU float64, channelID int64, payType int, remark, proofURL string) (applicationNo string, creditedAmount float64, storedAmountU float64, storedRate float64, err error) {
	w, err := s.ensureWallet(uid)
	if err != nil {
		return "", 0, 0, 0, err
	}
	if w.Status == 2 {
		return "", 0, 0, 0, errors.New("钱包已冻结，无法提交充值申请")
	}
	pending, err := s.db.countPendingRechargeApplications(uid)
	if err != nil {
		return "", 0, 0, 0, err
	}
	if pending >= maxPendingRechargeApplications {
		return "", 0, 0, 0, errors.New("待审核充值申请过多，请等待处理后再提交")
	}

	var ch *rechargeChannelPublic
	var pt int
	var cid int64
	if channelID > 0 {
		ch, err = s.db.getRechargeChannelEnabledByID(channelID)
		if err != nil {
			return "", 0, 0, 0, err
		}
		if ch == nil {
			return "", 0, 0, 0, errors.New("充值渠道无效或已停用")
		}
		pt = ch.PayType
		cid = ch.ID
	} else {
		if payType != 2 && payType != 3 && payType != 4 {
			return "", 0, 0, 0, errors.New("请选择支付方式")
		}
		pt = payType
		cid = 0
	}

	var finalAmount float64
	var uStored float64
	var rateStored float64

	if pt == 4 && amountU > 0 {
		if ch == nil {
			return "", 0, 0, 0, errors.New("USDT-TRC20 充值请选择渠道，以便按汇率折算")
		}
		rate, e := parseExchangeRateFromInstallKey(ch.InstallKey)
		if e != nil {
			return "", 0, 0, 0, e
		}
		amountU, e = normalizeSpendAmount(amountU)
		if e != nil {
			return "", 0, 0, 0, errors.New("U数量无效")
		}
		if amountU < minRechargeApplyAmount {
			return "", 0, 0, 0, errors.New("U数量过低")
		}
		finalAmount = amountU * rate
		uStored = amountU
		rateStored = rate
	} else {
		finalAmount = amount
	}

	finalAmount, err = normalizeSpendAmount(finalAmount)
	if err != nil {
		return "", 0, 0, 0, err
	}
	if finalAmount < minRechargeApplyAmount {
		return "", 0, 0, 0, errors.New("充值金额过低")
	}

	applicationNo = util.GenerUUID()
	m := &rechargeApplicationModel{
		ApplicationNo: applicationNo,
		UID:           uid,
		Amount:        finalAmount,
		AmountU:       uStored,
		ExchangeRate:  rateStored,
		ChannelID:     cid,
		PayType:       pt,
		Remark:        strings.TrimSpace(remark),
		ProofURL:      strings.TrimSpace(proofURL),
		Status:        0,
	}
	err = s.db.insertRechargeApplication(m)
	if err != nil {
		return "", 0, 0, 0, err
	}
	return applicationNo, finalAmount, uStored, rateStored, nil
}

// ApproveRechargeApplication 管理员通过充值申请并入账
func (s *service) ApproveRechargeApplication(id int64, operatorUID, adminRemark string) error {
	app, err := s.db.getRechargeApplication(id)
	if err != nil {
		return err
	}
	if app == nil {
		return errors.New("申请记录不存在")
	}
	if app.Status != 0 {
		return errors.New("该申请已处理，无法重复审核")
	}
	_, err = s.ensureWallet(app.UID)
	if err != nil {
		return err
	}

	tx, err := s.ctx.DB().Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				s.Warn("rollback failed", zap.Error(rbErr))
			}
		}
	}()

	res, err := tx.UpdateBySql(
		"UPDATE wallet_recharge_application SET status=1, admin_remark=?, reviewer_uid=?, updated_at=NOW() WHERE id=? AND status=0",
		adminRemark, operatorUID, id,
	).Exec()
	if err != nil {
		return err
	}
	var n int64
	n, err = res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		err = errors.New("该申请已处理或不存在")
		return err
	}

	err = s.db.addBalanceTx(app.UID, app.Amount, tx)
	if err != nil {
		return err
	}
	balAfter, err := s.db.walletSelectBalanceTx(app.UID, tx)
	if err != nil {
		return err
	}
	err = s.db.insertTransactionTx(&transactionModel{
		UID:          app.UID,
		Type:         "recharge",
		Amount:       app.Amount,
		RelatedID:    app.ApplicationNo,
		Remark:       "充值审核通过",
		BalanceAfter: balAfter,
	}, tx)
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	s.scheduleWalletBalanceIMNotify(app.UID, "recharge_approve", app.ApplicationNo)
	return nil
}

// RejectRechargeApplication 管理员拒绝充值申请（不入账）
func (s *service) RejectRechargeApplication(id int64, operatorUID, adminRemark string) error {
	if strings.TrimSpace(adminRemark) == "" {
		return errors.New("请输入拒绝原因")
	}
	app, err := s.db.getRechargeApplication(id)
	if err != nil {
		return err
	}
	if app == nil {
		return errors.New("申请记录不存在")
	}
	if app.Status != 0 {
		return errors.New("该申请已处理，无法重复审核")
	}
	res, err := s.ctx.DB().UpdateBySql(
		"UPDATE wallet_recharge_application SET status=2, admin_remark=?, reviewer_uid=?, updated_at=NOW() WHERE id=? AND status=0",
		strings.TrimSpace(adminRemark), operatorUID, id,
	).Exec()
	if err != nil {
		return err
	}
	var n int64
	n, err = res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("该申请已处理或不存在")
	}
	return nil
}
