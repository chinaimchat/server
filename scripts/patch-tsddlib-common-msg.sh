#!/usr/bin/env bash
# 降噪 TangSengDaoDaoServerLib/common/msg.go：GetFakeChannelIDWith 在 fromUID==toUID（自会话）时
# CRC32 必然相同，原实现每条请求刷 Warn。仅在「不同 uid 但 CRC32 碰撞」时保留 Warn。
set -euo pipefail
MODROOT="${GOPATH:-/go}/pkg/mod/github.com"
MSG_GO="$(find "$MODROOT" -path '*server!lib@*/common/msg.go' 2>/dev/null | head -1)"
if [[ -z "$MSG_GO" || ! -f "$MSG_GO" ]]; then
	echo "patch-tsddlib-common-msg: common/msg.go not found under $MODROOT" >&2
	exit 1
fi
if grep -q 'fakeChannel CRC32 collision (distinct uids)' "$MSG_GO"; then
	echo "patch-tsddlib-common-msg: already patched $MSG_GO"
	exit 0
fi
perl -0777 -i -pe 's/if fromUIDHash == toUIDHash \{.*?^\t\}/if fromUIDHash == toUIDHash \&\& fromUID != toUID {\n\t\timlog.Warn("fakeChannel CRC32 collision (distinct uids)", zap.Uint32("fromUIDHash", fromUIDHash), zap.Uint32("toUIDHash", toUIDHash), zap.String("fromUID", fromUID), zap.String("toUID", toUID))\n\t}/ms' "$MSG_GO"
echo "patch-tsddlib-common-msg: patched $MSG_GO"
