package operations

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
)

// ImplClientRenderer рендерит impl/httpclient/client.gen.go: HTTP-клиент.
// Заменяет Generator.implClientFile (internal/generator/impl_client.go).
type ImplClientRenderer struct{}

func NewImplClientRenderer() *ImplClientRenderer { return &ImplClientRenderer{} }

func (ImplClientRenderer) FilePath() string    { return "impl/httpclient/client.gen.go" }
func (ImplClientRenderer) PackageName() string { return "client" }

func (r *ImplClientRenderer) Render(ctx *render.RenderContext) ([]byte, *render.ImportTracker, error) {
	ops := allOperations(ctx.Project)
	if len(ops) == 0 {
		return nil, nil, nil
	}

	imps := render.NewImportTracker()
	ctx.Imports = imps

	// Always-needed imports.
	imps.Add(gogen.Import{Path: "context"})
	imps.Add(gogen.Import{Path: "fmt"})
	imps.Add(gogen.Import{Path: "net/http"})
	imps.Add(gogen.Import{Path: "strings"})
	imps.Add(gogen.Import{Path: "nschugorev/oapigenerator/pkg/httpclient", Alias: "httpclient"})

	if ctx.Project != nil && ctx.Project.Paths != nil {
		imps.Add(gogen.Import{Path: ctx.Project.Paths.Imports.ClientInterfaces.Path, Alias: "apiclient"})
	}

	// Conditional imports.
	needJSON, needBytes, needURL := implClientImports(ops)
	if needJSON {
		imps.Add(gogen.Import{Path: "encoding/json"})
	}
	if needBytes {
		imps.Add(gogen.Import{Path: "bytes"})
	}
	if needURL {
		imps.Add(gogen.Import{Path: "net/url"})
	}
	if implNeedsStrconv(ops) {
		imps.Add(gogen.Import{Path: "strconv"})
	}

	m := ctx.TypeMapper
	w := codegen.NewBufferWriter()

	renderImplClientStruct(w)
	for _, op := range ops {
		renderImplClientMethod(w, op, m)
	}

	return w.Content(), imps, nil
}

func implClientImports(ops []*parser.Method) (needJSON, needBytes, needURL bool) {
	for _, op := range ops {
		if op.RequestBody != nil {
			if requestBodyIsURLForm(op.RequestBody) {
				needBytes = true
			} else {
				needJSON = true
				needBytes = true
			}
		}
		for _, r := range op.Responses {
			if responseSchema(r) != nil {
				needJSON = true
			}
		}
		for _, p := range op.Parameters {
			if p.In == "path" || p.In == "query" {
				needURL = true
			}
		}
	}
	return
}

func renderImplClientStruct(w *codegen.BufferWriter) {
	w.Print("var _ apiclient.Client = (*Client)(nil)\n\n")
	w.Print("type Client struct {\n")
	w.Print("\thttp *httpclient.Client\n")
	w.Print("}\n\n")
	w.Print("func NewClient(baseURL string, opts ...httpclient.Option) (*Client, error) {\n")
	w.Print("\tc, err := httpclient.NewClient(baseURL, opts...)\n")
	w.Print("\tif err != nil {\n")
	w.Print("\t\treturn nil, err\n")
	w.Print("\t}\n")
	w.Print("\treturn &Client{http: c}, nil\n")
	w.Print("}\n\n")
}

