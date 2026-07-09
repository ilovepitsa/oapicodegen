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

	if g.modulePath != "" {
		m.addImport(g.modulePath+"/interfaces/client", "apiclient")
		m.addImport(g.modulePath+"/interfaces/server", "apiserver")
	}

	needBody := false

	for _, op := range g.doc.Operations {
		if op.RequestBody != nil {
			needBody = true

			break
		}
	}

	if needBody {
		m.addImport("bytes", "")
		m.addImport("encoding/json", "")
		m.addImport("io", "")
		m.addImport("net/http", "")
	}

	body := g.renderImplServer(needBody)

	return g.factory.Create(&gogen.File{
		Package: "server",
		Imports: m.imports,
		Body:    body,
	})
}

func (g *Generator) renderImplServer(needBody bool) []byte {
	w := codegen.NewBufferWriter()

	w.Print("type ServerHTTP struct {\n")
	w.Print("\timpl apiserver.Server\n")
	w.Print("}\n\n")

	w.Print("func NewServerHTTP(impl apiserver.Server) *ServerHTTP {\n")
	w.Print("\treturn &ServerHTTP{impl: impl}\n")
	w.Print("}\n\n")

	if needBody {
		w.WriteString("// bindBody читает тело запроса, сбрасывает его для c.Bind() и\n")
		w.WriteString("// десериализует в dst. c.Bind() повторно читает тело, но поля с\n")
		w.WriteString("// json:\"-\" игнорируются — поэтому Body декодируется отдельно.\n")
		w.Print("func bindBody(c echo.Context, dst any) error {\n")
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

	w.Print("func (s *ServerHTTP) Register(e *echo.Echo) {\n")

	for _, op := range g.doc.Operations {
		method := strings.ToUpper(op.Method)
		epath := echoPath(op.Path)
		handler := lowerFirst(operationMethodName(op))
		w.Print("\te.", method, "(\"", epath, "\", s.", handler, ")\n")
	}

	w.Print("}\n\n")

	for _, op := range g.doc.Operations {
		g.renderImplServerMethod(w, op)
	}

	return w.Content()
}

func (g *Generator) renderImplServerMethod(w *codegen.BufferWriter, op *parser.Operation) {
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

	w.Print("\tresp, err := s.impl.", name, "(c.Request().Context(), req)\n")
	w.Print("\tif err != nil {\n")
	w.Print("\t\treturn err\n")
	w.Print("\t}\n")

	g.renderImplServerResponse(w, op)

	w.Print("}\n\n")
}

func (g *Generator) renderImplServerResponse(w *codegen.BufferWriter, op *parser.Operation) {
	for _, r := range op.Responses {
		if r.StatusCode == oapiCodeDefault {
			continue
		}

		fieldName := responseFieldName(r.StatusCode)
		hasHeaders := hasResponseHeaders(r)

		if responseSchema(r) == nil {
			w.Print("\tif resp.", fieldName, " {\n")
			g.renderHeaderSet(w, fieldName, hasHeaders)
			w.Print("\t\treturn c.NoContent(", r.StatusCode, ")\n")
			w.Print("\t}\n")
		} else {
			w.Print("\tif resp.", fieldName, " != nil {\n")
			g.renderHeaderSet(w, fieldName, hasHeaders)
			w.Print("\t\treturn c.JSON(", r.StatusCode, ", resp.", fieldName, ")\n")
			w.Print("\t}\n")
		}
	}

	for _, r := range op.Responses {
		if r.StatusCode != oapiCodeDefault {
			continue
		}

		fieldName := "ResponseDefault"
		hasHeaders := hasResponseHeaders(r)

		if responseSchema(r) == nil {
			w.Print("\tif resp.", fieldName, " {\n")
			g.renderHeaderSet(w, fieldName, hasHeaders)
			w.Print("\t\treturn c.NoContent(resp.Code)\n")
			w.Print("\t}\n")
		} else {
			w.Print("\tif resp.", fieldName, " != nil {\n")
			g.renderHeaderSet(w, fieldName, hasHeaders)
			w.Print("\t\treturn c.JSON(resp.Code, resp.", fieldName, ")\n")
			w.Print("\t}\n")
		}
	}

	w.WriteString("\treturn c.NoContent(resp.Code)\n")
}

// renderHeaderSet копирует заголовки из Response-структуры в HTTP-ответ echo.
func (g *Generator) renderHeaderSet(w *codegen.BufferWriter, fieldName string, hasHeaders bool) {
	if !hasHeaders {
		return
	}

	w.Print("\t\tif resp.", fieldName, "Headers != nil {\n")
	w.Print("\t\t\tfor k, vs := range resp.", fieldName, "Headers {\n")
	w.Print("\t\t\t\tfor _, v := range vs {\n")
	w.Print("\t\t\t\t\tc.Response().Header().Add(k, v)\n")
	w.Print("\t\t\t\t}\n")
	w.Print("\t\t\t}\n")
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
