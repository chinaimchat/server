package wallet

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	wkutil "github.com/TangSengDaoDao/TangSengDaoDaoServer/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/log"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"github.com/gin-gonic/gin"
)

type WalletService interface {
	VerifyPayPassword(uid string, password string) error
	DeductBalance(uid string, amount float64, txType, relatedID, remark string) error
	AddBalance(uid string, amount float64, txType, relatedID, remark string) error
}

type Wallet struct {
	ctx     *config.Context
	service *service
	log.Log
}

func (w *Wallet) Service() WalletService {
	return w.service
}

func New(ctx *config.Context) *Wallet {
	return &Wallet{
		ctx:     ctx,
		service: newService(ctx),
		Log:     log.NewTLog("Wallet"),
	}
}

func (w *Wallet) Route(r *wkhttp.WKHttp) {
	wallet := r.Group("/v1/wallet", w.ctx.AuthMiddleware(r))
	{
		wallet.GET("/balance", w.getBalance)
		wallet.POST("/password", w.setPayPassword)
		wallet.PUT("/password", w.changePayPassword)
		wallet.GET("/transactions", w.getTransactions)
		wallet.POST("/recharge", w.recharge)
		wallet.POST("/withdrawal/apply", w.withdrawalApply)
		wallet.GET("/withdrawal/fee-config", w.userWithdrawalFeeConfig)
		wallet.GET("/withdrawal/fee-preview", w.userWithdrawalFeePreview)
		wallet.GET("/withdrawal/list", w.userWithdrawalList)
		wallet.GET("/withdrawal/detail/:withdrawal_no", w.userWithdrawalDetail)
		wallet.GET("/overview", w.walletOverview)
		wallet.GET("/customer_services", w.userCustomerServices)
		wallet.GET("/recharge/channels", w.userRechargeChannels)
		wallet.POST("/recharge/apply", w.userRechargeApply)
		wallet.GET("/recharge/applications", w.userRechargeApplications)
	}
}

func (w *Wallet) getBalance(c *wkhttp.Context) {
	uid := c.GetLoginUID()
	avail, frozen, total, err := w.service.getWalletBalances(uid)
	if err != nil {
		c.ResponseError(err)
		return
	}
	hasPwd, _ := w.service.hasPayPassword(uid)
	c.JSON(http.StatusOK, gin.H{
		"status":          200,
		"balance":         avail,
		"usdt_available":  avail,
		"usdt_frozen":     frozen,
		"usdt_balance":    total,
		"has_password":    hasPwd,
	})
}

