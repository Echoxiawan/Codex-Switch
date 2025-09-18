package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"codex-backup-tool/internal/api"
	"codex-backup-tool/internal/core"
)

func main() {
	configPath := flag.String("config", "config.json", "配置文件路径")
	flag.Parse()
	logger := log.New(os.Stdout, "[codex-backup] ", log.LstdFlags)
	cfg, usedDefaults, err := core.LoadConfig(*configPath)
	if err != nil {
		logger.Fatalf("加载配置失败: %v", err)
	}
	if usedDefaults {
		logger.Printf("未找到配置文件 %s，使用默认配置", *configPath)
	} else {
		logger.Printf("已加载配置文件 %s", *configPath)
	}
	svc, err := core.NewService(cfg, logger)
	if err != nil {
		logger.Fatalf("初始化服务失败: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	svc.Start(ctx)
	defer svc.Stop()

	mux := http.NewServeMux()
	api.New(svc).Register(mux)
	mountStatic(mux)

	addr := fmt.Sprintf(":%s", cfg.Port)
	srv := &http.Server{Addr: addr, Handler: loggingMiddleware(logger, mux)}

	go func() {
		logger.Printf("HTTP 服务启动，监听 %s", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatalf("HTTP 服务异常退出: %v", err)
		}
	}()

	if cfg.AutoOpenBrowser {
		go func() {
			time.Sleep(400 * time.Millisecond)
			url := fmt.Sprintf("http://localhost:%s", cfg.Port)
			if err := openBrowser(url); err != nil {
				logger.Printf("自动打开浏览器失败: %v", err)
			} else {
				logger.Printf("已尝试在浏览器打开 %s", url)
			}
		}()
	} else {
		logger.Println("已禁用自动打开浏览器，可手动访问服务页面")
	}

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Printf("HTTP 优雅关闭失败: %v", err)
	} else {
		logger.Println("HTTP 服务已停止")
	}
}

func mountStatic(mux *http.ServeMux) {
	webDir := "web"
	serveFile := func(path string) string {
		return filepath.Join(webDir, path)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, serveFile("index.html"))
	})
	fs := http.FileServer(http.Dir(webDir))
	mux.Handle("/style.css", fs)
	mux.Handle("/app.js", fs)
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func loggingMiddleware(logger *log.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		logger.Printf("%s %s %d %s", r.Method, r.URL.Path, rw.status, time.Since(start))
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}
