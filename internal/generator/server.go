package generator

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
)

// serverFile генерирует server.gen.go: интерфейс Server.
// Переиспользует request/response-структуры из interfaces/client.
func (g *Generator) serverFile() codegen.File {
	m := &typeMapper{currentPkg: "server", modulePath: g.modulePath}
	m.addImport("context", "")

	if g.modulePath != "" {
		m.addImport(g.modulePath+"/interfaces/client", "client")
	}

	body := g.renderServer(m)

	return g.factory.Create(&gogen.File{
		Package: "server",
		Imports: m.imports,
		Body:    body,
	})
}

func (g *Generator) renderServer(m *typeMapper) []byte {
	w := codegen.NewBufferWriter()

	w.Print("type Server interface {\n")

	for _, op := range g.doc.Operations {
		name := operationMethodName(op)

		if op.Deprecated {
			w.Print("\t// Deprecated: operation is marked as deprecated\n")
		}

		w.Print("\t", name, "(ctx context.Context, req *", qualifyClient(name, "Request", m), ") ")
		w.Print("(*", qualifyClient(name, "Response", m), ", error)\n")
	}

	w.Print("}\n")

	return w.Content()
}

// qualifyClient добавляет префикс "client." к имени типа, если сервер
// рендерится в отдельном пакете (modulePath задан).
func qualifyClient(name, suffix string, m *typeMapper) string {
	if m.modulePath == "" {
		return name + suffix
	}

	return "client." + name + suffix
}
