package wallet

import (
	"embed"
	"time"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/register"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
)

//go:embed sql
var sqlFS embed.FS

func init() {
	register.AddModule(func(ctx interface{}) register.Module {
		w := New(ctx.(*config.Context))
		go w.startExpiredTickers()
		return register.Module{
			Name: "wallet",
			SetupAPI: func() register.APIRouter {
				return &walletRouter{w: w}
			},
			SQLDir: register.NewSQLFS(sqlFS),
		}
	})

	register.AddModule(func(ctx interface{}) register.Module {
		w := New(ctx.(*config.Context))
		return register.Module{
			Name: "wallet_manager",
			SetupAPI: func() register.APIRouter {
				return &walletManager{w: w}
			},
		}
	})
}

type walletRouter struct {
	w *Wallet
}

func (wr *walletRouter) Route(r *wkhttp.WKHttp) {
	wr.w.Route(r)
	wr.w.RouteRedpacket(r)
	wr.w.RouteTransfer(r)
}

type walletManager struct {
	w *Wallet
}

func (wm *walletManager) Route(r *wkhttp.WKHttp) {
	wm.w.RouteManager(r)
}

func (w *Wallet) startExpiredTickers() {
	rpTicker := time.NewTicker(5 * time.Minute)
	tfTicker := time.NewTicker(5 * time.Minute)
	wdTicker := time.NewTicker(15 * time.Minute)
	defer rpTicker.Stop()
	defer tfTicker.Stop()
	defer wdTicker.Stop()
	for {
		select {
		case <-rpTicker.C:
			w.service.rpProcessExpired()
		case <-tfTicker.C:
			w.service.tfProcessExpired()
		case <-wdTicker.C:
			w.service.withdrawalProcessExpired()
		}
	}
}
