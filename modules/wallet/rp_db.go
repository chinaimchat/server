package wallet

import (
	"errors"
	"strings"
	"time"

	"github.com/gocraft/dbr/v2"
)

// redpacketConfigModel 与表 redpacket_config 一致，金额类字段为「分」
type redpacketConfigModel struct {
	ExpireHours            int `db:"expire_hours"`
	ExpireCheckIntervalMin int `db:"expire_check_interval_min"`
	MaxAmountPerRedpacket  int `db:"max_amount_per_redpacket"`
	MaxDailyAmountPerUser  int `db:"max_daily_amount_per_user"`
	MaxRedpacketsPerHour   int `db:"max_redpackets_per_hour"`
	MinAmountPerRedpacket  int `db:"min_amount_per_redpacket"`
	BatchExpireLimit       int `db:"batch_expire_limit"`
	EnableRiskControl      int `db:"enable_risk_control"`
	EnableAutoExpire       int `db:"enable_auto_expire"`
	RefundOnExpire         int `db:"refund_on_expire"`
}

type rpRiskRuleModel struct {
	ID          int64   `db:"id"`
	Type        string  `db:"type"`
	Threshold   float64 `db:"threshold"`
	TimeWindow  int     `db:"time_window"`
	Description string  `db:"description"`
}

type rpModel struct {
	ID              int64     `db:"id"`
	PacketNo        string    `db:"packet_no"`
	UID             string    `db:"uid"`
	ChannelID       string    `db:"channel_id"`
	ChannelType     int       `db:"channel_type"`
	Type            int       `db:"type"`
	TotalAmount     float64   `db:"total_amount"`
	TotalCount      int       `db:"total_count"`
	RemainingAmount float64   `db:"remaining_amount"`
	RemainingCount  int       `db:"remaining_count"`
	ToUID           string    `db:"to_uid"`
	Remark          string    `db:"remark"`
	Status          int       `db:"status"`
	ExpiredAt       time.Time `db:"expired_at"`
	CreatedAt       time.Time `db:"created_at"`
}

type rpRecordModel struct {
	ID        int64     `db:"id"`
	PacketNo  string    `db:"packet_no"`
	UID       string    `db:"uid"`
	Amount    float64   `db:"amount"`
	IsBest    int       `db:"is_best"`
	CreatedAt time.Time `db:"created_at"`
}

func (d *walletDB) rpInsertTx(m *rpModel, tx *dbr.Tx) error {
	_, err := tx.InsertInto("redpacket").
		Columns("packet_no", "uid", "channel_id", "channel_type", "type",
			"total_amount", "total_count", "remaining_amount", "remaining_count",
			"to_uid", "remark", "status", "expired_at").
		Values(m.PacketNo, m.UID, m.ChannelID, m.ChannelType, m.Type,
			m.TotalAmount, m.TotalCount, m.RemainingAmount, m.RemainingCount,
			m.ToUID, m.Remark, m.Status, m.ExpiredAt).Exec()
	return err
}

func (d *walletDB) rpGet(packetNo string) (*rpModel, error) {
	var m rpModel
	cnt, err := d.session.Select("*").From("redpacket").Where("packet_no=?", packetNo).Load(&m)
	if err != nil {
		return nil, err
	}
	if cnt == 0 {
		return nil, nil
	}
	return &m, nil
}

