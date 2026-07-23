package operations

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/generator/render"
)

// ServerInterfaceRenderer рендерит interfaces/server/server.gen.go: интерфейс
// Server. Переиспользует request/response-структуры из interfaces/client.
// Заменяет Generator.serverFile + Generator.renderServer (internal/generator/server.go).
type ServerInterfaceRenderer struct{}

func NewServerInterfaceRenderer() *ServerInterfaceRenderer { return &ServerInterfaceRenderer{} }

func (ServerInterfaceRenderer) FilePath() string { return "interfaces/server/server.gen.go" }

func (r *ServerInterfaceRenderer) Render(ctx *render.RenderContext) ([]byte, *render.ImportTracker, error) {
	imps := render.NewImportTracker()
	ctx.Imports = imps

	imps.Add(gogen.Import{Path: "context"})

	ops := allOperations(ctx.Project)
	clientImportPath := ""
	if ctx.Project != nil && ctx.Project.Paths != nil {
		clientImportPath = ctx.Project.Paths.Imports.ClientInterfaces.Path
	}
	if clientImportPath != "" {
		imps.Add(gogen.Import{Path: clientImportPath, Alias: "client"})
	}

	w := codegen.NewBufferWriter()
	w.Print("type Server interface {\n")
	for _, op := range ops {
		name := operationMethodName(op)
		if op.Deprecated {
			w.Print("\t// Deprecated: operation is marked as deprecated\n")
		}
		w.Print("\t", name, "(ctx context.Context, req *", qualifyClient(name, "Request", clientImportPath), ") ")
		w.Print("(*", qualifyClient(name, "Response", clientImportPath), ", error)\n")
	}
	w.Print("}\n")

	return w.Content(), imps, nil
}

var _ render.SingletonRenderer = (*ServerInterfaceRenderer)(nil)
