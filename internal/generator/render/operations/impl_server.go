package operations

import (
	"strings"

	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
)

// ImplServerRenderer рендерит impl/echoserver/server.gen.go: Echo-сервер.
// Заменяет Generator.implServerFile (internal/generator/impl_server.go).
type ImplServerRenderer struct{}

func NewImplServerRenderer() *ImplServerRenderer { return &ImplServerRenderer{} }

func (ImplServerRenderer) FilePath() string    { return "impl/echoserver/server.gen.go" }
func (ImplServerRenderer) PackageName() string { return "server" }

func (r *ImplServerRenderer) Render(ctx *render.RenderContext) ([]byte, *render.ImportTracker, error) {
	ops := allOperations(ctx.Project)
	if len(ops) == 0 {
		return nil, nil, nil
	}

	imps := render.NewImportTracker()
	ctx.Imports = imps

	imps.Add(gogen.Import{Path: "github.com/labstack/echo/v4"})

	if ctx.Project != nil && ctx.Project.Paths != nil {
		imps.Add(gogen.Import{Path: ctx.Project.Paths.Imports.ClientInterfaces.Path, Alias: "apiclient"})
		imps.Add(gogen.Import{Path: ctx.Project.Paths.Imports.ServerInterfaces.Path, Alias: "apiserver"})
	}

	needBody, needURLForm := false, false
	for _, op := range ops {
		if op.RequestBody != nil {
			needBody = true
			if requestBodyIsURLForm(op.RequestBody) {
				needURLForm = true
			}
		}
	}

	if needBody {
		imps.Add(gogen.Import{Path: "bytes"})
		imps.Add(gogen.Import{Path: "encoding/json"})
		imps.Add(gogen.Import{Path: "io"})
		imps.Add(gogen.Import{Path: "net/http"})
	}
	if needURLForm {
		imps.Add(gogen.Import{Path: "net/url"})
		imps.Add(gogen.Import{Path: "strings"})
	}

	noAutoDefaults := ctx.Features.ServerNoAutoDefaults.Value

	w := codegen.NewBufferWriter()
	renderImplServerStruct(w, needBody, needURLForm)

	// Register.
	w.Print("func (s *ServerHTTP) Register(e *echo.Echo) {\n")
	for _, op := range ops {
		method := strings.ToUpper(op.Method)
		epath := echoPath(op.Path)
		handler := lowerFirst(operationMethodName(op))
		w.Print("\te.", method, "(\"", epath, "\", s.", handler, ")\n")
	}
	w.Print("}\n\n")

	// Methods.
	for _, op := range ops {
		renderImplServerMethod(w, op, noAutoDefaults, ctx.Project)
	}

	return w.Content(), imps, nil
}

func renderImplServerStruct(w *codegen.BufferWriter, needBody, needURLForm bool) {
	w.Print("type ServerHTTP struct {\n")
	w.Print("\timpl apiserver.Server\n")
	w.Print("}\n\n")

	w.Print("func NewServerHTTP(impl apiserver.Server) *ServerHTTP {\n")
	w.Print("\treturn &ServerHTTP{impl: impl}\n")
	w.Print("}\n\n")

	if needBody {
		renderBindBody(w, needURLForm)
	}
}

func renderBindBody(w *codegen.BufferWriter, needURLForm bool) {
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
	w.Print("\tif err != nil {\n\t\treturn err\n\t}\n")
	w.Print("\tc.Request().Body = io.NopCloser(bytes.NewReader(body))\n")
	w.Print("\tif len(body) == 0 {\n\t\treturn nil\n\t}\n")
	w.Print("\tif err := json.Unmarshal(body, dst); err != nil {\n")
	w.Print("\t\treturn echo.NewHTTPError(http.StatusBadRequest, err.Error())\n")
	w.Print("\t}\n")
	w.Print("\treturn nil\n")
	w.Print("}\n\n")
}

func renderImplServerMethod(w *codegen.BufferWriter, op *parser.Method, noAutoDefaults bool, project *parser.Project) {
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

	w.Print("\tif err := c.Bind(req); err != nil {\n\t\treturn err\n\t}\n")

	if !noAutoDefaults && shouldCallSetDefaults(op, project) {
		w.Print("\treq.Body.SetDefaults()\n")
	}

	w.Print("\tresp, err := s.impl.", name, "(c.Request().Context(), req)\n")
	w.Print("\tif err != nil {\n\t\treturn err\n\t}\n")

	renderImplServerResponse(w, op)
	w.Print("}\n\n")
}

func shouldCallSetDefaults(op *parser.Method, project *parser.Project) bool {
	if op.RequestBody == nil {
		return false
	}
	sh := resolveBodySchema(op.RequestBody, project)
	if sh == nil || sh.Name == "" {
		return false
	}
	return schemaHasDefaults(sh)
}

// schemaHasDefaults проверяет, есть ли default-значения у самой схемы или её свойств.
func schemaHasDefaults(sh *parser.Schema) bool {
	for _, p := range sh.Properties {
		if p.Schema != nil && p.Schema.Default != nil {
			return true
		}
	}
	return false
}

func renderImplServerResponse(w *codegen.BufferWriter, op *parser.Method) {
	for _, r := range op.Responses {
		if r.StatusCode == "default" {
			continue
		}
		renderImplServerStatusCodeResponse(w, r, responseFieldName(r.StatusCode))
	}
	for _, r := range op.Responses {
		if r.StatusCode != "default" {
			continue
		}
		renderImplServerStatusCodeResponse(w, r, "ResponseDefault")
	}
	w.WriteString("\treturn c.NoContent(resp.Code)\n")
}

func renderImplServerStatusCodeResponse(w *codegen.BufferWriter, r *parser.Response, fieldName string) {
	hasHeaders := hasResponseHeaders(r)
	hasSchema := responseSchema(r) != nil

	if !hasHeaders && !hasSchema {
		w.Print("\tif resp.", fieldName, " {\n")
		renderStatusCodeReturn(w, r, "NoContent", "")
		w.Print("\t}\n")
		return
	}

	w.Print("\tif resp.", fieldName, " != nil {\n")
	if hasHeaders {
		w.Print("\t\tfor k, v := range resp.", fieldName, ".Headers() {\n")
		w.Print("\t\t\tc.Response().Header().Set(k, v)\n")
		w.Print("\t\t}\n")
	}
	if hasSchema {
		renderStatusCodeReturn(w, r, "JSON", "resp."+fieldName)
	} else {
		renderStatusCodeReturn(w, r, "NoContent", "")
	}
	w.Print("\t}\n")
}

func renderStatusCodeReturn(w *codegen.BufferWriter, r *parser.Response, method, field string) {
	codeExpr := r.StatusCode
	if r.StatusCode == "default" {
		codeExpr = "resp.Code"
	}
	if field == "" {
		w.Print("\t\treturn c.", method, "(", codeExpr, ")\n")
	} else {
		w.Print("\t\treturn c.", method, "(", codeExpr, ", ", field, ")\n")
	}
}

func echoPath(path string) string {
	return strings.NewReplacer("{", ":", "}", "").Replace(path)
}

func lowerFirst(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToLower(s[:1]) + s[1:]
}

var _ render.SingletonRenderer = (*ImplServerRenderer)(nil)
