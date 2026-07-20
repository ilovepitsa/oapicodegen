// Package schema: StructRenderer рендерит object-схемы (struct definition +
// SetDefaults/ValidateOwn через callbacks) и их split/Update-варианты.
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

// StructRenderer рендерит struct-определения для object-схем: <Name>,
// опционально Update<Name> (когда sh.IsUsedInUpdate), и split-варианты
// <Name>Request + <Name>Response (через OnSplitStruct).
//
// SetDefaults и ValidateOwn делегируются в render.SchemaCallbacks —
// реализации живут на Generator'е (Tasks 10-11 переедут в render/).
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
//	// SetDefaults и ValidateOwn — через callbacks.
//
// Если у схемы выставлен IsUsedInUpdate, после основной структуры
// добавляется Update<Name>-вариант (PATCH/PUT request body).
func (r *StructRenderer) OnStruct(s *parser.Schema) error {
	name := goName(s.Name)

	if s.Description != "" {
		writeDocComment(r.Buf, s.Description)
	}

	r.renderStructBody(s, name, nil)

	cb := r.Ctx.Callbacks
	if cb != nil {
		if cb.SchemaTreeHasDefaults(s, nil) {
			cb.RenderSetDefaults(r.Buf, s, r.currentMode(), name, nil)
		}

		cb.RenderValidateOwn(r.Buf, s, r.currentMode(), name, false, nil)
	}

	if s.IsUsedInUpdate {
		r.renderUpdateStruct(s, name)
	}

	return nil
}

// OnSplitStruct рендерит <Name>Request и <Name>Response вместо одного <Name>,
// когда включён GOLANG_SPLIT_REQUEST_RESPONSE. Request: свойства с
// ReadOnly=false (writeOnly + regular). Response: свойства с WriteOnly=false
// (readOnly + regular). TypeMapper переключается между modeRequest и
// modeResponse, чтобы $ref на splittable-схемы разрешался корректно.
//
// Если у схемы выставлен IsUsedInUpdate, после split-вариантов добавляется
// Update<Name> (PATCH/PUT request body). Update рендерится с mode=Request
// (update-тело — request-контекст), поэтому $ref на splittable-схемы
// разрешаются в <Name>Request.
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

	if s.IsUsedInUpdate {
		r.renderUpdateStruct(s, name)
	}

	return nil
}

// renderStructBody рендерит `type <Name> struct { ... }` с полями, проходящими
// фильтр keep (nil = все). Не рендерит SetDefaults/ValidateOwn — это
// ответственность вызывающего.
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

// renderFilteredStruct рендерит struct с фильтром keep + SetDefaults +
// ValidateOwn. Используется для <Name>Request/<Name>Response вариантов.
// keep передаётся в callbacks как фильтр свойств.
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

	cb := r.Ctx.Callbacks
	if cb == nil {
		return
	}

	if cb.SchemaTreeHasDefaults(s, keep) {
		cb.RenderSetDefaults(r.Buf, s, r.currentMode(), name, keep)
	}

	cb.RenderValidateOwn(r.Buf, s, r.currentMode(), name, false, keep)
}

// currentMode извлекает текущий режим typeMapper'а. StructRenderer не хранит
// режим отдельно — он выставляется через TypeMapper.SetMode перед вызовом
// renderFilteredStruct, и callbacks читают тот же режим из typeMapper'а
// (через generatorCallbacks.mode, который копируется в свежий typeMapper).
func (r *StructRenderer) currentMode() string {
	if mg, ok := r.Ctx.TypeMapper.(modeGetter); ok {
		return mg.Mode()
	}

	return ""
}

