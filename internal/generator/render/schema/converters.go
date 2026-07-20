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
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/generator/walk"
	"nschugorev/oapigenerator/internal/parser"
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
// Shared-поля копируются напрямую (pointer/struct/slice — shallow copy).
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
		r.Buf.Print("\tresp.", fieldName, " = req.", fieldName, "\n")
	}

	r.Buf.Print("\treturn resp\n")
	r.Buf.Print("}\n")
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
