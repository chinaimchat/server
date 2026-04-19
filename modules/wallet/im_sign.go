package wallet

import (
	"fmt"
	"os"
	"strings"

	"github.com/TangSengDaoDao/TangSengDaoDaoServer/pkg/util"
)

func walletIMSignSecret() string {
	secret := strings.TrimSpace(os.Getenv("WK_JWT_SECRET"))
	if secret == "" {
		secret = "wallet-im-sign-fallback"
	}
	return secret
}

func signRedpacketIM(packetNo string, packetType int, remark, fromUID, channelID string, channelType int, ts int64) string {
	raw := fmt.Sprintf("rp|%s|%d|%s|%s|%s|%d|%d",
		strings.TrimSpace(packetNo),
		packetType,
		strings.TrimSpace(remark),
		strings.TrimSpace(fromUID),
		strings.TrimSpace(channelID),
		channelType,
		ts,
	)
	return util.HmacSha256(raw, walletIMSignSecret())
}

func signTransferIM(transferNo string, amount float64, remark, fromUID, toUID, channelID string, channelType int, ts int64) string {
	raw := fmt.Sprintf("tf|%s|%.2f|%s|%s|%s|%s|%d|%d",
		strings.TrimSpace(transferNo),
		amount,
		strings.TrimSpace(remark),
		strings.TrimSpace(fromUID),
		strings.TrimSpace(toUID),
		strings.TrimSpace(channelID),
		channelType,
		ts,
	)
	return util.HmacSha256(raw, walletIMSignSecret())
}
