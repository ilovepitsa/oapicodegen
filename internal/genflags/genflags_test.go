package genflags

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// flagCfg собирает FlagConfig с разумными дефолтами для тестов. Caller передаёт
// функциональные override через variadic-mutators; базовый конфиг — валидный,
// включённый флаг без зависимостей.
func flagCfg(name string, mutators ...func(*FlagConfig)) FlagConfig {
	cfg := FlagConfig{
		Name:         name,
		Description:  "test flag",
		Enabled:      true,
		DefaultValue: false,
		TargetValue:  true,
		Affects:      []string{"golang"},
		DependsOn:    map[string]bool{},
	}

	for _, m := range mutators {
		m(&cfg)
	}

	return cfg
}

func TestRegistry_Register(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	r.Register(BoolFlag{FlagName: "FLAG_A"})
	r.Register(BoolFlag{FlagName: "FLAG_B"})

	names := r.Names()
	assert.Equal(t, []string{"FLAG_A", "FLAG_B"}, names)

	f, ok := r.Get("FLAG_A")
	require.True(t, ok)
	assert.Equal(t, "FLAG_A", f.Name())

	_, ok = r.Get("MISSING")
	assert.False(t, ok)
}

func TestRegistry_Register_Duplicate(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	r.Register(BoolFlag{FlagName: "FLAG_A"})

	assert.PanicsWithValue(
		t,
		"genflags: flag \"FLAG_A\" already registered",
		func() {
			r.Register(BoolFlag{FlagName: "FLAG_A"})
		},
	)
}

func TestRegistry_Register_EmptyName(t *testing.T) {
	t.Parallel()

	r := NewRegistry()

	assert.PanicsWithValue(
		t,
		"genflags: cannot register a flag with an empty name",
		func() {
			r.Register(BoolFlag{FlagName: ""})
		},
	)
}

func TestRegistry_MustGet(t *testing.T) {
	t.Parallel()

	t.Run("found", func(t *testing.T) {
		t.Parallel()

		r := NewRegistry()
		r.Register(BoolFlag{FlagName: "FLAG_A", FlagDefault: true})

		f := r.MustGet("FLAG_A")
		assert.Equal(t, "FLAG_A", f.Name())
		assert.Equal(t, true, f.Default())
	})

	t.Run("not_found_panics", func(t *testing.T) {
		t.Parallel()

		r := NewRegistry()

		assert.PanicsWithValue(
			t,
			"genflags: flag \"MISSING\" not registered",
			func() {
				_ = r.MustGet("MISSING")
			},
		)
	})
}

func TestRegistry_Names_ReturnsCopy(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	r.Register(BoolFlag{FlagName: "FLAG_A"})

	names := r.Names()
	names[0] = "MUTATED"

	// Исходный порядок в registry не должен меняться от мутаций caller.
	assert.Equal(t, []string{"FLAG_A"}, r.Names())
}

func TestBoolFlag_NameAndDefault(t *testing.T) {
	t.Parallel()

	f := BoolFlag{FlagName: "FLAG_X", FlagDefault: true}
	assert.Equal(t, "FLAG_X", f.Name())
	assert.Equal(t, true, f.Default())
}

func TestBoolFlag_ValidateOverride_Disabled(t *testing.T) {
	t.Parallel()

	flag := BoolFlag{FlagName: "FLAG_DISABLED"}
	cfg := flagCfg("FLAG_DISABLED", func(c *FlagConfig) {
		c.Enabled = false
		c.DefaultValue = false
		c.TargetValue = false
	})

	t.Run("default_value_allowed", func(t *testing.T) {
		t.Parallel()

		v, err := flag.ValidateOverride(false, map[string]bool{}, cfg)
		require.NoError(t, err)
		assert.False(t, v)
	})

	t.Run("non_default_rejected", func(t *testing.T) {
		t.Parallel()

		_, err := flag.ValidateOverride(true, map[string]bool{}, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "disabled")
		assert.Contains(t, err.Error(), "FLAG_DISABLED")
	})
}

