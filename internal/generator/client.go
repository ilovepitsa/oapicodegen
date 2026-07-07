package generator

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/parser"
	"sort"
	"strings"
)

// clientFile генерирует client.gen.go: интерфейс Client + request/response-структуры.
func (g *Generator) clientFile() codegen.File {
	m := &typeMapper{currentPkg: "client", modulePath: g.modulePath}
	m.addImport("context", "")
	body := g.renderClient(m)

	return g.factory.Create(&gogen.File{
		Package: "client",
		Imports: m.imports,
		Body:    body,
	})
}

func (g *Generator) renderClient(m *typeMapper) []byte {
	w := codegen.NewBufferWriter()

	w.Print("type Client interface {\n")

	for _, op := range g.doc.Operations {
		name := operationMethodName(op)

		if op.Deprecated {
			w.Print("\t// Deprecated: operation is marked as deprecated\n")
		}

		w.Print("\t", name, "(ctx context.Context, req *", name, "Request) (*", name, "Response, error)\n")
	}

	w.Print("}\n\n")

	for _, op := range g.doc.Operations {
		g.renderRequestStruct(w, op, m)
		g.renderResponseStruct(w, op, m)
	}

	return w.Content()
}

func (g *Generator) renderRequestStruct(w *codegen.BufferWriter, op *parser.Operation, m *typeMapper) {
	name := operationMethodName(op) + "Request"
	w.Print("type ", name, " struct {\n")

	for _, p := range op.Parameters {
		g.renderParamField(w, p, m)
	}

	if op.RequestBody != nil {
		g.renderBodyField(w, op.RequestBody, m)
	}

	w.Print("}\n\n")
}

func (g *Generator) renderParamField(w *codegen.BufferWriter, p *parser.Parameter, m *typeMapper) {
	if p.Schema != nil && p.Schema.Description != "" {
		writeDocComment(w, p.Schema.Description)
	}

	if p.Deprecated {
		w.Print("\t// Deprecated: parameter is marked as deprecated\n")
	}

	fieldName := goName(p.Name)
	fieldType := m.goType(p.Schema)

	if !p.Required && !strings.HasPrefix(fieldType, "*") && !isInherentlyNilable(fieldType) {
		fieldType = "*" + fieldType
	}

	w.Print("\t", fieldName, " ", fieldType, " `", echoTag(p.In, p.Name), "`\n")
}

func echoTag(in, name string) string {
	switch in {
	case oapiParamPath:
		return "param:\"" + name + "\""
	case oapiParamQuery:
		return "query:\"" + name + "\""
	case oapiParamHeader:
		return "header:\"" + name + "\""
	default:
		return ""
	}
}

func (g *Generator) renderBodyField(w *codegen.BufferWriter, rb *parser.RequestBody, m *typeMapper) {
	schema := bodySchema(rb)
	if schema == nil {
		return
	}

	if rb.Description != "" {
		writeDocComment(w, rb.Description)
	}

	fieldType := m.goType(schema)
	if !rb.Required && !strings.HasPrefix(fieldType, "*") && !isInherentlyNilable(fieldType) {
		fieldType = "*" + fieldType
	}

	w.Print("\tBody ", fieldType, " `json:\"-\"`\n")
}

func (g *Generator) renderResponseStruct(w *codegen.BufferWriter, op *parser.Operation, m *typeMapper) {
	name := operationMethodName(op) + "Response"
	w.Print("type ", name, " struct {\n")
	w.Print("\tCode int\n")

	codes := sortedResponseCodes(op.Responses)
	for _, code := range codes {
		resp := responseByCode(op.Responses, code)
		fieldName := responseFieldName(code)
		payloadType := responsePayloadType(resp, m)
		w.Print("\t", fieldName, " ", payloadType, "\n")
	}

	w.Print("}\n\n")
}

// responsePayloadType возвращает Go-тип поля для ответа.
// Есть content → *<SchemaType>. Нет content → bool (пустой ответ).
func responsePayloadType(resp *parser.Response, m *typeMapper) string {
	schema := responseSchema(resp)
	if schema == nil {
		return "bool"
	}

	return "*" + m.goType(schema)
}

// bodySchema возвращает схему тела запроса (первый content-type).
func bodySchema(rb *parser.RequestBody) *parser.Schema {
	if rb == nil || rb.Content == nil {
		return nil
	}

	return firstContentSchema(rb.Content)
}

// responseSchema возвращает схему ответа (первый content-type).
func responseSchema(resp *parser.Response) *parser.Schema {
	if resp == nil || resp.Content == nil {
		return nil
	}

	return firstContentSchema(resp.Content)
}

func firstContentSchema(content map[string]*parser.MediaType) *parser.Schema {
	if _, ok := content["application/json"]; ok {
		return content["application/json"].Schema
	}

	keys := make([]string, 0, len(content))
	for k := range content {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	if len(keys) == 0 {
		return nil
	}

	return content[keys[0]].Schema
}

func sortedResponseCodes(responses []*parser.Response) []string {
	codes := make([]string, 0, len(responses))
	for _, r := range responses {
		codes = append(codes, r.StatusCode)
	}

	sort.Slice(codes, func(i, j int) bool {
		if codes[i] == oapiCodeDefault {
			return false
		}

		if codes[j] == oapiCodeDefault {
			return true
		}

		return codes[i] < codes[j]
	})

	return codes
}

func responseByCode(responses []*parser.Response, code string) *parser.Response {
	for _, r := range responses {
		if r.StatusCode == code {
			return r
		}
	}

	return nil
}
