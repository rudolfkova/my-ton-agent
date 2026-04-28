package main

import (
	"log"
	"log/slog"

	"github.com/caarlos0/env/v11"
)

var logLevels = map[uint8]slog.Level{
	0: slog.LevelDebug,
	1: slog.LevelInfo,
	2: slog.LevelWarn,
	3: slog.LevelError,
}

type Config struct {
	Port                         string `env:"AGENT_PORT" envDefault:"9091"`
	AccessToken                  string `env:"AGENT_ACCESS_TOKEN" envDefault:""`
	LogLevel                     uint8  `env:"AGENT_LOG_LEVEL" envDefault:"1"` // 0 - debug, 1 - info, 2 - warn, 3 - error
	CoordinatorURL               string `env:"COORDINATOR_URL" envDefault:""`
	CoordinatorAccessToken       string `env:"COORDINATOR_ACCESS_TOKEN" envDefault:""`
	AgentID                      string `env:"AGENT_ID" envDefault:""`
	AgentPublicURL               string `env:"AGENT_PUBLIC_URL" envDefault:""`
	RegistrationIntervalSec      int    `env:"AGENT_REGISTRATION_INTERVAL_SEC" envDefault:"15"`
	CoordinatorRequestTimeoutSec int    `env:"AGENT_COORDINATOR_TIMEOUT_SEC" envDefault:"10"`
}

func loadConfig() *Config {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		log.Fatalf("failed to parse config: %v", err)
	}

	return cfg
}
