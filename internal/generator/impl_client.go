package generator

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/parser"
)

// implClientFile генерирует impl/httpclient/client.gen.go:
// реализацию client.Client через pkg/httpclient.
func (g *Generator) implClientFile() codegen.File {
	m := g.newTypeMapper("implclient")
	m.addImport("context", "")
	m.addImport("fmt", "")
	m.addImport("net/http", "")
	m.addImport("strings", "")

	const httpclientPkg = "nschugorev/oapigenerator/pkg/httpclient"

	m.addImport(httpclientPkg, "httpclient")

	if g.modulePath != "" {
		m.addImport(g.modulePath+"/interfaces/client", "apiclient")
	}

	needJSON, needBytes, needURL := g.implClientImports()
	if needJSON {
		m.addImport("encoding/json", "")
	}

	if needBytes {
		m.addImport("bytes", "")
	}

	if needURL {
		m.addImport("net/url", "")
	}

	body := g.renderImplClient(m)

	return g.factory.Create(&gogen.File{
		Package: "client",
		Imports: m.imports,
		Body:    body,
	})
}

//nolint:gocritic // unnamedResult conflicts with nonamedreturns
func (g *Generator) implClientImports() (bool, bool, bool) {
	var needJSON, needBytes, needURL bool

	for _, op := range g.doc.Operations {
		if op.RequestBody != nil {
			needJSON = true
			needBytes = true
		}

		for _, r := range op.Responses {
			if responseSchema(r) != nil {
				needJSON = true
			}
		}

		for _, p := range op.Parameters {
			if p.In == oapiParamPath || p.In == oapiParamQuery {
				needURL = true
			}
		}
	}

	return needJSON, needBytes, needURL
}

func (g *Generator) renderImplClient(m *typeMapper) []byte {
	w := codegen.NewBufferWriter()

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

	for _, op := range g.doc.Operations {
		g.renderImplClientMethod(w, op, m)
	}

	return w.Content()
}

//nolint:gocognit,gocyclo,cyclop,funlen,lll // template-style codegen, long signature
func (g *Generator) renderImplClientMethod(w *codegen.BufferWriter, op *parser.Operation, m *typeMapper) {
	name := operationMethodName(op)

	w.Print("func (c *Client) ", name, "(ctx context.Context, req *apiclient.", name, "Request) ")
	w.Print("(*apiclient.", name, "Response, error) {\n")

	w.Print("\tpath := \"", op.Path, "\"\n")

	for _, p := range op.Parameters {
		if p.In != oapiParamPath {
			continue
		}

		fieldName := goName(p.Name)
		w.Print("\tpath = strings.Replace(path, \"{", p.Name, "}\", ")
		w.Print("url.PathEscape(fmt.Sprint(req.", fieldName, ")), 1)\n")
	}

	hasQuery := false

	for _, p := range op.Parameters {
		if p.In != oapiParamQuery {
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

	w.Print("\tu := *c.http.ServerURL()\n")
	w.Print("\tu.Path = strings.TrimSuffix(u.Path, \"/\") + path\n")

	if hasQuery {
		w.Print("\tu.RawQuery = q.Encode()\n")
	}

	hasBody := op.RequestBody != nil
	if hasBody {
		w.Print("\tbody, err := json.Marshal(req.Body)\n")
		w.Print("\tif err != nil {\n")
		w.Print("\t\treturn nil, fmt.Errorf(\"encode body: %w\", err)\n")
		w.Print("\t}\n")
	}

	w.Print("\thttpReq, err := http.NewRequestWithContext(ctx, \"", op.Method, "\", u.String(), ")

	if hasBody {
		w.Print("bytes.NewReader(body)")
	} else {
		w.Print("nil")
	}

	w.Print(")\n")
	w.Print("\tif err != nil {\n")
	w.Print("\t\treturn nil, err\n")
	w.Print("\t}\n")

	for _, p := range op.Parameters {
		if p.In != oapiParamHeader {
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
		w.Print("\thttpReq.Header.Set(\"Content-Type\", \"application/json\")\n")
	}

	w.Print("\tresp, err := c.http.Do(ctx, httpReq)\n")
	w.Print("\tif err != nil {\n")
	w.Print("\t\treturn nil, err\n")
	w.Print("\t}\n")
	w.Print("\tdefer resp.Body.Close()\n")

	w.Print("\tresult := &apiclient.", name, "Response{Code: resp.StatusCode}\n")
	w.Print("\tswitch resp.StatusCode {\n")

	hasDefault := false

	var defaultResp *parser.Response

	for _, r := range op.Responses {
		if r.StatusCode == oapiCodeDefault {
			hasDefault = true
			defaultResp = r

			continue
		}

		g.renderImplResponseCase(w, r, m)
	}

	if hasDefault {
		w.Print("\tdefault:\n")
		g.renderImplResponseBody(w, oapiCodeDefault, defaultResp, "ResponseDefault", m)
	} else {
		w.Print("\tdefault:\n")
		w.WriteString("\t\treturn nil, fmt.Errorf(\"unexpected status code: %d\", resp.StatusCode)\n")
	}

	w.Print("\t}\n")
	w.Print("\treturn result, nil\n")
	w.Print("}\n\n")
}

func (g *Generator) renderImplResponseCase(w *codegen.BufferWriter, r *parser.Response, m *typeMapper) { //nolint:lll // function signature
	w.Print("\tcase ", r.StatusCode, ":\n")
	fieldName := responseFieldName(r.StatusCode)
	g.renderImplResponseBody(w, r.StatusCode, r, fieldName, m)
}

func (g *Generator) renderImplResponseBody(w *codegen.BufferWriter, label string, r *parser.Response, fieldName string, m *typeMapper) { //nolint:lll // function signature
	schema := responseSchema(r)
	if schema == nil {
		w.Print("\t\tresult.", fieldName, " = true\n")

		return
	}

	prevMode := m.mode
	m.mode = modeResponse
	typ := m.goType(schema)

	m.mode = prevMode

	w.Print("\t\tvar v ", typ, "\n")
	w.Print("\t\tif err := json.NewDecoder(resp.Body).Decode(&v); err != nil {\n")
	w.Print("\t\t\treturn nil, fmt.Errorf(\"decode ", label, ": %w\", err)\n")
	w.Print("\t\t}\n")
	w.Print("\t\tresult.", fieldName, " = &v\n")
}
