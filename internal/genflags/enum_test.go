package genflags

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func enumCfg(name string, allowed []string, def string, mutators ...func(*FlagConfig)) FlagConfig {
	cfg := FlagConfig{
		Name:        name,
		Description: "test enum flag",
		Enabled:     true,
		DefaultEnum: def,
		Affects:     []string{"golang"},
		DependsOn:   map[string]bool{},
	}
	for _, m := range mutators {
		m(&cfg)
	}
	// AllowedValues хранится вне FlagConfig (на самом флаге), поэтому тут не
	// задаётся — передаётся в EnumFlag напрямую.
	_ = allowed
	return cfg
}

func TestEnumFlag_DefaultWhenNoOverride(t *testing.T) {
	t.Parallel()

	f := EnumFlag{FlagName: "FLAG_E", Allowed: []string{"silent", "warn", "error"}, FlagDefault: "warn"}
	cfg := enumCfg("FLAG_E", f.Allowed, "warn")

	val, err := f.ValidateOverride(nil, map[string]bool{}, cfg)
	require.NoError(t, err)
	assert.Equal(t, "warn", val)
}

func TestEnumFlag_ValidOverride(t *testing.T) {
	t.Parallel()

	f := EnumFlag{FlagName: "FLAG_E", Allowed: []string{"silent", "warn", "error"}, FlagDefault: "warn"}
	cfg := enumCfg("FLAG_E", f.Allowed, "warn")

	val, err := f.ValidateOverride("error", map[string]bool{}, cfg)
	require.NoError(t, err)
	assert.Equal(t, "error", val)
}

func TestEnumFlag_RejectsUnknownValue(t *testing.T) {
	t.Parallel()

	f := EnumFlag{FlagName: "FLAG_E", Allowed: []string{"silent", "warn", "error"}, FlagDefault: "warn"}
	cfg := enumCfg("FLAG_E", f.Allowed, "warn")

	_, err := f.ValidateOverride("loud", map[string]bool{}, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be one of")
}

func TestEnumFlag_RejectsNonString(t *testing.T) {
	t.Parallel()

	f := EnumFlag{FlagName: "FLAG_E", Allowed: []string{"silent", "warn", "error"}, FlagDefault: "warn"}
	cfg := enumCfg("FLAG_E", f.Allowed, "warn")

	_, err := f.ValidateOverride(true, map[string]bool{}, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be string")
}

func TestEnumFlag_DisabledOnlyDefault(t *testing.T) {
	t.Parallel()

	f := EnumFlag{FlagName: "FLAG_E", Allowed: []string{"silent", "warn", "error"}, FlagDefault: "warn"}
	cfg := enumCfg("FLAG_E", f.Allowed, "warn", func(c *FlagConfig) { c.Enabled = false })

	_, err := f.ValidateOverride("error", map[string]bool{}, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disabled")
}
