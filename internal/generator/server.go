package generator

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
)

// serverFile генерирует server.gen.go: интерфейс Server.
// Переиспользует request/response-структуры из client.gen.go.
func (g *Generator) serverFile() codegen.File {
	m := &typeMapper{}
	m.addImport("context", "")
	body := g.renderServer()
	return g.factory.Create(&gogen.File{
		Package: g.packageName,
		Imports: m.imports,
		Body:    body,
	})
}

func (g *Generator) renderServer() []byte {
	w := codegen.NewBufferWriter()

	w.Print("type Server interface {\n")
	for _, op := range g.doc.Operations {
		name := operationMethodName(op)
		if op.Deprecated {
			w.Print("\t// Deprecated: operation is marked as deprecated\n")
		}
		w.Print("\t", name, "(ctx context.Context, req *", name, "Request) (*", name, "Response, error)\n")
	}
	w.Print("}\n")

	return w.Content()
}
