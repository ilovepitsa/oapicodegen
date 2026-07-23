package operations

import (
	"sort"
	"strings"
	"unicode"

	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
)

// allOperations возвращает плоский срез всех методов проекта.
func allOperations(project *parser.Project) []*parser.Method {
	var out []*parser.Method
	if project == nil || project.Paths == nil {
		return out
	}
	for _, svc := range project.Paths.Services {
		out = append(out, svc.Methods...)
	}
	return out
}

// ---- Имена (портированы из naming.go) ----

func goName(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	capitalizeNext := true
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if capitalizeNext {
				b.WriteRune(unicode.ToUpper(r))
				capitalizeNext = false
			} else {
				b.WriteRune(r)
			}
		} else {
			capitalizeNext = true
		}
	}
	name := b.String()
	abbreviations := []string{"Id", "Url", "Uri", "Http", "Https", "Json", "Xml", "Api", "Uuid", "Ip"}
	for _, abbr := range abbreviations {
		name = strings.ReplaceAll(name, abbr, strings.ToUpper(abbr))
	}
	return name
}

// ---- Имена операций (портированы из operation.go) ----

func operationMethodName(op *parser.Method) string {
	if op.OperationID != "" {
		return goName(op.OperationID)
	}
	return deriveMethodName(op.Method, op.Path)
}

func deriveMethodName(method, path string) string {
	var b strings.Builder
	b.WriteString(strings.ToLower(method))
	for _, seg := range strings.Split(path, "/") {
		if seg == "" {
			continue
		}
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			inner := seg[1 : len(seg)-1]
			b.WriteString("By")
			b.WriteString(goName(inner))
		} else {
			b.WriteString(goName(seg))
		}
	}
	return goName(b.String())
}

func responseFieldName(code string) string {
	return "Response" + goName(code)
}

func isSuccessCode(code string) bool {
	if code == "default" {
		return false
	}
	if len(code) < 3 {
		return false
	}
	return code[0] == '2'
}

// ---- Ответы (портированы из client.go) ----

