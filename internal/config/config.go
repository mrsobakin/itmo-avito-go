package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTP              HTTP
	Database          Database
	LogLevel          slog.Level
	ShutdownTimeout   time.Duration
	IdempotencyKeyTTL time.Duration
}

type HTTP struct {
	Addr              string
	ReadTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

type Database struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	ConnectTimeout  time.Duration
	QueryTimeout    time.Duration
}

func Load() (Config, error) {
	var l loader
	cfg := Config{
		HTTP: HTTP{
			Addr:              l.string("HTTP_ADDR"),
			ReadTimeout:       l.duration("HTTP_READ_TIMEOUT"),
			ReadHeaderTimeout: l.duration("HTTP_READ_HEADER_TIMEOUT"),
			WriteTimeout:      l.duration("HTTP_WRITE_TIMEOUT"),
			IdleTimeout:       l.duration("HTTP_IDLE_TIMEOUT"),
		},
		Database: Database{
			URL:             l.string("DATABASE_URL"),
			MaxConns:        l.int32("DATABASE_MAX_CONNS"),
			MinConns:        l.int32("DATABASE_MIN_CONNS"),
			MaxConnLifetime: l.duration("DATABASE_MAX_CONN_LIFETIME"),
			ConnectTimeout:  l.duration("DATABASE_CONNECT_TIMEOUT"),
			QueryTimeout:    l.duration("DATABASE_QUERY_TIMEOUT"),
		},
		LogLevel:          l.logLevel("LOG_LEVEL"),
		ShutdownTimeout:   l.duration("SHUTDOWN_TIMEOUT"),
		IdempotencyKeyTTL: l.duration("IDEMPOTENCY_KEY_TTL"),
	}
	if l.err != nil {
		return Config{}, l.err
	}
	if cfg.Database.MinConns > cfg.Database.MaxConns {
		return Config{}, errors.New("DATABASE_MIN_CONNS must not exceed DATABASE_MAX_CONNS")
	}
	return cfg, nil
}

type loader struct {
	err error
}

func (l *loader) fail(format string, args ...any) {
	l.err = errors.Join(l.err, fmt.Errorf(format, args...))
}

func (l *loader) string(name string) string {
	value := os.Getenv(name)
	if value == "" {
		l.fail("%s is required", name)
	}
	return value
}

func (l *loader) duration(name string) time.Duration {
	value := l.string(name)
	if value == "" {
		return 0
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		l.fail("%s must be a positive duration", name)
	}
	return parsed
}

func (l *loader) int32(name string) int32 {
	value := l.string(name)
	if value == "" {
		return 0
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil || parsed < 0 {
		l.fail("%s must be a non-negative integer", name)
	}
	return int32(parsed)
}

func (l *loader) logLevel(name string) slog.Level {
	value := l.string(name)
	if value == "" {
		return 0
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(value)); err != nil {
		l.fail("%s: %w", name, err)
	}
	return level
}
