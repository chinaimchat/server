package wallet

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func (w *Wallet) RouteManager(r *wkhttp.WKHttp) {
	auth := r.Group("/v1/manager/wallet", w.ctx.AuthMiddleware(r))
	{
		auth.GET("/statistics", w.managerStatistics)
		auth.GET("/list", w.managerWalletList)
		auth.POST("/balance/adjust", w.managerBalanceAdjust)
		auth.POST("/balance/freeze", w.managerFreeze)
		auth.POST("/balance/unfreeze", w.managerUnfreeze)
		auth.GET("/records", w.managerRecords)
		auth.POST("/password/reset", w.managerPasswordReset)
		auth.POST("/sync-userinfo", w.managerSyncUserInfo)
		auth.GET("/withdrawal/list", w.managerWithdrawalList)
		auth.GET("/withdrawal/config", w.managerWithdrawalConfigGet)
		auth.POST("/withdrawal/config", w.managerWithdrawalConfigPost)
		auth.POST("/withdrawal/approve", w.managerWithdrawalApprove)
		auth.POST("/withdrawal/reject", w.managerWithdrawalReject)
		auth.GET("/recharge/applications", w.managerRechargeApplicationList)
		auth.POST("/recharge/application/approve", w.managerRechargeApplicationApprove)
		auth.POST("/recharge/application/reject", w.managerRechargeApplicationReject)
		auth.GET("/operation-logs", w.managerOperationLogs)
		auth.POST("/recharge", w.managerRecharge)
		auth.GET("/transactions", w.managerTransactions)
		auth.GET("/export/wallets", w.managerExportWallets)
		auth.GET("/export/records", w.managerExportRecords)
		auth.GET("/export/withdrawals", w.managerExportWithdrawals)
	}

	// 管理端红包：路径挂在 /v1/manager/wallet/redpacket（不可用 auth.Group 嵌套，否则会退化为 gin.Group 导致 Handler 类型不匹配）
	rpAuth := r.Group("/v1/manager/wallet/redpacket", w.ctx.AuthMiddleware(r))
	{
		rpAuth.GET("/statistics", w.redpacketStatistics)
		rpAuth.GET("/list", w.redpacketList)
		rpAuth.GET("/detail/:packet_no", w.redpacketDetail)
		rpAuth.PUT("/status/:id", w.redpacketStatusUpdate)
		rpAuth.GET("/records", w.redpacketRecords)
		rpAuth.GET("/config", w.redpacketConfig)
		rpAuth.POST("/config", w.updateRedpacketConfig)
		rpAuth.PUT("/config", w.updateRedpacketConfig)
		rpAuth.GET("/analytics/type", w.analyticsType)
		rpAuth.GET("/analytics/scene", w.analyticsScene)
		rpAuth.GET("/analytics/trend", w.analyticsTrend)
		rpAuth.GET("/analytics/summary", w.analyticsSummary)
		rpAuth.GET("/export/redpackets", w.exportRedpackets)
		rpAuth.GET("/export/analytics", w.exportAnalytics)
		rpAuth.GET("/risk/monitor", w.riskMonitor)
		rpAuth.POST("/risk/action", w.riskAction)
		rpAuth.POST("/risk/batch_action", w.riskBatchAction)
		rpAuth.GET("/risk/rules", w.riskRulesList)
		rpAuth.POST("/risk/rules", w.riskRuleCreate)
		rpAuth.PUT("/risk/rules/:id", w.riskRuleUpdate)
		rpAuth.DELETE("/risk/rules/:id", w.riskRuleDelete)
		rpAuth.PUT("/risk/rules/:id/toggle", w.riskRuleToggle)
	}

	tfAuth := r.Group("/v1/manager/transfer", w.ctx.AuthMiddleware(r))
	{
		tfAuth.GET("/statistics", w.transferStatistics)
		tfAuth.GET("/list", w.transferList)
		tfAuth.GET("/detail/:transfer_no", w.transferDetail)
		tfAuth.PUT("/status/:transfer_no", w.transferStatusUpdate)
		tfAuth.POST("/refund/:transfer_no", w.transferRefund)
		tfAuth.GET("/analytics/user", w.transferAnalyticsUser)
		tfAuth.GET("/analytics/trend", w.transferAnalyticsTrend)
		tfAuth.GET("/analytics/amount", w.transferAnalyticsAmount)
		tfAuth.GET("/analytics/summary", w.transferAnalyticsSummary)
		tfAuth.GET("/risk/monitor", w.transferRiskMonitor)
		tfAuth.POST("/risk/action", w.transferRiskAction)
		tfAuth.GET("/risk/alerts", w.transferRiskAlerts)
		tfAuth.GET("/config", w.transferConfig)
		tfAuth.POST("/config", w.updateTransferConfig)
		tfAuth.PUT("/config", w.updateTransferConfig)
		tfAuth.GET("/export/transfers", w.transferExport)
		tfAuth.GET("/export/analytics", w.transferExportAnalytics)
	}
}

func (w *Wallet) managerStatistics(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	stats, err := w.service.db.getStatistics()
	if err != nil {
		c.ResponseError(err)
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (w *Wallet) managerWalletList(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", c.DefaultQuery("page_index", "1")))
	size, _ := strconv.Atoi(c.DefaultQuery("size", c.DefaultQuery("page_size", "20")))
	keyword := c.DefaultQuery("keyword", "")
	sortBy := c.DefaultQuery("sort_by", "created_at_desc")
	statusStr := c.DefaultQuery("status", "-1")
	status, _ := strconv.Atoi(statusStr)
	minAmountStr := c.DefaultQuery("min_amount", "0")
	maxAmountStr := c.DefaultQuery("max_amount", "0")
	minAmount, _ := strconv.ParseFloat(minAmountStr, 64)
	maxAmount, _ := strconv.ParseFloat(maxAmountStr, 64)
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}

	list, count, err := w.service.db.getWalletList(keyword, status, sortBy, minAmount, maxAmount, page, size)
	if err != nil {
		c.ResponseError(err)
		return
	}

	respList := make([]gin.H, 0, len(list))
	for _, item := range list {
		respList = append(respList, gin.H{
			"uid":        item.UID,
			"balance":    item.Balance,
			"amount":     item.Balance,
			"status":     item.Status,
			"phone":      item.Phone,
			"zone":       item.Zone,
			"name":       item.Name,
			"username":   item.Username,
			"created_at": item.CreatedAt.Format("2006-01-02 15:04:05"),
			"updated_at": item.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"list":  respList,
		"total": count,
		"page":  page,
		"size":  size,
	})
}

func (w *Wallet) managerBalanceAdjust(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	var req struct {
		UID    string  `json:"uid"`
		Amount float64 `json:"amount"`
		Reason string  `json:"reason"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(err)
		return
	}
	if req.Amount == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "msg": "金额不能为0"})
		return
	}

	operator := c.GetLoginUID()
	action := "balance_adjust"
	if req.Amount > 0 {
		err = w.service.AddBalance(req.UID, req.Amount, "admin_adjust", "", req.Reason)
	} else {
		err = w.service.DeductBalance(req.UID, -req.Amount, "admin_adjust", "", req.Reason)
	}
	result := "success"
	if err != nil {
		result = "failed"
	}
	_ = w.service.db.insertOperationLog(&operationLogModel{
		Operator:      operator,
		Action:        action,
		TargetUID:     req.UID,
		Amount:        req.Amount,
		Reason:        req.Reason,
		Result:        result,
		OperationDesc: fmt.Sprintf("调整用户 %s 余额 %.2f，原因: %s", req.UID, req.Amount, req.Reason),
		IPAddress:     c.ClientIP(),
		TargetInfo:    req.UID,
	})
	if err != nil {
		c.ResponseError(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

func (w *Wallet) managerFreeze(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	var req struct {
		UID    string `json:"uid"`
		Reason string `json:"reason"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(err)
		return
	}
	if req.Reason == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "msg": "冻结操作必须填写原因"})
		return
	}
	err = w.service.db.updateWalletStatus(req.UID, 2)
	operator := c.GetLoginUID()
	_ = w.service.db.insertOperationLog(&operationLogModel{
		Operator:      operator,
		Action:        "wallet_freeze",
		TargetUID:     req.UID,
		Reason:        req.Reason,
		Result:        "success",
		OperationDesc: fmt.Sprintf("冻结用户 %s 的钱包，原因: %s", req.UID, req.Reason),
		IPAddress:     c.ClientIP(),
		TargetInfo:    req.UID,
	})
	if err != nil {
		c.ResponseError(err)
		return
	}
	w.service.scheduleWalletBalanceIMNotify(req.UID, "wallet_status", "")
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

