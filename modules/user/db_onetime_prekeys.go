package user

import (
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/gocraft/dbr/v2"
)

type onetimePrekeysDB struct {
	session *dbr.Session
	ctx     *config.Context
}

func newOnetimePrekeysDB(ctx *config.Context) *onetimePrekeysDB {
	return &onetimePrekeysDB{
		session: ctx.DB(),
		ctx:     ctx,
	}
}

func (o *onetimePrekeysDB) queryCount(uid string) (int, error) {
	var cn int
	err := o.session.Select("count(*)").From("signal_onetime_prekeys").Where("uid=?", uid).LoadOne(&cn)
	return cn, err
}
