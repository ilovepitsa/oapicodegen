package generator

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/parser"
)

// clientSugarFile генерирует client_sugar.gen.go: ClientSugared обёртка над Client.
func (g *Generator) clientSugarFile() codegen.File {
	m := g.newTypeMapper("client")
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

	for _, op := range g.operations() {
		g.renderSugarMethod(w, op, m)
	}

	return w.Content()
}

func (g *Generator) renderSugarMethod(w *codegen.BufferWriter, op *parser.Method, m *typeMapper) { //nolint:lll // function signature
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

	w.Print("\treturn ")

	if hasReturn {
		w.Print("nil, ")
	}

	w.WriteString(`fmt.Errorf("unexpected status: %d", resp.Code)` + "\n")
	w.Print("}\n\n")
}

// sugarReturnType возвращает Go-тип возвращаемого значения sugar-метода
// (без ", error") и флаг hasReturn — есть ли возвращаемое значение помимо error.
//
// Если у success-ответа есть типизированные headers — возвращает
// *<Name><Code>PayloadWithHeaders. Иначе — body-тип (*<SchemaType>).
// Для ответа без body и без headers возвращает пустую строку (только error).
func sugarReturnType(
	op *parser.Method,
	successCode string,
	successSchema *parser.Schema,
	m *typeMapper,
) (string, bool) {
	if successCode != "" {
		resp := responseByCode(op.Responses, successCode)
		if hasResponseHeaders(resp) {
			return "*" + payloadWithHeadersTypeName(op, successCode), true
		}
	}

	if successSchema != nil {
		prevMode := m.mode
		m.mode = modeResponse
		typ := m.goType(successSchema)
		m.mode = prevMode

		return "*" + typ, true
	}

	return "", false
}

// firstSuccessResponse возвращает код и схему первого 2xx-ответа.
// Если 2xx нет —fallback на default-ответ (если у него есть схема).
// Если у ответа нет content — schema будет nil (пустой ответ).
func firstSuccessResponse(responses []*parser.Response) (string, *parser.Schema) {
	codes := sortedResponseCodes(responses)

	for _, code := range codes {
		if isSuccessCode(code) {
			resp := responseByCode(responses, code)

			return code, responseSchema(resp)
		}
	}

	if resp := responseByCode(responses, oapiCodeDefault); resp != nil {
		return oapiCodeDefault, responseSchema(resp)
	}

	return "", nil
}