func (d *walletDB) rpDeductTx(packetNo string, amount float64, tx *dbr.Tx) (int64, error) {
	result, err := tx.UpdateBySql(
		"UPDATE redpacket SET remaining_amount = remaining_amount - ?, remaining_count = remaining_count - 1 WHERE packet_no = ? AND remaining_count > 0",
		amount, packetNo).Exec()
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (d *walletDB) rpUpdateStatusTx(packetNo string, status int, tx *dbr.Tx) error {
	_, err := tx.Update("redpacket").Set("status", status).Where("packet_no=?", packetNo).Exec()
	return err
}

func (d *walletDB) rpUpdateStatus(packetNo string, status int) error {
	_, err := d.session.Update("redpacket").Set("status", status).Where("packet_no=?", packetNo).Exec()
	return err
}

func (d *walletDB) rpInsertRecordTx(m *rpRecordModel, tx *dbr.Tx) error {
	_, err := tx.InsertInto("redpacket_record").
		Columns("packet_no", "uid", "amount", "is_best").
		Values(m.PacketNo, m.UID, m.Amount, m.IsBest).Exec()
	return err
}

// rpRecordCount 该红包已领取条数（含刚写入的记录），即本次领取的序号
func (d *walletDB) rpRecordCount(packetNo string) (int, error) {
	var n int
	_, err := d.session.Select("count(*)").From("redpacket_record").Where("packet_no=?", packetNo).Load(&n)
	return n, err
}

func (d *walletDB) rpGetRecordByUID(packetNo, uid string) (*rpRecordModel, error) {
	var m rpRecordModel
	cnt, err := d.session.Select("*").From("redpacket_record").
		Where("packet_no=? AND uid=?", packetNo, uid).Load(&m)
	if err != nil {
		return nil, err
	}
	if cnt == 0 {
		return nil, nil
	}
	return &m, nil
}

func (d *walletDB) rpGetRecords(packetNo string) ([]*rpRecordModel, error) {
	var list []*rpRecordModel
	_, err := d.session.Select("*").From("redpacket_record").
		Where("packet_no=?", packetNo).OrderDir("created_at", false).Load(&list)
	return list, err
}

func (d *walletDB) rpGetExpired() ([]*rpModel, error) {
	var list []*rpModel
	_, err := d.session.Select("*").From("redpacket").
		Where("status=0 AND expired_at < NOW()").Load(&list)
	return list, err
}

func (d *walletDB) rpUpdateRecordBest(packetNo, uid string) error {
	_, err := d.session.Update("redpacket_record").Set("is_best", 1).
		Where("packet_no=? AND uid=?", packetNo, uid).Exec()
	return err
}

// rpUserDisplayName 领取提示等场景展示用昵称（优先 name，否则 username）
func (d *walletDB) rpUserDisplayName(uid string) string {
	if uid == "" {
		return ""
	}
	var row struct {
		Name     string `db:"name"`
		Username string `db:"username"`
	}
	n, err := d.session.Select("name", "username").From("user").Where("uid=?", uid).Load(&row)
	if err != nil || n == 0 {
		return ""
	}
	if s := strings.TrimSpace(row.Name); s != "" {
		return s
	}
	return strings.TrimSpace(row.Username)
}

func (d *walletDB) getRedpacketConfig() (*redpacketConfigModel, error) {
	var c redpacketConfigModel
	n, err := d.session.Select("*").From("redpacket_config").Where("id=1").Load(&c)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, errors.New("redpacket_config missing")
	}
	return &c, nil
}

func (d *walletDB) rpListEnabledRiskRules() ([]*rpRiskRuleModel, error) {
	var list []*rpRiskRuleModel
	_, err := d.session.SelectBySql(
		"SELECT id, `type`, threshold, time_window, description FROM redpacket_risk_rule WHERE enabled=1 ORDER BY sort_order ASC, id ASC",
	).Load(&list)
	return list, err
}

func (d *walletDB) rpInsertRiskEvent(uid, eventType, riskLevel, remark string) error {
	_, err := d.session.InsertInto("redpacket_risk_event").
		Columns("uid", "event_type", "risk_level", "remark", "status").
		Values(uid, eventType, riskLevel, remark, "pending").Exec()
	return err
}

func (d *walletDB) rpCountUserPacketsSince(uid string, windowSec int) (int, error) {
	if windowSec <= 0 {
		windowSec = 3600
	}
	var n int
	_, err := d.session.SelectBySql(
		"SELECT COUNT(*) FROM redpacket WHERE uid=? AND created_at >= DATE_SUB(NOW(), INTERVAL ? SECOND)",
		uid, windowSec,
	).Load(&n)
	return n, err
}

func (d *walletDB) rpUserTodaySendTotal(uid string) (float64, error) {
	var sum float64
	_, err := d.session.SelectBySql(
		"SELECT COALESCE(SUM(total_amount),0) FROM redpacket WHERE uid=? AND DATE(created_at)=CURDATE()",
		uid,
	).Load(&sum)
	return sum, err
}

func (d *walletDB) rpCountUserPacketsLastHour(uid string) (int, error) {
	var n int
	_, err := d.session.SelectBySql(
		"SELECT COUNT(*) FROM redpacket WHERE uid=? AND created_at >= DATE_SUB(NOW(), INTERVAL 1 HOUR)",
		uid,
	).Load(&n)
	return n, err
}

func (d *walletDB) rpCountUserReceivesSince(uid string, windowSec int) (int, error) {
	if windowSec <= 0 {
		windowSec = 3600
	}
	var n int
	_, err := d.session.SelectBySql(
		"SELECT COUNT(*) FROM redpacket_record WHERE uid=? AND created_at >= DATE_SUB(NOW(), INTERVAL ? SECOND)",
		uid, windowSec,
	).Load(&n)
	return n, err
}

// rpCountUserNightSendsSince 凌晨 2～6 点发出的红包数（配合 time_pattern 规则）
func (d *walletDB) rpCountUserNightSendsSince(uid string, windowSec int) (int, error) {
	if windowSec <= 0 {
		windowSec = 86400
	}
	var n int
	_, err := d.session.SelectBySql(
		`SELECT COUNT(*) FROM redpacket WHERE uid=? AND created_at >= DATE_SUB(NOW(), INTERVAL ? SECOND)
		 AND HOUR(created_at) >= 2 AND HOUR(created_at) < 6`,
		uid, windowSec,
	).Load(&n)
	return n, err
}