// modeGetter — optional-интерфейс для чтения текущего режима typeMapper'а.
// typeMapperAdapter реализует его; тестовые fakes могут опускать.
type modeGetter interface {
	Mode() string
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

// requiredForMode возвращает, является ли поле required в текущем режиме
// генерации. Логика зависит от флага USE_REQUIRED_V2 (см. комментарий к
// generator.requiredForMode — там же SSOT). Режим читается через
// TypeMapper.SetMode — StructRenderer хранит режим в typeMapper'е, не локально.
func (r *StructRenderer) requiredForMode(p *parser.Property) bool {
	if !r.Ctx.Features.UseRequiredV2.Value {
		return p.Required
	}

	switch r.currentMode() {
	case modeRequest:
		return p.RequestRequired
	case modeResponse:
		return p.ResponseRequired
	default:
		// Моно-режим при v2 on: если поле есть в x-* списках, required
		// только если в обоих; иначе fallback на OAS required.
		if p.RequestRequired || p.ResponseRequired {
			return p.RequestRequired && p.ResponseRequired
		}

		return p.Required
	}
}

// fieldIsOptional сообщает, нужно ли оборачивать поле в pointer.
// Поле optional, если оно не required и его Go-тип уже не nilable
// (не slice/map/any и не pointer).
//
// Дублировано из generator.fieldIsOptional.
func fieldIsOptional(required bool, fieldType string) bool {
	return !required && !strings.HasPrefix(fieldType, "*") && !isInherentlyNilable(fieldType)
}

// renderUpdateStruct рендерит Update<Name> — вариант схемы для PUT/PATCH
// request body. Содержит только свойства с IsUsedInUpdate=true. Каждое поле
// оборачивается в optional.Optional[T] безусловно — это даёт PATCH-семантику:
// отличаем "поле не задано" от "поле = null" от "поле = значение".
//
// mode typeMapper'а выставляется в Request при включённом split, чтобы
// $ref на splittable-схемы разрешались в <Name>Request (update-тело —
// это request-контекст). Без split mode="" — refs разрешаются в базовое
// имя. Update-режим временно выставляется и восстанавливается после
// рендера, чтобы не повлиять на последующие OnStruct-вызовы.
//
// v1-ограничение: marker не помечает nested $ref / array items, поэтому
// Update-суффикс к ссылкам не применяется — refs указывают на исходные
// схемы (или их Request-вариант при split).
func (r *StructRenderer) renderUpdateStruct(s *parser.Schema, name string) {
	if s.Description != "" {
		writeDocComment(r.Buf, "Update"+name+" — PATCH/PUT variant of "+name+".")
	}

	originalMode := r.currentMode()

	updateMode := ""
	if r.Ctx.Features.SplitRequestResponse.Value {
		updateMode = modeRequest
	}

	r.Ctx.TypeMapper.SetMode(updateMode)

	r.Buf.Print("type Update", name, " struct {\n")

	for _, p := range s.Properties {
		if !p.IsUsedInUpdate {
			continue
		}

		r.renderUpdateField(p)
	}

	r.Buf.Print("}\n\n")

	r.renderUpdateGetters(s, name)

	if cb := r.Ctx.Callbacks; cb != nil {
		cb.RenderValidateOwn(r.Buf, s, updateMode, "Update"+name, true, func(p *parser.Property) bool {
			return p.IsUsedInUpdate
		})
	}

	r.Ctx.TypeMapper.SetMode(originalMode)
}

// renderUpdateField рендерит поле Update<Name>-структуры. Все поля
// оборачиваются в optional.Optional[T] безусловно — независимо от флага
// GOLANG_USE_OPTIONAL и метки p.Optional. Теги json/yaml без omitempty:
// presence определяется самой обёрткой Optional.
func (r *StructRenderer) renderUpdateField(p *parser.Property) {
	if p.Schema != nil && p.Schema.Description != "" {
		writeDocComment(r.Buf, p.Schema.Description)
	}

	if p.Schema != nil && p.Schema.Deprecated {
		r.Buf.Print("// Deprecated: schema marks this field as deprecated\n")
	}

	fieldName := goName(p.Name)
	fieldType := r.updateFieldType(p.Schema)

	r.Imports.Add(gogen.Import{Path: optionalPkg, Alias: "optional"})
	r.Buf.Print(fieldName, " optional.Optional[", fieldType, "] `json:\"", p.Name, "\" yaml:\"", p.Name, "\"`\n") //nolint:lll // struct tag line
}

// updateFieldType возвращает Go-тип поля Update<Name> без nullable-указателя
// — Optional уже различает null через IsNil. TypeMapper.GoType добавляет
// pointer для nullable-полей, что здесь избыточно. Поэтому мы используем
// adapter-метод BaseType, если доступен; иначе — GoType (менее точно, но
// корректно для тестовых fakes).
func (r *StructRenderer) updateFieldType(s *parser.Schema) string {
	if bt, ok := r.Ctx.TypeMapper.(baseTypeMapper); ok {
		return bt.BaseType(s)
	}

	return r.Ctx.TypeMapper.GoType(s)
}

// baseTypeMapper — optional-интерфейс для typeMapper'а, возвращающий
// Go-тип без nullable-указателя. typeMapperAdapter реализует его; тестовые
// fakes могут опускать.
type baseTypeMapper interface {
	BaseType(s *parser.Schema) string
}

// renderUpdateGetters рендерит Get<Field>() (*T, bool) методы для каждого
// поля Update<Name>. Семантика:
//   - !IsSet() → (nil, false) — поле не в запросе, не меняем.
//   - IsSet() && IsNil() → (nil, true) — пользователь прислал null, чистим.
//   - IsSet() && !IsNil() → (&value, true) — новое значение.
func (r *StructRenderer) renderUpdateGetters(s *parser.Schema, name string) {
	for _, p := range s.Properties {
		if !p.IsUsedInUpdate {
			continue
		}

		r.renderUpdateGetter(p, name)
	}
}

func (r *StructRenderer) renderUpdateGetter(p *parser.Property, name string) {
	fieldName := goName(p.Name)
	fieldType := r.updateFieldType(p.Schema)
	getterName := "Get" + fieldName

	r.Buf.Print("// ", getterName, " возвращает значение поля ", fieldName, " и флаг presence.\n")
	r.Buf.Print("// Семантика: (nil, false) — поле не задано; (nil, true) — задано как null;\n")
	r.Buf.Print("// (&value, true) — задано значением.\n")
	r.Buf.Print("func (u *Update", name, ") ", getterName, "() (*", fieldType, ", bool) {\n")
	r.Buf.Print("\tif !u.", fieldName, ".IsSet() {\n")
	r.Buf.Print("\t\treturn nil, false\n")
	r.Buf.Print("\t}\n")
	r.Buf.Print("\tif u.", fieldName, ".IsNil() {\n")
	r.Buf.Print("\t\treturn nil, true\n")
	r.Buf.Print("\t}\n")
	r.Buf.Print("\tv := u.", fieldName, ".Value()\n")
	r.Buf.Print("\treturn &v, true\n")
	r.Buf.Print("}\n\n")
}
