// Package schema: StructRenderer рендерит object-схемы (struct definition).
// SetDefaults, ValidateOwn, Update<Name> рендерятся отдельными renderer'ами в
// том же pack'е (SetDefaultsRenderer, ValidateOwnRenderer, UpdateStructRenderer).
//
// StructRenderer НЕ использует OnStructProperty — он итерирует s.Properties
// напрямую в OnStruct/OnSplitStruct. Это соответствует существующей структуре
// renderStruct/renderSplitStruct: свойство может рендериться дважды (для
// <Name>Request и <Name>Response с разными фильтрами), а walker вызывает
// OnStructProperty один раз — паттерн не ложится на per-child хуки.
// OnStructProperty остаётся noop (наследуется от NoopSchemaRenderer) и будет
// задействован в более поздних тасках, когда renderer'ам понадобится
// per-field реактивность (например, URLFormRenderer).
package schema

import (
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/generator/walk"
	"nschugorev/oapigenerator/internal/parser"
	"strings"
)

// StructRenderer рендерит struct-определения для object-схем: <Name> и
// split-варианты <Name>Request + <Name>Response (через OnSplitStruct).
// SetDefaults/ValidateOwn/Update<Name> рендерятся отдельными renderer'ами в
// том же pack'е (SetDefaultsRenderer, ValidateOwnRenderer, UpdateStructRenderer).
//
// StructRenderer реализует SkipDescendants — walker не должен спускаться
// в properties, иначе вложенные $ref на object-схемы отрендерились бы
// повторно в тот же файл (например, Outer.inner → Inner: первый рендер
// в Inner.gen.go, второй — в Outer.gen.go). StructRenderer сам итерирует
// s.Properties в renderField, descent избыточен.
type StructRenderer struct {
	render.Base
	walk.NoopSchemaRenderer
}

// NewStructRenderer возвращает StructRenderer с нулевым состоянием. Buf и
// Imports вливаются через Base.Init в compose.FileComposer перед обходом.
func NewStructRenderer() *StructRenderer { return &StructRenderer{} }

// Skip реализует walk.SkipDescendants — StructRenderer не требует спуска
// walker'а в дочерние схемы: он итерирует Properties напрямую в renderField.
func (r *StructRenderer) Skip(_ *parser.Schema) bool { return true }

// OnStruct рендерит основную struct-схему:
//
//	type <Name> struct {
//	    <Field> <Type> `json:"..." yaml:"..."`
//	    ...
//	}
//
// SetDefaults/ValidateOwn рендерятся SetDefaultsRenderer/ValidateOwnRenderer
// в том же pack'е. Update<Name> рендерится UpdateStructRenderer'ом.
func (r *StructRenderer) OnStruct(s *parser.Schema) error {
	name := goName(s.Name)

	if s.Description != "" {
		writeDocComment(r.Buf, s.Description)
	}

	r.renderStructBody(s, name, nil)

	return nil
}

// OnSplitStruct рендерит <Name>Request и <Name>Response вместо одного <Name>,
// когда включён GOLANG_SPLIT_REQUEST_RESPONSE. Request: свойства с
// ReadOnly=false (writeOnly + regular). Response: свойства с WriteOnly=false
// (readOnly + regular). TypeMapper переключается между modeRequest и
// modeResponse, чтобы $ref на splittable-схемы разрешался корректно.
//
// Update<Name> для splittable-схем рендерится UpdateStructRenderer'ом в том
// же pack'е.
func (r *StructRenderer) OnSplitStruct(s *parser.Schema) error {
	name := goName(s.Name)

	if s.Description != "" {
		writeDocComment(r.Buf, s.Description)
	}

	r.Ctx.TypeMapper.SetMode(modeRequest)
	r.renderFilteredStruct(s, name+"Request", func(p *parser.Property) bool {
		return p.Schema == nil || !p.Schema.ReadOnly
	})

	r.Ctx.TypeMapper.SetMode(modeResponse)
	r.renderFilteredStruct(s, name+"Response", func(p *parser.Property) bool {
		return p.Schema == nil || !p.Schema.WriteOnly
	})

	return nil
}

