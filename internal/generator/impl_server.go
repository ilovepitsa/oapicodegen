package generator

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/parser"
	"strings"
)

// implServerFile генерирует impl/echoserver/server.gen.go:
// Echo-обработчики, делегирующие в apiserver.Server.
func (g *Generator) implServerFile() codegen.File {
	m := g.newTypeMapper("implserver")

	m.addImport("github.com/labstack/echo/v4", "")

	if g.project != nil {
		m.addImport(g.project.Paths.Imports.ClientInterfaces.Path, "apiclient")
		m.addImport(g.project.Paths.Imports.ServerInterfaces.Path, "apiserver")
	}

	needBody := false
	needURLForm := false

	for _, op := range g.operations() {
		if op.RequestBody != nil {
			needBody = true

			if requestBodyIsURLForm(op.RequestBody) {
				needURLForm = true
			}
		}
	}

	if needBody {
		m.addImport("bytes", "")
		m.addImport("encoding/json", "")
		m.addImport("io", "")
		m.addImport("net/http", "")
	}

	if needURLForm {
		m.addImport("net/url", "")
		m.addImport("strings", "")
	}

	body := g.renderImplServer(needBody, needURLForm)

	return g.factory.Create(&gogen.File{
		Package: "server",
		Imports: m.imports,
		Body:    body,
	})
}

func (g *Generator) renderImplServer(needBody, needURLForm bool) []byte {
	w := codegen.NewBufferWriter()

	w.Print("type ServerHTTP struct {\n")
	w.Print("\timpl apiserver.Server\n")
	w.Print("}\n\n")

	w.Print("func NewServerHTTP(impl apiserver.Server) *ServerHTTP {\n")
	w.Print("\treturn &ServerHTTP{impl: impl}\n")
	w.Print("}\n\n")

	if needBody {
		g.renderBindBody(w, needURLForm)
	}

	w.Print("func (s *ServerHTTP) Register(e *echo.Echo) {\n")

	for _, op := range g.operations() {
		method := strings.ToUpper(op.Method)
		epath := echoPath(op.Path)
		handler := lowerFirst(operationMethodName(op))
		w.Print("\te.", method, "(\"", epath, "\", s.", handler, ")\n")
	}

	w.Print("}\n\n")

	for _, op := range g.operations() {
		g.renderImplServerMethod(w, op)
	}

	return w.Content()
}

func (g *Generator) renderBindBody(w *codegen.BufferWriter, needURLForm bool) {
	w.WriteString("// bindBody читает тело запроса, сбрасывает его для c.Bind() и\n")
	w.WriteString("// десериализует в dst. c.Bind() повторно читает тело, но поля с\n")
	w.WriteString("// json:\"-\" игнорируются — поэтому Body декодируется отдельно.\n")
	w.Print("func bindBody(c echo.Context, dst any) error {\n")

	if needURLForm {
		w.Print("\tct := c.Request().Header.Get(\"Content-Type\")\n")
		w.Print("\tif strings.HasPrefix(ct, \"application/x-www-form-urlencoded\") {\n")
		w.Print("\t\tif err := c.Request().ParseForm(); err != nil {\n")
		w.Print("\t\t\treturn err\n")
		w.Print("\t\t}\n")
		w.Print("\t\tif u, ok := dst.(interface{ UnmarshalURLForm(url.Values) error }); ok {\n")
		w.Print("\t\t\treturn u.UnmarshalURLForm(c.Request().PostForm)\n")
		w.Print("\t\t}\n")
		w.Print("\t\treturn nil\n")
		w.Print("\t}\n")
	}

	w.Print("\tbody, err := io.ReadAll(c.Request().Body)\n")
	w.Print("\tif err != nil {\n")
	w.Print("\t\treturn err\n")
	w.Print("\t}\n")
	w.Print("\tc.Request().Body = io.NopCloser(bytes.NewReader(body))\n")
	w.Print("\tif len(body) == 0 {\n")
	w.Print("\t\treturn nil\n")
	w.Print("\t}\n")
	w.Print("\tif err := json.Unmarshal(body, dst); err != nil {\n")
	w.Print("\t\treturn echo.NewHTTPError(http.StatusBadRequest, err.Error())\n")
	w.Print("\t}\n")
	w.Print("\treturn nil\n")
	w.Print("}\n\n")
}

