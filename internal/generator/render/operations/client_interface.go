package operations

import (
	"fmt"
	"strings"

	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
)

// ClientInterfaceRenderer рендерит interfaces/client/client.gen.go: интерфейс
// Client + request/response-структуры. Заменяет Generator.clientFile +
// Generator.renderClient (internal/generator/client.go).
type ClientInterfaceRenderer struct{}

func NewClientInterfaceRenderer() *ClientInterfaceRenderer { return &ClientInterfaceRenderer{} }

func (ClientInterfaceRenderer) FilePath() string { return "interfaces/client/client.gen.go" }

func (r *ClientInterfaceRenderer) Render(ctx *render.RenderContext) ([]byte, *render.ImportTracker, error) {
	imps := render.NewImportTracker()
	ctx.Imports = imps

	ops := allOperations(ctx.Project)
	m := ctx.TypeMapper

	imps.Add(gogen.Import{Path: "context"})

	needJSON, needFmt := clientExtraImports(ops)
	if needJSON {
		imps.Add(gogen.Import{Path: "encoding/json"})
	}
	if needFmt {
		imps.Add(gogen.Import{Path: "fmt"})
	}

	w := codegen.NewBufferWriter()

	w.Print("type Client interface {\n")
	for _, op := range ops {
		name := operationMethodName(op)
		if op.Deprecated {
			w.Print("\t// Deprecated: operation is marked as deprecated\n")
		}
		w.Print("\t", name, "(ctx context.Context, req *", name, "Request) (*", name, "Response, error)\n")
	}
	w.Print("}\n\n")

	for _, op := range ops {
		renderRequestStruct(w, op, m)
		renderResponseStruct(w, op, m)
	}

	return w.Content(), imps, nil
}

// clientExtraImports проверяет, нужны ли импорты encoding/json и fmt
// для PayloadWithHeaders-структур.
func clientExtraImports(ops []*parser.Method) (needJSON, needFmt bool) {
	for _, op := range ops {
		for _, r := range op.Responses {
			if !hasResponseHeaders(r) {
				continue
			}
			needJSON = true
			for _, hdr := range r.Headers {
				if headerGoBaseType(hdr.Schema) != "string" {
					needFmt = true
				}
			}
		}
	}
	return
}

func renderRequestStruct(w *codegen.BufferWriter, op *parser.Method, m render.TypeMapper) {
	name := operationMethodName(op) + "Request"
	w.Print("type ", name, " struct {\n")
	m.SetMode("Request")
	for _, p := range op.Parameters {
		renderParamField(w, p, m)
	}
	if op.RequestBody != nil {
		renderBodyField(w, op.RequestBody, m)
	}
	w.Print("}\n\n")
}

func renderParamField(w *codegen.BufferWriter, p *parser.Parameter, m render.TypeMapper) {
	if p.Schema != nil && p.Schema.Description != "" {
		writeDocComment(w, p.Schema.Description)
	}
	if p.Deprecated {
		w.Print("\t// Deprecated: parameter is marked as deprecated\n")
	}
	fieldName := goName(p.Name)
	fieldType := m.GoType(p.Schema)
	required := p.Required || p.In == "path"
	if !required && !strings.HasPrefix(fieldType, "*") && !isInherentlyNilable(fieldType) {
		fieldType = "*" + fieldType
	}
	w.Print("\t", fieldName, " ", fieldType, " `", echoTag(p.In, p.Name), "`\n")
}

func renderBodyField(w *codegen.BufferWriter, rb *parser.RequestBody, m render.TypeMapper) {
	schema := bodySchema(rb)
	if schema == nil {
		return
	}
	if rb.Description != "" {
		writeDocComment(w, rb.Description)
	}
	fieldType := m.GoType(schema)
	if !rb.Required && !strings.HasPrefix(fieldType, "*") && !isInherentlyNilable(fieldType) {
		fieldType = "*" + fieldType
	}
	w.Print("\tBody ", fieldType, " `json:\"-\"`\n")
}

func renderResponseStruct(w *codegen.BufferWriter, op *parser.Method, m render.TypeMapper) {
	name := operationMethodName(op) + "Response"
	w.Print("type ", name, " struct {\n")
	w.Print("\tCode int\n")
	m.SetMode("Response")
	codes := sortedResponseCodes(op.Responses)
	for _, code := range codes {
		resp := responseByCode(op.Responses, code)
		fieldName := responseFieldName(code)
		if hasResponseHeaders(resp) {
			typeName := payloadWithHeadersTypeName(op, code)
			w.Print("\t", fieldName, " *", typeName, "\n")
		} else {
			payloadType := responsePayloadType(resp, m)
			w.Print("\t", fieldName, " ", payloadType, "\n")
		}
	}
	w.Print("}\n\n")
	for _, code := range codes {
		resp := responseByCode(op.Responses, code)
		if !hasResponseHeaders(resp) {
			continue
		}
		renderPayloadWithHeadersType(w, op, code, resp, m)
	}
}

func renderPayloadWithHeadersType(w *codegen.BufferWriter, op *parser.Method, code string, resp *parser.Response, m render.TypeMapper) {
	typeName := payloadWithHeadersTypeName(op, code)

	w.Print("type ", typeName, " struct {\n")
	w.Print("\tPayload *", m.GoType(responseSchema(resp)), "\n")

	for _, h := range sortedHeaders(resp.Headers) {
		goType := headerGoBaseType(h.Schema)
		w.Print("\t", goName(h.Name), " ", goType, "\n")
	}

	w.Print("}\n\n")

	// MarshalJSON — marshals only Payload, not header fields.
	w.Print("func (m ", typeName, ") MarshalJSON() ([]byte, error) {\n")
	w.Print("\treturn json.Marshal(m.Payload)\n")
	w.Print("}\n\n")

	// Headers — returns header fields as map[string]string.
	w.Print("func (m ", typeName, ") Headers() map[string]string {\n")
	w.Print("\treturn map[string]string{\n")

	for _, h := range sortedHeaders(resp.Headers) {
		fieldName := goName(h.Name)
		hdrType := headerGoBaseType(h.Schema)
		w.Print("\t\t\"", h.Name, "\": ", headerEncodeExpr(fieldName, hdrType), ",\n")
	}

	w.Print("\t}\n")
	w.Print("}\n\n")
}

// headerEncodeExpr возвращает выражение для сериализации header-значения в строку.
func headerEncodeExpr(fieldName, hdrType string) string {
	switch hdrType {
	case "string":
		return "m." + fieldName
	default:
		return fmt.Sprintf(`fmt.Sprintf("%%v", m.%s)`, fieldName)
	}
}

var _ render.SingletonRenderer = (*ClientInterfaceRenderer)(nil)