// renderStructBody рендерит `type <Name> struct { ... }` с полями, проходящими
// фильтр keep (nil = все). Не рендерит SetDefaults/ValidateOwn — это
// ответственность других renderer'ов в pack'е.
func (r *StructRenderer) renderStructBody(
	s *parser.Schema,
	name string,
	keep func(*parser.Property) bool,
) {
	r.Buf.Print("type ", name, " struct {\n")

	for _, p := range s.Properties {
		if keep != nil && !keep(p) {
			continue
		}

		r.renderField(p)
	}

	r.Buf.Print("}\n\n")
}

// renderFilteredStruct рендерит struct с фильтром keep. Используется для
// <Name>Request/<Name>Response вариантов. ValidateOwn для split-вариантов
// рендерится ValidateOwnRenderer в том же pack'е.
func (r *StructRenderer) renderFilteredStruct(
	s *parser.Schema,
	name string,
	keep func(*parser.Property) bool,
) {
	r.Buf.Print("type ", name, " struct {\n")

	for _, p := range s.Properties {
		if !keep(p) {
			continue
		}

		r.renderField(p)
	}

	r.Buf.Print("}\n\n")
}

// currentMode делегирован в package-level currentMode (см. mode.go).
// Сохранён как метод-обёртка для обратной совместимости с существующими
// вызовами r.currentMode() внутри StructRenderer.
func (r *StructRenderer) currentMode() string {
	return currentMode(r.Ctx)
}

// renderField рендерит одно поле struct'ы. Логика:
//   - p.Optional && GOLANG_USE_OPTIONAL → optional.Optional[T] (импорт optional);
//   - поле optional (не required и не nilable) → *T;
//   - иначе — примитивный тип с json/yaml-тегом, omitempty если не required.
func (r *StructRenderer) renderField(p *parser.Property) {
	if p.Schema != nil && p.Schema.Description != "" {
		writeDocComment(r.Buf, p.Schema.Description)
	}

	if p.Schema != nil && p.Schema.Deprecated {
		r.Buf.Print("// Deprecated: schema marks this field as deprecated\n")
	}

	fieldName := goName(p.Name)
	fieldType := r.Ctx.TypeMapper.GoType(p.Schema)
	required := r.requiredForMode(p)

	if r.Ctx.Features.UseOptional.Value && p.Optional {
		r.Imports.Add(gogen.Import{Path: optionalPkg, Alias: "optional"})
		r.Buf.Print(fieldName, " optional.Optional[", fieldType, "] `json:\"", p.Name, "\" yaml:\"", p.Name, "\"`\n") //nolint:lll // struct tag line

		return
	}

	if fieldIsOptional(required, fieldType) {
		fieldType = "*" + fieldType
	}

	omitEmpty := ""
	if !required {
		omitEmpty = ",omitempty"
	}

	r.Buf.Print(fieldName, " ", fieldType, " `json:\"", p.Name, omitEmpty, "\" yaml:\"", p.Name, omitEmpty, "\"`\n") //nolint:lll // struct tag line
}

// requiredForMode делегирован в package-level requiredForMode (см. mode.go).
// Сохранён как метод-обёртку для обратной совместимости с существующими
// вызовами r.requiredForMode(p) внутри StructRenderer.
func (r *StructRenderer) requiredForMode(p *parser.Property) bool {
	return requiredForMode(r.Ctx, p)
}

// fieldIsOptional сообщает, нужно ли оборачивать поле в pointer.
// Поле optional, если оно не required и его Go-тип уже не nilable
// (не slice/map/any и не pointer).
//
// Дублировано из generator.fieldIsOptional.
func fieldIsOptional(required bool, fieldType string) bool {
	return !required && !strings.HasPrefix(fieldType, "*") && !isInherentlyNilable(fieldType)
}
