package wkhttp

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/TangSengDaoDao/TangSengDaoDaoServer/pkg/buglog"
	"github.com/TangSengDaoDao/TangSengDaoDaoServer/pkg/cache"
	"github.com/TangSengDaoDao/TangSengDaoDaoServer/pkg/log"
	"github.com/gin-gonic/gin"
	"github.com/opentracing/opentracing-go"
	"go.uber.org/zap"
)

// UserRole 用户角色
type UserRole string

const (
	// Admin 管理员
	Admin UserRole = "admin"
	// SuperAdmin 超级管理员
	SuperAdmin UserRole = "superAdmin"
)

// WKHttp WKHttp
type WKHttp struct {
	r    *gin.Engine
	pool sync.Pool
}

// New New
func New() *WKHttp {
	l := &WKHttp{
		r:    gin.New(),
		pool: sync.Pool{},
	}
	l.r.Use(bugRecovery())
	l.pool.New = func() interface{} {
		return allocateContext()
	}
	return l
}

func allocateContext() *Context {
	return &Context{Context: nil, lg: log.NewTLog("context")}
}

// Context Context
type Context struct {
	*gin.Context
	lg log.Log
}

func (c *Context) reset() {
	c.Context = nil
}

// ResponseError ResponseError
func (c *Context) ResponseError(err error) {
	c.JSON(http.StatusBadRequest, gin.H{
		"msg":    err.Error(),
		"status": http.StatusBadRequest,
	})
}

// ResponseErrorf ResponseErrorf
func (c *Context) ResponseErrorf(msg string, err error) {
	if err != nil {
		c.lg.Error(msg, zap.Error(err), zap.String("path", c.FullPath()))
	}
	c.JSON(http.StatusBadRequest, gin.H{
		"msg":    msg,
		"status": http.StatusBadRequest,
	})
}

// ResponseErrorWithStatus ResponseErrorWithStatus
func (c *Context) ResponseErrorWithStatus(err error, status int) {
	c.JSON(http.StatusBadRequest, gin.H{
		"msg":    err.Error(),
		"status": status,
	})
}

// GetPage 获取页参数
func (c *Context) GetPage() (pageIndex int64, pageSize int64) {
	pageIndex, _ = strconv.ParseInt(c.Query("page_index"), 10, 64)
	pageSize, _ = strconv.ParseInt(c.Query("page_size"), 10, 64)
	if pageIndex <= 0 {
		pageIndex = 1
	}
	if pageSize <= 0 {
		pageSize = 15
	}
	return
}

// ResponseOK 返回成功
func (c *Context) ResponseOK() {
	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
	})
}

// Response Response
func (c *Context) Response(data interface{}) {
	c.JSON(http.StatusOK, data)
}

// ResponseWithStatus ResponseWithStatus
func (c *Context) ResponseWithStatus(status int, data interface{}) {
	c.JSON(status, data)
}

// GetLoginUID 获取当前登录的用户uid
func (c *Context) GetLoginUID() string {
	return c.MustGet("uid").(string)
}

// GetAppID appID
func (c *Context) GetAppID() string {
	return c.GetHeader("appid")
}

// GetLoginName 获取当前登录的用户名字
func (c *Context) GetLoginName() string {
	return c.MustGet("name").(string)
}

// GetLoginRole 获取当前登录用户的角色
func (c *Context) GetLoginRole() string {
	return c.GetString("role")
}

// GetSpanContext 获取当前请求的span context
func (c *Context) GetSpanContext() opentracing.SpanContext {
	return c.MustGet("spanContext").(opentracing.SpanContext)
}

// CheckLoginRole 检查登录角色权限
func (c *Context) CheckLoginRole() error {
	role := c.GetLoginRole()
	if role == "" {
		return errors.New("登录用户角色错误")
	}
	if role != string(Admin) && role != string(SuperAdmin) {
		return errors.New("该用户无权执行此操作")
	}
	return nil
}

// HandlerFunc HandlerFunc
type HandlerFunc func(c *Context)

