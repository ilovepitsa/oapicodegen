package generator

import (
	"fmt"
	"strings"

	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/parser"
)

// schemaFile генерирует Go-файл с определением типа для схемы.
func (g *Generator) schemaFile(sh *parser.Schema) codegen.File {
	m := &typeMapper{}
	body := g.renderSchema(sh, m)
	return g.factory.Create(&gogen.File{
		Package: g.packageName,
		Imports: m.imports,
		Body:    body,
	})
}

func (g *Generator) renderSchema(sh *parser.Schema, m *typeMapper) []byte {
	w := codegen.NewBufferWriter()
	name := goName(sh.Name)

	if sh.Description != "" {
		writeDocComment(w, sh.Description)
	}

	switch {
	case len(sh.OneOf) > 0 || len(sh.AnyOf) > 0:
		g.renderUnion(w, sh, m, name)
	case len(sh.AllOf) > 0:
		g.renderAllOf(w, sh, m, name)
	case sh.Type == "array":
		g.renderArraySchema(w, sh, m, name)
	case len(sh.Enum) > 0:
		g.renderEnum(w, sh, name)
	case sh.Type == "object" && len(sh.Properties) == 0:
		g.renderMapAlias(w, sh, m, name)
	case sh.Type == "object" || len(sh.Properties) > 0:
		g.renderStruct(w, sh, m, name)
	default:
		g.renderAlias(w, sh, m, name)
	}

	return w.Content()
}

func (g *Generator) renderStruct(w *codegen.BufferWriter, sh *parser.Schema, m *typeMapper, name string) {
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

	w.Print(fieldName, " ", fieldType, " `json:\"", p.Name, omitEmpty, "\" yaml:\"", p.Name, omitEmpty, "\"`\n")
}

func (g *Generator) renderEnum(w *codegen.BufferWriter, sh *parser.Schema, name string) {
	baseGo := enumBaseType(sh)
	w.Print("type ", name, " ", baseGo, "\n\n")

	w.Print("const (\n")
	for i, v := range sh.Enum {
		w.Print("\t", enumValueName(name, enumStringValue(v), i), " ", name, " = ", enumLiteral(v, baseGo), "\n")
	}
	w.Print(")\n")
}

func enumBaseType(sh *parser.Schema) string {
	switch sh.Type {
	case "integer":
		switch sh.Format {
		case "int32":
			return "int32"
		case "int64":
			return "int64"
		default:
			return "int"
		}
	case "number":
		switch sh.Format {
		case "float":
			return "float32"
		default:
			return "float64"
		}
	default:
		return "string"
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
	case "string":
		return fmt.Sprintf("%q", fmt.Sprint(v))
	default:
		return fmt.Sprint(v)
	}
}

func (g *Generator) renderArraySchema(w *codegen.BufferWriter, sh *parser.Schema, m *typeMapper, name string) {
	elem := "any"
	if sh.Items != nil {
		elem = m.goType(sh.Items)
	}
	w.Print("type ", name, " []", elem, "\n")
}

func (g *Generator) renderUnion(w *codegen.BufferWriter, sh *parser.Schema, m *typeMapper, name string) {
	variants := sh.OneOf
	if len(variants) == 0 {
		variants = sh.AnyOf
	}

	w.Print("type ", name, " struct {\n")
	for _, v := range variants {
		variantType := m.goType(v)
		if variantType == "" || variantType == "any" {
			continue
		}
		fieldName := goName(refToName(v.Ref))
		if fieldName == "" {
			fieldName = variantType
		}
		w.Print("\t", fieldName, " *", variantType, " `json:\"-,inline\"`\n")
	}
	w.Print("}\n")
}

func (g *Generator) renderAllOf(w *codegen.BufferWriter, sh *parser.Schema, m *typeMapper, name string) {
	w.Print("type ", name, " struct {\n")
	for _, part := range sh.AllOf {
		if part.Ref != "" {
			w.Print("\t", goName(refToName(part.Ref)), "\n")
		} else if part.Type == "object" {
			for _, p := range part.Properties {
				g.renderField(w, p, m)
			}
		}
	}
	w.Print("}\n")
}

func (g *Generator) renderAlias(w *codegen.BufferWriter, sh *parser.Schema, m *typeMapper, name string) {
	w.Print("type ", name, " ", m.goType(sh), "\n")
}

func (g *Generator) renderMapAlias(w *codegen.BufferWriter, sh *parser.Schema, m *typeMapper, name string) {
	elem := "any"
	if sh.AdditionalProperties != nil {
		elem = m.goType(sh.AdditionalProperties)
	}
	w.Print("type ", name, " map[string]", elem, "\n")
}

func writeDocComment(w *codegen.BufferWriter, desc string) {
	for _, line := range strings.Split(desc, "\n") {
		w.Print("// ", line, "\n")
	}
}
