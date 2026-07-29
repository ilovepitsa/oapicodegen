package genflags

import (
	"fmt"
	"slices"
	"strings"
)

// EnumFlag — реализация Flag для строковых enum-флагов. Значение обязано быть
// строкой из Allowed. Используется для tri-state флагов вроде
// GOLANG_SCHEMA_ANY (silent/warn/error).
//
// DependsOn для enum-флагов не применяется (зависимости имеют смысл только для
// bool-флагов миграции); cfg.DependsOn игнорируется.
type EnumFlag struct {
	FlagName    string
	Allowed     []string
	FlagDefault string
}

// Name возвращает стабильный идентификатор флага.
func (e EnumFlag) Name() string { return e.FlagName }

// Default возвращает дефолт флага как string (стёртый до any).
func (e EnumFlag) Default() any { return e.FlagDefault }

// ValidateOverride валидирует string per-project override против Allowed-множества.
// Правила:
//  1. value обязан быть string (nil → cfg.DefaultEnum); любой другой тип отвергается.
//  2. Если cfg.Enabled == false, разрешён только cfg.DefaultEnum.
//  3. Значение обязано входить в Allowed.
func (e EnumFlag) ValidateOverride(value any, _ map[string]bool, cfg FlagConfig) (any, error) {
	var s string
	if value == nil {
		s = cfg.DefaultEnum
	} else {
		str, ok := value.(string)
		if !ok {
			return e.FlagDefault, fmt.Errorf("flag %q: override must be string, got %T", cfg.Name, value)
		}
		s = str
	}

	if !cfg.Enabled && s != cfg.DefaultEnum {
		return e.FlagDefault, fmt.Errorf("flag %q is disabled, only default value %q is allowed", cfg.Name, cfg.DefaultEnum)
	}

	if !slices.Contains(e.Allowed, s) {
		return e.FlagDefault, fmt.Errorf("flag %q: value %q must be one of %s", cfg.Name, s, strings.Join(e.Allowed, ", "))
	}

	return s, nil
}
