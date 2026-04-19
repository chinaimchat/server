package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"

	_ "github.com/TangSengDaoDao/TangSengDaoDaoServer/internal"
	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/base/event"
	"github.com/TangSengDaoDao/TangSengDaoDaoServer/pkg/buglog"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/module"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/log"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/server"
	"github.com/gin-gonic/gin"
	"github.com/judwhite/go-svc"
	"github.com/robfig/cron"
	"github.com/spf13/viper"
)

// go ldflags
var Version string    // version
var Commit string     // git commit id
var CommitDate string // git commit date
var TreeState string  // git tree state

func loadConfigFromFile(cfgFile string) *viper.Viper {
	vp := viper.New()
	vp.SetConfigFile(cfgFile)
	_ = vp.ReadInConfig()
	return vp
}

func main() {
	var CfgFile string //config file
	flag.StringVar(&CfgFile, "config", "configs/tsdd.yaml", "config file")
	flag.Parse()
	vp := loadConfigFromFile(CfgFile)
	vp.SetEnvPrefix("ts")
	vp.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	vp.AutomaticEnv()

	gin.SetMode(gin.ReleaseMode)

	cfg := config.New()
	cfg.Version = Version
	cfg.ConfigureWithViper(vp)
	// Normalize external API base URL so clients never see doubled /v1 (e.g. .../api/v1/v1/... -> 404).
	cfg.External.APIBaseURL = normalizeExternalAPIBaseURL(cfg.External.APIBaseURL)

	// Runtime context shared by modules and HTTP stack.
	ctx := config.NewContext(cfg)
	ctx.Event = event.New(ctx)

	logOpts := log.NewOptions()
	logOpts.Level = cfg.Logger.Level
	logOpts.LineNum = cfg.Logger.LineNum
	logOpts.LogDir = cfg.Logger.Dir
	log.Configure(logOpts)

	var serverType string
	if len(os.Args) > 1 {
		serverType = strings.TrimSpace(os.Args[1])
		serverType = strings.Replace(serverType, "-", "", -1)
	}

	if serverType == "api" || serverType == "" || serverType == "config" { // default: run HTTP API
		runAPI(ctx)
	}

}

func runAPI(ctx *config.Context) {
	// Bug report log directory (same as Logger.Dir; default ./logs).
	bugLogDir := ctx.GetConfig().Logger.Dir
	if bugLogDir == "" {
		bugLogDir = "./logs"
	}
	buglog.Init(bugLogDir)

	// HTTP Server
	s := server.New(ctx)
	ctx.SetHttpRoute(s.GetRoute())
	// Write current API root into bundled web assets (assets/web/js/config.js) for legacy/embed clients.
	replaceWebConfig(ctx.GetConfig())
	// Tracing and access logging (routes are registered inside module.Setup).
	s.GetRoute().UseGin(ctx.Tracer().GinMiddle()) // Do not wrap api.Route here; module.Setup already registers handlers.
	s.GetRoute().UseGin(bugResponseLogger())
	s.GetRoute().UseGin(func(c *gin.Context) {
		ignorePaths := ignorePaths()
		for _, ignorePath := range ignorePaths {
			if ignorePath == c.FullPath() {
				c.Next()
				return
			}
		}
		gin.Logger()(c)
	})
	// Load feature modules and register HTTP routes.
	err := module.Setup(ctx)
	if err != nil {
		panic(err)
	}
	// Background cron jobs.
	cn := cron.New()
	// Periodic event push (expression uses robfig/cron extended format).
	err = cn.AddFunc("0/59 * * * * ?", func() {
		ctx.Event.(*event.Event).EventTimerPush()
	})
	if err != nil {
		panic(err)
	}
	// Daily at 02:00: prune old bug log files (keep last N days inside CleanOldLogs).
	err = cn.AddFunc("0 0 2 * * ?", func() {
		buglog.CleanOldLogs(3)
	})
	if err != nil {
		panic(err)
	}
	// One-shot cleanup at startup.
	go buglog.CleanOldLogs(3)
	cn.Start()

	// Pretty-print startup banner (ANSI) to the console.
	printServerInfo(ctx)

	// Block until the HTTP server stops.
	err = svc.Run(s)
	if err != nil {
		panic(err)
	}
}

// normalizeExternalAPIBaseURL collapses accidental /v1/v1 segments to a single /v1.
func normalizeExternalAPIBaseURL(u string) string {
	u = strings.TrimSpace(u)
	u = strings.TrimSuffix(u, "/")
	// Repeat until no consecutive /v1/v1 remains.
	for strings.Contains(u, "/v1/v1") {
		u = strings.ReplaceAll(u, "/v1/v1", "/v1")
	}
	return u
}

