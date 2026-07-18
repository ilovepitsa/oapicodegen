package generator

import "nschugorev/oapigenerator/internal/parser"

const (
	oapiTypeObject  = "object"
	oapiTypeString  = "string"
	oapiTypeInteger = "integer"
	oapiTypeNumber  = "number"
	oapiTypeBoolean = "boolean"
	oapiTypeArray   = "array"

	oapiFormatInt32    = "int32"
	oapiFormatInt64    = "int64"
	oapiFormatFloat    = "float"
	oapiFormatDateTime = "date-time"
	oapiFormatDate     = "date"
	oapiFormatBinary   = "binary"

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

	// modeRequest/modeResponse — алиасы на parser-константы (SSOT).
	// parser.SchemaIndex.LookupForMode использует те же значения.
	modeRequest  = parser.ModeRequest
	modeResponse = parser.ModeResponse

	// optionalPkg — import-path runtime-пакета optional.Optional[T],
	// который сгенерированный код использует для x-optional полей при
	// включённом GOLANG_USE_OPTIONAL.
	optionalPkg = "nschugorev/oapigenerator/pkg/optional"
)
