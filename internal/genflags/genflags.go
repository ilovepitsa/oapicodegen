// Package genflags предоставляет расширяемый registry generation-флагов.
//
// Generation-флаг — это именованный типизированный переключатель, влияющий на
// генерацию кода (например, GOLANG_SERVER_BODY_REQUEST_NO_AUTO_DEFAULTS,
// USE_REQUIRED_V2). Каждый флаг несёт FlagConfig (загружается из
// generation_flags.yaml), описывающий, разрешены ли override, значения
// default/target и ограничения по зависимостям.
//
// Registry владеет набором известных флагов и делегирует per-flag валидацию
// реализациям Flag. В T-reg.1 существуют только bool-флаги, поэтому BoolFlag —
// единственная реализация. Будущие типы флагов (enum, string) реализуют Flag
// и регистрируются самостоятельно.
//
// Слоистость: genflags НЕ должен импортировать пакет parser. Parser импортирует
// genflags и адаптирует существующий GenerationFlagConfig к FlagConfig.
package genflags

import (
	"errors"
	"fmt"
	"maps"
	"slices"
)

// FlagConfig — YAML-совместимая запись одной генерационной записи флага.
//
// YAML-ключи полей — camelCase, чтобы совпадать с внешним форматом
// generation_flags.yaml, который потребляет parser. Каждый тег подавляется
// индивидуально, потому что глобальное правило tagliatelle для yaml — snake_case.
type FlagConfig struct {
	Name string `yaml:"name"`
	// Description — человекочитаемое описание флага.
	Description string `yaml:"description"`
	// Enabled управляет тем, принимаются ли per-project override. Когда false,
	// разрешено только DefaultValue.
	Enabled bool `yaml:"enabled"`
	// DefaultValue — значение, используемое, когда проект не переопределяет флаг.
	DefaultValue bool `yaml:"defaultValue"` //nolint:tagliatelle // external camelCase key
	// TargetValue — цель миграции, к которой проекты должны стремиться.
	TargetValue bool `yaml:"targetValue"` //nolint:tagliatelle // external camelCase key
	// Affects — список платформ, которых касается флаг (например, "golang").
	Affects []string `yaml:"affects"`
	// DependsOn отображает имя флага-зависимости в его требуемое bool-значение.
	// Override этого флага (в не-default значение) допустим, только если каждая
	// зависимость в резолвнутом наборе совпадает с ожидаемым значением.
	DependsOn map[string]bool `yaml:"dependsOn"` //nolint:tagliatelle // external camelCase key
}

// Flag описывает один generation-флаг и правила валидации его override.
//
// Реализации инкапсулируют типоспецифичную валидацию (например, приведение к
// bool, membership в enum). Registry вызывает ValidateOverride с уже
// резолвнутыми значениями флагов проекта, чтобы можно было проверить зависимости.
type Flag interface {
	// Name — стабильный идентификатор, совпадающий с FlagConfig.Name.
	Name() string
	// Default — стёртый до any дефолт флага; используется, когда per-project
	// override не передан, а конфиг флага недоступен.
	Default() any
	// ValidateOverride проверяет per-project override против конфига флага и
	// возвращает валидированный bool. `resolved` несёт уже резолвнутые значения
	// флагов проекта — нужны для проверки DependsOn. Non-nil error отвергает
	// override.
	ValidateOverride(value any, resolved map[string]bool, cfg FlagConfig) (bool, error)
}

// BoolFlag — реализация Flag для boolean generation-флагов. Все флаги,
// поддерживаемые в T-reg.1, — boolean.
type BoolFlag struct {
	FlagName    string
	FlagDefault bool
}

// Name возвращает стабильный идентификатор флага.
func (b BoolFlag) Name() string {
	return b.FlagName
}

// Default возвращает дефолт флага как bool (стёртый до any, чтобы удовлетворять
// интерфейсу Flag).
func (b BoolFlag) Default() any {
	return b.FlagDefault
}

// ValidateOverride валидирует boolean per-project override против конфига флага
// и возвращает валидированный bool. Правила зеркалируют исходную логику parser:
//
//  1. value обязан быть bool; любой другой тип отвергается.
//  2. Если cfg.Enabled == false, разрешено только cfg.DefaultValue.
//  3. Если value совпадает с cfg.DefaultValue, override — no-op и принимается.
//  4. Каждая запись в cfg.DependsOn обязана присутствовать в `resolved` с
//     совпадающим bool-значением.
func (b BoolFlag) ValidateOverride(value any, resolved map[string]bool, cfg FlagConfig) (bool, error) { //nolint:lll // signature line
	boolValue, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf(
			"flag %q: override must be bool, got %T", cfg.Name, value,
		)
	}

	if !cfg.Enabled {
		if boolValue != cfg.DefaultValue {
			return false, fmt.Errorf(
				"flag %q is disabled, only default value %v is allowed",
				cfg.Name, cfg.DefaultValue,
			)
		}

		return boolValue, nil
	}

	if boolValue == cfg.DefaultValue {
		return boolValue, nil
	}

	for depName, depExpected := range cfg.DependsOn {
		depActual, found := resolved[depName]
		if !found {
			return false, fmt.Errorf(
				"flag %q: dependency %q is required but not found",
				cfg.Name, depName,
			)
		}

		if depActual != depExpected {
			return false, fmt.Errorf(
				"flag %q: dependency %q: expected %v, got %v",
				cfg.Name, depName, depExpected, depActual,
			)
		}
	}

	return boolValue, nil
}

