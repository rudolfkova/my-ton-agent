package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"

	"mytonstorage-agent/internal/checker"
	"mytonstorage-agent/internal/coordinator"
	agenthttp "mytonstorage-agent/internal/httpserver"
)

func main() {
	cfg := loadConfig()

	logLevel := slog.LevelInfo
	if level, ok := logLevels[cfg.LogLevel]; ok {
		logLevel = level
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))

	ch, err := checker.NewChecker(logger)
	if err != nil {
		logger.Error("failed to initialize checker", slog.String("error", err.Error()))
		os.Exit(1)
	}

	app := fiber.New()
	server := agenthttp.NewServer(app, cfg.AccessToken, ch, logger)
	server.RegisterRoutes()

	rootCtx, cancelRoot := context.WithCancel(context.Background())
	defer cancelRoot()

	coordClient := coordinator.New(
		cfg.CoordinatorURL,
		cfg.CoordinatorAccessToken,
		cfg.AgentID,
		cfg.AgentPublicURL,
		"v0.1.0",
		time.Duration(cfg.CoordinatorRequestTimeoutSec)*time.Second,
	)
	if coordClient.Enabled() {
		interval := time.Duration(cfg.RegistrationIntervalSec) * time.Second
		if interval <= 0 {
			interval = 15 * time.Second
		}
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			registered := false
			for {
				select {
				case <-rootCtx.Done():
					return
				default:
				}

				var err error
				if !registered {
					err = coordClient.Register(rootCtx)
					if err == nil {
						registered = true
						logger.Info("registered in coordinator", slog.String("coordinator_url", cfg.CoordinatorURL), slog.String("agent_id", cfg.AgentID))
					}
				} else {
					err = coordClient.Heartbeat(rootCtx)
				}
				if err != nil {
					logger.Warn("failed to report to coordinator", slog.String("error", err.Error()))
					registered = false
				}

				select {
				case <-rootCtx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	}

	go func() {
		if err := app.Listen(":" + cfg.Port); err != nil {
			logger.Error("failed to start http server", slog.String("error", err.Error()))
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	cancelRoot()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := app.ShutdownWithContext(ctx); err != nil {
		logger.Error("failed to shutdown server", slog.String("error", err.Error()))
	}
}
