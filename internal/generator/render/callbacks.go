package render

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/parser"
)

// SchemaCallbacks bridges renderers к ещё не мигрированным методам Generator
// (SetDefaults, ValidateOwn, schemaTreeHasDefaults). Когда Tasks 10-11
// приземлятся, реализации переедут в render/, и интерфейс можно будет убрать.
//
// Импорты, которые Generator-сторона накапливает во временный typeMapper при
// вызове, дренажатся в ImportTracker renderer'а (см. generatorCallbacks в
// internal/generator/render_callbacks_adapter.go).
type SchemaCallbacks interface {
	RenderSetDefaults(
		w *codegen.BufferWriter,
		sh *parser.Schema,
		mode, name string,
		keep func(*parser.Property) bool,
	)
	RenderValidateOwn(
		w *codegen.BufferWriter,
		sh *parser.Schema,
		mode, name string,
		isUpdate bool,
		keep func(*parser.Property) bool,
	)
	SchemaTreeHasDefaults(sh *parser.Schema, keep func(*parser.Property) bool) bool
}
