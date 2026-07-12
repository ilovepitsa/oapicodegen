package parser

import (
	"nschugorev/oapigenerator/internal/genflags"
)

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

	// FlagUseOptional — когда on, поля, помеченные x-optional, генерируются
	// как optional.Optional[T] вместо *T (трёхсостояние: absent / null / value).
	// Используется для PATCH/update-семантики, где нужно отличать "поле не
	// задано" от "поле явно null".
	FlagUseOptional = "GOLANG_USE_OPTIONAL"
)

// GenerationFlagConfig — alias для genflags.FlagConfig, YAML-совместимая запись
// из глобального generation_flags.yaml. Alias сохраняет обратную совместимость
// с существующим кодом парсера, делегируя валидацию в genflags.Registry.
type GenerationFlagConfig = genflags.FlagConfig

// ProjectFeature — финальное значение флага для конкретного проекта.
type ProjectFeature struct {
	Value bool
}

// ProjectFeatures — резолюнутый набор флагов для проекта. Каждое поле
// соответствует одному зарегистрированному флагу.
type ProjectFeatures struct {
	ServerNoAutoDefaults ProjectFeature
	SplitRequestResponse ProjectFeature
	UseRequiredV2        ProjectFeature
	UseUTCForDateTime    ProjectFeature
	UseOptional          ProjectFeature
}

// newDefaultRegistry создаёт Registry со всеми поддерживаемыми флагами.
// Регистрация выполняется в детерминированном порядке, что гарантирует
// стабильную итерацию в ValidateConfig и Resolve.
func newDefaultRegistry() *genflags.Registry {
	r := genflags.NewRegistry()
	r.Register(genflags.BoolFlag{FlagName: FlagServerNoAutoDefaults})
	r.Register(genflags.BoolFlag{FlagName: FlagSplitRequestResponse})
	r.Register(genflags.BoolFlag{FlagName: FlagUseRequiredV2})
	r.Register(genflags.BoolFlag{FlagName: FlagUseUTCForDateTime})
	r.Register(genflags.BoolFlag{FlagName: FlagUseOptional})

	return r
}