func (g *Generator) renderImplServerMethod(w *codegen.BufferWriter, op *parser.Method) {
	name := operationMethodName(op)
	handler := lowerFirst(name)
	hasBody := op.RequestBody != nil

	w.Print("func (s *ServerHTTP) ", handler, "(c echo.Context) error {\n")
	w.Print("\treq := &apiclient.", name, "Request{}\n")

	if hasBody {
		w.Print("\tif err := bindBody(c, &req.Body); err != nil {\n")
		w.Print("\t\treturn err\n")
		w.Print("\t}\n")
	}

	w.Print("\tif err := c.Bind(req); err != nil {\n")
	w.Print("\t\treturn err\n")
	w.Print("\t}\n")

	// SetDefaults вызывается только когда флаг ServerNoAutoDefaults выключен
	// и body-схема имеет default-поля, попадающие в request-фильтр (не readOnly).
	// При включённом split метод генерируется для <Name>Request варианта;
	// проверка requestDefaultsCovered гарантирует, что метод существует.
	if !g.project.Features.ServerNoAutoDefaults.Value && g.shouldCallSetDefaults(op) {
		w.Print("\treq.Body.SetDefaults()\n")
	}

	w.Print("\tresp, err := s.impl.", name, "(c.Request().Context(), req)\n")
	w.Print("\tif err != nil {\n")
	w.Print("\t\treturn err\n")
	w.Print("\t}\n")

	g.renderImplServerResponse(w, op)

	w.Print("}\n\n")
}

// shouldCallSetDefaults сообщает, нужно ли генерировать req.Body.SetDefaults()
// для операции. True только если body-схема разрешается в object-схему с
// default-полями, попадающими в request-фильтр (не readOnly). При включённом
// GOLANG_SPLIT_REQUEST_RESPONSE учитывается Request-фильтр, чтобы метод
// SetDefaults гарантированно существовал на <Name>Request.
func (g *Generator) shouldCallSetDefaults(op *parser.Method) bool {
	if op.RequestBody == nil {
		return false
	}

	sh := g.resolveBodySchema(op.RequestBody)
	if sh == nil || sh.Name == "" {
		return false
	}

	keep := func(*parser.Property) bool { return true }
	if g.project.Features.SplitRequestResponse.Value {
		keep = func(p *parser.Property) bool {
			return p.Schema == nil || !p.Schema.ReadOnly
		}
	}

	return filteredSchemaHasDefaults(g, sh, keep)
}

// resolveBodySchema возвращает object-схему тела запроса. Если body — $ref,
// ищет схему по имени в project.Model; иначе возвращает inline-схему.
// Возвращает nil для не-object схем (array/alias/enum) и для cross-service
// $ref (ExternalRef) — они разрешаются через SchemaIndex в typeMapper, а не
// здесь. SetDefaults для них не генерируется.
func (g *Generator) resolveBodySchema(rb *parser.RequestBody) *parser.Schema {
	sh := bodySchema(rb)
	if sh == nil {
		return nil
	}

	if sh.ExternalRef != "" {
		return nil
	}

	if sh.Ref != "" {
		name := refToName(sh.Ref)
		if s, ok := g.project.Model.Lookup(name); ok {
			return s
		}

		return nil
	}

	return sh
}

// resolveRefSchema возвращает схему из project.Model по $ref.
// Если s — не $ref (inline-схема), cross-service $ref (ExternalRef), или имя
// не найдено — возвращает nil. Используется в SetDefaults для разрешения
// вложенных object-схем (M3).
func (g *Generator) resolveRefSchema(s *parser.Schema) *parser.Schema {
	if s == nil || s.ExternalRef != "" || s.Ref == "" {
		return nil
	}

	name := refToName(s.Ref)
	if sh, ok := g.project.Model.Lookup(name); ok {
		return sh
	}

	return nil
}

