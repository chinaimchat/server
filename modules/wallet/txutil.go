package wallet

// RechargePayTypeName 与支付配置管理里 pay_type 选项一致（2 支付宝 3 微信 4 USDT-TRC20）
func RechargePayTypeName(payType int) string {
	switch payType {
	case 2:
		return "支付宝"
	case 3:
		return "微信"
	case 4:
		return "USDT-TRC20"
	default:
		return "其它"
	}
}

// TxTypeTitle 交易类型中文说明（与后台交易记录展示一致，供 C 端 / 管理端共用）
func TxTypeTitle(t string) string {
	m := map[string]string{
		"recharge":            "充值",
		"admin_recharge":      "管理员充值",
		"admin_adjust":        "管理员调整",
		"redpacket_send":      "红包发出",
		"redpacket_receive":   "红包收入",
		"transfer_out":        "转账发出",
		"transfer_in":         "转账收入",
		"transfer_refund":     "转账退回",
		"refund":              "退款",
		"withdrawal":          "提现",
		"withdrawal_freeze":   "提现冻结",
		"withdrawal_refund":   "提现退款",
		"fee":                 "手续费",
	}
	if v, ok := m[t]; ok {
		return v
	}
	return t
}
