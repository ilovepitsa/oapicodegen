package generator

import (
	"fmt"
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/parser"
	"strings"
)

// schemaFile генерирует Go-файл с определением типа для схемы, не
// обрабатываемой composer-путём (array / union / allof). Object-struct и
// split-схемы рендерятся через StructRenderer (Task 8) в renderSchemaFile.
// alias/enum/mapAlias — через AliasRenderer/EnumRenderer (Task 7).
//
// Update<Name>-вариант не рендерится здесь: для object-схем его генерирует
// StructRenderer.OnStruct при sh.IsUsedInUpdate. union/array/allof-схемы с
// IsUsedInUpdate игнорируются (update-marker помечает только top-level
// $ref-схемы, которые практически всегда object-struct — см. update_marker.go).
func (g *Generator) schemaFile(sh *parser.Schema) codegen.File {
	m := g.newTypeMapper("model")
	body := g.renderSchema(sh, m)

	return g.factory.Create(&gogen.File{
		Package: "model",
		Imports: m.imports,
		Body:    body,
	})
}

// renderSchema рендерит body для array/union/allof-схем. Object-struct и
// split-схемы рендерятся через StructRenderer (Task 8) — см. renderSchemaFile;
// alias/enum/mapAlias — через AliasRenderer/EnumRenderer (Task 7). Здесь
// остаются только те типы, которые Task 9 мигрирует позже.
func (g *Generator) renderSchema(sh *parser.Schema, m *typeMapper) []byte {
	w := codegen.NewBufferWriter()
	name := goName(sh.Name)

	if sh.Description != "" {
		writeDocComment(w, sh.Description)
	}

	switch {
	case len(sh.OneOf) > 0 || len(sh.AnyOf) > 0:
		g.renderUnion(w, sh, m, name)
	case len(sh.AllOf) == 1 && sh.AllOf[0].Ref == "" && sh.AllOf[0].Type != oapiTypeObject:
		// allOf с единственным inline-не-object членом трактуется как alias.
		// Редиректим через render/schema.AliasRenderer, переиспользуя
		// composer-инфраструктуру (Task 7) для консистентности.
		g.renderAllOfSingleInlineAlias(w, sh, m, name)
	case len(sh.AllOf) > 0:
		g.renderAllOf(w, sh, m, name)
	case sh.Type == oapiTypeArray:
		g.renderArraySchema(w, sh, m, name)
	}

	return w.Content()
}

// renderAllOfSingleInlineAlias рендерит `type <Name> <GoType>` для allOf с
// единственным inline-не-object членом. Логика перенесена из старого
// renderAlias (Task 7): тот же m.goType(sh.AllOf[0]), но вызывается только
// из renderSchema для allOf-single-inline-alias кейса.
func (g *Generator) renderAllOfSingleInlineAlias(
	w *codegen.BufferWriter,
	sh *parser.Schema,
	m *typeMapper,
	name string,
) {
	w.Print("type ", name, " ", m.goType(sh.AllOf[0]), "\n")
}

// renderField рендерит одно поле struct'ы. Используется renderAllOf для
// inline object-членов allOf. Основной struct-рендер мигрировал в
// render/schema/StructRenderer (Task 8), но renderAllOf останется в
// generator до Task 9 — поэтому этот метод сохраняется здесь.
func (g *Generator) renderField(w *codegen.BufferWriter, p *parser.Property, m *typeMapper) { //nolint:lll // function signature
	if p.Schema != nil && p.Schema.Description != "" {
		writeDocComment(w, p.Schema.Description)
	}

	if p.Schema != nil && p.Schema.Deprecated {
		w.Print("// Deprecated: schema marks this field as deprecated\n")
	}

	fieldName := goName(p.Name)
	fieldType := m.goType(p.Schema)
	required := g.requiredForMode(p, m.mode)

	if g.project.Features.UseOptional.Value && p.Optional {
		m.addImport(optionalPkg, "optional")
		w.Print(fieldName, " optional.Optional[", fieldType, "] `json:\"", p.Name, "\" yaml:\"", p.Name, "\"`\n") //nolint:lll // struct tag line

		return
	}

	if fieldIsOptional(required, fieldType) {
		fieldType = "*" + fieldType
	}

	omitEmpty := ""
	if !required {
		omitEmpty = ",omitempty"
	}

	w.Print(fieldName, " ", fieldType, " `json:\"", p.Name, omitEmpty, "\" yaml:\"", p.Name, omitEmpty, "\"`\n") //nolint:lll // struct tag line
}

