// Package schema: ConvertersRenderer рендерит <Name>RequestToResponse функцию
// для splittable object-схемы с хотя бы одним shared-полем (не readOnly &&
// не writeOnly). Копирует shared-поля из Request в Response (shallow copy).
//
// Портирован из Generator.converterMethodsFile + renderRequestToResponse
// (internal/generator/converter_methods.go). Старый путь удаляется из
// generator.go (converterMethodsFile), сам файл converter_methods.go остаётся —
// schemaHasSharedFields используется Generator'ом для условия
// shouldGenerateConverters.
//
// Renderer embed'ит render.Base (Buf/Imports/Ctx) и walk.NoopSchemaRenderer.
// Рендер только в OnSplitStruct — конвертеры имеют смысл только для split-схем.
package schema

import (
	"github.com/ilovepitsa/oapicodegen/internal/generator/render"
	"github.com/ilovepitsa/oapicodegen/internal/generator/walk"
	"github.com/ilovepitsa/oapicodegen/internal/parser"
)

// ConvertersRenderer рендерит <Name>RequestToResponse для split-схем с
// shared-полями. OnStruct — noop (конвертеры имеют смысл только в split-режиме).
type ConvertersRenderer struct {
	render.Base
	walk.NoopSchemaRenderer
}

// NewConvertersRenderer возвращает ConvertersRenderer с нулевым состоянием.
// Buf и Imports вливаются через Base.Init в compose.FileComposer перед обходом.
func NewConvertersRenderer() *ConvertersRenderer { return &ConvertersRenderer{} }

// OnStruct — noop. Конвертеры <Name>RequestToResponse имеют смысл только для
// split-схем, рендерятся в OnSplitStruct.
func (r *ConvertersRenderer) OnStruct(_ *parser.Schema) error { return nil }

// OnSplitStruct рендерит <Name>RequestToResponse, если у схемы есть хотя бы
// одно shared-поле (не readOnly && не writeOnly).
func (r *ConvertersRenderer) OnSplitStruct(s *parser.Schema) error {
	defer r.Ctx.TypeMapper.SetMode("")
	r.Ctx.TypeMapper.SetMode(modeRequest)

	if !schemaHasSharedFields(s) {
		return nil
	}

	r.renderRequestToResponse(s, goName(s.Name))

	return nil
}

// renderRequestToResponse рендерит:
//
//	func <Name>RequestToResponse(req <Name>Request) <Name>Response {
//	    var resp <Name>Response
//	    resp.<SharedField> = req.<SharedField>
//	    // ...
//	    return resp
//	}
//
// Shared-поля копируются напрямую (pointer/struct/slice — shallow copy), КРОМЕ
// полей, чей тип — splittable-схема: их Request- и Response-варианты — разные
// Go-типы (*<Type>Request vs *<Type>Response), прямое присваивание не
// компилируется. Такие поля конвертируются через вложенный
// <Type>RequestToResponse (с квалификацией subpackage); см.
// isSplittableField / renderSplittableFieldConvert.
//
// Тело перенесено из Generator.renderRequestToResponse (converter_methods.go:55-78)
// с заменой w.Print → r.Buf.Print.
func (r *ConvertersRenderer) renderRequestToResponse(s *parser.Schema, name string) {
	r.Buf.Print("func ", name, "RequestToResponse(req ", name, "Request) ", name, "Response {\n")
	r.Buf.Print("\tvar resp ", name, "Response\n")

	for _, p := range s.Properties {
		if p.Schema == nil {
			continue
		}

		if p.Schema.ReadOnly || p.Schema.WriteOnly {
			continue
		}

		fieldName := goName(p.Name)

		if r.isSplittableField(p.Schema) {
			r.renderSplittableFieldConvert(p, fieldName)
			continue
		}

		r.Buf.Print("\tresp.", fieldName, " = req.", fieldName, "\n")
	}

	r.Buf.Print("\treturn resp\n")
	r.Buf.Print("}\n")
}

// isSplittableField сообщает, является ли тип поля splittable-схемой. Целевое
// имя берётся из $ref (refToName) или Schema.Name (когда rolodex резолвит
// inline). Поля-примитивы/enum/не-splittable объекты — false (прямое copy).
func (r *ConvertersRenderer) isSplittableField(s *parser.Schema) bool {
	if s == nil || r.Ctx == nil || r.Ctx.Splittable == nil {
		return false
	}

	targetName := s.Name
	if s.Ref != "" {
		targetName = refToName(s.Ref)
	}

	return targetName != "" && r.Ctx.Splittable[targetName]
}

// renderSplittableFieldConvert рендерит конверсию splittable-поля через вложенный
// <Type>RequestToResponse. converterCall — квалифицированное имя вызова
// (с subpackage), полученное из Request-типа поля (GoType в modeRequest) с
// суффиксом "ToResponse". Pointer-поле разыменовывается и переоборачивается.
func (r *ConvertersRenderer) renderSplittableFieldConvert(p *parser.Property, fieldName string) {
	r.Ctx.TypeMapper.SetMode(modeRequest)
	base := r.Ctx.TypeMapper.GoType(p.Schema)

	// Pointer-wrapping повторяет renderField: optional non-nilable поле → *T.
	required := requiredForMode(r.Ctx, p)
	pointer := fieldIsOptional(required, base)

	converterCall := base + "ToResponse"

	if !pointer {
		r.Buf.Print("\tresp.", fieldName, " = ", converterCall, "(req.", fieldName, ")\n")
		return
	}

	// pointer: deref, convert, re-wrap. nil → оставить nil.
	r.Buf.Print("\tif req.", fieldName, " != nil {\n")
	r.Buf.Print("\t\tv := ", converterCall, "(*req.", fieldName, ")\n")
	r.Buf.Print("\t\tresp.", fieldName, " = &v\n")
	r.Buf.Print("\t}\n")
}

// schemaHasSharedFields сообщает, есть ли у схемы хотя бы одно shared-поле
// (не readOnly && не writeOnly) — поле, существующее в обоих split-вариантах.
func schemaHasSharedFields(s *parser.Schema) bool {
	for _, p := range s.Properties {
		if p.Schema == nil {
			continue
		}

		if !p.Schema.ReadOnly && !p.Schema.WriteOnly {
			return true
		}
	}

	return false
}
