package buglog

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	instance *BugLogger
	once     sync.Once
)

type BugLogger struct {
	mu     sync.Mutex
	logDir string
}

func Init(logDir string) {
	once.Do(func() {
		instance = &BugLogger{logDir: logDir}
		if err := os.MkdirAll(logDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "[buglog] 无法创建日志目录 %s: %v\n", logDir, err)
		}
	})
}

func logFileName() string {
	return fmt.Sprintf("bug-%s.log", time.Now().Format("2006-01-02"))
}

func write(entry string) {
	if instance == nil {
		return
	}
	instance.mu.Lock()
	defer instance.mu.Unlock()

	fp := filepath.Join(instance.logDir, logFileName())
	f, err := os.OpenFile(fp, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[buglog] 写入失败: %v\n", err)
		return
	}
	defer f.Close()
	f.WriteString(entry)
}

func LogPanic(method, path, clientIP string, panicVal interface{}) {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	stack := string(buf[:n])

	ts := time.Now().Format("2006-01-02 15:04:05")
	entry := fmt.Sprintf(
		"================================================================================\n"+
			"[%s] PANIC\n"+
			"Request : %s %s\n"+
			"Client  : %s\n"+
			"Panic   : %v\n"+
			"Stack   :\n%s\n"+
			"================================================================================\n\n",
		ts, method, path, clientIP, panicVal, stack,
	)
	write(entry)
}

func LogError(method, path, clientIP string, statusCode int, errMsg string) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	entry := fmt.Sprintf(
		"--------------------------------------------------------------------------------\n"+
			"[%s] ERROR %d\n"+
			"Request : %s %s\n"+
			"Client  : %s\n"+
			"Message : %s\n"+
			"--------------------------------------------------------------------------------\n\n",
		ts, statusCode, method, path, clientIP, errMsg,
	)
	write(entry)
}

// Log 供业务代码主动记录 bug 级别事件
func Log(module, msg string, kvs ...string) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	var extra string
	for i := 0; i+1 < len(kvs); i += 2 {
		extra += fmt.Sprintf("  %s: %s\n", kvs[i], kvs[i+1])
	}
	entry := fmt.Sprintf(
		"- - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -\n"+
			"[%s] BUG 【%s】\n"+
			"%s\n"+
			"%s"+
			"- - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -\n\n",
		ts, module, msg, extra,
	)
	write(entry)
}

// CleanOldLogs 删除 maxAge 天前的 bug 日志文件
func CleanOldLogs(maxAge int) {
	if instance == nil {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -maxAge)
	entries, err := os.ReadDir(instance.logDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "bug-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		dateStr := strings.TrimSuffix(strings.TrimPrefix(name, "bug-"), ".log")
		t, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}
		if t.Before(cutoff) {
			fp := filepath.Join(instance.logDir, name)
			os.Remove(fp)
		}
	}
}