func (w *Wallet) managerUnfreeze(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	var req struct {
		UID    string `json:"uid"`
		Reason string `json:"reason"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(err)
		return
	}
	err = w.service.db.updateWalletStatus(req.UID, 1)
	operator := c.GetLoginUID()
	_ = w.service.db.insertOperationLog(&operationLogModel{
		Operator:      operator,
		Action:        "wallet_unfreeze",
		TargetUID:     req.UID,
		Reason:        req.Reason,
		Result:        "success",
		OperationDesc: fmt.Sprintf("解冻用户 %s 的钱包", req.UID),
		IPAddress:     c.ClientIP(),
		TargetInfo:    req.UID,
	})
	if err != nil {
		c.ResponseError(err)
		return
	}
	w.service.scheduleWalletBalanceIMNotify(req.UID, "wallet_status", "")
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

func (w *Wallet) managerRecords(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	keyword := c.DefaultQuery("keyword", c.DefaultQuery("uid", ""))
	page, _ := strconv.Atoi(c.DefaultQuery("page", c.DefaultQuery("page_index", "1")))
	size, _ := strconv.Atoi(c.DefaultQuery("size", c.DefaultQuery("page_size", "20")))

	list, count, err := w.service.db.getTransactionsAdmin(keyword, page, size)
	if err != nil {
		c.ResponseError(err)
		return
	}

	respList := make([]gin.H, 0, len(list))
	for _, item := range list {
		respList = append(respList, gin.H{
			"id":             item.ID,
			"uid":            item.UID,
			"type":           item.Type,
			"title":          w.getTxTypeTitle(item.Type),
			"amount":         item.Amount,
			"changed_amount": item.Amount,
			"balance":        item.BalanceAfter,
			"related_id":     item.RelatedID,
			"remark":         item.Remark,
			"created_at":     item.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"list":  respList,
		"total": count,
		"page":  page,
		"size":  size,
	})
}

func (w *Wallet) getTxTypeTitle(t string) string {
	return TxTypeTitle(t)
}

func (w *Wallet) managerPasswordReset(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	var req struct {
		UID    string `json:"uid"`
		Reason string `json:"reason"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(err)
		return
	}
	err = w.service.db.resetPayPassword(req.UID)
	operator := c.GetLoginUID()
	_ = w.service.db.insertOperationLog(&operationLogModel{
		Operator:      operator,
		Action:        "password_reset",
		TargetUID:     req.UID,
		Reason:        req.Reason,
		Result:        "success",
		OperationDesc: fmt.Sprintf("重置用户 %s 的支付密码", req.UID),
		IPAddress:     c.ClientIP(),
		TargetInfo:    req.UID,
	})
	if err != nil {
		c.ResponseError(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

func (w *Wallet) managerSyncUserInfo(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}

	list, _, err := w.service.db.getWalletList("", -1, "", 0, 0, 1, 10000)
	if err != nil {
		c.ResponseError(err)
		return
	}
	synced := 0
	for _, item := range list {
		var phone, zone string
		_, _ = w.service.db.session.SelectBySql("SELECT COALESCE(phone,'') as phone, COALESCE(zone,'') as zone FROM user WHERE uid=?", item.UID).Load(&struct {
			Phone string `db:"phone"`
			Zone  string `db:"zone"`
		}{})
		row := struct {
			Phone string `db:"phone"`
			Zone  string `db:"zone"`
		}{}
		_, _ = w.service.db.session.Select("phone", "zone").From("user").Where("uid=?", item.UID).Load(&row)
		phone = row.Phone
		zone = row.Zone
		if phone != "" || zone != "" {
			_ = w.service.db.syncUserInfo(item.UID, phone, zone)
			synced++
		}
	}
	_ = w.service.db.insertOperationLog(&operationLogModel{
		Operator:      c.GetLoginUID(),
		Action:        "user_sync",
		OperationDesc: fmt.Sprintf("同步用户信息，共 %d 条", synced),
		Result:        "success",
		IPAddress:     c.ClientIP(),
	})
	c.JSON(http.StatusOK, gin.H{"status": 200, "synced": synced})
}

func (w *Wallet) managerWithdrawalConfigGet(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	cfg, err := w.service.db.getWithdrawalConfig()
	if err != nil {
		c.ResponseError(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":    200,
		"fee_rate":  cfg.FeeRate,
		"fee_fixed": cfg.FeeFixed,
	})
}

func (w *Wallet) managerWithdrawalConfigPost(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	var req struct {
		FeeRate  string `json:"fee_rate"`
		FeeFixed string `json:"fee_fixed"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(err)
		return
	}
	rateStr := strings.TrimSpace(req.FeeRate)
	fixedStr := strings.TrimSpace(req.FeeFixed)
	if rateStr == "" {
		rateStr = "0"
	}
	if fixedStr == "" {
		fixedStr = "0"
	}
	rate, err := strconv.ParseFloat(rateStr, 64)
	if err != nil || math.IsNaN(rate) || math.IsInf(rate, 0) || rate < 0 || rate > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "msg": "手续费比例需在 0～100 之间（表示占提现金额的百分比）"})
		return
	}
	fixed, err := strconv.ParseFloat(fixedStr, 64)
	if err != nil || math.IsNaN(fixed) || math.IsInf(fixed, 0) || fixed < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "msg": "固定手续费不能为负数"})
		return
	}
	rateStr = fmt.Sprintf("%.4g", rate)
	fixedStr = fmt.Sprintf("%.2f", fixed)
	if err := w.service.db.updateWithdrawalConfig(rateStr, fixedStr); err != nil {
		c.ResponseError(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

func (w *Wallet) managerWithdrawalList(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	keyword := c.DefaultQuery("keyword", "")
	statusStr := c.DefaultQuery("status", "-1")
	status, _ := strconv.Atoi(statusStr)

	list, count, err := w.service.db.getWithdrawalList(keyword, status, page, size)
	if err != nil {
		c.ResponseError(err)
		return
	}
	respList := make([]gin.H, 0, len(list))
	for _, item := range list {
		respList = append(respList, gin.H{
			"id":            item.ID,
			"withdrawal_no": item.WithdrawalNo,
			"uid":           item.UID,
			"amount":        item.Amount,
			"fee":           item.Fee,
			"actual_amount": item.Amount - item.Fee,
			"status":        item.Status,
			"status_text":   withdrawalStatusLabel(item.Status),
			"address":       item.Address,
			"remark":        item.Remark,
			"admin_remark":  item.AdminRemark,
			"created_at":    item.CreatedAt.Format("2006-01-02 15:04:05"),
			"updated_at":    item.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"list":  respList,
		"total": count,
		"page":  page,
		"size":  size,
	})
}

func (w *Wallet) managerWithdrawalApprove(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	var req struct {
		ID     int64  `json:"id"`
		Remark string `json:"remark"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(err)
		return
	}
	wd, err := w.service.db.getWithdrawal(req.ID)
	if err != nil || wd == nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "msg": "提现记录不存在"})
		return
	}
	if wd.Status != 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "msg": "该提现已处理，无法重复审核"})
		return
	}
	err = w.service.ApproveWithdrawal(req.ID, req.Remark)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "msg": err.Error()})
		return
	}
	operator := c.GetLoginUID()
	_ = w.service.db.insertOperationLog(&operationLogModel{
		Operator:      operator,
		Action:        "withdrawal_approve",
		TargetUID:     wd.UID,
		Amount:        wd.Amount,
		Reason:        req.Remark,
		Result:        "success",
		Detail:        fmt.Sprintf("withdrawal_no: %s", wd.WithdrawalNo),
		OperationDesc: fmt.Sprintf("批准提现 %s (¥%.2f)", wd.WithdrawalNo, wd.Amount),
		IPAddress:     c.ClientIP(),
		TargetInfo:    wd.UID,
	})
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

