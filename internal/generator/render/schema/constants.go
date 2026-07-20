package schema

import "nschugorev/oapigenerator/internal/parser"

// OpenAPI type/format-константы и Go-типы, используемые alias/enum/struct/json
// renderer'ами. Дублированы из generator.constants, чтобы render/schema
// не зависел от generator. Future task может вынести константы в общий
// пакет и убрать дублирование.
const (
	oapiTypeObject  = "object"
	oapiTypeString  = "string"
	oapiTypeInteger = "integer"
	oapiTypeNumber  = "number"
	oapiTypeBoolean = "boolean"
	oapiTypeArray   = "array"

	oapiFormatInt32 = "int32"
	oapiFormatInt64 = "int64"
	oapiFormatFloat = "float"

	// oapiFormatDateTime/Date/Binary — string-форматы, маппящиеся на
	// не-примитивные Go-типы (time.Time, []byte). Используются
	// SetDefaultsRenderer'ом для пропуска полей, чьи default-литералы
	// нельзя присвоить (см. isNonPrimitiveStringFormat).
	oapiFormatDateTime = "date-time"
	oapiFormatDate     = "date"
	oapiFormatBinary   = "binary"

	goTypeAny     = "any"
	goTypeFloat32 = "float32"
	goTypeFloat64 = "float64"

	// modeRequest/modeResponse — алиасы на parser-константы (SSOT).
	// Совпадают со значениями parser.ModeRequest/ModeResponse и
	// generator.modeRequest/modeResponse — typeMapper использует те же строки
	// для qualifyModelType (суффикс "Request"/"Response" splittable-схем).
	modeRequest  = parser.ModeRequest
	modeResponse = parser.ModeResponse

	// optionalPkg — import-path runtime-пакета optional.Optional[T].
	// Сгенерированный код использует его для x-optional полей при включённом
	// GOLANG_USE_OPTIONAL и безусловно для Update<Name> PATCH-вариантов.
	optionalPkg = "nschugorev/oapigenerator/pkg/optional"
)
