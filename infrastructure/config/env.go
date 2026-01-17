package config

import (
	"fmt"

	"github.com/caarlos0/env/v10"
	"github.com/go-playground/validator/v10"
)

type Config struct {
	Server struct {
		Port    int `env:"SERVER_PORT" envDefault:"8080"`
		Timeout int `env:"SERVER_TIMEOUT" envDefault:"30"`
	}

	Database struct {
		URL               string `env:"DATABASE_URL" validate:"required"`
		MaxConnections    int    `env:"DB_MAX_CONN" envDefault:"25"`
		ConnectionTimeout int    `env:"DB_CONN_TIMEOUT" envDefault:"5"`
	}

	RabbitMQ struct {
		URL           string `env:"RABBITMQ_URL" validate:"required"`
		Workers       int    `env:"RABBITMQ_WORKERS" envDefault:"5"`
		DLQTTL        int    `env:"RABBITMQ_DLQ_TTL_MS" envDefault:"30000"`
		PrefetchCount int    `env:"RABBITMQ_PREFETCH_COUNT" envDefault:"1"`
	}

	LegacyAPI struct {
		URL              string `env:"LEGACY_API_URL" validate:"required"`
		Timeout          int    `env:"LEGACY_API_TIMEOUT" envDefault:"30"`
		TimeoutSec       int    `env:"LEGACY_API_TIMEOUT_SEC" envDefault:"30"`
		RateLimit        int    `env:"LEGACY_API_RATE_LIMIT" envDefault:"100"`
		CircuitThreshold int    `env:"LEGACY_API_CIRCUIT_THRESHOLD" envDefault:"5"`
	}

	Outbox struct {
		PollIntervalSec int `env:"OUTBOX_POLL_INTERVAL_SEC" envDefault:"2"`
		FetchLimit      int `env:"OUTBOX_FETCH_LIMIT" envDefault:"100"`
	}

	CircuitBreaker struct {
		MaxFailures   int `env:"CB_MAX_FAILURES" envDefault:"5"`
		ResetTimeoutS int `env:"CB_RESET_TIMEOUT_SEC" envDefault:"60"`
	}

	SMTP struct {
		Host string `env:"SMTP_HOST" envDefault:""`
		Port int    `env:"SMTP_PORT" envDefault:"1025"`
	}

	CheckOut struct {
		DuplicateWindowSec int `env:"CHECKOUT_DUPLICATE_WINDOW_SEC" envDefault:"60"`
	}

	OpenTelemetry struct {
		Exporter     string `env:"OTEL_EXPORTER" envDefault:""`
		OtlpEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT" envDefault:""`
	}

	Environment string `env:"ENVIRONMENT" envDefault:"development"`
	LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`
	MetricsPort int    `env:"METRICS_PORT" envDefault:"9090"`
}

var Cfg *Config

func LoadConfig() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	validate := validator.New()
	if err := validate.Struct(cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	Cfg = cfg
	return cfg, nil
}
