package hotline

import (
	"time"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/gocraft/dbr/v2"
)

type onlineDB struct {
	ctx *config.Context
	db  *dbr.Session
}

func newOnlineDB(ctx *config.Context) *onlineDB {
	return &onlineDB{
		ctx: ctx,
		db:  ctx.DB(),
	}
}

func (o *onlineDB) updateOrAddOnline(userType int, deviceFlag uint8, userID string) error {
	_, err := o.db.InsertBySql("INSERT INTO hotline_online (user_id,user_type,online,last_online,device_flag) VALUES (?,?,?,?,?) ON DUPLICATE KEY UPDATE online=VALUES(online),last_online=VALUES(last_online)", userID, userType, 1, time.Now().Unix(), deviceFlag).Exec()
	return err
}

func (o *onlineDB) updateOrAddOffline(userType int, deviceFlag uint8, userID string) error {
	_, err := o.db.InsertBySql("INSERT INTO hotline_online (user_id,user_type,online,last_offline,device_flag) VALUES (?,?,?,?,?) ON DUPLICATE KEY UPDATE online=VALUES(online),last_offline=VALUES(last_offline)", userID, userType, 0, time.Now().Unix(), deviceFlag).Exec()
	return err
}

func (o *onlineDB) queryWithUserIDAndDeviceFlag(userID string, deviceFlag uint8) (*onlineModel, error) {
	var m *onlineModel
	_, err := o.db.Select("*").From("hotline_online").Where("user_id=? and device_flag=?", userID, deviceFlag).Load(&m)
	return m, err
}

type onlineModel struct {
	AppID       string `json:"app_id"`
	UserType    int    `json:"user_type"`
	UserID      string `json:"user_id"`
	Online      int    `json:"online"`
	LastOffline int    `json:"last_offline"`
	LastOnline  int    `json:"last_online"`
}
