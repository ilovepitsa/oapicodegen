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
	"strings"

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

	// GoType вызывается для сравнения Request/Response-типов каждого поля и
	// добавляет import'ы как side-effect (qualifyUTCTime/qualifyModelType).
	// Для direct-copy полей (тип не эмитится в тело) import остаётся unused —
	// убираем висячие import'ы, иначе compile-error "imported and not used".
	r.Imports.PruneUnused(r.Buf.Content())

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
// полей, чьи Request- и Response-варианты — разные Go-типы (splittable $ref,
// резолвящийся в <Type>Request/<Type>Response): прямое присваивание не
// компилируется, такие поля конвертируются через вложенный <Type>RequestToResponse
// (с квалификацией subpackage); см. renderSplittableFieldConvert.
//
// Критерий конверсии — фактические Go-типы поля в modeRequest и modeResponse
// различаются. Это точнее name-based проверки Splittable: если тип поля не
// резолвится (any — следствие неразрешённого cross-file $ref, дефект rolodex),
// оба режима дают одинаковый any → прямой copy. Иначе name-based gate мог
// сработать по имени ref, а GoType дать any → эмитился undefined anyToResponse
// (регрессия v1.3.0).
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

		// Request- и Response-типы поля. Различаются только для splittable $ref,
		// резолвящегося в типизированные <Type>Request/<Type>Response — в этом
		// случае нужна конверсия; иначе прямой copy (включая неразрешённый any).
		r.Ctx.TypeMapper.SetMode(modeRequest)
		reqType := r.Ctx.TypeMapper.GoType(p.Schema)
		r.Ctx.TypeMapper.SetMode(modeResponse)
		respType := r.Ctx.TypeMapper.GoType(p.Schema)

		if reqType != respType {
			r.renderSplittableFieldConvert(p, fieldName, reqType, respType)
			continue
		}

		r.Buf.Print("\tresp.", fieldName, " = req.", fieldName, "\n")
	}

	r.Buf.Print("\treturn resp\n")
	r.Buf.Print("}\n")
}

// renderSplittableFieldConvert рендерит конверсию splittable-поля через вложенный
// <Type>RequestToResponse. reqType/respType — Go-типы поля в modeRequest/
// modeResponse (вычислены в renderRequestToResponse). Pointer-поле разыменовывается
// и переоборачивается: nullable-поля уже несут "*" в reqType (fieldIsOptional
// их не оборачивает повторно), optional-поля оборачиваются StructRenderer'ом в *T.
//
// Array-поля ([]<Item> → []<Item> с splittable item-типом) рендерятся per-item
// циклом — см. renderSplittableSliceConvert. Прямое `[]<Conv>(slice)` некорректно
// (Go трактует <Conv> как тип в type-conversion, а <Conv> — функция → compile error).
func (r *ConvertersRenderer) renderSplittableFieldConvert(p *parser.Property, fieldName, reqType, respType string) {
	// requiredForMode читает режим typeMapper'а — выставляем modeRequest, как
	// при рендере Request-варианта поля (renderField).
	r.Ctx.TypeMapper.SetMode(modeRequest)

	// Array of splittable items: per-item loop.
	if strings.HasPrefix(reqType, "[]") {
		r.renderSplittableSliceConvert(fieldName, reqType, respType)
		return
	}

	required := requiredForMode(r.Ctx, p)
	pointer := fieldIsOptional(required, reqType)

	converterCall := reqType + "ToResponse"

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

// renderSplittableSliceConvert рендерит конверсию array-поля с splittable
// item-типом per-item циклом:
//
//	resp.Items = make([]<ItemResp>, len(req.Items))
//	for i, v := range req.Items {
//	    resp.Items[i] = <ItemReq>ToResponse(v)
//	}
//
// elemReq/elemResp — item-типы (reqType/respType без префикса "[]"). Для
// pointer-элементов ([]*T) — deref + re-wrap через временную переменную, nil
// пропускается. converterCall строится из item-типа без "*", с суффиксом
// "ToResponse" (и квалификацией subpackage, если item — cross-subpackage ref).
func (r *ConvertersRenderer) renderSplittableSliceConvert(fieldName, reqType, respType string) {
	elemReq := strings.TrimPrefix(reqType, "[]")
	elemResp := strings.TrimPrefix(respType, "[]")
	converterCall := strings.TrimPrefix(elemReq, "*") + "ToResponse"

	r.Buf.Print("\tresp.", fieldName, " = make([]", elemResp, ", len(req.", fieldName, "))\n")
	r.Buf.Print("\tfor i, v := range req.", fieldName, " {\n")

	if strings.HasPrefix(elemReq, "*") {
		// pointer elements: deref, convert, re-wrap. nil → оставить nil.
		r.Buf.Print("\t\tif v != nil {\n")
		r.Buf.Print("\t\t\tt := ", converterCall, "(*v)\n")
		r.Buf.Print("\t\t\tresp.", fieldName, "[i] = &t\n")
		r.Buf.Print("\t\t}\n")
		r.Buf.Print("\t}\n")
		return
	}

	r.Buf.Print("\t\tresp.", fieldName, "[i] = ", converterCall, "(v)\n")
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