func (w *Wallet) setPayPassword(c *wkhttp.Context) {
	var req struct {
		Password string `json:"password"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(err)
		return
	}
	if len(req.Password) != 6 {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "msg": "密码必须为6位"})
		return
	}
	uid := c.GetLoginUID()
	err := w.service.setPayPassword(uid, req.Password)
	if err != nil {
		c.ResponseError(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

func (w *Wallet) changePayPassword(c *wkhttp.Context) {
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(err)
		return
	}
	uid := c.GetLoginUID()
	err := w.service.changePayPassword(uid, req.OldPassword, req.NewPassword)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

func (w *Wallet) getTransactions(c *wkhttp.Context) {
	uid := c.GetLoginUID()
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	txType := c.DefaultQuery("type", "")
	startDate := c.DefaultQuery("start_date", c.DefaultQuery("start", ""))
	endDate := c.DefaultQuery("end_date", c.DefaultQuery("end", ""))

	list, total, err := w.service.db.getTransactionsFiltered(uid, txType, startDate, endDate, page, size)
	if err != nil {
		c.ResponseError(err)
		return
	}

	respList := make([]gin.H, 0, len(list))
	for _, item := range list {
		respList = append(respList, gin.H{
			"id":             item.ID,
			"type":           item.Type,
			"title":          TxTypeTitle(item.Type),
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
		"total": total,
		"page":  page,
		"size":  size,
	})
}

func walletStatusText(st int) string {
	if st == 2 {
		return "冻结"
	}
	return "正常"
}

func (w *Wallet) walletOverview(c *wkhttp.Context) {
	uid := c.GetLoginUID()
	if _, err := w.service.ensureWallet(uid); err != nil {
		c.ResponseError(err)
		return
	}
	avail, frozen, total, err := w.service.getWalletBalances(uid)
	if err != nil {
		c.ResponseError(err)
		return
	}
	hasPwd, _ := w.service.hasPayPassword(uid)
	wm, _ := w.service.db.getWallet(uid)
	st := 1
	if wm != nil && wm.UID != "" {
		st = wm.Status
	}
	pending, _ := w.service.db.countPendingWithdrawals(uid)
	c.JSON(http.StatusOK, gin.H{
		"status":                   200,
		"balance":                  avail,
		"usdt_available":           avail,
		"usdt_frozen":              frozen,
		"usdt_balance":             total,
		"has_password":             hasPwd,
		"wallet_status":            st,
		"wallet_status_text":       walletStatusText(st),
		"pending_withdrawal_count": pending,
	})
}

func (w *Wallet) userCustomerServices(c *wkhttp.Context) {
	appID := c.GetAppID()
	var list []*customerServiceModel
	var err error
	if appID != "" {
		list, err = w.service.db.getHotlineCustomerServicesByAppID(appID)
	} else {
		list, err = w.service.db.getCustomerServiceList()
	}
	if err != nil {
		c.ResponseError(err)
		return
	}
	respList := make([]gin.H, 0, len(list))
	for _, item := range list {
		respList = append(respList, gin.H{
			"id":         item.ID,
			"name":       item.Name,
			"uid":        item.UID,
			"created_at": item.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	c.JSON(http.StatusOK, gin.H{"status": 200, "list": respList})
}

func (w *Wallet) userRechargeChannels(c *wkhttp.Context) {
	list, err := w.service.db.listEnabledRechargeChannels()
	if err != nil {
		c.ResponseError(err)
		return
	}
	out := make([]gin.H, 0, len(list))
	cfg := w.ctx.GetConfig()
	// 使用请求 Host 作为文件 URL 的 base，使返回的图片地址与客户端请求同源，避免跨域或内网域名不可达
	apiBase := wkutil.RequestAPIBaseURL(c.Request)
	if apiBase == "" {
		apiBase = cfg.External.APIBaseURL
	}
	for _, ch := range list {
		name := ch.Title
		qrImageFullURL := ""
		if p := strings.TrimSpace(ch.QrImageURL); p != "" {
			qrImageFullURL = wkutil.FullAPIURLForFilePreview(apiBase, p, cfg.Minio.UploadURL, cfg.Minio.DownloadURL)
		}
		iconFullURL := ""
		if p := strings.TrimSpace(ch.Icon); p != "" {
			iconFullURL = wkutil.FullAPIURLForFilePreview(apiBase, p, cfg.Minio.UploadURL, cfg.Minio.DownloadURL)
		}
		out = append(out, gin.H{
			"id":              ch.ID,
			"app_id":          ch.AppID,
			"pay_type":        ch.PayType,
			"pay_type_name":   RechargePayTypeName(ch.PayType),
			"icon":            iconFullURL,
			"qr_url":          ch.QrURL,
			"qr_image_url":    qrImageFullURL,
			"pay_address":     ch.PayAddress,
			"install_key":     ch.InstallKey,
			"title":           name,
			"channel_name":    name,
			"name":            name,
			"remark":          ch.Remark,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"status": 200,
		"list":   out,
		"data":   out,
	})
}

func rechargeApplicationStatusLabel(status int) string {
	switch status {
	case 0:
		return "待审核"
	case 1:
		return "已通过"
	case 2:
		return "已拒绝"
	default:
		return fmt.Sprintf("未知(%d)", status)
	}
}

func (w *Wallet) userRechargeApply(c *wkhttp.Context) {
	uid := c.GetLoginUID()
	var req struct {
		Amount    float64 `json:"amount"`
		AmountU   float64 `json:"amount_u"`
		ChannelID int64   `json:"channel_id"`
		PayType   int     `json:"pay_type"`
		Remark    string  `json:"remark"`
		ProofURL  string  `json:"proof_url"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(err)
		return
	}
	no, credited, uAmt, rate, err := w.service.SubmitRechargeApplication(uid, req.Amount, req.AmountU, req.ChannelID, req.PayType, req.Remark, req.ProofURL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":         200,
		"application_no": no,
		"amount":         credited,
		"amount_u":       uAmt,
		"exchange_rate":  rate,
	})
}

