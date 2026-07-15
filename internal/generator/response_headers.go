package generator

import (
	"fmt"
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/parser"
	"sort"
)

const payloadWithHeadersSuffix = "PayloadWithHeaders"

// payloadWithHeadersTypeName возвращает имя типа для обёртки body+headers.
// Например: ListPetsResponse200PayloadWithHeaders.
func payloadWithHeadersTypeName(op *parser.Method, code string) string {
	return operationMethodName(op) + "Response" + goName(code) + payloadWithHeadersSuffix
}

// renderPayloadWithHeadersType генерирует тип <Name><Code>PayloadWithHeaders
// с полями Payload (body) и типизированными полями для каждого header.
// Также генерирует MarshalJSON (маршалит только Payload) и Headers() map[string]string
// (для server-side установки заголовков в HTTP-ответ).
func (g *Generator) renderPayloadWithHeadersType(
	w *codegen.BufferWriter,
	op *parser.Method,
	code string,
	resp *parser.Response,
	m *typeMapper,
) {
	typeName := payloadWithHeadersTypeName(op, code)
	schema := responseSchema(resp)

	w.Print("type ", typeName, " struct {\n")

	if schema != nil {
		prevMode := m.mode
		m.mode = modeResponse
		w.Print("\tPayload *", m.goType(schema), "\n")
		m.mode = prevMode
	} else {
		w.Print("\tPayload bool\n")
	}

	for _, hdr := range sortedHeaders(resp.Headers) {
		fieldName := goName(hdr.Name)
		hdrType := headerGoBaseType(hdr.Schema)

		w.Print("\t", fieldName, " ", hdrType, "\n")
	}

	w.Print("}\n\n")

	g.renderPayloadWithHeadersMarshalJSON(w, typeName)
	g.renderPayloadWithHeadersHeadersMethod(w, typeName, resp.Headers)
}

func (g *Generator) renderPayloadWithHeadersMarshalJSON(w *codegen.BufferWriter, typeName string) {
	w.Print("func (m ", typeName, ") MarshalJSON() ([]byte, error) {\n")
	w.Print("\treturn json.Marshal(m.Payload)\n")
	w.Print("}\n\n")
}

func (g *Generator) renderPayloadWithHeadersHeadersMethod(w *codegen.BufferWriter, typeName string, headers map[string]*parser.Parameter) { //nolint:lll // function signature
	w.Print("func (m ", typeName, ") Headers() map[string]string {\n")
	w.Print("\treturn map[string]string{\n")

	for _, hdr := range sortedHeaders(headers) {
		fieldName := goName(hdr.Name)
		hdrType := headerGoBaseType(hdr.Schema)
		w.Print("\t\t\"", hdr.Name, "\": ", headerEncodeExpr(fieldName, hdrType), ",\n")
	}

	w.Print("\t}\n")
	w.Print("}\n\n")
}

// sortedHeaders возвращает headers, отсортированные по имени для детерминированного вывода.
func sortedHeaders(headers map[string]*parser.Parameter) []*parser.Parameter {
	out := make([]*parser.Parameter, 0, len(headers))

	for _, h := range headers {
		out = append(out, h)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})

	return out
}

// headerGoBaseType возвращает Go-тип для header-поля без pointer/nullable обёртки.
//
//nolint:cyclop // table-lookup by type/format
func headerGoBaseType(s *parser.Schema) string {
	if s == nil {
		return goTypeString
	}

	switch s.Type {
	case oapiTypeString:
		return goTypeString
	case oapiTypeInteger:
		switch s.Format {
		case oapiFormatInt32:
			return goTypeInt32
		case oapiFormatInt64:
			return goTypeInt64
		default:
			return goTypeInt
		}
	case oapiTypeNumber:
		switch s.Format {
		case oapiFormatFloat:
			return goTypeFloat32
		default:
			return goTypeFloat64
		}
	case oapiTypeBoolean:
		return goTypeBool
	default:
		return goTypeString
	}
}

// headerEncodeExpr возвращает Go-выражение для конвертации типизированного
// header-поля в string (для метода Headers() на server-side).
func headerEncodeExpr(fieldName, goType string) string {
	if goType == goTypeString {
		return "m." + fieldName
	}

	return fmt.Sprintf(`fmt.Sprintf("%%v", m.%s)`, fieldName)
}

// headerDecodeExpr возвращает Go-выражение для декодирования header из string
// в типизированное поле (для client-side decoder).
// Возвращает (expression, needsErrorCheck).
func headerDecodeExpr(headerName, goType string) (string, bool) {
	getCall := `resp.Header.Get("` + headerName + `")`

	switch goType {
	case goTypeString:
		return getCall, false
	case goTypeInt:
		return "strconv.Atoi(" + getCall + ")", true
	case goTypeInt32:
		return "strconv.ParseInt(" + getCall + ", 10, 32)", true
	case goTypeInt64:
		return "strconv.ParseInt(" + getCall + ", 10, 64)", true
	case goTypeFloat32:
		return "strconv.ParseFloat(" + getCall + ", 32)", true
	case goTypeFloat64:
		return "strconv.ParseFloat(" + getCall + ", 64)", true
	case goTypeBool:
		return "strconv.ParseBool(" + getCall + ")", true
	default:
		return getCall, false
	}
}

// headerDecodeConvert возвращает конверсионное выражение для типов, где
// strconv возвращает значение другого типа (int32, float32).
func headerDecodeConvert(expr, goType string) string {
	switch goType {
	case goTypeInt32:
		return "int32(" + expr + ")"
	case goTypeFloat32:
		return "float32(" + expr + ")"
	default:
		return expr
	}
}
