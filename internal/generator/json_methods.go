package generator

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/parser"
)

// needsJSONMethods сообщает, нужна ли схеме отдельная функция UnmarshalJSON.
func needsJSONMethods(sh *parser.Schema) bool {
	return len(sh.OneOf) > 0 || len(sh.AnyOf) > 0
}

// jsonMethodsFile генерирует MarshalJSON/UnmarshalJSON для union-схем (oneOf/anyOf).
// Методы на *<Name> валидны, т.к. union рендерится как struct, не interface.
func (g *Generator) jsonMethodsFile(sh *parser.Schema) codegen.File {
	m := &typeMapper{currentPkg: "model", modulePath: g.modulePath}
	body := g.renderJSONMethods(sh, m)

	return g.factory.Create(&gogen.File{
		Package: "model",
		Imports: m.imports,
		Body:    body,
	})
}

func (g *Generator) renderJSONMethods(sh *parser.Schema, m *typeMapper) []byte {
	m.addImport("encoding/json", "")
	m.addImport("fmt", "")

	w := codegen.NewBufferWriter()
	name := goName(sh.Name)

	variants := sh.OneOf
	if len(variants) == 0 {
		variants = sh.AnyOf
	}

	type variant struct {
		field string
		typ   string
	}

	vs := make([]variant, 0, len(variants))

	for _, v := range variants {
		variantType := m.goType(v)
		if variantType == "" || variantType == goTypeAny {
			continue
		}

		fieldName := goName(refToName(v.Ref))
		if fieldName == "" {
			fieldName = variantType
		}

		vs = append(vs, variant{field: fieldName, typ: variantType})
	}

	w.Print("func (m *", name, ") UnmarshalJSON(data []byte) error {\n")

	for i, v := range vs {
		w.Print("\tvar v_", i, " ", v.typ, "\n")
		w.Print("\tif err := json.Unmarshal(data, &v_", i, "); err == nil {\n")
		w.Print("\t\tm.", v.field, " = &v_", i, "\n")
		w.Print("\t\treturn nil\n")
		w.Print("\t}\n")
	}

	w.Print("\treturn fmt.Errorf(\"", name, ": no variant matched\")\n")
	w.Print("}\n\n")

	w.Print("func (m ", name, ") MarshalJSON() ([]byte, error) {\n")

	for _, v := range vs {
		w.Print("\tif m.", v.field, " != nil {\n")
		w.Print("\t\treturn json.Marshal(m.", v.field, ")\n")
		w.Print("\t}\n")
	}

	w.Print("\treturn json.Marshal(nil)\n")
	w.Print("}\n")

	return w.Content()
}
