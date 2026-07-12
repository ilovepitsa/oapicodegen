package generator

const (
	oapiTypeObject  = "object"
	oapiTypeString  = "string"
	oapiTypeInteger = "integer"
	oapiTypeNumber  = "number"
	oapiTypeBoolean = "boolean"

	oapiFormatInt32 = "int32"
	oapiFormatInt64 = "int64"
	oapiFormatFloat = "float"

	oapiParamPath   = "path"
	oapiParamQuery  = "query"
	oapiParamHeader = "header"
	oapiParamCookie = "cookie"

	oapiCodeDefault = "default"

	goTypeAny     = "any"
	goTypeString  = "string"
	goTypeBool    = "bool"
	goTypeInt     = "int"
	goTypeInt32   = "int32"
	goTypeInt64   = "int64"
	goTypeFloat32 = "float32"
	goTypeFloat64 = "float64"

	modeRequest  = "Request"
	modeResponse = "Response"

	// optionalPkg — import-path runtime-пакета optional.Optional[T],
	// который сгенерированный код использует для x-optional полей при
	// включённом GOLANG_USE_OPTIONAL.
	optionalPkg = "nschugorev/oapigenerator/pkg/optional"
)
