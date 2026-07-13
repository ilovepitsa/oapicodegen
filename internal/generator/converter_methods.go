package generator

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/parser"
)

// converterMethodsFile генерирует model/<name>_converters.gen.go с функцией
// <Name>RequestToResponse для splittable-схемы. Копирует shared-поля
// (не readOnly && не writeOnly) из Request в Response.
//
// Генерируется только при включённом GOLANG_SPLIT_REQUEST_RESPONSE и наличии
// хотя бы одного shared-поля. Вызов в writeSchemaFiles — см. generator.go.
func (g *Generator) converterMethodsFile(sh *parser.Schema) codegen.File {
	m := g.newTypeMapper("model")
	w := codegen.NewBufferWriter()

	name := goName(sh.Name)
	g.renderRequestToResponse(w, sh, name)

	return g.factory.Create(&gogen.File{
		Package: "model",
		Imports: m.imports,
		Body:    w.Content(),
	})
}

// schemaHasSharedFields сообщает, есть ли у схемы хотя бы одно shared-поле
// (не readOnly && не writeOnly) — поле, существующее в обоих split-вариантах.
func schemaHasSharedFields(sh *parser.Schema) bool {
	for _, p := range sh.Properties {
		if p.Schema == nil {
			continue
		}

		if !p.Schema.ReadOnly && !p.Schema.WriteOnly {
			return true
		}
	}

	return false
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
func (g *Generator) renderRequestToResponse(
	w *codegen.BufferWriter,
	sh *parser.Schema,
	name string,
) {
	w.Print("func ", name, "RequestToResponse(req ", name, "Request) ", name, "Response {\n")
	w.Print("\tvar resp ", name, "Response\n")

	for _, p := range sh.Properties {
		if p.Schema == nil {
			continue
		}

		if p.Schema.ReadOnly || p.Schema.WriteOnly {
			continue
		}

		fieldName := goName(p.Name)
		w.Print("\tresp.", fieldName, " = req.", fieldName, "\n")
	}

	w.Print("\treturn resp\n")
	w.Print("}\n")
}