// WKHttpHandler WKHttpHandler
func (l *WKHttp) WKHttpHandler(handlerFunc HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		hc := l.pool.Get().(*Context)
		hc.reset()
		hc.Context = c
		defer l.pool.Put(hc)

		handlerFunc(hc)

		//handlerFunc(&Context{Context: c})
	}
}

// Run Run
func (l *WKHttp) Run(addr ...string) error {
	return l.r.Run(addr...)
}

func (l *WKHttp) RunTLS(addr, certFile, keyFile string) error {
	return l.r.RunTLS(addr, certFile, keyFile)
}

// POST POST
func (l *WKHttp) POST(relativePath string, handlers ...HandlerFunc) {
	l.r.POST(relativePath, l.handlersToGinHandleFuncs(handlers)...)
}

// GET GET
func (l *WKHttp) GET(relativePath string, handlers ...HandlerFunc) {
	l.r.GET(relativePath, l.handlersToGinHandleFuncs(handlers)...)
}

// Any Any
func (l *WKHttp) Any(relativePath string, handlers ...HandlerFunc) {
	l.r.Any(relativePath, l.handlersToGinHandleFuncs(handlers)...)
}

// Static Static
func (l *WKHttp) Static(relativePath string, root string) {
	l.r.Static(relativePath, root)
}

// LoadHTMLGlob LoadHTMLGlob
func (l *WKHttp) LoadHTMLGlob(pattern string) {
	l.r.LoadHTMLGlob(pattern)
}

// UseGin UseGin
func (l *WKHttp) UseGin(handlers ...gin.HandlerFunc) {
	l.r.Use(handlers...)
}

// Use Use
func (l *WKHttp) Use(handlers ...HandlerFunc) {
	l.r.Use(l.handlersToGinHandleFuncs(handlers)...)
}

// ServeHTTP ServeHTTP
func (l *WKHttp) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// 兼容 Android 客户端 WKApiConfig 拼出的 /v1/v1/ 路径重复 bug:
	// 在路由匹配之前把 /v1/v1 归一为 /v1，对业务层完全透明。
	if strings.Contains(req.URL.Path, "/v1/v1") {
		req.URL.Path = strings.ReplaceAll(req.URL.Path, "/v1/v1", "/v1")
		if req.URL.RawPath != "" {
			req.URL.RawPath = strings.ReplaceAll(req.URL.RawPath, "/v1/v1", "/v1")
		}
	}
	l.r.ServeHTTP(w, req)
}

// Group Group
func (l *WKHttp) Group(relativePath string, handlers ...HandlerFunc) *RouterGroup {
	return newRouterGroup(l.r.Group(relativePath, l.handlersToGinHandleFuncs(handlers)...), l)
}

// HandleContext HandleContext
func (l *WKHttp) HandleContext(c *Context) {
	l.r.HandleContext(c.Context)
}

func bugRecovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rv := recover(); rv != nil {
				buglog.LogPanic(c.Request.Method, c.Request.URL.Path, c.ClientIP(), rv)
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()
		c.Next()
	}
}

func (l *WKHttp) handlersToGinHandleFuncs(handlers []HandlerFunc) []gin.HandlerFunc {
	newHandlers := make([]gin.HandlerFunc, 0, len(handlers))
	for _, handler := range handlers {
		newHandlers = append(newHandlers, l.WKHttpHandler(handler))
	}
	return newHandlers
}