func renderImplClientMethod(w *codegen.BufferWriter, op *parser.Method, m render.TypeMapper) {
	name := operationMethodName(op)

	w.Print("func (c *Client) ", name, "(ctx context.Context, req *apiclient.", name, "Request) ")
	w.Print("(*apiclient.", name, "Response, error) {\n")

	// Path construction.
	w.Print("\tpath := \"", op.Path, "\"\n")
	for _, p := range op.Parameters {
		if p.In != "path" {
			continue
		}
		fieldName := goName(p.Name)
		w.Print("\tpath = strings.Replace(path, \"{", p.Name, "}\", ")
		w.Print("url.PathEscape(fmt.Sprint(req.", fieldName, ")), 1)\n")
	}

	// Query params.
	hasQuery := false
	for _, p := range op.Parameters {
		if p.In != "query" {
			continue
		}
		if !hasQuery {
			w.Print("\tq := url.Values{}\n")
			hasQuery = true
		}
		fieldName := goName(p.Name)
		if p.Required {
			w.Print("\tq.Set(\"", p.Name, "\", fmt.Sprint(req.", fieldName, "))\n")
		} else {
			w.Print("\tif req.", fieldName, " != nil {\n")
			w.Print("\t\tq.Set(\"", p.Name, "\", fmt.Sprint(*req.", fieldName, "))\n")
			w.Print("\t}\n")
		}
	}

	// URL assembly.
	w.Print("\tu := *c.http.ServerURL()\n")
	w.Print("\tu.Path = strings.TrimSuffix(u.Path, \"/\") + path\n")
	if hasQuery {
		w.Print("\tu.RawQuery = q.Encode()\n")
	}

	// Body encoding.
	hasBody := op.RequestBody != nil
	if hasBody {
		if requestBodyIsURLForm(op.RequestBody) {
			w.Print("\tvalues, err := req.Body.MarshalURLForm()\n")
			w.Print("\tif err != nil {\n")
			w.Print("\t\treturn nil, fmt.Errorf(\"encode body: %w\", err)\n")
			w.Print("\t}\n")
			w.Print("\tbody := []byte(values.Encode())\n")
		} else {
			w.Print("\tbody, err := json.Marshal(req.Body)\n")
			w.Print("\tif err != nil {\n")
			w.Print("\t\treturn nil, fmt.Errorf(\"encode body: %w\", err)\n")
			w.Print("\t}\n")
		}
	}

	// HTTP request.
	w.Print("\thttpReq, err := http.NewRequestWithContext(ctx, \"", op.Method, "\", u.String(), ")
	if hasBody {
		w.Print("bytes.NewReader(body)")
	} else {
		w.Print("nil")
	}
	w.Print(")\n")
	w.Print("\tif err != nil {\n\t\treturn nil, err\n\t}\n")

	// Headers.
	for _, p := range op.Parameters {
		if p.In != "header" {
			continue
		}
		fieldName := goName(p.Name)
		if p.Required {
			w.Print("\thttpReq.Header.Set(\"", p.Name, "\", fmt.Sprint(req.", fieldName, "))\n")
		} else {
			w.Print("\tif req.", fieldName, " != nil {\n")
			w.Print("\t\thttpReq.Header.Set(\"", p.Name, "\", fmt.Sprint(*req.", fieldName, "))\n")
			w.Print("\t}\n")
		}
	}
	if hasBody {
		if requestBodyIsURLForm(op.RequestBody) {
			w.Print("\thttpReq.Header.Set(\"Content-Type\", \"application/x-www-form-urlencoded\")\n")
		} else {
			w.Print("\thttpReq.Header.Set(\"Content-Type\", \"application/json\")\n")
		}
	}

	// Execute + decode.
	w.Print("\tresp, err := c.http.Do(ctx, httpReq)\n")
	w.Print("\tif err != nil {\n\t\treturn nil, err\n\t}\n")
	w.Print("\tdefer resp.Body.Close()\n")
	w.Print("\tresult := &apiclient.", name, "Response{Code: resp.StatusCode}\n")
	w.Print("\tswitch resp.StatusCode {\n")

	var defaultResp *parser.Response
	for _, r := range op.Responses {
		if r.StatusCode == "default" {
			defaultResp = r
			continue
		}
		renderImplResponseCase(w, op, r, m)
	}
	if defaultResp != nil {
		w.Print("\tdefault:\n")
		renderImplResponseBody(w, op, "default", defaultResp, "ResponseDefault", m)
	} else {
		w.Print("\tdefault:\n")
		w.WriteString("\t\treturn nil, fmt.Errorf(\"unexpected status code: %d\", resp.StatusCode)\n")
	}

	w.Print("\t}\n")
	w.Print("\treturn result, nil\n")
	w.Print("}\n\n")
}

func renderImplResponseCase(w *codegen.BufferWriter, op *parser.Method, r *parser.Response, m render.TypeMapper) {
	w.Print("\tcase ", r.StatusCode, ":\n")
	renderImplResponseBody(w, op, r.StatusCode, r, responseFieldName(r.StatusCode), m)
}

func renderImplResponseBody(w *codegen.BufferWriter, op *parser.Method, label string, r *parser.Response, fieldName string, m render.TypeMapper) {
	schema := responseSchema(r)

	if !hasResponseHeaders(r) {
		if schema == nil {
			w.Print("\t\tresult.", fieldName, " = true\n")
			return
		}
		m.SetMode("Response")
		typ := m.GoType(schema)
		w.Print("\t\tvar v ", typ, "\n")
		w.Print("\t\tif err := json.NewDecoder(resp.Body).Decode(&v); err != nil {\n")
		w.Print("\t\t\treturn nil, fmt.Errorf(\"decode ", label, ": %w\", err)\n")
		w.Print("\t\t}\n")
		w.Print("\t\tresult.", fieldName, " = &v\n")
		return
	}

	// Response with headers.
	typeName := payloadWithHeadersTypeName(op, r.StatusCode)
	w.Print("\t\tresult.", fieldName, " = &", typeName, "{}\n")

	if schema != nil {
		m.SetMode("Response")
		typ := m.GoType(schema)
		w.Print("\t\tvar v ", typ, "\n")
		w.Print("\t\tif err := json.NewDecoder(resp.Body).Decode(&v); err != nil {\n")
		w.Print("\t\t\treturn nil, fmt.Errorf(\"decode ", label, ": %w\", err)\n")
		w.Print("\t\t}\n")
		w.Print("\t\tresult.", fieldName, ".Payload = &v\n")
	} else {
		w.Print("\t\tresult.", fieldName, ".Payload = true\n")
	}

	for _, hdr := range sortedHeaders(r.Headers) {
		hdrType := headerGoBaseType(hdr.Schema)
		field := goName(hdr.Name)
		expr, needsErr := headerDecodeExpr(hdr.Name, hdrType)

		if needsErr {
			w.Print("\t\t{\n")
			w.Print("\t\t\traw, err := ", expr, "\n")
			w.Print("\t\t\tif err != nil {\n")
			w.Print("\t\t\t\treturn nil, fmt.Errorf(\"decode header ", hdr.Name, ": %w\", err)\n")
			w.Print("\t\t\t}\n")
			w.Print("\t\t\tresult.", fieldName, ".", field, " = ", headerDecodeConvert("raw", hdrType), "\n")
			w.Print("\t\t}\n")
		} else {
			w.Print("\t\tresult.", fieldName, ".", field, " = ", expr, "\n")
		}
	}
}

var _ render.SingletonRenderer = (*ImplClientRenderer)(nil)