func (w *Wallet) managerWithdrawalReject(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	var req struct {
		ID     int64  `json:"id"`
		Remark string `json:"remark"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(err)
		return
	}
	if req.Remark == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "msg": "请输入拒绝原因"})
		return
	}
	wd, err := w.service.db.getWithdrawal(req.ID)
	if err != nil || wd == nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "msg": "提现记录不存在"})
		return
	}
	if wd.Status != 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "msg": "该提现已处理，无法重复审核"})
		return
	}
	err = w.service.RejectWithdrawal(req.ID, req.Remark)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "msg": err.Error()})
		return
	}
	operator := c.GetLoginUID()
	_ = w.service.db.insertOperationLog(&operationLogModel{
		Operator:      operator,
		Action:        "withdrawal_reject",
		TargetUID:     wd.UID,
		Amount:        wd.Amount,
		Reason:        req.Remark,
		Result:        "success",
		Detail:        fmt.Sprintf("withdrawal_no: %s", wd.WithdrawalNo),
		OperationDesc: fmt.Sprintf("拒绝提现 %s (¥%.2f)，原因: %s", wd.WithdrawalNo, wd.Amount, req.Remark),
		IPAddress:     c.ClientIP(),
		TargetInfo:    wd.UID,
	})
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

func (w *Wallet) managerRechargeApplicationList(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	keyword := c.DefaultQuery("keyword", "")
	statusStr := c.DefaultQuery("status", "-1")
	status, _ := strconv.Atoi(statusStr)

	list, count, err := w.service.db.getRechargeApplicationList(keyword, status, page, size)
	if err != nil {
		c.ResponseError(err)
		return
	}
	respList := make([]gin.H, 0, len(list))
	for _, item := range list {
		respList = append(respList, gin.H{
			"id":              item.ID,
			"application_no":  item.ApplicationNo,
			"uid":             item.UID,
			"amount":          item.Amount,
			"amount_u":        item.AmountU,
			"exchange_rate":   item.ExchangeRate,
			"channel_id":      item.ChannelID,
			"pay_type":        item.PayType,
			"pay_type_name":   RechargePayTypeName(item.PayType),
			"remark":          item.Remark,
			"proof_url":       item.ProofURL,
			"status":          item.Status,
			"status_text":     rechargeApplicationStatusLabel(item.Status),
			"admin_remark":    item.AdminRemark,
			"reviewer_uid":    item.ReviewerUID,
			"created_at":      item.CreatedAt.Format("2006-01-02 15:04:05"),
			"updated_at":      item.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"list":  respList,
		"total": count,
		"page":  page,
		"size":  size,
	})
}

func (w *Wallet) managerRechargeApplicationApprove(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	var req struct {
		ID     int64  `json:"id"`
		Remark string `json:"remark"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(err)
		return
	}
	app, err := w.service.db.getRechargeApplication(req.ID)
	if err != nil || app == nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "msg": "申请记录不存在"})
		return
	}
	err = w.service.ApproveRechargeApplication(req.ID, c.GetLoginUID(), req.Remark)
	operator := c.GetLoginUID()
	logResult := "success"
	if err != nil {
		logResult = "fail"
	}
	_ = w.service.db.insertOperationLog(&operationLogModel{
		Operator:      operator,
		Action:        "recharge_apply_approve",
		TargetUID:     app.UID,
		Amount:        app.Amount,
		Reason:        req.Remark,
		Result:        logResult,
		Detail:        fmt.Sprintf("application_no: %s", app.ApplicationNo),
		OperationDesc: fmt.Sprintf("通过充值申请 %s (¥%.2f)", app.ApplicationNo, app.Amount),
		IPAddress:     c.ClientIP(),
		TargetInfo:    app.UID,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

func (w *Wallet) managerRechargeApplicationReject(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	var req struct {
		ID     int64  `json:"id"`
		Remark string `json:"remark"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(err)
		return
	}
	if req.Remark == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "msg": "请输入拒绝原因"})
		return
	}
	app, err := w.service.db.getRechargeApplication(req.ID)
	if err != nil || app == nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "msg": "申请记录不存在"})
		return
	}
	err = w.service.RejectRechargeApplication(req.ID, c.GetLoginUID(), req.Remark)
	operator := c.GetLoginUID()
	logResult := "success"
	if err != nil {
		logResult = "fail"
	}
	_ = w.service.db.insertOperationLog(&operationLogModel{
		Operator:      operator,
		Action:        "recharge_apply_reject",
		TargetUID:     app.UID,
		Amount:        app.Amount,
		Reason:        req.Remark,
		Result:        logResult,
		Detail:        fmt.Sprintf("application_no: %s", app.ApplicationNo),
		OperationDesc: fmt.Sprintf("拒绝充值申请 %s (¥%.2f)，原因: %s", app.ApplicationNo, app.Amount, req.Remark),
		IPAddress:     c.ClientIP(),
		TargetInfo:    app.UID,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

func (w *Wallet) managerOperationLogs(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	operatorUID := c.DefaultQuery("operator_uid", c.DefaultQuery("operator", ""))
	targetUID := c.DefaultQuery("target_uid", "")
	operationType := c.DefaultQuery("operation_type", c.DefaultQuery("action", ""))

	list, count, err := w.service.db.getOperationLogs(operatorUID, targetUID, operationType, page, size)
	if err != nil {
		c.ResponseError(err)
		return
	}
	respList := make([]gin.H, 0, len(list))
	for _, item := range list {
		opDesc := item.OperationDesc
		if opDesc == "" {
			opDesc = item.Reason
		}
		targetInfo := item.TargetInfo
		if targetInfo == "" {
			targetInfo = item.TargetUID
		}
		respList = append(respList, gin.H{
			"id":              item.ID,
			"operator":        item.Operator,
			"operator_name":   item.Operator,
			"action":          item.Action,
			"operation_type":  item.Action,
			"target_uid":      item.TargetUID,
			"target_info":     targetInfo,
			"amount":          item.Amount,
			"reason":          item.Reason,
			"operation_desc":  opDesc,
			"result":          item.Result,
			"detail":          item.Detail,
			"ip_address":      item.IPAddress,
			"user_agent":      item.UserAgent,
			"error_msg":       item.ErrorMsg,
			"operation_data":  item.OperationData,
			"created_at":      item.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"list":  respList,
		"total": count,
		"page":  page,
		"size":  size,
	})
}

func (w *Wallet) managerRecharge(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	var req struct {
		UID    string  `json:"uid"`
		Amount float64 `json:"amount"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(err)
		return
	}
	if req.Amount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "msg": "金额必须大于0"})
		return
	}
	err = w.service.AddBalance(req.UID, req.Amount, "admin_recharge", "", "管理员充值")
	operator := c.GetLoginUID()
	_ = w.service.db.insertOperationLog(&operationLogModel{
		Operator:      operator,
		Action:        "admin_recharge",
		TargetUID:     req.UID,
		Amount:        req.Amount,
		Reason:        "管理员充值",
		Result:        "success",
		OperationDesc: fmt.Sprintf("为用户 %s 充值 ¥%.2f", req.UID, req.Amount),
		IPAddress:     c.ClientIP(),
		TargetInfo:    req.UID,
	})
	if err != nil {
		c.ResponseError(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

func (w *Wallet) managerTransactions(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	keyword := c.DefaultQuery("keyword", c.DefaultQuery("uid", ""))
	page, _ := strconv.Atoi(c.DefaultQuery("page_index", c.DefaultQuery("page", "1")))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", c.DefaultQuery("size", "20")))

	list, count, err := w.service.db.getTransactionsAdmin(keyword, page, size)
	if err != nil {
		c.ResponseError(err)
		return
	}

	respList := make([]gin.H, 0, len(list))
	for _, item := range list {
		respList = append(respList, gin.H{
			"id":             item.ID,
			"uid":            item.UID,
			"type":           item.Type,
			"title":          w.getTxTypeTitle(item.Type),
			"amount":         item.Amount,
			"changed_amount": item.Amount,
			"balance":        item.BalanceAfter,
			"related_id":     item.RelatedID,
			"remark":         item.Remark,
			"created_at":     item.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"list":  respList,
		"count": count,
		"total": count,
	})
}

func (w *Wallet) managerExportWallets(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	keyword := c.DefaultQuery("keyword", "")
	statusStr := c.DefaultQuery("status", "-1")
	status, _ := strconv.Atoi(statusStr)
	sortBy := c.DefaultQuery("sort_by", "created_at_desc")

	list, err := w.service.db.getAllWallets(keyword, status, sortBy)
	if err != nil {
		c.ResponseError(err)
		return
	}

	buf := &bytes.Buffer{}
	buf.WriteString("\xEF\xBB\xBF")
	writer := csv.NewWriter(buf)
	_ = writer.Write([]string{"用户ID", "余额", "状态", "手机号", "创建时间", "更新时间"})
	for _, item := range list {
		statusText := "正常"
		if item.Status != 1 {
			statusText = "冻结"
		}
		_ = writer.Write([]string{
			item.UID,
			fmt.Sprintf("%.2f", item.Balance),
			statusText,
			item.Phone,
			item.CreatedAt.Format("2006-01-02 15:04:05"),
			item.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	writer.Flush()

	_ = w.service.db.insertOperationLog(&operationLogModel{
		Operator:      c.GetLoginUID(),
		Action:        "data_export",
		OperationDesc: fmt.Sprintf("导出钱包列表，共 %d 条", len(list)),
		Result:        "success",
		IPAddress:     c.ClientIP(),
	})

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=wallets.csv")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

func (w *Wallet) managerExportRecords(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	keyword := c.DefaultQuery("keyword", "")

	list, err := w.service.db.getAllTransactions(keyword)
	if err != nil {
		c.ResponseError(err)
		return
	}

	buf := &bytes.Buffer{}
	buf.WriteString("\xEF\xBB\xBF")
	writer := csv.NewWriter(buf)
	_ = writer.Write([]string{"记录ID", "用户ID", "交易类型", "变动金额", "余额", "备注", "创建时间"})
	for _, item := range list {
		_ = writer.Write([]string{
			fmt.Sprintf("%d", item.ID),
			item.UID,
			w.getTxTypeTitle(item.Type),
			fmt.Sprintf("%.2f", item.Amount),
			fmt.Sprintf("%.2f", item.BalanceAfter),
			item.Remark,
			item.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	writer.Flush()

	_ = w.service.db.insertOperationLog(&operationLogModel{
		Operator:      c.GetLoginUID(),
		Action:        "data_export",
		OperationDesc: fmt.Sprintf("导出交易记录，共 %d 条", len(list)),
		Result:        "success",
		IPAddress:     c.ClientIP(),
	})

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=records.csv")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

func (w *Wallet) managerExportWithdrawals(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	keyword := c.DefaultQuery("keyword", "")
	statusStr := c.DefaultQuery("status", "-1")
	status, _ := strconv.Atoi(statusStr)

	list, err := w.service.db.getAllWithdrawals(keyword, status)
	if err != nil {
		c.ResponseError(err)
		return
	}

	statusName := map[int]string{0: "待审核", 1: "已批准", 2: "已拒绝"}
	buf := &bytes.Buffer{}
	buf.WriteString("\xEF\xBB\xBF")
	writer := csv.NewWriter(buf)
	_ = writer.Write([]string{"提现ID", "提现编号", "用户ID", "提现金额", "手续费", "实际到账", "状态", "申请时间"})
	for _, item := range list {
		st := statusName[item.Status]
		if st == "" {
			st = "未知"
		}
		_ = writer.Write([]string{
			fmt.Sprintf("%d", item.ID),
			item.WithdrawalNo,
			item.UID,
			fmt.Sprintf("%.2f", item.Amount),
			fmt.Sprintf("%.2f", item.Fee),
			fmt.Sprintf("%.2f", item.Amount-item.Fee),
			st,
			item.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	writer.Flush()

	_ = w.service.db.insertOperationLog(&operationLogModel{
		Operator:      c.GetLoginUID(),
		Action:        "data_export",
		OperationDesc: fmt.Sprintf("导出提现记录，共 %d 条", len(list)),
		Result:        "success",
		IPAddress:     c.ClientIP(),
	})

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=withdrawals.csv")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

func (w *Wallet) redpacketStatistics(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}

	var totalRedpackets int
	w.service.db.session.Select("count(*)").From("redpacket").Load(&totalRedpackets)

	var totalAmount float64
	w.service.db.session.SelectBySql("SELECT COALESCE(SUM(total_amount),0) FROM redpacket").Load(&totalAmount)

	var totalReceived float64
	w.service.db.session.SelectBySql("SELECT COALESCE(SUM(amount),0) FROM redpacket_record").Load(&totalReceived)

	var activeRedpackets int
	w.service.db.session.Select("count(*)").From("redpacket").Where("status=0").Load(&activeRedpackets)

	var todayRedpackets int
	w.service.db.session.Select("count(*)").From("redpacket").Where("DATE(created_at) = CURDATE()").Load(&todayRedpackets)

	var todayAmount float64
	w.service.db.session.SelectBySql("SELECT COALESCE(SUM(total_amount),0) FROM redpacket WHERE DATE(created_at) = CURDATE()").Load(&todayAmount)

	completionRate := 0.0
	if totalAmount > 0 {
		completionRate = totalReceived / totalAmount * 100
	}

	c.Response(map[string]interface{}{
		"total_redpackets":  totalRedpackets,
		"total_amount":      totalAmount,
		"total_received":    totalReceived,
		"active_redpackets": activeRedpackets,
		"today_redpackets":  todayRedpackets,
		"today_amount":      todayAmount,
		"completion_rate":   completionRate,
	})
}

func (w *Wallet) redpacketConfig(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type configModel struct {
		ID                       int64 `db:"id" json:"id"`
		ExpireHours              int   `db:"expire_hours" json:"expire_hours"`
		ExpireCheckIntervalMin   int   `db:"expire_check_interval_min" json:"expire_check_interval_min"`
		MaxAmountPerRedpacket    int   `db:"max_amount_per_redpacket" json:"max_amount_per_redpacket"`
		MaxDailyAmountPerUser    int   `db:"max_daily_amount_per_user" json:"max_daily_amount_per_user"`
		MaxRedpacketsPerHour     int   `db:"max_redpackets_per_hour" json:"max_redpackets_per_hour"`
		MinAmountPerRedpacket    int   `db:"min_amount_per_redpacket" json:"min_amount_per_redpacket"`
		BatchExpireLimit         int   `db:"batch_expire_limit" json:"batch_expire_limit"`
		EnableRiskControl        int   `db:"enable_risk_control" json:"enable_risk_control"`
		EnableAutoExpire         int   `db:"enable_auto_expire" json:"enable_auto_expire"`
		RefundOnExpire           int   `db:"refund_on_expire" json:"refund_on_expire"`
	}
	var cfg configModel
	_, err = w.service.db.session.Select("*").From("redpacket_config").Where("id=1").Load(&cfg)
	if err != nil {
		c.ResponseError(err)
		return
	}
	c.Response(cfg)
}

func (w *Wallet) updateRedpacketConfig(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type reqVO struct {
		ExpireHours            int `json:"expire_hours"`
		ExpireCheckIntervalMin int `json:"expire_check_interval_min"`
		MaxAmountPerRedpacket  int `json:"max_amount_per_redpacket"`
		MaxDailyAmountPerUser  int `json:"max_daily_amount_per_user"`
		MaxRedpacketsPerHour   int `json:"max_redpackets_per_hour"`
		MinAmountPerRedpacket  int `json:"min_amount_per_redpacket"`
		BatchExpireLimit       int `json:"batch_expire_limit"`
		EnableRiskControl      int `json:"enable_risk_control"`
		EnableAutoExpire       int `json:"enable_auto_expire"`
		RefundOnExpire         int `json:"refund_on_expire"`
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(err)
		return
	}
	_, err = w.service.db.session.Update("redpacket_config").
		Set("expire_hours", req.ExpireHours).
		Set("expire_check_interval_min", req.ExpireCheckIntervalMin).
		Set("max_amount_per_redpacket", req.MaxAmountPerRedpacket).
		Set("max_daily_amount_per_user", req.MaxDailyAmountPerUser).
		Set("max_redpackets_per_hour", req.MaxRedpacketsPerHour).
		Set("min_amount_per_redpacket", req.MinAmountPerRedpacket).
		Set("batch_expire_limit", req.BatchExpireLimit).
		Set("enable_risk_control", req.EnableRiskControl).
		Set("enable_auto_expire", req.EnableAutoExpire).
		Set("refund_on_expire", req.RefundOnExpire).
		Where("id=1").Exec()
	if err != nil {
		c.ResponseError(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

// ===== 红包列表/详情/状态/记录 =====

func (w *Wallet) redpacketList(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", c.DefaultQuery("page_index", "1")))
	size, _ := strconv.Atoi(c.DefaultQuery("size", c.DefaultQuery("page_size", "20")))
	keyword := c.DefaultQuery("keyword", "")
	statusStr := c.DefaultQuery("status", "-1")
	status, _ := strconv.Atoi(statusStr)
	rpType := c.DefaultQuery("type", "")

	where := "1=1"
	var args []interface{}
	if keyword != "" {
		like := "%" + keyword + "%"
		where += " AND (r.packet_no LIKE ? OR r.uid LIKE ? OR r.creater_name LIKE ?)"
		args = append(args, like, like, like)
	}
	if status >= 0 {
		where += " AND r.status=?"
		args = append(args, status)
	}
	if rpType != "" {
		where += " AND r.type=?"
		args = append(args, rpType)
	}

	var count int
	_, _ = w.service.db.session.SelectBySql("SELECT COUNT(*) FROM redpacket r WHERE "+where, args...).Load(&count)

	dataSQL := "SELECT r.*, COALESCE(u.name,'') as creater_name FROM redpacket r LEFT JOIN `user` u ON r.uid=u.uid WHERE " + where + " ORDER BY r.created_at DESC LIMIT ? OFFSET ?"
	dataArgs := append(args, size, (page-1)*size)

	type rpModel struct {
		ID              int64   `db:"id" json:"id"`
		PacketNo        string  `db:"packet_no" json:"redpacket_no"`
		UID             string  `db:"uid" json:"uid"`
		ChannelID       string  `db:"channel_id" json:"channel_id"`
		ChannelType     int     `db:"channel_type" json:"channel_type"`
		Type            int     `db:"type" json:"type"`
		TotalAmount     float64 `db:"total_amount" json:"total_amount"`
		TotalCount      int     `db:"total_count" json:"total_count"`
		RemainingAmount float64 `db:"remaining_amount" json:"remaining_amount"`
		RemainingCount  int     `db:"remaining_count" json:"remaining_count"`
		ToUID           string  `db:"to_uid" json:"to_uid"`
		Remark          string  `db:"remark" json:"remark"`
		Status          int     `db:"status" json:"status"`
		SceneType       int     `db:"scene_type" json:"scene_type"`
		CreaterName     string  `db:"creater_name" json:"creater_name"`
		CreatedAt       string  `db:"created_at" json:"created_at"`
	}
	var list []rpModel
	_, _ = w.service.db.session.SelectBySql(dataSQL, dataArgs...).Load(&list)

	respList := make([]gin.H, 0, len(list))
	for _, item := range list {
		receiveNum := item.TotalCount - item.RemainingCount
		completionRate := 0.0
		if item.TotalCount > 0 {
			completionRate = float64(receiveNum) / float64(item.TotalCount) * 100
		}
		respList = append(respList, gin.H{
			"id": item.ID, "redpacket_no": item.PacketNo, "packet_no": item.PacketNo,
			"uid": item.UID, "creater_name": item.CreaterName,
			"type": item.Type, "scene_type": item.SceneType, "channel_type": item.ChannelType,
			"amount": item.TotalAmount, "total_amount": item.TotalAmount,
			"num": item.TotalCount, "total_count": item.TotalCount,
			"receive_num": receiveNum, "remaining_amount": item.RemainingAmount, "balance": item.RemainingAmount,
			"completion_rate": completionRate,
			"status": item.Status, "remark": item.Remark,
			"created_at": item.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"list": respList, "total": count, "count": count, "page": page, "size": size})
}

func (w *Wallet) redpacketDetail(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	packetNo := c.Param("packet_no")

	type rpModel struct {
		ID              int64   `db:"id"`
		PacketNo        string  `db:"packet_no"`
		UID             string  `db:"uid"`
		ChannelType     int     `db:"channel_type"`
		Type            int     `db:"type"`
		TotalAmount     float64 `db:"total_amount"`
		TotalCount      int     `db:"total_count"`
		RemainingAmount float64 `db:"remaining_amount"`
		RemainingCount  int     `db:"remaining_count"`
		Remark          string  `db:"remark"`
		Status          int     `db:"status"`
		SceneType       int     `db:"scene_type"`
		CreaterName     string  `db:"creater_name"`
		CreatedAt       string  `db:"created_at"`
	}
	var rp rpModel
	_, err = w.service.db.session.SelectBySql("SELECT r.*, COALESCE(u.name,'') as creater_name FROM redpacket r LEFT JOIN `user` u ON r.uid=u.uid WHERE r.packet_no=?", packetNo).Load(&rp)
	if err != nil {
		c.ResponseError(err)
		return
	}

	type recModel struct {
		ID        int64   `db:"id" json:"id"`
		PacketNo  string  `db:"packet_no" json:"redpacket_no"`
		UID       string  `db:"uid" json:"uid"`
		Amount    float64 `db:"amount" json:"amount"`
		IsBest    int     `db:"is_best" json:"is_best"`
		IsWorst   int     `db:"-" json:"is_worst"`
		CreatedAt string  `db:"created_at" json:"created_at"`
	}
	var records []recModel
	_, _ = w.service.db.session.SelectBySql("SELECT * FROM redpacket_record WHERE packet_no=? ORDER BY created_at DESC", packetNo).Load(&records)

	if len(records) >= 2 {
		minAmt := records[0].Amount
		for i := 1; i < len(records); i++ {
			if records[i].Amount < minAmt {
				minAmt = records[i].Amount
			}
		}
		for i := range records {
			if math.Abs(records[i].Amount-minAmt) < 1e-6 {
				records[i].IsWorst = 1
			}
		}
	}

	receiveNum := rp.TotalCount - rp.RemainingCount
	c.JSON(http.StatusOK, gin.H{
		"id": rp.ID, "redpacket_no": rp.PacketNo, "packet_no": rp.PacketNo,
		"uid": rp.UID, "creater_name": rp.CreaterName,
		"type": rp.Type, "scene_type": rp.SceneType, "channel_type": rp.ChannelType,
		"amount": rp.TotalAmount, "total_amount": rp.TotalAmount,
		"num": rp.TotalCount, "total_count": rp.TotalCount,
		"receive_num": receiveNum, "remaining_amount": rp.RemainingAmount, "balance": rp.RemainingAmount,
		"status": rp.Status, "remark": rp.Remark, "blessing": rp.Remark,
		"created_at": rp.CreatedAt, "records": records,
	})
}

func (w *Wallet) redpacketStatusUpdate(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	idStr := c.Param("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	var req struct {
		Status int    `json:"status"`
		Reason string `json:"reason"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(err)
		return
	}
	_, err = w.service.db.session.Update("redpacket").Set("status", req.Status).Where("id=?", id).Exec()
	if err != nil {
		c.ResponseError(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

func (w *Wallet) redpacketRecords(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", c.DefaultQuery("page_index", "1")))
	size, _ := strconv.Atoi(c.DefaultQuery("size", c.DefaultQuery("page_size", "20")))
	keyword := strings.TrimSpace(c.DefaultQuery("keyword", ""))
	redpacketNo := strings.TrimSpace(c.DefaultQuery("redpacket_no", ""))
	receiver := strings.TrimSpace(c.DefaultQuery("receiver", ""))

	where := "1=1"
	var args []interface{}
	if redpacketNo != "" {
		where += " AND rr.packet_no LIKE ?"
		args = append(args, "%"+redpacketNo+"%")
	}
	if receiver != "" {
		like := "%" + receiver + "%"
		where += " AND (rr.uid LIKE ? OR u.name LIKE ? OR u.username LIKE ?)"
		args = append(args, like, like, like)
	}
	if keyword != "" {
		like := "%" + keyword + "%"
		where += " AND (rr.uid LIKE ? OR rr.packet_no LIKE ? OR u.name LIKE ? OR u.username LIKE ?)"
		args = append(args, like, like, like, like)
	}

	fromJoin := "redpacket_record rr LEFT JOIN `user` u ON rr.uid=u.uid"
	countSQL := "SELECT COUNT(*) FROM " + fromJoin + " WHERE " + where
	var count int
	_, _ = w.service.db.session.SelectBySql(countSQL, args...).Load(&count)

	dataSQL := "SELECT rr.id, rr.packet_no, rr.uid, rr.amount, rr.is_best, rr.created_at, " +
		"COALESCE(u.name,'') as receiver_name, COALESCE(r.type,0) as rp_type, COALESCE(r.channel_type,0) as channel_type, " +
		"CASE WHEN COALESCE(rp_agg.cnt,0) >= 2 AND rr.amount = rp_agg.min_amount THEN 1 ELSE 0 END as is_worst " +
		"FROM redpacket_record rr " +
		"LEFT JOIN `user` u ON rr.uid=u.uid " +
		"LEFT JOIN redpacket r ON r.packet_no=rr.packet_no " +
		"LEFT JOIN (SELECT packet_no, MIN(amount) as min_amount, COUNT(*) as cnt FROM redpacket_record GROUP BY packet_no) rp_agg ON rp_agg.packet_no=rr.packet_no " +
		"WHERE " + where + " ORDER BY rr.created_at DESC LIMIT ? OFFSET ?"
	dataArgs := append(append([]interface{}{}, args...), size, (page-1)*size)

	type recModel struct {
		ID           int64   `db:"id"`
		PacketNo     string  `db:"packet_no"`
		UID          string  `db:"uid"`
		Amount       float64 `db:"amount"`
		IsBest       int     `db:"is_best"`
		IsWorst      int     `db:"is_worst"`
		ReceiverName string  `db:"receiver_name"`
		RpType       int     `db:"rp_type"`
		ChannelType  int     `db:"channel_type"`
		CreatedAt    string  `db:"created_at"`
	}
	var list []recModel
	_, _ = w.service.db.session.SelectBySql(dataSQL, dataArgs...).Load(&list)

	respList := make([]gin.H, 0, len(list))
	for _, item := range list {
		respList = append(respList, gin.H{
			"id": item.ID, "redpacket_no": item.PacketNo, "packet_no": item.PacketNo,
			"uid": item.UID, "receiver": item.UID, "receiver_name": item.ReceiverName,
			"amount": item.Amount, "is_best": item.IsBest, "is_luck": item.IsBest, "is_lucky": item.IsBest,
			"is_worst": item.IsWorst,
			"type": item.RpType, "channel_type": item.ChannelType,
			"created_at": item.CreatedAt, "received_at": item.CreatedAt, "received": 1,
		})
	}
	c.JSON(http.StatusOK, gin.H{"list": respList, "total": count, "count": count, "page": page, "size": size})
}

// ===== 数据分析 =====

func (w *Wallet) analyticsType(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type typeStats struct{ Count int; Amount float64 }
	individual := typeStats{}
	groupRandom := typeStats{}
	groupNormal := typeStats{}
	exclusive := typeStats{}
	w.service.db.session.SelectBySql("SELECT COUNT(*) as `count`, COALESCE(SUM(total_amount),0) as amount FROM redpacket WHERE type=1").Load(&individual)
	w.service.db.session.SelectBySql("SELECT COUNT(*) as `count`, COALESCE(SUM(total_amount),0) as amount FROM redpacket WHERE type=2").Load(&groupRandom)
	w.service.db.session.SelectBySql("SELECT COUNT(*) as `count`, COALESCE(SUM(total_amount),0) as amount FROM redpacket WHERE type=3").Load(&groupNormal)
	w.service.db.session.SelectBySql("SELECT COUNT(*) as `count`, COALESCE(SUM(total_amount),0) as amount FROM redpacket WHERE type=4").Load(&exclusive)
	c.JSON(http.StatusOK, gin.H{
		"individual":   gin.H{"count": individual.Count, "amount": individual.Amount},
		"group_random": gin.H{"count": groupRandom.Count, "amount": groupRandom.Amount},
		"group_normal": gin.H{"count": groupNormal.Count, "amount": groupNormal.Amount},
		"exclusive":    gin.H{"count": exclusive.Count, "amount": exclusive.Amount},
	})
}

func (w *Wallet) analyticsScene(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type sceneStats struct{ Count int; Amount float64 }
	priv := sceneStats{}
	group := sceneStats{}
	w.service.db.session.SelectBySql("SELECT COUNT(*) as `count`, COALESCE(SUM(total_amount),0) as amount FROM redpacket WHERE channel_type=1").Load(&priv)
	w.service.db.session.SelectBySql("SELECT COUNT(*) as `count`, COALESCE(SUM(total_amount),0) as amount FROM redpacket WHERE channel_type=2").Load(&group)
	c.JSON(http.StatusOK, gin.H{
		"private_chat": gin.H{"count": priv.Count, "amount": priv.Amount},
		"group_chat":   gin.H{"count": group.Count, "amount": group.Amount},
	})
}

func (w *Wallet) analyticsTrend(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type dayRow struct {
		Day    string  `db:"day"`
		Count  int     `db:"cnt"`
		Amount float64 `db:"amt"`
	}
	var rpDays []dayRow
	w.service.db.session.SelectBySql("SELECT DATE(created_at) as day, COUNT(*) as cnt, COALESCE(SUM(total_amount),0) as amt FROM redpacket WHERE created_at >= DATE_SUB(CURDATE(), INTERVAL 30 DAY) GROUP BY DATE(created_at) ORDER BY day").Load(&rpDays)

	var recDays []dayRow
	w.service.db.session.SelectBySql("SELECT DATE(created_at) as day, COUNT(*) as cnt, COALESCE(SUM(amount),0) as amt FROM redpacket_record WHERE created_at >= DATE_SUB(CURDATE(), INTERVAL 30 DAY) GROUP BY DATE(created_at) ORDER BY day").Load(&recDays)

	dailyRedpackets := make([]gin.H, 0)
	dailyAmount := make([]gin.H, 0)
	dailyReceived := make([]gin.H, 0)
	for _, d := range rpDays {
		dailyRedpackets = append(dailyRedpackets, gin.H{"date": d.Day, "count": d.Count})
		dailyAmount = append(dailyAmount, gin.H{"date": d.Day, "amount": d.Amount})
	}
	for _, d := range recDays {
		dailyReceived = append(dailyReceived, gin.H{"date": d.Day, "amount": d.Amount})
	}
	c.JSON(http.StatusOK, gin.H{
		"daily_redpackets": dailyRedpackets,
		"daily_amount":     dailyAmount,
		"daily_received":   dailyReceived,
	})
}

func (w *Wallet) analyticsSummary(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	var totalRedpackets int
	var totalAmount, totalReceived float64
	var todayRedpackets int
	var todayAmount float64
	var expireCount int
	var expireAmount float64
	w.service.db.session.SelectBySql("SELECT COUNT(*) FROM redpacket").Load(&totalRedpackets)
	w.service.db.session.SelectBySql("SELECT COALESCE(SUM(total_amount),0) FROM redpacket").Load(&totalAmount)
	w.service.db.session.SelectBySql("SELECT COALESCE(SUM(amount),0) FROM redpacket_record").Load(&totalReceived)
	w.service.db.session.SelectBySql("SELECT COUNT(*) FROM redpacket WHERE DATE(created_at)=CURDATE()").Load(&todayRedpackets)
	w.service.db.session.SelectBySql("SELECT COALESCE(SUM(total_amount),0) FROM redpacket WHERE DATE(created_at)=CURDATE()").Load(&todayAmount)
	w.service.db.session.SelectBySql("SELECT COUNT(*) FROM redpacket WHERE status=2").Load(&expireCount)
	w.service.db.session.SelectBySql("SELECT COALESCE(SUM(total_amount),0) FROM redpacket WHERE status=2").Load(&expireAmount)

	completionRate := 0.0
	if totalAmount > 0 {
		completionRate = totalReceived / totalAmount * 100
	}
	c.JSON(http.StatusOK, gin.H{
		"total_redpackets": totalRedpackets, "total_amount": totalAmount,
		"total_received": totalReceived, "completion_rate": completionRate,
		"today_redpackets": todayRedpackets, "today_amount": todayAmount,
		"expire_count": expireCount, "expire_amount": expireAmount,
	})
}

// redpacketExportStatusLabel 与后台列表一致：0 领取中、1 已完成、2 已过期；多份未领完时带剩余份数。
func redpacketExportStatusLabel(status, totalCount, remainingCount int) string {
	received := totalCount - remainingCount
	if received < 0 {
		received = 0
	}
	hasRemain := totalCount > 0 && received < totalCount
	switch status {
	case 2:
		if hasRemain {
			return "已过期(未领完)"
		}
		return "已过期"
	case 1:
		return "已完成"
	default:
		if hasRemain {
			return fmt.Sprintf("领取中(剩%d份)", totalCount-received)
		}
		return "领取中"
	}
}

func (w *Wallet) exportRedpackets(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type rpRow struct {
		PacketNo        string  `db:"packet_no"`
		UID             string  `db:"uid"`
		Type            int     `db:"type"`
		TotalAmount     float64 `db:"total_amount"`
		TotalCount      int     `db:"total_count"`
		RemainingCount  int     `db:"remaining_count"`
		Status          int     `db:"status"`
		CreatedAt       string  `db:"created_at"`
	}
	var list []rpRow
	w.service.db.session.SelectBySql("SELECT packet_no, uid, type, total_amount, total_count, remaining_count, status, created_at FROM redpacket ORDER BY created_at DESC").Load(&list)

	buf := &bytes.Buffer{}
	buf.WriteString("\xEF\xBB\xBF")
	writer := csv.NewWriter(buf)
	_ = writer.Write([]string{"红包编号", "创建者", "类型", "金额", "数量", "状态", "创建时间"})
	typeNames := map[int]string{1: "个人红包", 2: "拼手气红包", 3: "普通红包", 4: "专属红包"}
	for _, item := range list {
		statusLabel := redpacketExportStatusLabel(item.Status, item.TotalCount, item.RemainingCount)
		_ = writer.Write([]string{item.PacketNo, item.UID, typeNames[item.Type], fmt.Sprintf("%.2f", item.TotalAmount), fmt.Sprintf("%d", item.TotalCount), statusLabel, item.CreatedAt})
	}
	writer.Flush()
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=redpackets.csv")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

func (w *Wallet) exportAnalytics(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	buf := &bytes.Buffer{}
	buf.WriteString("\xEF\xBB\xBF")
	writer := csv.NewWriter(buf)
	_ = writer.Write([]string{"日期", "红包数", "发放金额", "领取金额"})
	type dayRow struct {
		Day string `db:"day"`
		Cnt int    `db:"cnt"`
		Amt float64 `db:"amt"`
	}
	var days []dayRow
	w.service.db.session.SelectBySql("SELECT DATE(created_at) as day, COUNT(*) as cnt, COALESCE(SUM(total_amount),0) as amt FROM redpacket WHERE created_at >= DATE_SUB(CURDATE(), INTERVAL 30 DAY) GROUP BY DATE(created_at) ORDER BY day").Load(&days)
	for _, d := range days {
		_ = writer.Write([]string{d.Day, fmt.Sprintf("%d", d.Cnt), fmt.Sprintf("%.2f", d.Amt), "0"})
	}
	writer.Flush()
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=redpacket_analytics.csv")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// ===== 风控管理 =====

func (w *Wallet) riskMonitor(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	var totalEvents, todayEvents, pendingEvents, highRiskEvents int
	w.service.db.session.SelectBySql("SELECT COUNT(*) FROM redpacket_risk_event").Load(&totalEvents)
	w.service.db.session.SelectBySql("SELECT COUNT(*) FROM redpacket_risk_event WHERE DATE(created_at)=CURDATE()").Load(&todayEvents)
	w.service.db.session.SelectBySql("SELECT COUNT(*) FROM redpacket_risk_event WHERE status='pending'").Load(&pendingEvents)
	w.service.db.session.SelectBySql("SELECT COUNT(*) FROM redpacket_risk_event WHERE risk_level='high'").Load(&highRiskEvents)

	type eventModel struct {
		ID        int64  `db:"id" json:"id"`
		UID       string `db:"uid" json:"uid"`
		EventType string `db:"event_type" json:"event_type"`
		RiskLevel string `db:"risk_level" json:"risk_level"`
		Remark    string `db:"remark" json:"remark"`
		Status    string `db:"status" json:"status"`
		CreatedAt string `db:"created_at" json:"created_at"`
	}
	var events []eventModel
	w.service.db.session.SelectBySql("SELECT id, uid, event_type, risk_level, remark, status, created_at FROM redpacket_risk_event ORDER BY created_at DESC LIMIT 100").Load(&events)

	c.JSON(http.StatusOK, gin.H{
		"statistics": gin.H{
			"total_events": totalEvents, "today_events": todayEvents,
			"pending_events": pendingEvents, "high_risk_events": highRiskEvents,
		},
		"events": events,
	})
}

func (w *Wallet) riskAction(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	var req struct {
		EventID int64  `json:"event_id"`
		Action  string `json:"action"`
		Reason  string `json:"reason"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(err)
		return
	}
	_, err = w.service.db.session.Update("redpacket_risk_event").
		Set("status", req.Action).Set("handler", c.GetLoginUID()).Set("handle_remark", req.Reason).
		Where("id=?", req.EventID).Exec()
	if err != nil {
		c.ResponseError(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

func (w *Wallet) riskBatchAction(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	var req struct {
		EventIDs []int64 `json:"event_ids"`
		Action   string  `json:"action"`
		Reason   string  `json:"reason"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(err)
		return
	}
	for _, eid := range req.EventIDs {
		_, _ = w.service.db.session.Update("redpacket_risk_event").
			Set("status", req.Action).Set("handler", c.GetLoginUID()).Set("handle_remark", req.Reason).
			Where("id=?", eid).Exec()
	}
	c.JSON(http.StatusOK, gin.H{"status": 200, "count": len(req.EventIDs)})
}

func (w *Wallet) riskRulesList(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type ruleModel struct {
		ID          int64   `db:"id" json:"id"`
		Name        string  `db:"name" json:"name"`
		Type        string  `db:"type" json:"type"`
		Threshold   float64 `db:"threshold" json:"threshold"`
		TimeWindow  int     `db:"time_window" json:"time_window"`
		Description string  `db:"description" json:"description"`
		Enabled     int     `db:"enabled" json:"enabled"`
		SortOrder   int     `db:"sort_order" json:"sort_order"`
		CreatedAt   string  `db:"created_at" json:"created_at"`
	}
	var rules []ruleModel
	_, _ = w.service.db.session.SelectBySql("SELECT * FROM redpacket_risk_rule ORDER BY sort_order ASC, id ASC").Load(&rules)
	c.JSON(http.StatusOK, rules)
}

func (w *Wallet) riskRuleCreate(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	var req struct {
		Name        string  `json:"name"`
		Type        string  `json:"type"`
		Threshold   float64 `json:"threshold"`
		TimeWindow  int     `json:"time_window"`
		Description string  `json:"description"`
		SortOrder   int     `json:"sort_order"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(err)
		return
	}
	_, err = w.service.db.session.InsertInto("redpacket_risk_rule").
		Columns("name", "type", "threshold", "time_window", "description", "sort_order").
		Values(req.Name, req.Type, req.Threshold, req.TimeWindow, req.Description, req.SortOrder).Exec()
	if err != nil {
		c.ResponseError(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

func (w *Wallet) riskRuleUpdate(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	idStr := c.Param("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	var req struct {
		Name        string  `json:"name"`
		Type        string  `json:"type"`
		Threshold   float64 `json:"threshold"`
		TimeWindow  int     `json:"time_window"`
		Description string  `json:"description"`
		SortOrder   int     `json:"sort_order"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(err)
		return
	}
	_, err = w.service.db.session.Update("redpacket_risk_rule").
		Set("name", req.Name).Set("type", req.Type).Set("threshold", req.Threshold).
		Set("time_window", req.TimeWindow).Set("description", req.Description).Set("sort_order", req.SortOrder).
		Where("id=?", id).Exec()
	if err != nil {
		c.ResponseError(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

func (w *Wallet) riskRuleDelete(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	idStr := c.Param("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	_, err = w.service.db.session.DeleteFrom("redpacket_risk_rule").Where("id=?", id).Exec()
	if err != nil {
		c.ResponseError(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

func (w *Wallet) riskRuleToggle(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	idStr := c.Param("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	var req struct {
		Enabled int `json:"enabled"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(err)
		return
	}
	_, err = w.service.db.session.Update("redpacket_risk_rule").Set("enabled", req.Enabled).Where("id=?", id).Exec()
	if err != nil {
		c.ResponseError(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

func (w *Wallet) transferStatistics(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}

	var totalTransfers int
	w.service.db.session.Select("count(*)").From("transfer").Load(&totalTransfers)

	var totalAmount float64
	w.service.db.session.SelectBySql("SELECT COALESCE(SUM(amount),0) FROM `transfer`").Load(&totalAmount)

	var todayTransfers int
	w.service.db.session.SelectBySql("SELECT COUNT(*) FROM `transfer` WHERE DATE(created_at)=CURDATE()").Load(&todayTransfers)

	var todayAmount float64
	w.service.db.session.SelectBySql("SELECT COALESCE(SUM(amount),0) FROM `transfer` WHERE DATE(created_at)=CURDATE()").Load(&todayAmount)

	var completedTransfers int
	w.service.db.session.SelectBySql("SELECT COUNT(*) FROM `transfer` WHERE status=1").Load(&completedTransfers)

	var pendingTransfers int
	w.service.db.session.SelectBySql("SELECT COUNT(*) FROM `transfer` WHERE status=0").Load(&pendingTransfers)

	completionRate := 0.0
	if totalTransfers > 0 {
		completionRate = float64(completedTransfers) / float64(totalTransfers) * 100
	}

	c.Response(map[string]interface{}{
		"total_transfers":     totalTransfers,
		"total_amount":        totalAmount,
		"today_transfers":     todayTransfers,
		"today_amount":        todayAmount,
		"completed_transfers": completedTransfers,
		"pending_transfers":   pendingTransfers,
		"completion_rate":     completionRate,
	})
}

func (w *Wallet) transferConfig(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type configModel struct {
		ID             int64  `db:"id" json:"id"`
		DailyLimit     string `db:"daily_limit" json:"daily_limit"`
		ExpireHours    string `db:"expire_hours" json:"expire_hours"`
		FeeRate        string `db:"fee_rate" json:"fee_rate"`
		MaxAmount      string `db:"max_amount" json:"max_amount"`
		MinAmount      string `db:"min_amount" json:"min_amount"`
		RiskThreshold  string `db:"risk_threshold" json:"risk_threshold"`
	}
	var cfg configModel
	_, err = w.service.db.session.Select("*").From("transfer_config").Where("id=1").Load(&cfg)
	if err != nil {
		c.ResponseError(err)
		return
	}
	c.Response(cfg)
}

func (w *Wallet) updateTransferConfig(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type reqVO struct {
		DailyLimit    string `json:"daily_limit"`
		ExpireHours   string `json:"expire_hours"`
		FeeRate       string `json:"fee_rate"`
		MaxAmount     string `json:"max_amount"`
		MinAmount     string `json:"min_amount"`
		RiskThreshold string `json:"risk_threshold"`
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(err)
		return
	}
	_, err = w.service.db.session.Update("transfer_config").
		Set("daily_limit", req.DailyLimit).
		Set("expire_hours", req.ExpireHours).
		Set("fee_rate", req.FeeRate).
		Set("max_amount", req.MaxAmount).
		Set("min_amount", req.MinAmount).
		Set("risk_threshold", req.RiskThreshold).
		Where("id=1").Exec()
	if err != nil {
		c.ResponseError(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

// ===== 转账列表/详情/状态/退回 =====

func (w *Wallet) transferList(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", c.DefaultQuery("page_index", "1")))
	size, _ := strconv.Atoi(c.DefaultQuery("size", c.DefaultQuery("page_size", "20")))
	keyword := c.DefaultQuery("keyword", "")
	statusStr := c.Query("status")
	status := -1
	if statusStr != "" {
		if v, e := strconv.Atoi(statusStr); e == nil {
			status = v
		}
	}
	minAmountStr := c.DefaultQuery("min_amount", "")
	maxAmountStr := c.DefaultQuery("max_amount", "")
	startTime := strings.TrimSpace(c.DefaultQuery("start_time", ""))
	endTime := strings.TrimSpace(c.DefaultQuery("end_time", ""))

	where := "1=1"
	var args []interface{}
	if keyword != "" {
		like := "%" + keyword + "%"
		where += " AND (t.transfer_no LIKE ? OR t.from_uid LIKE ? OR t.to_uid LIKE ?)"
		args = append(args, like, like, like)
	}
	if startTime != "" {
		where += " AND DATE(t.created_at) >= ?"
		args = append(args, startTime)
	}
	if endTime != "" {
		where += " AND DATE(t.created_at) <= ?"
		args = append(args, endTime)
	}
	if status >= 0 {
		where += " AND t.status=?"
		args = append(args, status)
	}
	if minAmountStr != "" {
		where += " AND t.amount>=?"
		args = append(args, minAmountStr)
	}
	if maxAmountStr != "" {
		where += " AND t.amount<=?"
		args = append(args, maxAmountStr)
	}

	var count int
	_, _ = w.service.db.session.SelectBySql("SELECT COUNT(*) FROM `transfer` t WHERE "+where, args...).Load(&count)

	dataSQL := "SELECT t.*, COALESCE(uf.name,'') as from_name, COALESCE(ut.name,'') as to_name FROM `transfer` t LEFT JOIN `user` uf ON t.from_uid=uf.uid LEFT JOIN `user` ut ON t.to_uid=ut.uid WHERE " + where + " ORDER BY t.created_at DESC LIMIT ? OFFSET ?"
	dataArgs := append(args, size, (page-1)*size)

	type tfModel struct {
		ID         int64   `db:"id"`
		TransferNo string  `db:"transfer_no"`
		FromUID    string  `db:"from_uid"`
		ToUID      string  `db:"to_uid"`
		Amount     float64 `db:"amount"`
		Remark     string  `db:"remark"`
		Status     int     `db:"status"`
		FromName   string  `db:"from_name"`
		ToName     string  `db:"to_name"`
		CreatedAt  string  `db:"created_at"`
	}
	var list []tfModel
	_, _ = w.service.db.session.SelectBySql(dataSQL, dataArgs...).Load(&list)

	respList := make([]gin.H, 0, len(list))
	for _, item := range list {
		respList = append(respList, gin.H{
			"id": item.ID, "transfer_no": item.TransferNo,
			"from_uid": item.FromUID, "to_uid": item.ToUID,
			"from_name": item.FromName, "to_name": item.ToName,
			"amount": item.Amount, "remark": item.Remark,
			"status": item.Status, "created_at": item.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"list": respList, "total": count, "count": count, "page": page, "size": size})
}

func (w *Wallet) transferDetail(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	transferNo := c.Param("transfer_no")
	type tfModel struct {
		ID         int64   `db:"id"`
		TransferNo string  `db:"transfer_no"`
		FromUID    string  `db:"from_uid"`
		ToUID      string  `db:"to_uid"`
		Amount     float64 `db:"amount"`
		Remark     string  `db:"remark"`
		Status     int     `db:"status"`
		FromName   string  `db:"from_name"`
		ToName     string  `db:"to_name"`
		ExpiredAt  *string `db:"expired_at"`
		CreatedAt  string  `db:"created_at"`
	}
	var tf tfModel
	_, err = w.service.db.session.SelectBySql("SELECT t.*, COALESCE(uf.name,'') as from_name, COALESCE(ut.name,'') as to_name FROM `transfer` t LEFT JOIN `user` uf ON t.from_uid=uf.uid LEFT JOIN `user` ut ON t.to_uid=ut.uid WHERE t.transfer_no=?", transferNo).Load(&tf)
	if err != nil {
		c.ResponseError(err)
		return
	}
	expiredAt := ""
	if tf.ExpiredAt != nil {
		expiredAt = *tf.ExpiredAt
	}
	c.JSON(http.StatusOK, gin.H{
		"id": tf.ID, "transfer_no": tf.TransferNo,
		"from_uid": tf.FromUID, "to_uid": tf.ToUID,
		"from_name": tf.FromName, "to_name": tf.ToName,
		"amount": tf.Amount, "remark": tf.Remark,
		"status": tf.Status, "expired_at": expiredAt, "created_at": tf.CreatedAt,
	})
}

func (w *Wallet) transferStatusUpdate(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	transferNo := c.Param("transfer_no")
	var req struct {
		Status int    `json:"status"`
		Remark string `json:"remark"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(err)
		return
	}
	_, err = w.service.db.session.Update("transfer").Set("status", req.Status).Where("transfer_no=?", transferNo).Exec()
	if err != nil {
		c.ResponseError(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

func (w *Wallet) transferRefund(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	transferNo := c.Param("transfer_no")
	var req struct {
		Reason string `json:"reason"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(err)
		return
	}
	if err = w.service.tfManagerRefund(transferNo, req.Reason); err != nil {
		c.ResponseError(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

// ===== 转账数据分析 =====

func (w *Wallet) transferAnalyticsUser(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type userRank struct {
		UID    string  `db:"uid" json:"uid"`
		Name   string  `db:"name" json:"name"`
		Count  int     `db:"cnt" json:"count"`
		Amount float64 `db:"amt" json:"amount"`
	}
	var senders []userRank
	w.service.db.session.SelectBySql("SELECT t.from_uid as uid, COALESCE(u.name,'') as name, COUNT(*) as cnt, COALESCE(SUM(t.amount),0) as amt FROM `transfer` t LEFT JOIN `user` u ON t.from_uid=u.uid GROUP BY t.from_uid ORDER BY amt DESC LIMIT 10").Load(&senders)
	var receivers []userRank
	w.service.db.session.SelectBySql("SELECT t.to_uid as uid, COALESCE(u.name,'') as name, COUNT(*) as cnt, COALESCE(SUM(t.amount),0) as amt FROM `transfer` t LEFT JOIN `user` u ON t.to_uid=u.uid GROUP BY t.to_uid ORDER BY amt DESC LIMIT 10").Load(&receivers)
	c.JSON(http.StatusOK, gin.H{"top_senders": senders, "top_receivers": receivers})
}

func (w *Wallet) transferAnalyticsTrend(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type dayRow struct {
		Day   string  `db:"day"`
		Count int     `db:"cnt"`
		Amt   float64 `db:"amt"`
	}
	var days []dayRow
	w.service.db.session.SelectBySql("SELECT DATE(created_at) as day, COUNT(*) as cnt, COALESCE(SUM(amount),0) as amt FROM `transfer` WHERE created_at >= DATE_SUB(CURDATE(), INTERVAL 30 DAY) GROUP BY DATE(created_at) ORDER BY day").Load(&days)

	var completedDays []dayRow
	w.service.db.session.SelectBySql("SELECT DATE(created_at) as day, COUNT(*) as cnt, COALESCE(SUM(amount),0) as amt FROM `transfer` WHERE status=1 AND created_at >= DATE_SUB(CURDATE(), INTERVAL 30 DAY) GROUP BY DATE(created_at) ORDER BY day").Load(&completedDays)

	dailyCounts := make([]gin.H, 0)
	dailyAmounts := make([]gin.H, 0)
	for _, d := range days {
		dailyCounts = append(dailyCounts, gin.H{"date": d.Day, "count": d.Count})
		dailyAmounts = append(dailyAmounts, gin.H{"date": d.Day, "amount": d.Amt})
	}
	dailyCompleted := make([]gin.H, 0)
	for _, d := range completedDays {
		dailyCompleted = append(dailyCompleted, gin.H{"date": d.Day, "count": d.Count, "amount": d.Amt})
	}
	c.JSON(http.StatusOK, gin.H{"daily_counts": dailyCounts, "daily_amounts": dailyAmounts, "daily_completed": dailyCompleted})
}

func (w *Wallet) transferAnalyticsAmount(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	var avgAmount float64
	var maxTransfer float64
	var totalAmount float64
	w.service.db.session.SelectBySql("SELECT COALESCE(AVG(amount),0) FROM `transfer`").Load(&avgAmount)
	w.service.db.session.SelectBySql("SELECT COALESCE(MAX(amount),0) FROM `transfer`").Load(&maxTransfer)
	w.service.db.session.SelectBySql("SELECT COALESCE(SUM(amount),0) FROM `transfer`").Load(&totalAmount)

	type hourRow struct {
		Hour  int `db:"h"`
		Count int `db:"cnt"`
	}
	var hours []hourRow
	w.service.db.session.SelectBySql("SELECT HOUR(created_at) as h, COUNT(*) as cnt FROM `transfer` WHERE DATE(created_at)=CURDATE() GROUP BY HOUR(created_at) ORDER BY h").Load(&hours)
	hourly := make([]gin.H, 0)
	for _, h := range hours {
		hourly = append(hourly, gin.H{"hour": h.Hour, "count": h.Count})
	}
	c.JSON(http.StatusOK, gin.H{"avg_amount": avgAmount, "max_transfer": maxTransfer, "total_amount": totalAmount, "hourly_distribution": hourly})
}

func (w *Wallet) transferAnalyticsSummary(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	var total int
	var totalAmt, avgAmt float64
	var completed, pending, failed int
	w.service.db.session.SelectBySql("SELECT COUNT(*) FROM `transfer`").Load(&total)
	w.service.db.session.SelectBySql("SELECT COALESCE(SUM(amount),0) FROM `transfer`").Load(&totalAmt)
	w.service.db.session.SelectBySql("SELECT COALESCE(AVG(amount),0) FROM `transfer`").Load(&avgAmt)
	w.service.db.session.SelectBySql("SELECT COUNT(*) FROM `transfer` WHERE status=1").Load(&completed)
	w.service.db.session.SelectBySql("SELECT COUNT(*) FROM `transfer` WHERE status=0").Load(&pending)
	w.service.db.session.SelectBySql("SELECT COUNT(*) FROM `transfer` WHERE status=2").Load(&failed)
	c.JSON(http.StatusOK, gin.H{
		"total_transfers": total, "total_amount": totalAmt, "avg_amount": avgAmt,
		"completed": completed, "pending": pending, "failed": failed,
	})
}

// ===== 转账风控 =====

func (w *Wallet) transferRiskMonitor(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	riskThFen := int64(100000)
	if cfg, e := w.service.db.getTransferConfigParsed(); e == nil && cfg != nil && cfg.RiskThresholdFen > 0 {
		riskThFen = cfg.RiskThresholdFen
	}
	minYuanLarge := float64(riskThFen) / 100.0

	var largeToday, frequentToday, failedToday, suspiciousToday int
	w.service.db.session.SelectBySql("SELECT COUNT(*) FROM `transfer` WHERE amount >= ? AND DATE(created_at)=CURDATE()", minYuanLarge).Load(&largeToday)
	w.service.db.session.SelectBySql("SELECT COUNT(*) FROM (SELECT from_uid FROM `transfer` WHERE DATE(created_at)=CURDATE() GROUP BY from_uid HAVING COUNT(*)>10) tfreq").Load(&frequentToday)
	w.service.db.session.SelectBySql("SELECT COUNT(*) FROM `transfer` WHERE status=2 AND DATE(created_at)=CURDATE()").Load(&failedToday)
	w.service.db.session.SelectBySql("SELECT COUNT(*) FROM (SELECT from_uid FROM `transfer` WHERE DATE(created_at)=CURDATE() GROUP BY from_uid HAVING COUNT(*)>5) tsusp").Load(&suspiciousToday)
	c.JSON(http.StatusOK, gin.H{
		"risk_statistics": gin.H{
			"large_transfers_today":    largeToday,
			"frequent_users_today":     frequentToday,
			"failed_transfers_today":   failedToday,
			"suspicious_patterns_today": suspiciousToday,
		},
	})
}

func (w *Wallet) transferRiskAction(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	var req struct {
		Action     string `json:"action"`
		TransferNo string `json:"transfer_no"`
		Reason     string `json:"reason"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(err)
		return
	}
	w.Info("转账风控操作",
		zap.String("action", req.Action),
		zap.String("transfer_no", req.TransferNo),
		zap.String("operator", c.GetLoginUID()),
		zap.String("reason", req.Reason),
	)
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

func (w *Wallet) transferRiskAlerts(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type alertItem struct {
		ID          int64  `json:"id"`
		Type        string `json:"type"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Level       string `json:"level"`
		TransferNo  string `json:"transfer_no"`
		UID         string `json:"uid"`
		Amount      float64 `json:"amount"`
		CreatedAt   string `json:"created_at"`
		Status      string `json:"status"`
	}
	alerts := make([]alertItem, 0)

	riskThFen := int64(100000)
	if cfg, e := w.service.db.getTransferConfigParsed(); e == nil && cfg != nil && cfg.RiskThresholdFen > 0 {
		riskThFen = cfg.RiskThresholdFen
	}
	minYuanLarge := float64(riskThFen) / 100.0

	type largeRow struct {
		TransferNo string  `db:"transfer_no"`
		FromUID    string  `db:"from_uid"`
		Amount     float64 `db:"amount"`
		CreatedAt  string  `db:"created_at"`
	}
	var larges []largeRow
	w.service.db.session.SelectBySql("SELECT transfer_no, from_uid, amount, created_at FROM `transfer` WHERE amount >= ? AND DATE(created_at)=CURDATE() ORDER BY created_at DESC LIMIT 20", minYuanLarge).Load(&larges)
	for i, l := range larges {
		alerts = append(alerts, alertItem{
			ID: int64(i + 1), Type: "large_amount", Title: "大额转账预警",
			Description: fmt.Sprintf("用户 %s 发起大额转账 ¥%.2f", l.FromUID, l.Amount),
			Level: "high", TransferNo: l.TransferNo, UID: l.FromUID, Amount: l.Amount,
			CreatedAt: l.CreatedAt, Status: "pending",
		})
	}
	c.JSON(http.StatusOK, gin.H{"alerts": alerts})
}

func (w *Wallet) transferExport(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type tfRow struct {
		TransferNo string  `db:"transfer_no"`
		FromUID    string  `db:"from_uid"`
		ToUID      string  `db:"to_uid"`
		Amount     float64 `db:"amount"`
		Status     int     `db:"status"`
		Remark     string  `db:"remark"`
		CreatedAt  string  `db:"created_at"`
	}
	var list []tfRow
	w.service.db.session.SelectBySql("SELECT transfer_no, from_uid, to_uid, amount, status, remark, created_at FROM `transfer` ORDER BY created_at DESC").Load(&list)

	buf := &bytes.Buffer{}
	buf.WriteString("\xEF\xBB\xBF")
	writer := csv.NewWriter(buf)
	_ = writer.Write([]string{"转账编号", "发送方", "接收方", "金额", "状态", "备注", "创建时间"})
	sMap := map[int]string{0: "待确认", 1: "已完成", 2: "已退回"}
	for _, item := range list {
		_ = writer.Write([]string{item.TransferNo, item.FromUID, item.ToUID, fmt.Sprintf("%.2f", item.Amount), sMap[item.Status], item.Remark, item.CreatedAt})
	}
	writer.Flush()
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=transfers.csv")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

func (w *Wallet) transferExportAnalytics(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type dayRow struct {
		Day string  `db:"day"`
		Cnt int     `db:"cnt"`
		Amt float64 `db:"amt"`
	}
	var days []dayRow
	w.service.db.session.SelectBySql("SELECT DATE(created_at) as day, COUNT(*) as cnt, COALESCE(SUM(amount),0) as amt FROM `transfer` WHERE created_at >= DATE_SUB(CURDATE(), INTERVAL 30 DAY) GROUP BY DATE(created_at) ORDER BY day").Load(&days)
	buf := &bytes.Buffer{}
	buf.WriteString("\xEF\xBB\xBF")
	writer := csv.NewWriter(buf)
	_ = writer.Write([]string{"日期", "转账数", "转账金额"})
	for _, d := range days {
		_ = writer.Write([]string{d.Day, fmt.Sprintf("%d", d.Cnt), fmt.Sprintf("%.2f", d.Amt)})
	}
	writer.Flush()
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=transfer_analytics.csv")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}