// AuthMiddleware 认证中间件
func (l *WKHttp) AuthMiddleware(cache cache.Cache, tokenPrefix string) HandlerFunc {

	return func(c *Context) {
		// token 优先从 `token` 请求头读取；兼容 `Authorization: Bearer xxx` 或 query `token=xxx`。
		token := strings.TrimSpace(c.GetHeader("token"))
		if token == "" {
			// 兼容 Authorization: Bearer xxx / Authorization: xxx
			authz := strings.TrimSpace(c.GetHeader("Authorization"))
			if strings.HasPrefix(strings.ToLower(authz), "bearer ") {
				token = strings.TrimSpace(authz[len("bearer "):])
			} else if authz != "" {
				token = authz
			}
		}
		if token == "" {
			// 兼容常见自定义头
			token = strings.TrimSpace(c.GetHeader("X-Token"))
		}
		if token == "" {
			token = strings.TrimSpace(c.GetHeader("X-Access-Token"))
		}
		if token == "" {
			// 直接从原始 URL query 读取，避免 gin Query 在某些场景取不到
			token = strings.TrimSpace(c.Query("token"))
		}
		if token == "" {
			// 再兜底：从原始 URL query 读取
			token = strings.TrimSpace(c.Request.URL.Query().Get("token"))
		}
		if token == "" {
			// 最兜底：手动从 RawQuery 解析（规避 url.Values/Query 解析在某些场景下取不到）
			raw := ""
			if c.Request != nil && c.Request.URL != nil {
				raw = c.Request.URL.RawQuery
			}
			if raw != "" {
				parts := strings.Split(raw, "&")
				for _, p := range parts {
					kv := strings.SplitN(p, "=", 2)
					if len(kv) == 2 && kv[0] == "token" {
						// Query 参数一般是无空格，不强行 URLDecode，交给下游 cache 做严格匹配
						token = strings.TrimSpace(kv[1])
						break
					}
				}
			}
		}
		if token == "" {
			// 兼容 cookie（扫码落地页若由浏览器打开，可能带 cookie）
			if ck, err := c.Request.Cookie("token"); err == nil {
				token = strings.TrimSpace(ck.Value)
			}
		}
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"msg": "token不能为空，请先登录！",
			})
			return
		}
		uidAndName := GetLoginUID(token, tokenPrefix, cache)
		if strings.TrimSpace(uidAndName) == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"msg": "请先登录！",
			})
			return
		}
		uidAndNames := strings.Split(uidAndName, "@")
		if len(uidAndNames) < 2 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"msg": "token有误！",
			})
			return
		}
		c.Set("uid", uidAndNames[0])
		c.Set("name", uidAndNames[1])
		if len(uidAndNames) > 2 {
			c.Set("role", uidAndNames[2])
		}
		c.Next()
	}
}

// GetLoginUID GetLoginUID
func GetLoginUID(token string, tokenPrefix string, cache cache.Cache) string {
	uid, err := cache.Get(tokenPrefix + token)
	if err != nil {
		return ""
	}
	return uid
}

// RouterGroup RouterGroup
type RouterGroup struct {
	*gin.RouterGroup
	L *WKHttp
}

func newRouterGroup(g *gin.RouterGroup, l *WKHttp) *RouterGroup {
	return &RouterGroup{RouterGroup: g, L: l}
}

// POST POST
func (r *RouterGroup) POST(relativePath string, handlers ...HandlerFunc) {
	r.RouterGroup.POST(relativePath, r.L.handlersToGinHandleFuncs(handlers)...)
}

// GET GET
func (r *RouterGroup) GET(relativePath string, handlers ...HandlerFunc) {
	r.RouterGroup.GET(relativePath, r.L.handlersToGinHandleFuncs(handlers)...)
}

// HEAD HEAD（与 GET 共用处理器时用于 Glide/OkHttp 等先发 HEAD 探测）
func (r *RouterGroup) HEAD(relativePath string, handlers ...HandlerFunc) {
	r.RouterGroup.HEAD(relativePath, r.L.handlersToGinHandleFuncs(handlers)...)
}

// DELETE DELETE
func (r *RouterGroup) DELETE(relativePath string, handlers ...HandlerFunc) {
	r.RouterGroup.DELETE(relativePath, r.L.handlersToGinHandleFuncs(handlers)...)
}

// PUT PUT
func (r *RouterGroup) PUT(relativePath string, handlers ...HandlerFunc) {
	r.RouterGroup.PUT(relativePath, r.L.handlersToGinHandleFuncs(handlers)...)
}

// CORSMiddleware 跨域
func CORSMiddleware() HandlerFunc {

	return func(c *Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, token, accept, origin, Cache-Control, X-Requested-With, appid, noncestr, sign, timestamp")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT,DELETE,PATCH")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
