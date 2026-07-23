package operations

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
)

// ClientSugarRenderer рендерит interfaces/client/client_sugar.gen.go:
// ClientSugared обёртка над Client. Заменяет Generator.clientSugarFile +
// Generator.renderClientSugar (internal/generator/client_sugar.go).
type ClientSugarRenderer struct{}

func NewClientSugarRenderer() *ClientSugarRenderer { return &ClientSugarRenderer{} }

func (ClientSugarRenderer) FilePath() string { return "interfaces/client/client_sugar.gen.go" }

func (r *ClientSugarRenderer) Render(ctx *render.RenderContext) ([]byte, *render.ImportTracker, error) {
	imps := render.NewImportTracker()
	ctx.Imports = imps

	ops := allOperations(ctx.Project)
	m := ctx.TypeMapper

	imps.Add(gogen.Import{Path: "context"})
	imps.Add(gogen.Import{Path: "fmt"})

	w := codegen.NewBufferWriter()
	w.Print("type ClientSugared struct {\n")
	w.Print("\timpl Client\n")
	w.Print("}\n\n")
	w.Print("func NewClientSugared(impl Client) *ClientSugared {\n")
	w.Print("\treturn &ClientSugared{impl: impl}\n")
	w.Print("}\n\n")

	for _, op := range ops {
		renderSugarMethod(w, op, m)
	}

	return w.Content(), imps, nil
}

func renderSugarMethod(w *codegen.BufferWriter, op *parser.Method, m render.TypeMapper) {
	name := operationMethodName(op)
	successCode, successSchema := firstSuccessResponse(op.Responses)
	retType, hasReturn := sugarReturnType(op, successCode, successSchema, m)

	if op.Deprecated {
		w.Print("// Deprecated: operation is marked as deprecated\n")
	}
	w.Print("func (x *ClientSugared) ", name, "(ctx context.Context, req *", name, "Request) ")
	if hasReturn {
		w.Print("(", retType, ", error) {\n")
	} else {
		w.Print("error {\n")
	}
	w.Print("\tresp, err := x.impl.", name, "(ctx, req)\n")
	w.Print("\tif err != nil {\n")
	if hasReturn {
		w.Print("\t\treturn nil, err\n")
	} else {
		w.Print("\t\treturn err\n")
	}
	w.Print("\t}\n")

	if successCode != "" {
		field := responseFieldName(successCode)
		if hasReturn {
			w.Print("\tif resp.", field, " != nil {\n")
			w.Print("\t\treturn resp.", field, ", nil\n")
		} else {
			w.Print("\tif resp.", field, " {\n")
			w.Print("\t\treturn nil\n")
		}
		w.Print("\t}\n")
	}
	w.WriteString("\treturn ")
	if hasReturn {
		w.WriteString("nil, ")
	}
	w.WriteString("fmt.Errorf(\"unexpected status: %d\", resp.Code)\n")
	w.WriteString("}\n\n")
}

var _ render.SingletonRenderer = (*ClientSugarRenderer)(nil)