// requiredForMode возвращает, является ли поле required в текущем режиме
// генерации. Логика зависит от флага USE_REQUIRED_V2.
//
// Используется audit_model.go, validate.go, set_defaults.go,
// url_form_methods.go и этим файлом (renderField). render/schema/StructRenderer
// имеет собственную копию (дублирование для развязки пакетов).
func (g *Generator) requiredForMode(p *parser.Property, mode string) bool {
	if !g.project.Features.UseRequiredV2.Value {
		return p.Required
	}

	switch mode {
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
// Используется audit_model.go, validate.go, set_defaults.go,
// url_form_methods.go и этим файлом (renderField). render/schema/StructRenderer
// имеет собственную копию (дублирование для развязки пакетов).
func fieldIsOptional(required bool, fieldType string) bool {
	return !required && !strings.HasPrefix(fieldType, "*") && !isInherentlyNilable(fieldType)
}

// filteredSchemaHasDefaults сообщает, есть ли default хотя бы у одного
// property, проходящего фильтр keep, или у вложенной object-схемы через
// $ref. Используется impl_server.go для audit-server-response-фильтрации.
// render/schema/ имеет собственную копию (SchemaTreeHasDefaults).
func filteredSchemaHasDefaults(
	g *Generator,
	sh *parser.Schema,
	keep func(*parser.Property) bool,
) bool {
	if sh == nil {
		return false
	}

	return g.schemaTreeHasDefaults(sh, keep, map[string]bool{sh.Name: true})
}

// enumBaseType и enumStringValue используются render/schema/ для
// default-value-литералов enum-схем. enumValueName живёт в naming.go.
// renderEnum, renderAlias, renderMapAlias, enumLiteral, renderStruct,
// renderSplitStruct, renderFilteredStruct, renderUpdateStruct и сопутствующие
// удалены в Tasks 7-8 — их функциональность перенесена в render/schema/.

func enumBaseType(sh *parser.Schema) string {
	switch sh.Type {
	case oapiTypeInteger:
		switch sh.Format {
		case oapiFormatInt32:
			return oapiFormatInt32
		case oapiFormatInt64:
			return oapiFormatInt64
		default:
			return "int"
		}
	case oapiTypeNumber:
		switch sh.Format {
		case oapiFormatFloat:
			return goTypeFloat32
		default:
			return goTypeFloat64
		}
	default:
		return oapiTypeString
	}
}

func enumStringValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}

	return fmt.Sprint(v)
}

func (g *Generator) renderArraySchema(w *codegen.BufferWriter, sh *parser.Schema, m *typeMapper, name string) { //nolint:lll // function signature
	elem := goTypeAny
	if sh.Items != nil {
		elem = m.goType(sh.Items)
	}

	w.Print("type ", name, " []", elem, "\n")
}

func (g *Generator) renderUnion(w *codegen.BufferWriter, sh *parser.Schema, m *typeMapper, name string) { //nolint:lll // function signature
	variants := sh.OneOf
	if len(variants) == 0 {
		variants = sh.AnyOf
	}

	w.Print("type ", name, " struct {\n")

	for _, v := range variants {
		variantType := m.goType(v)
		if variantType == "" || variantType == goTypeAny {
			continue
		}

		fieldName := goName(refToName(v.Ref))
		if fieldName == "" {
			fieldName = inlineVariantName(variantType)
		}

		w.Print("\t", fieldName, " *", variantType, " `json:\"-\"`\n")
	}

	w.Print("}\n")
}

func (g *Generator) renderAllOf(w *codegen.BufferWriter, sh *parser.Schema, m *typeMapper, name string) { //nolint:lll // function signature with params
	w.Print("type ", name, " struct {\n")

	for _, part := range sh.AllOf {
		if part.Ref != "" {
			w.Print("\t", goName(refToName(part.Ref)), "\n")
		} else if part.Type == oapiTypeObject {
			for _, p := range part.Properties {
				g.renderField(w, p, m)
			}
		}
	}

	w.Print("}\n")
}

func writeDocComment(w *codegen.BufferWriter, desc string) {
	for line := range strings.SplitSeq(desc, "\n") {
		w.Print("// ", line, "\n")
	}
}
