package generator

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/parser"
)

// clientSugarFile генерирует client_sugar.gen.go: ClientSugared обёртка над Client.
func (g *Generator) clientSugarFile() codegen.File {
	m := &typeMapper{currentPkg: "client", modulePath: g.modulePath}
	m.addImport("context", "")
	m.addImport("fmt", "")
	body := g.renderClientSugar(m)
	return g.factory.Create(&gogen.File{
		Package: "client",
		Imports: m.imports,
		Body:    body,
	})
}

func (g *Generator) renderClientSugar(m *typeMapper) []byte {
	w := codegen.NewBufferWriter()

	w.Print("type ClientSugared struct {\n")
	w.Print("\timpl Client\n")
	w.Print("}\n\n")

	w.Print("func NewClientSugared(impl Client) *ClientSugared {\n")
	w.Print("\treturn &ClientSugared{impl: impl}\n")
	w.Print("}\n\n")

	for _, op := range g.doc.Operations {
		g.renderSugarMethod(w, op, m)
	}

	return w.Content()
}

func (g *Generator) renderSugarMethod(w *codegen.BufferWriter, op *parser.Operation, m *typeMapper) {
	name := operationMethodName(op)
	successCode, successSchema := firstSuccessResponse(op.Responses)

	if op.Deprecated {
		w.Print("// Deprecated: operation is marked as deprecated\n")
	}

	w.Print("func (x *ClientSugared) ", name, "(ctx context.Context, req *", name, "Request) (")

	if successSchema != nil {
		w.Print("*", m.goType(successSchema), ", error) {\n")
	} else {
		w.Print("error) {\n")
	}

	w.Print("\tresp, err := x.impl.", name, "(ctx, req)\n")
	w.Print("\tif err != nil {\n")
	if successSchema != nil {
		w.Print("\t\treturn nil, err\n")
	} else {
		w.Print("\t\treturn err\n")
	}
	w.Print("\t}\n")

	if successCode != "" {
		field := responseFieldName(successCode)
		w.Print("\tif resp.", field, " != nil {\n")
		if successSchema != nil {
			w.Print("\t\treturn resp.", field, ", nil\n")
		} else {
			w.Print("\t\treturn nil\n")
		}
		w.Print("\t}\n")
	}

	w.Print("\treturn ")
	if successSchema != nil {
		w.Print("nil, ")
	}
	w.WriteString(`fmt.Errorf("unexpected status: %d", resp.Code)` + "\n")
	w.Print("}\n\n")
}

// firstSuccessResponse возвращает код и схему первого 2xx-ответа.
// Если у ответа нет content — schema будет nil (пустой ответ).
func firstSuccessResponse(responses []*parser.Response) (string, *parser.Schema) {
	codes := sortedResponseCodes(responses)
	for _, code := range codes {
		if !isSuccessCode(code) {
			continue
		}
		resp := responseByCode(responses, code)
		return code, responseSchema(resp)
	}
	return "", nil
}