func (g *Generator) renderImplServerResponse(w *codegen.BufferWriter, op *parser.Method) {
	for _, r := range op.Responses {
		if r.StatusCode == oapiCodeDefault {
			continue
		}

		g.renderImplServerStatusCodeResponse(w, r, responseFieldName(r.StatusCode))
	}

	for _, r := range op.Responses {
		if r.StatusCode != oapiCodeDefault {
			continue
		}

		g.renderImplServerStatusCodeResponse(w, r, "ResponseDefault")
	}

	w.WriteString("\treturn c.NoContent(resp.Code)\n")
}

// renderImplServerStatusCodeResponse рендерит ветку ответа для конкретного status code.
// Четыре случая по наличию headers и schema:
//  1. Нет headers, нет schema → bool-поле, NoContent.
//  2. Нет headers, есть schema → указатель, JSON.
//  3. Есть headers, нет schema → указатель, renderHeaderSet + NoContent
//     (без c.JSON — body пустой, только заголовки).
//  4. Есть headers, есть schema → указатель, renderHeaderSet + JSON
//     (c.JSON вызовет MarshalJSON обёртки, который маршалит Payload).
//
// Для default-ответа (StatusCode == oapiCodeDefault) используется resp.Code
// вместо литерала.
func (g *Generator) renderImplServerStatusCodeResponse(
	w *codegen.BufferWriter,
	r *parser.Response,
	fieldName string,
) {
	hasHeaders := hasResponseHeaders(r)
	hasSchema := responseSchema(r) != nil

	if !hasHeaders && !hasSchema {
		// Случай 1: bool-поле.
		w.Print("\tif resp.", fieldName, " {\n")
		g.renderStatusCodeReturn(w, r, "NoContent", "")
		w.Print("\t}\n")

		return
	}

	// Случаи 2-4: указатель, проверка != nil.
	w.Print("\tif resp.", fieldName, " != nil {\n")
	g.renderHeaderSet(w, fieldName, hasHeaders)

	if hasSchema {
		g.renderStatusCodeReturn(w, r, "JSON", "resp."+fieldName)
	} else {
		g.renderStatusCodeReturn(w, r, "NoContent", "")
	}

	w.Print("\t}\n")
}

// renderStatusCodeReturn рендерит `return c.<Method>(<code>, <field>)`.
// codeExpr — литерал status code (для не-default) или resp.Code (для default).
// field — выражение для второго аргумента (пустая строка для NoContent).
func (g *Generator) renderStatusCodeReturn(
	w *codegen.BufferWriter,
	r *parser.Response,
	method, field string,
) {
	codeExpr := r.StatusCode
	if r.StatusCode == oapiCodeDefault {
		codeExpr = "resp.Code"
	}

	if field == "" {
		w.Print("\t\treturn c.", method, "(", codeExpr, ")\n")

		return
	}

	w.Print("\t\treturn c.", method, "(", codeExpr, ", ", field, ")\n")
}

// renderHeaderSet копирует заголовки из PayloadWithHeaders-обёртки в HTTP-ответ echo.
// Вызывается внутри уже проверенного `if resp.<Field> != nil { ... }` блока,
// поэтому дополнительная nil-проверка не требуется.
// Метод Headers() возвращает map[string]string (одна строка на заголовок),
// поэтому используется Header().Set, а не Header().Add.
func (g *Generator) renderHeaderSet(w *codegen.BufferWriter, fieldName string, hasHeaders bool) {
	if !hasHeaders {
		return
	}

	w.Print("\t\tfor k, v := range resp.", fieldName, ".Headers() {\n")
	w.Print("\t\t\tc.Response().Header().Set(k, v)\n")
	w.Print("\t\t}\n")
}

// echoPath конвертирует OpenAPI path в Echo path: {param} → :param.
func echoPath(path string) string {
	return strings.NewReplacer("{", ":", "}", "").Replace(path)
}

// lowerFirst делает первую букву строчной: ListPets → listPets.
func lowerFirst(s string) string {
	if s == "" {
		return ""
	}

	return strings.ToLower(s[:1]) + s[1:]
}