func printServerInfo(ctx *config.Context) {
	//lint:ignore ST1018 Banner uses terminal ANSI art; keep raw ESC sequences in the string.
	infoStr := `
[?25l[?7lLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLL
LLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLL
LLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLL
LLLLLLLLLLLL0CLLLLLLLLLLLLLLLLLLLLLLLLLL
LLLLLLLLLL08@880CfLLLLLLLLLLLLLLLLLLLLLL
LLLLLLLLfL8@8@@8LfLLLLLLLLLLLLLLLLLLLLLL
ffffffffft0@@8@8ffffffffffffffffffffffff
fffffffffCCL8@GLLfLLLfffffffffffffffffff
ffffffffCLLC0@GCCLLLLCffffffffffffffffff
ffffffffG0@@@@@@@8Ltffffffffffffffffffff
ffffffftC888888888Gtffffffffffffffffffff
ffffffftttttttttttttffffffffffffffffffff
fffffffttttttttttttfffffffffffffffffffff
tttttttttttttfftffttttttttttttttttfttttt
tttttttttttttttttttttttttttttttttttttttt
tttttttttttttttttttttttttttttttttttttttt
tttttttttttttttttttttttttttttttttttttttt
tttttttttttttttttttttttttttttttttttttttt
tttttttttttttttttttttttttttttttttttttttt
111t111111111tt1111111tt1111111t11111111[0m
[20A[9999999D[43C[0m[0m 
[43C[0m[1m[32mTangSengDaoDao is running[0m 
[43C[0m-------------------------[0m 
[43C[0m[1m[33mMode[0m[0m:[0m #mode#[0m 
[43C[0m[1m[33mConfig[0m[0m:[0m #configPath#[0m 
[43C[0m[1m[33mApp name[0m[0m:[0m #appname#[0m 
[43C[0m[1m[33mVersion[0m[0m:[0m #version#[0m 
[43C[0m[1m[33mGit[0m[0m:[0m #git#[0m 
[43C[0m[1m[33mGo build[0m[0m:[0m #gobuild#[0m 
[43C[0m[1m[33mIM URL[0m[0m:[0m #imurl#[0m 
[43C[0m[1m[33mFile Service[0m[0m:[0m #fileService#[0m 
[43C[0m[1m[33mThe API is listening at[0m[0m:[0m #apiAddr#[0m 

[43C[30m[40m   [31m[41m   [32m[42m   [33m[43m   [34m[44m   [35m[45m   [36m[46m   [37m[47m   [m
[43C[38;5;8m[48;5;8m   [38;5;9m[48;5;9m   [38;5;10m[48;5;10m   [38;5;11m[48;5;11m   [38;5;12m[48;5;12m   [38;5;13m[48;5;13m   [38;5;14m[48;5;14m   [38;5;15m[48;5;15m   [m






[?25h[?7h
	`
	cfg := ctx.GetConfig()
	infoStr = strings.Replace(infoStr, "#mode#", string(cfg.Mode), -1)
	infoStr = strings.Replace(infoStr, "#appname#", cfg.AppName, -1)
	infoStr = strings.Replace(infoStr, "#version#", cfg.Version, -1)
	infoStr = strings.Replace(infoStr, "#git#", fmt.Sprintf("%s-%s", CommitDate, Commit), -1)
	infoStr = strings.Replace(infoStr, "#gobuild#", runtime.Version(), -1)
	infoStr = strings.Replace(infoStr, "#fileService#", cfg.FileService.String(), -1)
	infoStr = strings.Replace(infoStr, "#imurl#", cfg.WuKongIM.APIURL, -1)
	infoStr = strings.Replace(infoStr, "#apiAddr#", cfg.Addr, -1)
	infoStr = strings.Replace(infoStr, "#configPath#", cfg.ConfigFileUsed(), -1)
	fmt.Println(infoStr)
}

func ignorePaths() []string {
	return []string{
		"/v1/robots/:robot_id/:app_key/events",
		"/v1/ping",
	}
}

func bugResponseLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		status := c.Writer.Status()
		if status >= 500 {
			errMsg := c.Errors.ByType(gin.ErrorTypePrivate).String()
			if errMsg == "" {
				errMsg = fmt.Sprintf("HTTP %d", status)
			}
			buglog.LogError(c.Request.Method, c.Request.URL.Path, c.ClientIP(), status, errMsg)
		}
	}
}

func replaceWebConfig(cfg *config.Config) {
	path := "./assets/web/js/config.js"
	newConfigContent := fmt.Sprintf(`const apiURL = "%s/"`, cfg.External.APIBaseURL)
	_ = os.WriteFile(path, []byte(newConfigContent), 0644)
}