func sortedResponseCodes(responses []*parser.Response) []string {
	codes := make([]string, 0, len(responses))
	for _, r := range responses {
		codes = append(codes, r.StatusCode)
	}
	sort.Slice(codes, func(i, j int) bool {
		if codes[i] == "default" {
			return false
		}
		if codes[j] == "default" {
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

func responseSchema(resp *parser.Response) *parser.Schema {
	if resp == nil || resp.Content == nil {
		return nil
	}
	return firstContentSchema(resp.Content)
}

func bodySchema(rb *parser.RequestBody) *parser.Schema {
	if rb == nil || rb.Content == nil {
		return nil
	}
	return firstContentSchema(rb.Content)
}

func hasResponseHeaders(resp *parser.Response) bool {
	return resp != nil && len(resp.Headers) > 0
}

func responsePayloadType(resp *parser.Response, m render.TypeMapper) string {
	schema := responseSchema(resp)
	if schema == nil {
		return "bool"
	}
	return "*" + m.GoType(schema)
}

// ---- Request helpers (портированы из client.go) ----

func isInherentlyNilable(t string) bool {
	return strings.HasPrefix(t, "[]") || strings.HasPrefix(t, "map[") || t == "any"
}

func echoTag(in, name string) string {
	switch in {
	case "path":
		return "param:\"" + name + "\""
	case "query":
		return "query:\"" + name + "\""
	case "header":
		return "header:\"" + name + "\""
	case "cookie":
		return "cookie:\"" + name + "\""
	default:
		return ""
	}
}

func writeDocComment(w *codegen.BufferWriter, desc string) {
	if desc == "" {
		return
	}
	w.Print("// ", desc, "\n")
}

// ---- Response headers (портированы из response_headers.go) ----

func payloadWithHeadersTypeName(op *parser.Method, code string) string {
	return operationMethodName(op) + responseFieldName(code) + "PayloadWithHeaders"
}

func headerGoBaseType(s *parser.Schema) string {
	if s == nil {
		return "string"
	}
	switch s.Type {
	case "string":
		return "string"
	case "integer":
		switch s.Format {
		case "int32":
			return "int32"
		case "int64":
			return "int64"
		default:
			return "int"
		}
	case "number":
		switch s.Format {
		case "float":
			return "float32"
		default:
			return "float64"
		}
	case "boolean":
		return "bool"
	default:
		return "string"
	}
}

// headerDecodeExpr возвращает Go-выражение для декодирования header из string.
func headerDecodeExpr(headerName, goType string) (string, bool) {
	getCall := `resp.Header.Get("` + headerName + `")`
	switch goType {
	case "string":
		return getCall, false
	case "int":
		return "strconv.Atoi(" + getCall + ")", true
	case "int32":
		return "strconv.ParseInt(" + getCall + ", 10, 32)", true
	case "int64":
		return "strconv.ParseInt(" + getCall + ", 10, 64)", true
	case "float32":
		return "strconv.ParseFloat(" + getCall + ", 32)", true
	case "float64":
		return "strconv.ParseFloat(" + getCall + ", 64)", true
	case "bool":
		return "strconv.ParseBool(" + getCall + ")", true
	default:
		return getCall, false
	}
}

// headerDecodeConvert возвращает выражение конверсии для strconv-типов.
func headerDecodeConvert(expr, goType string) string {
	switch goType {
	case "int32":
		return "int32(" + expr + ")"
	case "float32":
		return "float32(" + expr + ")"
	default:
		return expr
	}
}

// requestBodyIsURLForm проверяет, закодировано ли тело как form-urlencoded.
func requestBodyIsURLForm(rb *parser.RequestBody) bool {
	if rb == nil || rb.Content == nil {
		return false
	}
	_, ok := rb.Content["application/x-www-form-urlencoded"]
	return ok
}

// implNeedsStrconv проверяет, нужен ли импорт strconv для header-декодинга.
func implNeedsStrconv(ops []*parser.Method) bool {
	for _, op := range ops {
		for _, r := range op.Responses {
			for _, hdr := range r.Headers {
				if headerGoBaseType(hdr.Schema) != "string" {
					return true
				}
			}
		}
	}
	return false
}

func firstSuccessResponse(responses []*parser.Response) (string, *parser.Schema) {
	codes := sortedResponseCodes(responses)
	for _, code := range codes {
		if isSuccessCode(code) {
			resp := responseByCode(responses, code)
			return code, responseSchema(resp)
		}
	}
	if resp := responseByCode(responses, "default"); resp != nil {
		return "default", responseSchema(resp)
	}
	return "", nil
}

func sugarReturnType(op *parser.Method, successCode string, successSchema *parser.Schema, m render.TypeMapper) (string, bool) {
	if successCode != "" {
		resp := responseByCode(op.Responses, successCode)
		if hasResponseHeaders(resp) {
			return "*" + payloadWithHeadersTypeName(op, successCode), true
		}
	}
	if successSchema != nil {
		if ms, ok := m.(modeSettable); ok {
			prevMode := ms.Mode()
			m.SetMode("Response")
			typ := m.GoType(successSchema)
			m.SetMode(prevMode)
			return "*" + typ, true
		}
		typ := m.GoType(successSchema)
		return "*" + typ, true
	}
	return "", false
}

func filterParamsByIn(params []*parser.Parameter, in string) []*parser.Parameter {
	out := make([]*parser.Parameter, 0, len(params))
	for _, p := range params {
		if p.In == in {
			out = append(out, p)
		}
	}
	return out
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

func qualifyClient(name, suffix string, modelImportPath string) string {
	if modelImportPath == "" {
		return name + suffix
	}
	return "client." + name + suffix
}

// modeSettable — внутренний интерфейс для доступа к Mode() на typeMapperAdapter.
type modeSettable interface {
	Mode() string
}

// ---- Audit helpers (портированы из audit_server.go) ----

// resolveBodySchema возвращает object-схему тела запроса. Если body — $ref,
// ищет схему по имени в project.Model; иначе возвращает inline-схему.
// Возвращает nil для не-object схем и для cross-service $ref (ExternalRef).
func resolveBodySchema(rb *parser.RequestBody, project *parser.Project) *parser.Schema {
	sh := bodySchema(rb)
	if sh == nil {
		return nil
	}
	if sh.ExternalRef != "" {
		return nil
	}
	if sh.Ref != "" {
		if project == nil || project.Model == nil {
			return nil
		}
		name := refToName(sh.Ref)
		if s, ok := project.Model.Lookup(name); ok {
			return s
		}
		return nil
	}
	return sh
}

// refToName извлекает имя схемы из $ref-пути.
// "#/components/schemas/Pet" → "Pet".
func refToName(ref string) string {
	if idx := strings.LastIndex(ref, "/"); idx >= 0 {
		return ref[idx+1:]
	}
	return ref
}
