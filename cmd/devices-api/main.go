package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mtsous/devices-api/internal/config"
	"github.com/mtsous/devices-api/internal/device"
	"github.com/mtsous/devices-api/internal/health"
	"github.com/mtsous/devices-api/internal/postgres"
	"github.com/mtsous/devices-api/internal/router"
)

const shutdownTimeout = 10 * time.Second

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(context.Background(), logger); err != nil {
		logger.Error("server stopped with error", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Restore default signal handling once the first signal arrives,
	// so a second Ctrl+C kills the process immediately.
	context.AfterFunc(ctx, stop)

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("create database pool: %w", err)
	}
	defer pool.Close()

	if err := postgres.Migrate(ctx, pool); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	repo := postgres.NewDeviceRepository(pool)

	deviceService := device.NewService(repo, logNotifier{logger: logger}, logger)

	deviceHandler := device.NewHandler(deviceService, logger)

	srv := newServer(cfg, logger, deviceHandler)

	return serve(ctx, srv, logger)
}

func newServer(cfg config.Config, logger *slog.Logger, deviceHandler *device.Handler) *http.Server {
	handler := router.New(
		health.NewHandler(logger),
		deviceHandler,
	)

	return &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
}

func serve(ctx context.Context, srv *http.Server, logger *slog.Logger) error {
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("server listening", "addr", srv.Addr)
		serverErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("listen: %w", err)
	case <-ctx.Done():
	}

	logger.Info("shutting down", "timeout", shutdownTimeout.String())

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-serverErr; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("listen: %w", err)
	}

	logger.Info("server stopped")
	return nil
}

// logNotifier logs state changes. It is a placeholder until state changes are
// streamed to clients over SSE.
type logNotifier struct {
	logger *slog.Logger
}

func (n logNotifier) Notify(ctx context.Context, change device.StateChange) {
	n.logger.InfoContext(ctx, "device state changed",
		"device_id", change.DeviceID,
		"previous_state", change.Previous,
		"state", change.Current,
	)
}
