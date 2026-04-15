package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/layer-3/nitrolite-go-example/internal/config"
	internalhttp "github.com/layer-3/nitrolite-go-example/internal/httpapi"
	"github.com/layer-3/nitrolite-go-example/internal/nitrolite"
	"github.com/layer-3/nitrolite-go-example/internal/service"
	"github.com/layer-3/nitrolite-go-example/internal/signing"
	"github.com/layer-3/nitrolite-go-example/internal/store"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load(".env")
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger := newLogger(cfg.LogLevel)
	signer, err := signing.NewEnvSigner(cfg.DemoPrivateKey)
	if err != nil {
		logger.Error("failed to initialize signer", "error", err)
		os.Exit(1)
	}

	manager, err := nitrolite.NewSDKManager(ctx, cfg, signer, logger)
	if err != nil {
		logger.Error("failed to initialize nitrolite manager", "error", err)
		os.Exit(1)
	}

	appStore, err := store.New(cfg.SQLitePath)
	if err != nil {
		logger.Error("failed to initialize sqlite store", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := appStore.Close(); err != nil {
			logger.Error("failed to close sqlite store", "error", err)
		}
	}()
	if err := appStore.ClearLease(ctx); err != nil {
		logger.Error("failed to clear operator lease", "error", err)
		os.Exit(1)
	}

	runner := service.NewMerchantOperationRunner(cfg, appStore, manager, signer, logger)
	go manager.Run(ctx)
	go runner.Run(ctx)

	handler, err := internalhttp.NewHandler(cfg, manager, signer, appStore, logger)
	if err != nil {
		logger.Error("failed to build handler", "error", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("server shutdown failed", "error", err)
		}
	}()

	logger.Info("starting server", "addr", srv.Addr, "mode", "merchant-dashboard-reference-advanced")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server exited", "error", err)
		os.Exit(1)
	}
}

func newLogger(level string) *slog.Logger {
	var slogLevel slog.Level
	switch level {
	case "debug":
		slogLevel = slog.LevelDebug
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slogLevel})
	return slog.New(handler)
}
