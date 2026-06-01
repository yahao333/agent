package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/yahao333/ralph/internal/cli"
)

func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func main() {
	os.Exit(run())
}

func run() int {
	// 1. 日志：级别通过 RALPH_LOG_LEVEL 环境变量配置，默认 info
	level := parseLogLevel(os.Getenv("RALPH_LOG_LEVEL"))
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(logger)

	// 2. 信号处理：Ctrl+C 时优雅退出
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 3. 执行 cobra root command
	if err := cli.NewRootCmd().ExecuteContext(ctx); err != nil {
		slog.Error("execution failed", "err", err)
		// 恢复默认信号处理（defer 不会在 os.Exit 前执行）
		stop()
		os.Exit(1)
	}
	return 0
}
