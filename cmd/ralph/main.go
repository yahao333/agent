package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/yahao333/ralph/internal/cli"
)

func main() {
	// 1. 日志：开发期用 Text，生产期可切 JSON
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	// 2. 信号处理：Ctrl+C 时优雅退出
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 3. 执行 cobra root command
	if err := cli.NewRootCmd().ExecuteContext(ctx); err != nil {
		slog.Error("execution failed", "err", err)
		os.Exit(1)
	}
}
