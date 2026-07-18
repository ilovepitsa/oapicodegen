package schema

// OpenAPI type/format-константы и Go-типы, используемые alias/enum
// renderer'ами. Дублированы из generator.constants, чтобы render/schema
// не зависел от generator. Future task может вынести константы в общий
// пакет и убрать дублирование.
const (
	oapiTypeObject  = "object"
	oapiTypeString  = "string"
	oapiTypeInteger = "integer"
	oapiTypeNumber  = "number"
	oapiTypeBoolean = "boolean"

	oapiFormatInt32 = "int32"
	oapiFormatInt64 = "int64"
	oapiFormatFloat = "float"

	goTypeAny     = "any"
	goTypeFloat32 = "float32"
	goTypeFloat64 = "float64"
)
