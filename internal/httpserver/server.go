package httpserver

import (
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"mytonstorage-agent/internal/checker"
	"mytonstorage-agent/internal/constants"
	"mytonstorage-agent/internal/model"
)

type Server struct {
	app         *fiber.App
	accessToken string
	checker     checker.Checker
	logger      *slog.Logger
}

func NewServer(app *fiber.App, accessToken string, c checker.Checker, logger *slog.Logger) *Server {
	return &Server{
		app:         app,
		accessToken: accessToken,
		checker:     c,
		logger:      logger,
	}
}

func (s *Server) RegisterRoutes() {
	s.app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	s.app.Post("/internal/v1/jobs/provider-check", s.authMiddleware, s.handleProviderCheck)
}

func (s *Server) authMiddleware(c *fiber.Ctx) error {
	if s.accessToken == "" {
		return c.Next()
	}

	accessToken := c.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(accessToken), "bearer ") {
		accessToken = accessToken[7:]
	}
	if accessToken != s.accessToken {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	return c.Next()
}

func (s *Server) handleProviderCheck(c *fiber.Ctx) error {
	start := time.Now()

	var req model.ProviderCheckRequest
	if err := c.BodyParser(&req); err != nil {
		s.logger.Warn(
			"provider check request parse failed",
			slog.String("remote_ip", c.IP()),
			slog.String("error", err.Error()),
		)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid payload"})
	}
	if len(req.Contracts) == 0 {
		s.logger.Warn(
			"provider check request has empty contracts",
			slog.String("job_id", req.JobID),
			slog.String("provider_pubkey", req.Provider.PublicKey),
			slog.String("remote_ip", c.IP()),
		)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "contracts must not be empty"})
	}

	s.logger.Info(
		"provider check job received",
		slog.String("job_id", req.JobID),
		slog.String("provider_pubkey", req.Provider.PublicKey),
		slog.Int("contracts_count", len(req.Contracts)),
		slog.String("remote_ip", c.IP()),
	)

	result, err := s.checker.CheckProvider(c.Context(), req)
	if err != nil {
		s.logger.Error(
			"provider check failed",
			slog.String("job_id", req.JobID),
			slog.String("provider_pubkey", req.Provider.PublicKey),
			slog.Int("contracts_count", len(req.Contracts)),
			slog.String("error", err.Error()),
			slog.String("duration", time.Since(start).String()),
		)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "provider check failed"})
	}

	valid := 0
	for _, item := range result {
		if item.Reason == constants.ValidStorageProof {
			valid++
		}
	}

	s.logger.Info(
		"provider check job finished",
		slog.String("job_id", req.JobID),
		slog.String("provider_pubkey", req.Provider.PublicKey),
		slog.Int("contracts_count", len(req.Contracts)),
		slog.Int("result_count", len(result)),
		slog.Int("valid_count", valid),
		slog.String("duration", time.Since(start).String()),
	)

	return c.JSON(model.ProviderCheckResponse{
		JobID:  req.JobID,
		Result: result,
	})
}
