// Package logging — zap-логер, конфигурируемый из CLI-флагов. Замена
// git.mws-team.ru/mws/devp/platform-go/pkg/cli/logging.
//
// Конвейер: NewLoggerConfiguratorFromFlags(flagSet) → Create() → *zap.Logger.
// Caller обычно зовёт .Sugar() для SugaredLogger (Fatal/Fatalf/With/Warn/Panic).
package logging

import (
	"flag"
	"fmt"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Configurator хранит флаги логирования, зарегистрированные на FlagSet.
type Configurator struct {
	level       string
	format      string
	development bool
}

// NewLoggerConfiguratorFromFlags создаёт Configurator и регистрирует флаги:
//
//	-log-level        (default "info")    — debug|info|warn|error|fatal
//	-log-format       (default "console") — console|json
//	-log-development  (default false)     — zap development mode (stacktraces,
//	                                        no sampling, console-friendly)
func NewLoggerConfiguratorFromFlags(fs *flag.FlagSet) *Configurator {
	c := &Configurator{
		level:       "info",
		format:      "console",
		development: false,
	}
	fs.StringVar(&c.level, "log-level", c.level, "log level (debug, info, warn, error, fatal)")
	fs.StringVar(&c.format, "log-format", c.format, "log format (console, json)")
	fs.BoolVar(&c.development, "log-development", c.development, "use zap development mode (stacktraces, no sampling)")

	return c
}

// Create собирает *zap.Logger из зарегистрированных флагов. Возвращает ошибку
// при некорректном level или format.
func (c *Configurator) Create() (*zap.Logger, error) {
	level, err := parseLevel(c.level)
	if err != nil {
		return nil, err
	}

	var cfg zap.Config
	if c.development {
		cfg = zap.NewDevelopmentConfig()
	} else {
		cfg = zap.NewProductionConfig()
	}

	cfg.Level = zap.NewAtomicLevelAt(level)

	switch strings.ToLower(c.format) {
	case "console":
		cfg.Encoding = "console"
	case "json":
		cfg.Encoding = "json"
	default:
		return nil, fmt.Errorf("logging: unknown log format %q", c.format)
	}

	logger, err := cfg.Build()
	if err != nil {
		return nil, fmt.Errorf("init zap logger: %w", err)
	}

	return logger, nil
}

func parseLevel(s string) (zapcore.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return zapcore.DebugLevel, nil
	case "info":
		return zapcore.InfoLevel, nil
	case "warn", "warning":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	case "fatal":
		return zapcore.FatalLevel, nil
	default:
		return 0, fmt.Errorf("logging: unknown log level %q", s)
	}
}
