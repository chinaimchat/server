package wallet

import (
	"errors"
	"strings"
	"time"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/gocraft/dbr/v2"
)

type walletDB struct {
	session *dbr.Session
	ctx     *config.Context
}

func newWalletDB(ctx *config.Context) *walletDB {
	return &walletDB{
		session: ctx.DB(),
		ctx:     ctx,
	}
}

type walletModel struct {
	UID           string    `db:"uid"`
	Balance       float64   `db:"balance"`
	FrozenBalance float64   `db:"frozen_balance"`
	PayPassword   string    `db:"pay_password"`
	Status        int       `db:"status"`
	Phone       string    `db:"phone"`
	Zone        string    `db:"zone"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
	Name        string    `db:"name"`
	Username    string    `db:"username"`
}

type transactionModel struct {
	ID           int64     `db:"id"`
	UID          string    `db:"uid"`
	Type         string    `db:"type"`
	Amount       float64   `db:"amount"`
	RelatedID    string    `db:"related_id"`
	Remark       string    `db:"remark"`
	BalanceAfter float64   `db:"balance_after"`
	CreatedAt    time.Time `db:"created_at"`
}

type withdrawalModel struct {
	ID           int64     `db:"id"`
	WithdrawalNo string    `db:"withdrawal_no"`
	UID          string    `db:"uid"`
	Amount       float64   `db:"amount"`
	Fee          float64   `db:"fee"`
	Status       int       `db:"status"`
	Address      string    `db:"address"`
	Remark       string    `db:"remark"`
	AdminRemark  string    `db:"admin_remark"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}

