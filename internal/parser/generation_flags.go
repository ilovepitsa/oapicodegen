package parser

// Имена generation flags, поддерживаемых генератором. Совпадают с ключами
// в generation_flags.yaml (поле name каждой записи).
const (
	// FlagServerNoAutoDefaults — когда on, HTTP-server request-decoder не
	// вызывает SetDefaults() на теле запроса. Миграция: auto-defaults
	// выключаются, клиент обязан явно передавать все поля.
	FlagServerNoAutoDefaults = "GOLANG_SERVER_BODY_REQUEST_NO_AUTO_DEFAULTS"

	// FlagSplitRequestResponse — когда on, генерируются раздельные
	// <Name>Request и <Name>Response модели вместо одной мономодели.
	// Request: без readOnly, с writeOnly; Response: наоборот.
	FlagSplitRequestResponse = "GOLANG_SPLIT_REQUEST_RESPONSE"

	// FlagUseRequiredV2 — когда on, парсер учитывает list-атрибуты
	// x-request-required / x-response-required, переопределяющие required
	// для Request/Response моделей соответственно. Требует FlagSplitRequestResponse.
	FlagUseRequiredV2 = "USE_REQUIRED_V2"

	// FlagUseUTCForDateTime — когда on, поля time.Time сериализуются
	// и десериализуются в UTC (принудительный .UTC() в marshal/unmarshal).
	FlagUseUTCForDateTime = "USE_UTC_FOR_DATE_TIME"
)

// supportedFlags — список всех поддерживаемых флагов в стабильном порядке.
// Порядок важен для детерминированной валидации в Load.
//
//nolint:gochecknoglobals // stable ordered list for deterministic validation
var supportedFlags = []string{
	FlagServerNoAutoDefaults,
	FlagSplitRequestResponse,
	FlagUseRequiredV2,
	FlagUseUTCForDateTime,
}

// GenerationFlagConfig — одна запись из глобального generation_flags.yaml.
// Описывает правила работы флага: включён ли, какие default/target значения,
// какие платформы затрагивает, от каких флагов зависит.
// YAML-ключи camelCase — формат extern-конфига, менять нельзя.
type GenerationFlagConfig struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	// Enabled — разрешён ли override в per-project конфиге.
	// Disabled-флаги принимают только defaultValue.
	Enabled bool `yaml:"enabled"`
	// DependsOn — флаг→ожидаемое значение. Override этого флага в не-default
	// значении требует, чтобы все зависимости были выставлены проектом.
	DependsOn map[string]bool `yaml:"dependsOn"` //nolint:tagliatelle // external camelCase key
	// TargetValue — значение, к которому проекты должны мигрировать.
	TargetValue bool `yaml:"targetValue"` //nolint:tagliatelle // external camelCase key
	//nolint:tagliatelle // external camelCase key
	// DefaultValue — значение по умолчанию для проектов без override.
	DefaultValue bool `yaml:"defaultValue"`
	// Affects — платформы, на которые влияет флаг. Должен содержать "golang".
	Affects []string `yaml:"affects"`
}

// ProjectFeature — финальное значение флага для конкретного проекта.
type ProjectFeature struct {
	Value bool
}

// ProjectFeatures — резолюнутый набор флагов для проекта. Каждое поле
// соответствует одному флагу из supportedFlags.
type ProjectFeatures struct {
	ServerNoAutoDefaults ProjectFeature
	SplitRequestResponse ProjectFeature
	UseRequiredV2        ProjectFeature
	UseUTCForDateTime    ProjectFeature
}
