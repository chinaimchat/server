package wallet

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gocraft/dbr/v2"
)

// transferConfigParsed 与 transfer_config 表一致，金额类为「分」
type transferConfigParsed struct {
	DailyLimitFen    int64
	ExpireHours      int
	MaxAmountFen     int64
	MinAmountFen     int64
	RiskThresholdFen int64
}

func parseTransferConfigInt64(s string, def int64) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return def
	}
	return v
}

func parseTransferConfigExpireHours(s string, def int) int {
	v := parseTransferConfigInt64(s, int64(def))
	if v < 1 || v > 8760 {
		return def
	}
	return int(v)
}

func (d *walletDB) getTransferConfigParsed() (*transferConfigParsed, error) {
	type row struct {
		DailyLimit    string `db:"daily_limit"`
		ExpireHours   string `db:"expire_hours"`
		MaxAmount     string `db:"max_amount"`
		MinAmount     string `db:"min_amount"`
		RiskThreshold string `db:"risk_threshold"`
	}
	var r row
	n, err := d.session.Select("daily_limit", "expire_hours", "max_amount", "min_amount", "risk_threshold").From("transfer_config").Where("id=1").Load(&r)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, errors.New("transfer_config missing")
	}
	return &transferConfigParsed{
		DailyLimitFen:    parseTransferConfigInt64(r.DailyLimit, 1000000),
		ExpireHours:      parseTransferConfigExpireHours(r.ExpireHours, 24),
		MaxAmountFen:     parseTransferConfigInt64(r.MaxAmount, 500000),
		MinAmountFen:     parseTransferConfigInt64(r.MinAmount, 100),
		RiskThresholdFen: parseTransferConfigInt64(r.RiskThreshold, 100000),
	}, nil
}

func (d *walletDB) tfUserTodaySendTotal(uid string) (float64, error) {
	var sum float64
	_, err := d.session.SelectBySql(
		"SELECT COALESCE(SUM(amount),0) FROM `transfer` WHERE from_uid=? AND DATE(created_at)=CURDATE()",
		uid,
	).Load(&sum)
	return sum, err
}

type tfModel struct {
	ID          int64     `db:"id"`
	TransferNo  string    `db:"transfer_no"`
	FromUID     string    `db:"from_uid"`
	ToUID       string    `db:"to_uid"`
	ChannelID   string    `db:"channel_id"`
	ChannelType int       `db:"channel_type"`
	Amount      float64   `db:"amount"`
	Remark      string    `db:"remark"`
	Status      int       `db:"status"`
	ExpiredAt   time.Time `db:"expired_at"`
	CreatedAt   time.Time `db:"created_at"`
}

func (d *walletDB) tfInsertTx(m *tfModel, tx *dbr.Tx) error {
	_, err := tx.InsertInto("transfer").
		Columns("transfer_no", "from_uid", "to_uid", "channel_id", "channel_type", "amount", "remark", "status", "expired_at").
		Values(m.TransferNo, m.FromUID, m.ToUID, m.ChannelID, m.ChannelType, m.Amount, m.Remark, m.Status, m.ExpiredAt).Exec()
	return err
}

func (d *walletDB) tfGet(transferNo string) (*tfModel, error) {
	var m tfModel
	cnt, err := d.session.Select("*").From("transfer").Where("transfer_no=?", transferNo).Load(&m)
	if err != nil {
		return nil, err
	}
	if cnt == 0 {
		return nil, nil
	}
	return &m, nil
}

func (d *walletDB) tfUpdateStatus(transferNo string, status int) error {
	_, err := d.session.Update("transfer").Set("status", status).Where("transfer_no=?", transferNo).Exec()
	return err
}

func (d *walletDB) tfUpdateStatusTx(transferNo string, status int, tx *dbr.Tx) error {
	_, err := tx.Update("transfer").Set("status", status).Where("transfer_no=?", transferNo).Exec()
	return err
}

func (d *walletDB) tfGetExpired() ([]*tfModel, error) {
	var list []*tfModel
	_, err := d.session.Select("*").From("transfer").
		Where("status=0 AND expired_at < NOW()").Load(&list)
	return list, err
}