func TestBoolFlag_ValidateOverride_DefaultEqualsTarget(t *testing.T) {
	t.Parallel()

	flag := BoolFlag{FlagName: "FLAG_NOOP"}
	cfg := flagCfg("FLAG_NOOP", func(c *FlagConfig) {
		c.DefaultValue = true
		c.TargetValue = true
	})

	// value == defaultValue → принимается как no-op, хотя и совпадает с target.
	v, err := flag.ValidateOverride(true, map[string]bool{}, cfg)
	require.NoError(t, err)
	assert.True(t, v)
}

func TestBoolFlag_ValidateOverride_DependsOn(t *testing.T) {
	t.Parallel()

	flag := BoolFlag{FlagName: "FLAG_DEP"}
	cfg := flagCfg("FLAG_DEP", func(c *FlagConfig) {
		c.DefaultValue = false
		c.TargetValue = true
		c.DependsOn = map[string]bool{"FLAG_PARENT": true}
	})

	t.Run("satisfied", func(t *testing.T) {
		t.Parallel()

		resolved := map[string]bool{"FLAG_PARENT": true}
		v, err := flag.ValidateOverride(true, resolved, cfg)
		require.NoError(t, err)
		assert.True(t, v)
	})

	t.Run("missing_dependency", func(t *testing.T) {
		t.Parallel()

		_, err := flag.ValidateOverride(true, map[string]bool{}, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "dependency")
		assert.Contains(t, err.Error(), "FLAG_PARENT")
	})

	t.Run("wrong_dependency_value", func(t *testing.T) {
		t.Parallel()

		resolved := map[string]bool{"FLAG_PARENT": false}
		_, err := flag.ValidateOverride(true, resolved, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected true")
		assert.Contains(t, err.Error(), "got false")
	})

	t.Run("default_value_skips_dependency_check", func(t *testing.T) {
		t.Parallel()

		// Override равен default → зависимости не вычисляются.
		v, err := flag.ValidateOverride(false, map[string]bool{}, cfg)
		require.NoError(t, err)
		assert.False(t, v)
	})
}

func TestBoolFlag_ValidateOverride_NonBool(t *testing.T) {
	t.Parallel()

	flag := BoolFlag{FlagName: "FLAG_TYPE"}
	cfg := flagCfg("FLAG_TYPE")

	_, err := flag.ValidateOverride("true", map[string]bool{}, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be bool")
	assert.Contains(t, err.Error(), "string")
}

func TestRegistry_Resolve_Default(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	r.Register(BoolFlag{FlagName: "FLAG_DEFAULTED"})
	cfg := flagCfg("FLAG_DEFAULTED", func(c *FlagConfig) {
		c.DefaultValue = false
	})

	value, err := r.Resolve("FLAG_DEFAULTED", nil, map[string]bool{}, cfg)
	require.NoError(t, err)
	assert.False(t, value)
}

func TestRegistry_Resolve_DefaultWhenEnabledTargetDifferent(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	r.Register(BoolFlag{FlagName: "FLAG_TARGETED"})
	cfg := flagCfg("FLAG_TARGETED", func(c *FlagConfig) {
		c.DefaultValue = false
		c.TargetValue = true
		c.Enabled = true
	})

	// Override равен default → defaultValue, НЕ targetValue.
	value, err := r.Resolve("FLAG_TARGETED", nil, map[string]bool{}, cfg)
	require.NoError(t, err)
	assert.False(t, value)
}

func TestRegistry_Resolve_Override(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	r.Register(BoolFlag{FlagName: "FLAG_OVR"})
	cfg := flagCfg("FLAG_OVR")

	value, err := r.Resolve("FLAG_OVR", true, map[string]bool{}, cfg)
	require.NoError(t, err)
	assert.True(t, value)
}

func TestRegistry_Resolve_OverrideInvalid(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	r.Register(BoolFlag{FlagName: "FLAG_OVR_DISABLED"})
	cfg := flagCfg("FLAG_OVR_DISABLED", func(c *FlagConfig) {
		c.Enabled = false
		c.DefaultValue = false
		c.TargetValue = false
	})

	_, err := r.Resolve("FLAG_OVR_DISABLED", true, map[string]bool{}, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "FLAG_OVR_DISABLED")
	assert.Contains(t, err.Error(), "resolve")
}

func TestRegistry_Resolve_UnregisteredFlag(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	cfg := flagCfg("NEVER_REGISTERED")

	_, err := r.Resolve("NEVER_REGISTERED", true, map[string]bool{}, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestRegistry_ValidateConfig_Success(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	r.Register(BoolFlag{FlagName: "FLAG_OK"})

	cfg := flagCfg("FLAG_OK")
	err := r.ValidateConfig("FLAG_OK", cfg)
	require.NoError(t, err)
}

func TestRegistry_ValidateConfig_UnregisteredFlag(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	cfg := flagCfg("NEVER_REGISTERED")

	err := r.ValidateConfig("NEVER_REGISTERED", cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestRegistry_ValidateConfig_NameMismatch(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	r.Register(BoolFlag{FlagName: "FLAG_REGISTERED"})

	cfg := flagCfg("FLAG_REGISTERED", func(c *FlagConfig) {
		c.Name = "WRONG_NAME"
	})

	err := r.ValidateConfig("FLAG_REGISTERED", cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match")
}

func TestRegistry_ValidateConfig_MissingGolangInAffects(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	r.Register(BoolFlag{FlagName: "FLAG_AFFECTS"})

	cfg := flagCfg("FLAG_AFFECTS", func(c *FlagConfig) {
		c.Affects = []string{"kotlin"}
	})

	err := r.ValidateConfig("FLAG_AFFECTS", cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "golang")
}

func TestRegistry_ValidateConfig_DefaultEqualsTargetWithDependsOn(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	r.Register(BoolFlag{FlagName: "FLAG_BAD_DEPS"})

	cfg := flagCfg("FLAG_BAD_DEPS", func(c *FlagConfig) {
		c.DefaultValue = false
		c.TargetValue = false
		c.DependsOn = map[string]bool{"FLAG_PARENT": true}
	})

	err := r.ValidateConfig("FLAG_BAD_DEPS", cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dependsOn")
}

func TestCloneResolved(t *testing.T) {
	t.Parallel()

	original := map[string]bool{"A": true, "B": false}
	clone := CloneResolved(original)

	// Мутация клона не должна влиять на оригинал.
	clone["A"] = false
	delete(clone, "B")

	assert.True(t, original["A"])
	_, stillThere := original["B"]
	assert.True(t, stillThere)
	assert.False(t, original["B"])
}

// Resolve обязан отвергать non-bool override через ValidateOverride. Этот
// error-path доказывает, что после валидации не нужен защитный type assertion.
func TestRegistry_Resolve_NonBoolOverride(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	r.Register(BoolFlag{FlagName: "FLAG_NON_BOOL"})
	cfg := flagCfg("FLAG_NON_BOOL")

	_, err := r.Resolve("FLAG_NON_BOOL", "true", map[string]bool{}, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be bool")
	assert.Contains(t, err.Error(), "string")
}

// ValidateConfig обязан проходить для disabled-флага, у которого default
// совпадает с target, при пустом DependsOn — это терминальное состояние
// полностью мигрировавшего флага.
func TestRegistry_ValidateConfig_DisabledFlagSuccess(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	r.Register(BoolFlag{FlagName: "FLAG_DISABLED_OK"})

	cfg := flagCfg("FLAG_DISABLED_OK", func(c *FlagConfig) {
		c.Enabled = false
		c.DefaultValue = true
		c.TargetValue = true
		c.DependsOn = map[string]bool{}
	})

	err := r.ValidateConfig("FLAG_DISABLED_OK", cfg)
	require.NoError(t, err)
}
