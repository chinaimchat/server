package user

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"

	"github.com/gocraft/dbr/v2"
)

// stagedUIDRanges 阶段性随机号段：先 80000–90000，再 90001–100000，之后每 1 万一档直到 999999。
func stagedUIDRanges() []struct{ Min, Max int } {
	r := []struct{ Min, Max int}{
		{80000, 90000},
		{90001, 100000},
	}
	for start := 100001; start+9999 <= 999999; start += 10000 {
		r = append(r, struct{ Min, Max int }{start, start + 9999})
	}
	return r
}

func randIntInclusive(min, max int) (int, error) {
	if min > max {
		return 0, errors.New("invalid range")
	}
	span := uint64(max-min) + 1
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, err
	}
	v := binary.BigEndian.Uint64(b[:]) % span
	return min + int(v), nil
}

func (d *DB) countNumericUIDInRangeTx(tx *dbr.Tx, minVal, maxVal int) (int, error) {
	var n int
	_, err := tx.SelectBySql(
		`SELECT COUNT(*) FROM user WHERE uid REGEXP '^[0-9]+$' AND CAST(uid AS UNSIGNED) >= ? AND CAST(uid AS UNSIGNED) <= ?`,
		minVal, maxVal,
	).Load(&n)
	return n, err
}

func (d *DB) uidExistsTx(tx *dbr.Tx, uid string) (bool, error) {
	var n int
	_, err := tx.SelectBySql(`SELECT COUNT(*) FROM user WHERE uid = ?`, uid).Load(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// AllocateStagedNumericUID 在当前未满的号段内随机分配 uid 字符串；须在创建用户同一事务内调用。
func (d *DB) AllocateStagedNumericUID(tx *dbr.Tx) (string, error) {
	if tx == nil {
		return "", errors.New("AllocateStagedNumericUID: tx required")
	}
	for _, st := range stagedUIDRanges() {
		capacity := st.Max - st.Min + 1
		used, err := d.countNumericUIDInRangeTx(tx, st.Min, st.Max)
		if err != nil {
			return "", err
		}
		if used >= capacity {
			continue
		}
		const maxRandTries = 200
		for i := 0; i < maxRandTries; i++ {
			n, err := randIntInclusive(st.Min, st.Max)
			if err != nil {
				return "", err
			}
			s := strconv.Itoa(n)
			exists, err := d.uidExistsTx(tx, s)
			if err != nil {
				return "", err
			}
			if !exists {
				return s, nil
			}
		}
		for n := st.Min; n <= st.Max; n++ {
			s := strconv.Itoa(n)
			exists, err := d.uidExistsTx(tx, s)
			if err != nil {
				return "", err
			}
			if !exists {
				return s, nil
			}
		}
		return "", fmt.Errorf("号段 %d-%d 已满但统计不一致", st.Min, st.Max)
	}
	return "", errors.New("数字 uid 号段已全部用尽(80000-999999)")
}