// Registry — расширяемый набор известных generation-флагов. Флаги хранятся
// в порядке регистрации, поэтому итерация (Names) детерминирована.
type Registry struct {
	flags map[string]Flag
	order []string
}

// NewRegistry возвращает пустой Registry, готовый к вызовам Register.
func NewRegistry() *Registry {
	return &Registry{
		flags: make(map[string]Flag),
	}
}

// Register добавляет флаг в registry. Паникует, если флаг с тем же именем уже
// зарегистрирован — дублирующая регистрация это программная ошибка и должна
// всплывать при init, а не в рантайме.
func (r *Registry) Register(f Flag) {
	name := f.Name()
	if name == "" {
		panic("genflags: cannot register a flag with an empty name")
	}

	if _, exists := r.flags[name]; exists {
		panic(fmt.Sprintf("genflags: flag %q already registered", name))
	}

	r.flags[name] = f
	r.order = append(r.order, name)
}

// MustGet возвращает флаг по имени и паникует, если тот не зарегистрирован.
// Используется в коде, где отсутствие флага — программная ошибка (например,
// захардкоженные имена); для пользовательских lookup используйте Get.
func (r *Registry) MustGet(name string) Flag {
	f, ok := r.flags[name]
	if !ok {
		panic(fmt.Sprintf("genflags: flag %q not registered", name))
	}

	return f
}

// Get возвращает флаг по имени и bool-флаг наличия.
func (r *Registry) Get(name string) (Flag, bool) {
	f, ok := r.flags[name]

	return f, ok
}

// Names возвращает имена всех зарегистрированных флагов в порядке регистрации.
// Возвращаемый slice — копия; caller может мутировать его свободно.
func (r *Registry) Names() []string {
	return slices.Clone(r.order)
}

// ValidateConfig проверяет FlagConfig против структурных инвариантов
// зарегистрированного флага с совпадающим именем. Per-project override тут не
// учитываются. Правила:
//
//   - Флаг обязан быть зарегистрирован.
//   - cfg.Name обязан совпадать с именем зарегистрированного флага.
//   - Affects обязан содержать "golang".
//   - Когда DefaultValue совпадает с TargetValue, DependsOn обязан быть пустым
//     (флаг, который не может двигаться, не имеет осмысленных зависимостей).
//
// Non-nil error отвергает конфиг при loader.Load.
func (r *Registry) ValidateConfig(name string, cfg FlagConfig) error {
	f, ok := r.flags[name]
	if !ok {
		return fmt.Errorf("generation flag %q is not registered", name)
	}

	if cfg.Name != f.Name() {
		return fmt.Errorf(
			"config name %q does not match registered flag name %q",
			cfg.Name, f.Name(),
		)
	}

	if !slices.Contains(cfg.Affects, "golang") {
		return errors.New("'golang' is not found in the affects list")
	}

	if cfg.DefaultValue == cfg.TargetValue && len(cfg.DependsOn) != 0 {
		return errors.New(
			"flag with same default and target values must not contain dependsOn",
		)
	}

	return nil
}

// Resolve вычисляет финальное bool-значение флага для проекта. Nil-override
// даёт cfg.DefaultValue; любое другое значение проходит валидацию через
// ValidateOverride флага (которая отвергает non-bool и нарушения DependsOn),
// после чего возвращается.
//
// `resolved` несёт уже резолвнутые значения флагов проекта; используется для
// проверки DependsOn и не мутируется.
func (r *Registry) Resolve(
	name string,
	override any,
	resolved map[string]bool,
	cfg FlagConfig,
) (bool, error) {
	f, ok := r.flags[name]
	if !ok {
		return false, fmt.Errorf("generation flag %q is not registered", name)
	}

	if override == nil {
		return cfg.DefaultValue, nil
	}

	boolValue, err := f.ValidateOverride(override, resolved, cfg)
	if err != nil {
		return false, fmt.Errorf("resolve flag %q: %w", name, err)
	}

	return boolValue, nil
}

// CloneResolved возвращает shallow-копию map резолвнутых значений, чтобы
// caller не мог мутировать исходный map через возвращённую ссылку. Удобный
// хелпер для тестов и потребителей, которым нужен стабильный снапшот.
func CloneResolved(resolved map[string]bool) map[string]bool {
	return maps.Clone(resolved)
}
