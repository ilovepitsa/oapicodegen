// Package schema: UpdateStructRenderer рендерит Update<Name>-вариант object-схемы
// для PUT/PATCH request body. Срабатывает на OnStruct/OnSplitStruct только если
// s.IsUsedInUpdate=true. Рендерит:
//   - type Update<Name> struct { ... } с полями в optional.Optional[T]
//   - Get<Field>() методы для каждого поля
//   - func (x Update<Name>) ValidateOwn(reg *validator.Registry) error (если есть правила)
//
// Портирован из StructRenderer.renderUpdateStruct + renderUpdateField +
// renderUpdateGetters + renderUpdateGetter + updateFieldType (struct.go).
// Update-вариант ValidateOwn рендерится через общий renderValidateOwnInto с
// isUpdate=true (fields wrapped in Optional[T]).
package schema

import (
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/generator/walk"
	"nschugorev/oapigenerator/internal/parser"
)

// UpdateStructRenderer рендерит Update<Name>-вариант object-схемы для PUT/PATCH
// request body. Renderer embed'ит render.Base (Buf/Imports/Ctx) и
// walk.NoopSchemaRenderer (остальные хуки не нужны). Skip не реализуется —
// StructRenderer первым в pack'е реализует SkipDescendants, поэтому walker не
// доходит до детей.
type UpdateStructRenderer struct {
	render.Base
	walk.NoopSchemaRenderer
}

// NewUpdateStructRenderer возвращает UpdateStructRenderer с нулевым состоянием.
// Buf и Imports вливаются через Base.Init в compose.FileComposer перед обходом.
func NewUpdateStructRenderer() *UpdateStructRenderer { return &UpdateStructRenderer{} }

// OnStruct рендерит Update<Name> для моно-режима (без split). mode
// typeMapper'а выставляется в "" — $ref на splittable-схемы разрешаются в
// базовое имя (Update-режим на рендер-пути не нужен).
func (r *UpdateStructRenderer) OnStruct(s *parser.Schema) error {
	defer r.Ctx.TypeMapper.SetMode("")
	r.Ctx.TypeMapper.SetMode("")

	if !s.IsUsedInUpdate {
		return nil
	}

	name := goName(s.Name)
	r.renderUpdateStruct(s, name)

	return nil
}

// OnSplitStruct рендерит Update<Name> для split-режима. Update-тело — это
// request-контекст, поэтому mode=modeRequest: $ref на splittable-схемы
// разрешаются в <Name>Request.
func (r *UpdateStructRenderer) OnSplitStruct(s *parser.Schema) error {
	defer r.Ctx.TypeMapper.SetMode("")

	if !s.IsUsedInUpdate {
		return nil
	}

	r.Ctx.TypeMapper.SetMode(modeRequest)
	name := goName(s.Name)
	r.renderUpdateStruct(s, name)

	return nil
}

// renderUpdateStruct рендерит Update<Name> — вариант схемы для PUT/PATCH
// request body. Содержит только свойства с IsUsedInUpdate=true. Каждое поле
// оборачивается в optional.Optional[T] безусловно — это даёт PATCH-семантику:
// отличаем "поле не задано" от "поле = null" от "поле = значение".
//
// mode typeMapper'а выставляется вызывающим (OnStruct="", OnSplitStruct=modeRequest).
// Здесь mode НЕ меняется: рендер использует текущий mode typeMapper'а для
// разрешения $ref. В конце вызывается renderValidateOwnInto с isUpdate=true и
// keep=IsUsedInUpdate.
//
// v1-ограничение: marker не помечает nested $ref / array items, поэтому
// Update-суффикс к ссылкам не применяется — refs указывают на исходные схемы
// (или их Request-вариант при split).
func (r *UpdateStructRenderer) renderUpdateStruct(s *parser.Schema, name string) {
	if s.Description != "" {
		writeDocComment(r.Buf, "Update"+name+" — PATCH/PUT variant of "+name+".")
	}

	r.Buf.Print("type Update", name, " struct {\n")

	for _, p := range s.Properties {
		if !p.IsUsedInUpdate {
			continue
		}

		r.renderUpdateField(p)
	}

	r.Buf.Print("}\n\n")

	r.renderUpdateGetters(s, name)

	renderValidateOwnInto(&r.Base, s, "Update"+name, true, func(p *parser.Property) bool {
		return p.IsUsedInUpdate
	})
}

// renderUpdateField рендерит поле Update<Name>-структуры. Все поля
// оборачиваются в optional.Optional[T] безусловно — независимо от флага
// GOLANG_USE_OPTIONAL и метки p.Optional. Теги json/yaml без omitempty:
// presence определяется самой обёрткой Optional.
func (r *UpdateStructRenderer) renderUpdateField(p *parser.Property) {
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

// renderUpdateGetters рендерит Get<Field>() (*T, bool) методы для каждого
// поля Update<Name>. Семантика:
//   - !IsSet() → (nil, false) — поле не в запросе, не меняем.
//   - IsSet() && IsNil() → (nil, true) — пользователь прислал null, чистим.
//   - IsSet() && !IsNil() → (&value, true) — новое значение.
func (r *UpdateStructRenderer) renderUpdateGetters(s *parser.Schema, name string) {
	for _, p := range s.Properties {
		if !p.IsUsedInUpdate {
			continue
		}

		r.renderUpdateGetter(p, name)
	}
}

// renderUpdateGetter рендерит один Get<Field>() метод для Update<Name>.
func (r *UpdateStructRenderer) renderUpdateGetter(p *parser.Property, name string) {
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

// updateFieldType возвращает Go-тип поля Update<Name> без nullable-указателя
// — Optional уже различает null через IsNil. TypeMapper.GoType добавляет
// pointer для nullable-полей, что здесь избыточно. Поэтому мы используем
// adapter-метод BaseType, если доступен; иначе — GoType (менее точно, но
// корректно для тестовых fakes).
func (r *UpdateStructRenderer) updateFieldType(s *parser.Schema) string {
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
