package generator

import (
	"fmt"
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/parser"
	"strings"
)

// schemaFile генерирует Go-файл с определением типа для схемы.
func (g *Generator) schemaFile(sh *parser.Schema) codegen.File {
	m := g.newTypeMapper("model")
	body := g.renderSchema(sh, m)

	return g.factory.Create(&gogen.File{
		Package: "model",
		Imports: m.imports,
		Body:    body,
	})
}

//nolint:gocyclo,cyclop // dispatch switch, branching is inherent
func (g *Generator) renderSchema(sh *parser.Schema, m *typeMapper) []byte {
	w := codegen.NewBufferWriter()
	name := goName(sh.Name)

	if sh.Description != "" {
		writeDocComment(w, sh.Description)
	}

	if g.features.SplitRequestResponse.Value && g.splittable[sh.Name] {
		g.renderSplitStruct(w, sh, m, name)

		return w.Content()
	}

	switch {
	case len(sh.OneOf) > 0 || len(sh.AnyOf) > 0:
		g.renderUnion(w, sh, m, name)
	case len(sh.AllOf) == 1 && sh.AllOf[0].Ref == "" && sh.AllOf[0].Type != oapiTypeObject:
		g.renderAlias(w, sh.AllOf[0], m, name)
	case len(sh.AllOf) > 0:
		g.renderAllOf(w, sh, m, name)
	case sh.Type == "array":
		g.renderArraySchema(w, sh, m, name)
	case len(sh.Enum) > 0:
		g.renderEnum(w, sh, name)
	case sh.Type == oapiTypeObject && len(sh.Properties) == 0:
		g.renderMapAlias(w, sh, m, name)
	case sh.Type == oapiTypeObject || len(sh.Properties) > 0:
		g.renderStruct(w, sh, m, name)
	default:
		g.renderAlias(w, sh, m, name)
	}

	return w.Content()
}

// renderSplitStruct рендерит <Name>Request и <Name>Response вместо одного
// <Name>, когда включён GOLANG_SPLIT_REQUEST_RESPONSE.
// Request: свойства с ReadOnly=false (writeOnly + regular).
// Response: свойства с WriteOnly=false (readOnly + regular).
func (g *Generator) renderSplitStruct(
	w *codegen.BufferWriter,
	sh *parser.Schema,
	m *typeMapper,
	name string,
) {
	m.mode = modeRequest
	g.renderFilteredStruct(w, sh, m, name+"Request", func(p *parser.Property) bool {
		return p.Schema == nil || !p.Schema.ReadOnly
	})

	m.mode = modeResponse
	g.renderFilteredStruct(w, sh, m, name+"Response", func(p *parser.Property) bool {
		return p.Schema == nil || !p.Schema.WriteOnly
	})
}

func (g *Generator) renderFilteredStruct(
	w *codegen.BufferWriter,
	sh *parser.Schema,
	m *typeMapper,
	name string,
	keep func(*parser.Property) bool,
) {
	w.Print("type ", name, " struct {\n")

	for _, p := range sh.Properties {
		if !keep(p) {
			continue
		}

		g.renderField(w, p, m)
	}

	w.Print("}\n\n")
}

func (g *Generator) renderStruct(w *codegen.BufferWriter, sh *parser.Schema, m *typeMapper, name string) { //nolint:lll // function signature with params
	w.Print("type ", name, " struct {\n")

	for _, p := range sh.Properties {
		g.renderField(w, p, m)
	}

	w.Print("}\n")
}

func (g *Generator) renderField(w *codegen.BufferWriter, p *parser.Property, m *typeMapper) {
	if p.Schema != nil && p.Schema.Description != "" {
		writeDocComment(w, p.Schema.Description)
	}

	if p.Schema != nil && p.Schema.Deprecated {
		w.Print("// Deprecated: schema marks this field as deprecated\n")
	}

	fieldName := goName(p.Name)
	fieldType := m.goType(p.Schema)

	if !p.Required && !strings.HasPrefix(fieldType, "*") && !isInherentlyNilable(fieldType) {
		fieldType = "*" + fieldType
	}

	omitEmpty := ""
	if !p.Required {
		omitEmpty = ",omitempty"
	}

	w.Print(fieldName, " ", fieldType, " `json:\"", p.Name, omitEmpty, "\" yaml:\"", p.Name, omitEmpty, "\"`\n") //nolint:lll // struct tag line
}

func (g *Generator) renderEnum(w *codegen.BufferWriter, sh *parser.Schema, name string) {
	baseGo := enumBaseType(sh)
	w.Print("type ", name, " ", baseGo, "\n\n")

	w.Print("const (\n")

	seen := make(map[string]bool, len(sh.Enum))

	for i, v := range sh.Enum {
		valStr := enumStringValue(v)
		if seen[valStr] {
			continue
		}

		seen[valStr] = true

		w.Print("\t", enumValueName(name, valStr, i), " ", name, " = ", enumLiteral(v, baseGo), "\n") //nolint:lll // const declaration line
	}

	w.Print(")\n")
}

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

func enumLiteral(v any, baseGo string) string {
	switch baseGo {
	case oapiTypeString:
		return fmt.Sprintf("%q", fmt.Sprint(v))
	default:
		return fmt.Sprint(v)
	}
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

func (g *Generator) renderAlias(w *codegen.BufferWriter, sh *parser.Schema, m *typeMapper, name string) { //nolint:lll // function signature
	w.Print("type ", name, " ", m.goType(sh), "\n")
}

func (g *Generator) renderMapAlias(w *codegen.BufferWriter, sh *parser.Schema, m *typeMapper, name string) { //nolint:lll // function signature
	if sh.AdditionalPropertiesFalse {
		w.Print("type ", name, " struct{}\n")

		return
	}

	elem := goTypeAny
	if sh.AdditionalProperties != nil {
		elem = m.goType(sh.AdditionalProperties)
	}

	w.Print("type ", name, " map[string]", elem, "\n")
}

func writeDocComment(w *codegen.BufferWriter, desc string) {
	for line := range strings.SplitSeq(desc, "\n") {
		w.Print("// ", line, "\n")
	}
}