func (w *Wallet) userRechargeApplications(c *wkhttp.Context) {
	uid := c.GetLoginUID()
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	list, total, err := w.service.db.getRechargeApplicationsByUID(uid, page, size)
	if err != nil {
		c.ResponseError(err)
		return
	}
	out := make([]gin.H, 0, len(list))
	for _, item := range list {
		out = append(out, gin.H{
			"id":              item.ID,
			"application_no":  item.ApplicationNo,
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
			"created_at":      item.CreatedAt.Format("2006-01-02 15:04:05"),
			"updated_at":      item.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"status": 200,
		"list":   out,
		"total":  total,
		"page":   page,
		"size":   size,
	})
}

func (w *Wallet) userWithdrawalDetail(c *wkhttp.Context) {
	uid := c.GetLoginUID()
	no := c.Param("withdrawal_no")
	m, err := w.service.db.getWithdrawalByNoAndUID(no, uid)
	if err != nil {
		c.ResponseError(err)
		return
	}
	if m == nil {
		c.JSON(http.StatusNotFound, gin.H{"status": 404, "msg": "提现记录不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":            200,
		"id":                m.ID,
		"withdrawal_no":     m.WithdrawalNo,
		"amount":            m.Amount,
		"fee":               m.Fee,
		"actual_amount":     m.Amount - m.Fee,
		"withdrawal_status": m.Status,
		"status_text":       withdrawalStatusLabel(m.Status),
		"address":           m.Address,
		"remark":            m.Remark,
		"admin_remark":      m.AdminRemark,
		"created_at":        m.CreatedAt.Format("2006-01-02 15:04:05"),
		"updated_at":        m.UpdatedAt.Format("2006-01-02 15:04:05"),
	})
}

func (w *Wallet) recharge(c *wkhttp.Context) {
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

	err := w.service.AddBalance(req.UID, req.Amount, "recharge", "", "充值")
	if err != nil {
		c.ResponseError(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

func withdrawalStatusLabel(status int) string {
	switch status {
	case 0:
		return "待审核"
	case 1:
		return "已批准"
	case 2:
		return "已拒绝"
	case 3:
		return "已完成"
	default:
		return fmt.Sprintf("未知(%d)", status)
	}
}

func (w *Wallet) withdrawalApply(c *wkhttp.Context) {
	var req struct {
		Amount   float64 `json:"amount"`
		Address  string  `json:"address"`
		Remark   string  `json:"remark"`
		Password string  `json:"password"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(err)
		return
	}
	uid := c.GetLoginUID()
	no, fee, err := w.service.applyWithdrawal(uid, req.Amount, req.Address, req.Remark, req.Password)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":         200,
		"withdrawal_no":  no,
		"fee":            fee,
		"amount":         req.Amount,
		"actual_amount":  req.Amount - fee,
	})
}

func (w *Wallet) userWithdrawalFeeConfig(c *wkhttp.Context) {
	cfg, err := w.service.db.getWithdrawalConfig()
	if err != nil {
		c.ResponseError(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":    200,
		"fee_rate":  cfg.FeeRate,
		"fee_fixed": cfg.FeeFixed,
		"hint":      "fee_rate 为提现金额的百分比(如 1 表示 1%)；fee_fixed 为每笔另加的固定金额；实际到账 = 提现金额 - 手续费",
	})
}

func (w *Wallet) userWithdrawalFeePreview(c *wkhttp.Context) {
	amountStr := c.DefaultQuery("amount", "")
	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil || amount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "msg": "请传入有效参数 amount"})
		return
	}
	fee, err := w.service.ComputeWithdrawalFeeForAmount(amount)
	if err != nil {
		c.ResponseError(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":        200,
		"amount":        amount,
		"fee":           fee,
		"actual_amount": amount - fee,
	})
}

func (w *Wallet) userWithdrawalList(c *wkhttp.Context) {
	uid := c.GetLoginUID()
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	list, count, err := w.service.db.getWithdrawalsByUID(uid, page, size)
	if err != nil {
		c.ResponseError(err)
		return
	}
	respList := make([]gin.H, 0, len(list))
	for _, item := range list {
		respList = append(respList, gin.H{
			"id":            item.ID,
			"withdrawal_no": item.WithdrawalNo,
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