type rechargeApplicationModel struct {
	ID             int64     `db:"id"`
	ApplicationNo  string    `db:"application_no"`
	UID            string    `db:"uid"`
	Amount         float64   `db:"amount"`
	AmountU        float64   `db:"amount_u"`
	ExchangeRate   float64   `db:"exchange_rate"`
	ChannelID      int64     `db:"channel_id"`
	PayType        int       `db:"pay_type"`
	Remark         string    `db:"remark"`
	ProofURL       string    `db:"proof_url"`
	Status         int       `db:"status"`
	AdminRemark    string    `db:"admin_remark"`
	ReviewerUID    string    `db:"reviewer_uid"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
}

type operationLogModel struct {
	ID            int64     `db:"id"`
	Operator      string    `db:"operator"`
	Action        string    `db:"action"`
	TargetUID     string    `db:"target_uid"`
	Amount        float64   `db:"amount"`
	Reason        string    `db:"reason"`
	Result        string    `db:"result"`
	Detail        string    `db:"detail"`
	OperationDesc string    `db:"operation_desc"`
	IPAddress     string    `db:"ip_address"`
	UserAgent     string    `db:"user_agent"`
	ErrorMsg      string    `db:"error_msg"`
	OperationData string    `db:"operation_data"`
	TargetInfo    string    `db:"target_info"`
	CreatedAt     time.Time `db:"created_at"`
}

type customerServiceModel struct {
	ID        int64     `db:"id"`
	Name      string    `db:"name"`
	UID       string    `db:"uid"`
	CreatedAt time.Time `db:"created_at"`
}

// ===== Wallet =====

func (d *walletDB) getWallet(uid string) (*walletModel, error) {
	var m walletModel
	_, err := d.session.Select("*").From("wallet").Where("uid=?", uid).Load(&m)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (d *walletDB) insertWallet(uid string) error {
	_, err := d.session.InsertInto("wallet").Columns("uid").Values(uid).Exec()
	return err
}

func (d *walletDB) updatePayPassword(uid string, password string) error {
	_, err := d.session.Update("wallet").Set("pay_password", password).Where("uid=?", uid).Exec()
	return err
}

func (d *walletDB) addBalanceTx(uid string, amount float64, tx *dbr.Tx) error {
	_, err := tx.UpdateBySql("UPDATE wallet SET balance = balance + ?, updated_at = NOW() WHERE uid = ?", amount, uid).Exec()
	return err
}

func (d *walletDB) deductBalanceTx(uid string, amount float64, tx *dbr.Tx) (int64, error) {
	result, err := tx.UpdateBySql("UPDATE wallet SET balance = balance - ?, updated_at = NOW() WHERE uid = ? AND balance >= ?", amount, uid, amount).Exec()
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// freezeBalanceForWithdrawalTx 提现申请：从可用划入冻结（总额 amount+fee 由调用方传入 total）
func (d *walletDB) freezeBalanceForWithdrawalTx(uid string, total float64, tx *dbr.Tx) (int64, error) {
	result, err := tx.UpdateBySql(
		"UPDATE wallet SET balance = balance - ?, frozen_balance = frozen_balance + ?, updated_at = NOW() WHERE uid = ? AND balance >= ?",
		total, total, uid, total).Exec()
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// unfreezeBalanceForWithdrawalTx 拒绝/超时：冻结退回可用
func (d *walletDB) unfreezeBalanceForWithdrawalTx(uid string, total float64, tx *dbr.Tx) (int64, error) {
	result, err := tx.UpdateBySql(
		"UPDATE wallet SET balance = balance + ?, frozen_balance = frozen_balance - ?, updated_at = NOW() WHERE uid = ? AND frozen_balance >= ?",
		total, total, uid, total).Exec()
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// finalizeFrozenWithdrawalTx 审核通过：从冻结中划出（真实扣减，不再回到可用）
func (d *walletDB) finalizeFrozenWithdrawalTx(uid string, total float64, tx *dbr.Tx) (int64, error) {
	result, err := tx.UpdateBySql(
		"UPDATE wallet SET frozen_balance = frozen_balance - ?, updated_at = NOW() WHERE uid = ? AND frozen_balance >= ?",
		total, uid, total).Exec()
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// walletSelectBalanceTx 事务内读取当前余额（用于扣款后写入 balance_after）
func (d *walletDB) walletSelectBalanceTx(uid string, tx *dbr.Tx) (float64, error) {
	var bal float64
	n, err := tx.Select("balance").From("wallet").Where("uid=?", uid).Load(&bal)
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, errors.New("钱包不存在")
	}
	return bal, nil
}

func (d *walletDB) insertTransactionTx(m *transactionModel, tx *dbr.Tx) error {
	_, err := tx.InsertInto("wallet_transaction").Columns("uid", "type", "amount", "related_id", "remark", "balance_after").Values(m.UID, m.Type, m.Amount, m.RelatedID, m.Remark, m.BalanceAfter).Exec()
	return err
}

// getTransactionsFiltered C 端交易记录：支持类型、日期区间、分页
func (d *walletDB) getTransactionsFiltered(uid, txType, startTime, endTime string, page, size int) ([]*transactionModel, int, error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	offset := (page - 1) * size
	q := d.session.Select("*").From("wallet_transaction").Where("uid=?", uid)
	cq := d.session.Select("count(*)").From("wallet_transaction").Where("uid=?", uid)
	if txType != "" {
		q = q.Where("type=?", txType)
		cq = cq.Where("type=?", txType)
	}
	if startTime != "" {
		q = q.Where("created_at >= ?", startTime+" 00:00:00")
		cq = cq.Where("created_at >= ?", startTime+" 00:00:00")
	}
	if endTime != "" {
		q = q.Where("created_at <= ?", endTime+" 23:59:59")
		cq = cq.Where("created_at <= ?", endTime+" 23:59:59")
	}
	var total int
	_, _ = cq.Load(&total)
	var list []*transactionModel
	_, err := q.OrderDir("created_at", false).Limit(uint64(size)).Offset(uint64(offset)).Load(&list)
	return list, total, err
}

func (d *walletDB) countPendingWithdrawals(uid string) (int, error) {
	var n int
	_, err := d.session.Select("count(*)").From("wallet_withdrawal").Where("uid=? AND status=?", uid, 0).Load(&n)
	return n, err
}

func (d *walletDB) getWithdrawalByNoAndUID(withdrawalNo, uid string) (*withdrawalModel, error) {
	var m withdrawalModel
	n, err := d.session.Select("*").From("wallet_withdrawal").Where("withdrawal_no=? AND uid=?", withdrawalNo, uid).Load(&m)
	if err != nil {
		return nil, err
	}
	if n == 0 || m.WithdrawalNo == "" {
		return nil, nil
	}
	return &m, nil
}

// rechargeChannelPublic C 端充值渠道（仅启用项，与后台「充值渠道管理」同源表 recharge_channel）
type rechargeChannelPublic struct {
	ID          int64  `db:"id" json:"id"`
	AppID       string `db:"app_id" json:"app_id"`
	PayType     int    `db:"pay_type" json:"pay_type"`
	Icon        string `db:"icon" json:"icon"`
	QrURL       string `db:"qr_url" json:"qr_url"`
	QrImageURL  string `db:"qr_image_url" json:"qr_image_url"`
	PayAddress  string `db:"pay_address" json:"pay_address"`
	InstallKey  string `db:"install_key" json:"install_key"`
	Title       string `db:"title" json:"title"`
	Remark      string `db:"remark" json:"remark"`
}

func (d *walletDB) listEnabledRechargeChannels() ([]*rechargeChannelPublic, error) {
	var list []*rechargeChannelPublic
	_, err := d.session.Select("id", "app_id", "pay_type", "icon", "qr_url", "qr_image_url", "pay_address", "install_key", "title", "remark").
		From("recharge_channel").Where("status=?", 1).OrderDir("created_at", false).Load(&list)
	if list == nil {
		list = []*rechargeChannelPublic{}
	}
	return list, err
}

func (d *walletDB) getRechargeChannelEnabledByID(id int64) (*rechargeChannelPublic, error) {
	if id <= 0 {
		return nil, nil
	}
	var ch rechargeChannelPublic
	n, err := d.session.Select("id", "app_id", "pay_type", "icon", "qr_url", "qr_image_url", "pay_address", "install_key", "title", "remark").
		From("recharge_channel").Where("id=? AND status=?", id, 1).Load(&ch)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}
	return &ch, nil
}

func (d *walletDB) getWalletList(keyword string, status int, sortBy string, minAmount, maxAmount float64, page, size int) ([]*walletModel, int, error) {
	offset := (page - 1) * size
	where := "1=1"
	var args []interface{}

	if keyword != "" {
		like := "%" + keyword + "%"
		where += " AND (w.uid LIKE ? OR w.phone LIKE ? OR u.name LIKE ? OR u.username LIKE ?)"
		args = append(args, like, like, like, like)
	}
	if status >= 0 {
		where += " AND w.status=?"
		args = append(args, status)
	}
	if minAmount > 0 {
		where += " AND w.balance >= ?"
		args = append(args, minAmount)
	}
	if maxAmount > 0 {
		where += " AND w.balance <= ?"
		args = append(args, maxAmount)
	}

	countSQL := "SELECT COUNT(*) FROM wallet w LEFT JOIN `user` u ON w.uid=u.uid WHERE " + where
	var count int
	_, _ = d.session.SelectBySql(countSQL, args...).Load(&count)

	orderClause := " ORDER BY w.created_at DESC"
	switch sortBy {
	case "created_at_asc":
		orderClause = " ORDER BY w.created_at ASC"
	case "amount_desc":
		orderClause = " ORDER BY w.balance DESC"
	case "amount_asc":
		orderClause = " ORDER BY w.balance ASC"
	case "updated_at_desc":
		orderClause = " ORDER BY w.updated_at DESC"
	}

	dataSQL := "SELECT w.*, COALESCE(u.name,'') as name, COALESCE(u.username,'') as username FROM wallet w LEFT JOIN `user` u ON w.uid=u.uid WHERE " + where + orderClause + " LIMIT ? OFFSET ?"
	dataArgs := append(args, size, offset)

	var list []*walletModel
	_, err := d.session.SelectBySql(dataSQL, dataArgs...).Load(&list)
	return list, count, err
}

func (d *walletDB) getTransactionsAdmin(keyword string, page, size int) ([]*transactionModel, int, error) {
	offset := (page - 1) * size
	query := d.session.Select("*").From("wallet_transaction")
	countQuery := d.session.Select("count(*)").From("wallet_transaction")
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("uid LIKE ? OR related_id LIKE ? OR remark LIKE ?", like, like, like)
		countQuery = countQuery.Where("uid LIKE ? OR related_id LIKE ? OR remark LIKE ?", like, like, like)
	}
	var count int
	_, _ = countQuery.Load(&count)
	var list []*transactionModel
	_, err := query.OrderDir("created_at", false).Limit(uint64(size)).Offset(uint64(offset)).Load(&list)
	return list, count, err
}

func (d *walletDB) updateWalletStatus(uid string, status int) error {
	_, err := d.session.Update("wallet").Set("status", status).Set("updated_at", dbr.Now).Where("uid=?", uid).Exec()
	return err
}

func (d *walletDB) resetPayPassword(uid string) error {
	_, err := d.session.Update("wallet").Set("pay_password", "").Where("uid=?", uid).Exec()
	return err
}

func (d *walletDB) syncUserInfo(uid, phone, zone string) error {
	_, err := d.session.Update("wallet").Set("phone", phone).Set("zone", zone).Where("uid=?", uid).Exec()
	return err
}

// ===== Statistics =====

type walletStatistics struct {
	TotalWallets     int     `json:"total_wallets"`
	TotalAmount      float64 `json:"total_amount"`
	ActiveWallets    int     `json:"active_wallets"`
	FrozenWallets    int     `json:"frozen_wallets"`
	TodayNewWallets  int     `json:"today_new_wallets"`
	TodayTransactions int    `json:"today_transactions"`
	TodayAmount      float64 `json:"today_amount"`
}

func (d *walletDB) getStatistics() (*walletStatistics, error) {
	s := &walletStatistics{}
	_, _ = d.session.SelectBySql("SELECT COUNT(*) FROM wallet").Load(&s.TotalWallets)
	_, _ = d.session.SelectBySql("SELECT COALESCE(SUM(balance + frozen_balance),0) FROM wallet").Load(&s.TotalAmount)
	_, _ = d.session.SelectBySql("SELECT COUNT(*) FROM wallet WHERE status=1").Load(&s.ActiveWallets)
	_, _ = d.session.SelectBySql("SELECT COUNT(*) FROM wallet WHERE status=2").Load(&s.FrozenWallets)
	_, _ = d.session.SelectBySql("SELECT COUNT(*) FROM wallet WHERE DATE(created_at)=CURDATE()").Load(&s.TodayNewWallets)
	_, _ = d.session.SelectBySql("SELECT COUNT(*) FROM wallet_transaction WHERE DATE(created_at)=CURDATE()").Load(&s.TodayTransactions)
	_, _ = d.session.SelectBySql("SELECT COALESCE(SUM(ABS(amount)),0) FROM wallet_transaction WHERE DATE(created_at)=CURDATE()").Load(&s.TodayAmount)
	return s, nil
}

// ===== Withdrawal =====

func (d *walletDB) getWithdrawalList(keyword string, status, page, size int) ([]*withdrawalModel, int, error) {
	offset := (page - 1) * size
	query := d.session.Select("*").From("wallet_withdrawal")
	countQuery := d.session.Select("count(*)").From("wallet_withdrawal")
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("withdrawal_no LIKE ? OR uid LIKE ?", like, like)
		countQuery = countQuery.Where("withdrawal_no LIKE ? OR uid LIKE ?", like, like)
	}
	if status >= 0 {
		query = query.Where("status=?", status)
		countQuery = countQuery.Where("status=?", status)
	}
	var count int
	_, _ = countQuery.Load(&count)
	var list []*withdrawalModel
	_, err := query.OrderDir("created_at", false).Limit(uint64(size)).Offset(uint64(offset)).Load(&list)
	return list, count, err
}

func (d *walletDB) updateWithdrawalStatusTx(id int64, status int, adminRemark string, tx *dbr.Tx) error {
	_, err := tx.Update("wallet_withdrawal").Set("status", status).Set("admin_remark", adminRemark).Set("updated_at", dbr.Now).Where("id=?", id).Exec()
	return err
}

// listPendingWithdrawalsExpired 待审核且早于 cutoff 的记录（超时自动退回）
func (d *walletDB) listPendingWithdrawalsExpired(before time.Time) ([]*withdrawalModel, error) {
	var list []*withdrawalModel
	_, err := d.session.Select("*").From("wallet_withdrawal").Where("status=? AND created_at<?", 0, before).Load(&list)
	if list == nil {
		list = []*withdrawalModel{}
	}
	return list, err
}

func (d *walletDB) getWithdrawal(id int64) (*withdrawalModel, error) {
	var m withdrawalModel
	_, err := d.session.Select("*").From("wallet_withdrawal").Where("id=?", id).Load(&m)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (d *walletDB) insertWithdrawalTx(m *withdrawalModel, tx *dbr.Tx) error {
	_, err := tx.InsertInto("wallet_withdrawal").
		Columns("withdrawal_no", "uid", "amount", "fee", "status", "address", "remark", "admin_remark").
		Values(m.WithdrawalNo, m.UID, m.Amount, m.Fee, m.Status, m.Address, m.Remark, "").
		Exec()
	return err
}

func (d *walletDB) getWithdrawalsByUID(uid string, page, size int) ([]*withdrawalModel, int, error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	offset := (page - 1) * size
	countQuery := d.session.Select("count(*)").From("wallet_withdrawal").Where("uid=?", uid)
	var count int
	_, _ = countQuery.Load(&count)
	var list []*withdrawalModel
	_, err := d.session.Select("*").From("wallet_withdrawal").Where("uid=?", uid).OrderDir("created_at", false).Limit(uint64(size)).Offset(uint64(offset)).Load(&list)
	return list, count, err
}

// ===== Recharge application（用户提交、管理员审核入账）=====

func (d *walletDB) countPendingRechargeApplications(uid string) (int, error) {
	var n int
	_, err := d.session.Select("count(*)").From("wallet_recharge_application").Where("uid=? AND status=?", uid, 0).Load(&n)
	return n, err
}

func (d *walletDB) insertRechargeApplication(m *rechargeApplicationModel) error {
	_, err := d.session.InsertInto("wallet_recharge_application").
		Columns("application_no", "uid", "amount", "amount_u", "exchange_rate", "channel_id", "pay_type", "remark", "proof_url", "status", "admin_remark", "reviewer_uid").
		Values(m.ApplicationNo, m.UID, m.Amount, m.AmountU, m.ExchangeRate, m.ChannelID, m.PayType, m.Remark, m.ProofURL, m.Status, "", "").
		Exec()
	return err
}

func (d *walletDB) getRechargeApplication(id int64) (*rechargeApplicationModel, error) {
	var m rechargeApplicationModel
	n, err := d.session.Select("*").From("wallet_recharge_application").Where("id=?", id).Load(&m)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}
	return &m, nil
}

func (d *walletDB) getRechargeApplicationList(keyword string, status, page, size int) ([]*rechargeApplicationModel, int, error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	offset := (page - 1) * size
	query := d.session.Select("*").From("wallet_recharge_application")
	countQuery := d.session.Select("count(*)").From("wallet_recharge_application")
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("application_no LIKE ? OR uid LIKE ?", like, like)
		countQuery = countQuery.Where("application_no LIKE ? OR uid LIKE ?", like, like)
	}
	if status >= 0 {
		query = query.Where("status=?", status)
		countQuery = countQuery.Where("status=?", status)
	}
	var count int
	_, _ = countQuery.Load(&count)
	var list []*rechargeApplicationModel
	_, err := query.OrderDir("created_at", false).Limit(uint64(size)).Offset(uint64(offset)).Load(&list)
	return list, count, err
}

func (d *walletDB) getRechargeApplicationsByUID(uid string, page, size int) ([]*rechargeApplicationModel, int, error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	offset := (page - 1) * size
	countQuery := d.session.Select("count(*)").From("wallet_recharge_application").Where("uid=?", uid)
	var count int
	_, _ = countQuery.Load(&count)
	var list []*rechargeApplicationModel
	_, err := d.session.Select("*").From("wallet_recharge_application").Where("uid=?", uid).OrderDir("created_at", false).Limit(uint64(size)).Offset(uint64(offset)).Load(&list)
	return list, count, err
}

// ===== Operation Logs =====

func (d *walletDB) insertOperationLog(m *operationLogModel) error {
	_, err := d.session.InsertInto("wallet_operation_log").
		Columns("operator", "action", "target_uid", "amount", "reason", "result", "detail", "operation_desc", "ip_address", "user_agent", "target_info").
		Values(m.Operator, m.Action, m.TargetUID, m.Amount, m.Reason, m.Result, m.Detail, m.OperationDesc, m.IPAddress, m.UserAgent, m.TargetInfo).Exec()
	return err
}

func (d *walletDB) getOperationLogs(operatorUID, targetUID, operationType string, page, size int) ([]*operationLogModel, int, error) {
	offset := (page - 1) * size

	where := "1=1"
	var args []interface{}
	if operatorUID != "" {
		where += " AND operator LIKE ?"
		args = append(args, "%"+operatorUID+"%")
	}
	if targetUID != "" {
		where += " AND target_uid LIKE ?"
		args = append(args, "%"+targetUID+"%")
	}
	if operationType != "" {
		where += " AND action=?"
		args = append(args, operationType)
	}

	var count int
	countArgs := append([]interface{}{}, args...)
	_, _ = d.session.SelectBySql("SELECT COUNT(*) FROM wallet_operation_log WHERE "+where, countArgs...).Load(&count)

	cols := "id, operator, action, target_uid, amount, reason, result, COALESCE(detail,'') as detail, operation_desc, ip_address, COALESCE(user_agent,'') as user_agent, COALESCE(error_msg,'') as error_msg, COALESCE(operation_data,'') as operation_data, target_info, created_at"
	dataSQL := "SELECT " + cols + " FROM wallet_operation_log WHERE " + where + " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	dataArgs := append(append([]interface{}{}, args...), size, offset)

	var list []*operationLogModel
	_, err := d.session.SelectBySql(dataSQL, dataArgs...).Load(&list)
	return list, count, err
}

// ===== Customer Service =====

func (d *walletDB) getCustomerServiceList() ([]*customerServiceModel, error) {
	var list []*customerServiceModel
	_, err := d.session.Select("*").From("wallet_customer_service").OrderDir("created_at", false).Load(&list)
	return list, err
}

// getHotlineCustomerServicesByAppID 与「客服配置管理」一致：按应用从 hotline_agent 取启用坐席（供钱包端展示联系客服）。
func (d *walletDB) getHotlineCustomerServicesByAppID(appID string) ([]*customerServiceModel, error) {
	var list []*customerServiceModel
	if appID == "" {
		return list, nil
	}
	_, err := d.session.Select("id", "name", "uid", "created_at").From("hotline_agent").Where("app_id=? AND status=1", appID).OrderDir("created_at", false).Load(&list)
	return list, err
}

func (d *walletDB) getAllWallets(keyword string, status int, sortBy string) ([]*walletModel, error) {
	where := "1=1"
	var args []interface{}
	if keyword != "" {
		like := "%" + keyword + "%"
		where += " AND (w.uid LIKE ? OR w.phone LIKE ?)"
		args = append(args, like, like)
	}
	if status >= 0 {
		where += " AND w.status=?"
		args = append(args, status)
	}
	orderClause := " ORDER BY w.created_at DESC"
	switch sortBy {
	case "created_at_asc":
		orderClause = " ORDER BY w.created_at ASC"
	case "amount_desc":
		orderClause = " ORDER BY w.balance DESC"
	case "amount_asc":
		orderClause = " ORDER BY w.balance ASC"
	}
	sql := "SELECT w.*, COALESCE(u.name,'') as name, COALESCE(u.username,'') as username FROM wallet w LEFT JOIN `user` u ON w.uid=u.uid WHERE " + where + orderClause
	var list []*walletModel
	_, err := d.session.SelectBySql(sql, args...).Load(&list)
	return list, err
}

func (d *walletDB) getAllTransactions(keyword string) ([]*transactionModel, error) {
	query := d.session.Select("*").From("wallet_transaction")
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("uid LIKE ? OR related_id LIKE ? OR remark LIKE ?", like, like, like)
	}
	var list []*transactionModel
	_, err := query.OrderDir("created_at", false).Load(&list)
	return list, err
}

func (d *walletDB) getAllWithdrawals(keyword string, status int) ([]*withdrawalModel, error) {
	query := d.session.Select("*").From("wallet_withdrawal")
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("withdrawal_no LIKE ? OR uid LIKE ?", like, like)
	}
	if status >= 0 {
		query = query.Where("status=?", status)
	}
	var list []*withdrawalModel
	_, err := query.OrderDir("created_at", false).Load(&list)
	return list, err
}

// ===== Withdrawal fee config =====

type withdrawalConfigModel struct {
	ID       int64  `db:"id"`
	FeeRate  string `db:"fee_rate"`
	FeeFixed string `db:"fee_fixed"`
}

func (d *walletDB) getWithdrawalConfig() (*withdrawalConfigModel, error) {
	var m withdrawalConfigModel
	n, err := d.session.Select("*").From("withdrawal_config").Where("id=?", 1).Load(&m)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return &withdrawalConfigModel{ID: 1, FeeRate: "0", FeeFixed: "0"}, nil
	}
	return &m, nil
}

func (d *walletDB) updateWithdrawalConfig(feeRate, feeFixed string) error {
	_, err := d.session.Update("withdrawal_config").
		Set("fee_rate", feeRate).
		Set("fee_fixed", feeFixed).
		Where("id=?", 1).Exec()
	return err
}

// groupForbiddenAddFriend 与群模块 forbidden_add_friend 一致：1=群内禁止通过群成员来源加好友
func (d *walletDB) groupForbiddenAddFriend(groupNo string) int {
	groupNo = strings.TrimSpace(groupNo)
	if groupNo == "" {
		return 0
	}
	var n int
	cnt, err := d.session.Select("forbidden_add_friend").From("group").Where("group_no=?", groupNo).Load(&n)
	if err != nil || cnt == 0 {
		return 0
	}
	return n
}
