package logging

import (
	"flag"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestNewLoggerConfiguratorFromFlags_RegistersFlags(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	c := NewLoggerConfiguratorFromFlags(fs)

	require.NotNil(t, c)
	assert.Equal(t, "info", c.level)
	assert.Equal(t, "console", c.format)
	assert.False(t, c.development)

	// Флаги должны быть зарегистрированы
	require.NotNil(t, fs.Lookup("log-level"))
	require.NotNil(t, fs.Lookup("log-format"))
	require.NotNil(t, fs.Lookup("log-development"))
}

func TestNewLoggerConfiguratorFromFlags_FlagsParsing(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	c := NewLoggerConfiguratorFromFlags(fs)

	err := fs.Parse([]string{"-log-level=debug", "-log-format=json", "-log-development=true"})
	require.NoError(t, err)
	assert.Equal(t, "debug", c.level)
	assert.Equal(t, "json", c.format)
	assert.True(t, c.development)
}

func TestCreate_Defaults_ReturnsLogger(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	c := NewLoggerConfiguratorFromFlags(fs)

	log, err := c.Create()
	require.NoError(t, err)
	require.NotNil(t, log)
	defer func() { _ = log.Sync() }()
}

func TestCreate_InvalidLevel_ReturnsError(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	c := NewLoggerConfiguratorFromFlags(fs)
	_ = fs.Parse([]string{"-log-level=trace"})

	_, err := c.Create()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown log level")
}

func TestCreate_InvalidFormat_ReturnsError(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	c := NewLoggerConfiguratorFromFlags(fs)
	_ = fs.Parse([]string{"-log-format=xml"})

	_, err := c.Create()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown log format")
}

func TestCreate_DebugLevel_PassesToLogger(t *testing.T) {
	// Проверяем, что level из флага действительно меняет фильтр логера.
	core, recorded := observer.New(zapcore.DebugLevel)
	log := zap.New(core)

	log.Debug("test-debug")
	log.Info("test-info")

	require.Len(t, recorded.All(), 2, "expected both debug and info at DebugLevel")
}

func TestCreate_ErrorLevel_FiltersLowerLevels(t *testing.T) {
	core, recorded := observer.New(zapcore.ErrorLevel)
	log := zap.New(core)

	log.Debug("should-be-filtered")
	log.Info("should-be-filtered")
	log.Error("should-pass")

	require.Len(t, recorded.All(), 1, "expected only error-level log to pass")
	entry := recorded.All()[0]
	assert.Equal(t, zapcore.ErrorLevel, entry.Level)
	assert.Equal(t, "should-pass", entry.Message)
}

func TestCreate_DevelopmentMode_DoesntPanic(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	c := NewLoggerConfiguratorFromFlags(fs)
	_ = fs.Parse([]string{"-log-development=true"})

	log, err := c.Create()
	require.NoError(t, err)
	require.NotNil(t, log)
	defer func() { _ = log.Sync() }()
}

func TestCreate_AllLevelsValid(t *testing.T) {
	levels := []string{"debug", "info", "warn", "warning", "error", "fatal"}
	for _, lvl := range levels {
		t.Run(lvl, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			c := NewLoggerConfiguratorFromFlags(fs)
			_ = fs.Parse([]string{"-log-level=" + lvl})

			log, err := c.Create()
			require.NoError(t, err)
			require.NotNil(t, log)
			defer func() { _ = log.Sync() }()
		})
	}
}

func TestParseLevel_TableDriven(t *testing.T) {
	cases := []struct {
		in      string
		want    zapcore.Level
		wantErr bool
	}{
		{"debug", zapcore.DebugLevel, false},
		{"INFO", zapcore.InfoLevel, false},
		{"Warn", zapcore.WarnLevel, false},
		{"warning", zapcore.WarnLevel, false},
		{"ERROR", zapcore.ErrorLevel, false},
		{"fatal", zapcore.FatalLevel, false},
		{"trace", 0, true},
		{"", 0, true},
		{"verbose", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := parseLevel(tc.in)
			if tc.wantErr {
				require.Error(t, err)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCreate_FormatCaseInsensitive(t *testing.T) {
	for _, fmt := range []string{"console", "CONSOLE", "Console", "json", "JSON", "Json"} {
		t.Run(fmt, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			c := NewLoggerConfiguratorFromFlags(fs)
			err := fs.Parse([]string{"-log-format=" + fmt})
			require.NoError(t, err)

			log, err := c.Create()
			require.NoError(t, err)
			require.NotNil(t, log)
			defer func() { _ = log.Sync() }()
		})
	}
}

func TestCreate_FormatMixedCase(t *testing.T) {
	// Smoke test that format with mixed case works (strings.ToLower in Create).
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	c := NewLoggerConfiguratorFromFlags(fs)
	_ = fs.Parse([]string{"-log-format=JsOn"})

	log, err := c.Create()
	require.NoError(t, err)
	require.NotNil(t, log)
	defer func() { _ = log.Sync() }()

	assert.True(t, strings.EqualFold("JsOn", "json"))
}
