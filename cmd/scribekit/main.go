// Command scribekit 启动 ScribeKit 的单文件服务：REST、WebSocket 和内嵌前端。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/cloudwego/eino/callbacks"
	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"

	"github.com/chhongzh/scribekit/internal/agent/analyzer"
	"github.com/chhongzh/scribekit/internal/agent/scaffolder"
	"github.com/chhongzh/scribekit/internal/api"
	"github.com/chhongzh/scribekit/internal/config"
	"github.com/chhongzh/scribekit/internal/guide"
	"github.com/chhongzh/scribekit/internal/llm"
	"github.com/chhongzh/scribekit/internal/logging"
	"github.com/chhongzh/scribekit/internal/store"
	"github.com/chhongzh/scribekit/internal/ws"
	"github.com/chhongzh/scribekit/web"
)

func main() {
	logConfig := zap.NewProductionConfig()
	logConfig.Sampling = nil
	logger, err := logConfig.Build()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	zap.ReplaceGlobals(logger)
	callbacks.AppendGlobalHandlers(logging.EinoHandler())
	defer logger.Sync()
	cfg, err := config.Parse(os.Args[1:], os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		os.Exit(2)
	}
	if err := run(cfg); err != nil {
		slog.Error("scribekit 退出", "err", err)
		_ = logger.Sync()
		os.Exit(1)
	}
}

func run(cfg config.Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.DataDir)
	if err != nil {
		return fmt.Errorf("打开数据库: %w", err)
	}
	defer st.Close()

	guides, err := guide.Embedded()
	if err != nil {
		return fmt.Errorf("加载写作指引: %w", err)
	}
	models := llm.NewManager(st)
	hub := ws.NewHub(analyzer.New(st, models, guides), scaffolder.New(st, models, guides))
	app := api.NewApp(api.Deps{
		Store: st, Models: models, Guides: guides,
		WS: hub.Handler(ws.LocalOrigins(cfg.Addr)),
	}, web.Dist())

	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("监听 %s: %w", cfg.Addr, err)
	}
	warnIfExposed(ln.Addr())
	url := "http://" + browserHost(ln.Addr()) + "/"
	slog.Info("ScribeKit 已启动", "url", url, "data", cfg.DataDir)
	if !cfg.NoBrowser {
		go func() {
			if err := openBrowser(url); err != nil {
				slog.Warn("无法自动打开浏览器，请手动访问上面的地址", "err", err)
			}
		}()
	}

	err = app.Listener(ln, fiber.ListenConfig{
		DisableStartupMessage: true,
		GracefulContext:       ctx,
		ShutdownTimeout:       5 * time.Second,
	})
	if ctx.Err() != nil {
		slog.Info("已退出")
		return nil
	}
	return err
}

func warnIfExposed(addr net.Addr) {
	if tcp, ok := addr.(*net.TCPAddr); ok && !tcp.IP.IsLoopback() {
		slog.Warn("监听地址不是回环地址：ScribeKit 没有鉴权，局域网内任何人都能读写你的文章并使用你的 API Key", "addr", addr.String())
	}
}

// browserHost 把通配监听地址换成可在浏览器里打开的回环地址。
func browserHost(addr net.Addr) string {
	tcp, ok := addr.(*net.TCPAddr)
	if !ok {
		return addr.String()
	}
	ip := tcp.IP
	if ip == nil || ip.IsUnspecified() {
		ip = net.IPv4(127, 0, 0, 1)
	}
	return net.JoinHostPort(ip.String(), fmt.Sprint(tcp.Port))
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
