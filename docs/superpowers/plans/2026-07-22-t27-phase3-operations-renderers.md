# T27 Phase 3 — Operations-side Singleton Renderers Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Migrate `clientFile()`, `clientSugarFile()`, `auditClientFile()`, `serverFile()` from `*Generator` methods into `SingletonRenderer` implementations in `internal/generator/render/operations/`, dispatched via `compose.FileComposer.ComposeSingletonFile`.

**Architecture:** Four new `SingletonRenderer` types in `render/operations/`, each producing one file. Shared helpers (`goName`, `operationMethodName`, `sortedResponseCodes`, etc.) extracted to `helpers.go`. Renderers access operations via `ctx.Project.Paths.Services` and types via `ctx.TypeMapper`. Imports tracked per-renderer via local `ImportTracker` set on `ctx.Imports`.

**Tech Stack:** Go 1.22+, `nschugorev/oapigenerator/internal/codegen`, `nschugorev/oapigenerator/internal/codegen/gogen`, `nschugorev/oapigenerator/internal/generator/render`, `nschugorev/oapigenerator/internal/generator/compose`, `nschugorev/oapigenerator/internal/parser`, `github.com/stretchr/testify/assert` + `require`.

**Design:** `docs/superpowers/specs/2026-07-22-t27-phase3-operations-renderers-design.md`

---

## File Structure

**Create:**
- `internal/generator/render/operations/helpers.go` — shared helpers ported from `client.go`, `naming.go`, `operation.go`, `response_headers.go`, `type.go`, `audit_server.go`, `server.go`
- `internal/generator/render/operations/client_interface.go` — `ClientInterfaceRenderer`
- `internal/generator/render/operations/client_interface_test.go` — unit tests
- `internal/generator/render/operations/server_interface.go` — `ServerInterfaceRenderer`
- `internal/generator/render/operations/server_interface_test.go` — unit tests
- `internal/generator/render/operations/client_sugar.go` — `ClientSugarRenderer`
- `internal/generator/render/operations/client_sugar_test.go` — unit tests
- `internal/generator/render/operations/audit_client.go` — `AuditClientRenderer`
- `internal/generator/render/operations/audit_client_test.go` — unit tests

**Modify:**
- `internal/generator/generator.go` — add `newOperationsRenderContext`, rewrite `writeOperationFiles` to dispatch interface files via `ComposeSingletonFile`; keep impl/mocks/sdk as legacy
- `internal/generator/generator_test.go` — append 4 wire tests

**Delete:**
- `internal/generator/client.go`
- `internal/generator/server.go`
- `internal/generator/client_sugar.go`
- `internal/generator/audit_server.go`

**Keep unchanged:**
- `internal/generator/compose/composer.go` — `ComposeSingletonFile` already exists
- `internal/generator/render/base.go` — `SingletonRenderer` interface already exists
- `internal/generator/impl_client.go`, `impl_server.go`, `mocks.go`, `sdk.go` — out of Phase 3 scope
- `internal/generator/response_headers.go` — `renderPayloadWithHeadersType` is used by `client.go`, will be ported; `writePayloadWithHeadersFile` is used from `writeSchemaAuxFiles` via `renderPayloadWithHeadersType` — must keep the function but it's only called during schema rendering, not operations. Check: `response_headers.go` has `writePayloadWithHeadersFile` used by `schemaAuxRenderer` in generator.go:163. Keep `response_headers.go` intact.

---

### Task 1: Helpers + ClientInterfaceRenderer

**Files:**
- Create: `internal/generator/render/operations/helpers.go`
- Create: `internal/generator/render/operations/client_interface.go`
- Create: `internal/generator/render/operations/client_interface_test.go`

This task creates the foundational helpers file and the first (largest) renderer. All subsequent renderer tasks depend on `helpers.go` existing.

- [ ] **Step 1: Create helpers.go with shared functions**

Create `internal/generator/render/operations/helpers.go`:

```go
package operations

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
	"sort"
	"strings"
	"unicode"
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

// goName конвертирует PascalCase (портировано из generator.goName).
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

// operationMethodName возвращает Go-имя метода интерфейса для операции.
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

// responseFieldName строит имя поля Response-структуры для кода ответа.
func responseFieldName(code string) string {
	return "Response" + goName(code)
}

// isSuccessCode сообщает, является ли код ответа 2xx.
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

// responsePayloadType возвращает Go-тип поля для ответа.
func responsePayloadType(resp *parser.Response, m render.TypeMapper) string {
	schema := responseSchema(resp)
	if schema == nil {
		return "bool"
	}
	return "*" + m.GoType(schema)
}

// ---- Request helpers (портированы из client.go) ----

func isInherentlyNilable(t string) bool {
	switch t {
	case "string", "bool", "error":
		return true
	default:
		return strings.HasPrefix(t, "[]") || strings.HasPrefix(t, "map[")
	}
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

// writeDocComment пишет комментарий к полю, если описание не пустое.
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
	case "integer":
		return "int"
	case "number":
		return "float64"
	case "boolean":
		return "bool"
	default:
		return "string"
	}
}

// firstSuccessResponse возвращает код и схему первого 2xx-ответа.
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

// sugarReturnType возвращает Go-тип возвращаемого значения sugar-метода.
func sugarReturnType(op *parser.Method, successCode string, successSchema *parser.Schema, m render.TypeMapper) (string, bool) {
	if successCode != "" {
		resp := responseByCode(op.Responses, successCode)
		if hasResponseHeaders(resp) {
			return "*" + payloadWithHeadersTypeName(op, successCode), true
		}
	}
	if successSchema != nil {
		prevMode := m.(modeSettable).Mode()
		m.SetMode("Response")
		typ := m.GoType(successSchema)
		m.SetMode(prevMode)
		return "*" + typ, true
	}
	return "", false
}

// filterParamsByIn возвращает параметры с указанным In.
func filterParamsByIn(params []*parser.Parameter, in string) []*parser.Parameter {
	out := make([]*parser.Parameter, 0, len(params))
	for _, p := range params {
		if p.In == in {
			out = append(out, p)
		}
	}
	return out
}

// qualifyClient добавляет префикс "client." к имени типа, если model-импорт
// задан (сервер рендерится в отдельном пакете).
func qualifyClient(name, suffix string, modelImportPath string) string {
	if modelImportPath == "" {
		return name + suffix
	}
	return "client." + name + suffix
}

// modeSettable — внутренний интерфейс для доступа к Mode() на typeMapperAdapter.
// sugarReturnType нужно сохранять/восстанавливать режим, не расширяя публичный TypeMapper.
type modeSettable interface {
	Mode() string
}
```

Note: `sugarReturnType` uses a type assertion to `modeSettable` to save/restore mode. The `typeMapperAdapter` already has `Mode() string` (generator.go:67). If the assertion fails in tests, mock implementations must implement `Mode()` too. This is an internal interface — not exported.

- [ ] **Step 2: Write failing test for ClientInterfaceRenderer**

Create `internal/generator/render/operations/client_interface_test.go`:

```go
package operations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
)

func TestClientInterfaceRenderer_FilePath(t *testing.T) {
	t.Parallel()
	r := NewClientInterfaceRenderer()
	assert.Equal(t, "interfaces/client/client.gen.go", r.FilePath())
}

func TestClientInterfaceRenderer_EmptyProject_EmptyInterface(t *testing.T) {
	t.Parallel()
	r := NewClientInterfaceRenderer()
	ctx := &render.RenderContext{
		Project: &parser.Project{},
	}

	body, _, err := r.Render(ctx)
	require.NoError(t, err)
	got := string(body)
	assert.Contains(t, got, "type Client interface {")
	assert.Contains(t, got, "}")
}

func TestClientInterfaceRenderer_SingleOperation_RendersInterfaceAndStructs(t *testing.T) {
	t.Parallel()

	ctx := newClientTestCtx(t, []*parser.Method{
		{
			OperationID: "listPets",
			Parameters: []*parser.Parameter{
				{Name: "limit", In: "query", Schema: &parser.Schema{Type: "integer"}},
			},
		},
	})

	r := NewClientInterfaceRenderer()
	body, _, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)
	assert.Contains(t, got, "type Client interface {")
	assert.Contains(t, got, "ListPets(ctx context.Context, req *ListPetsRequest) (*ListPetsResponse, error)")
	assert.Contains(t, got, "type ListPetsRequest struct {")
	assert.Contains(t, got, "Limit *int `query:\"limit\"`")
	assert.Contains(t, got, "type ListPetsResponse struct {")
	assert.Contains(t, got, "Code int")
}

func TestClientInterfaceRenderer_DeprecatedOperation_HasComment(t *testing.T) {
	t.Parallel()

	ctx := newClientTestCtx(t, []*parser.Method{
		{
			OperationID: "oldMethod",
			Deprecated:  true,
		},
	})

	r := NewClientInterfaceRenderer()
	body, _, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)
	assert.Contains(t, got, "// Deprecated: operation is marked as deprecated")
}

func TestClientInterfaceRenderer_PathParam_RendersWithoutPointer(t *testing.T) {
	t.Parallel()

	ctx := newClientTestCtx(t, []*parser.Method{
		{
			OperationID: "getItem",
			Parameters: []*parser.Parameter{
				{Name: "id", In: "path", Required: true, Schema: &parser.Schema{Type: "string"}},
			},
		},
	})

	r := NewClientInterfaceRenderer()
	body, _, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)
	assert.Contains(t, got, "ID string `param:\"id\"`")
}

// newClientTestCtx строит RenderContext с project, содержащим один сервис
// с переданными методами. TypeMapper — mock, возвращающий schema.Type как Go-тип.
func newClientTestCtx(t *testing.T, methods []*parser.Method) *render.RenderContext {
	t.Helper()
	return &render.RenderContext{
		Project: &parser.Project{
			Paths: &parser.Paths{
				Services: []*parser.Service{
					{Name: "default", Methods: methods},
				},
			},
		},
		TypeMapper: &mockTypeMapper{},
	}
}

// mockTypeMapper реализует render.TypeMapper и modeSettable для тестов.
type mockTypeMapper struct{ mode string }

func (m *mockTypeMapper) GoType(s *parser.Schema) string {
	if s == nil {
		return "string"
	}
	switch s.Type {
	case "integer":
		return "int"
	case "number":
		return "float64"
	case "boolean":
		return "bool"
	default:
		if s.Ref != "" {
			return "model." + goName(s.Ref)
		}
		return s.Type
	}
}

func (m *mockTypeMapper) SetMode(mode string) { m.mode = mode }
func (m *mockTypeMapper) Mode() string        { return m.mode }
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/generator/render/operations/ -run TestClientInterfaceRenderer -v`
Expected: FAIL with `undefined: NewClientInterfaceRenderer`.

- [ ] **Step 4: Write ClientInterfaceRenderer implementation**

Create `internal/generator/render/operations/client_interface.go`:

```go
package operations

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
	"strings"
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

// renderPayloadWithHeadersType рендерит тип PayloadWithHeaders для ответа с headers.
// Портировано из response_headers.go:renderPayloadWithHeadersType (legacy).
func renderPayloadWithHeadersType(w *codegen.BufferWriter, op *parser.Method, code string, resp *parser.Response, m render.TypeMapper) {
	typeName := payloadWithHeadersTypeName(op, code)

	w.Print("type ", typeName, " struct {\n")
	w.Print("\tPayload *", m.GoType(responseSchema(resp)), "\n")

	for _, h := range resp.Headers {
		goType := headerGoBaseType(h.Schema)
		w.Print("\t", goName(h.Name), " ", goType, "\n")
	}

	w.Print("}\n\n")

	// MarshalJSON
	w.Print("func (p ", typeName, ") MarshalJSON() ([]byte, error) {\n")
	w.Print("\ttype payload ", typeName, "\n")
	w.Print("\treturn json.Marshal(payload(p))\n")
	w.Print("}\n\n")

	// Headers
	w.Print("func (p ", typeName, ") Headers() ([]string, map[string]string) {\n")
	w.Print("\theaders := map[string]string{}\n")
	for _, h := range resp.Headers {
		f := goName(h.Name)
		base := headerGoBaseType(h.Schema)
		switch base {
		case "string":
			w.Print("\theaders[\"", h.Name, "\"] = string(p.", f, ")\n")
		default:
			w.Print("\theaders[\"", h.Name, "\"] = fmt.Sprint(p.", f, ")\n")
		}
	}
	w.Print("\treturn nil, headers\n")
	w.Print("}\n\n")
}

var _ render.SingletonRenderer = (*ClientInterfaceRenderer)(nil)
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/generator/render/operations/ -run TestClientInterfaceRenderer -v`
Expected: PASS — all 5 tests green.

- [ ] **Step 6: Commit**

```bash
git add internal/generator/render/operations/
git commit -m "T27 Phase 3 Task 1: helpers + ClientInterfaceRenderer"
```

---

### Task 2: Wire ClientInterfaceRenderer + delete legacy client.go + response_headers.go clean up

**Files:**
- Modify: `internal/generator/generator.go` — add `newOperationsRenderContext`, rewrite `writeOperationFiles`
- Modify: `internal/generator/generator_test.go` — append wire test
- Modify: `internal/generator/response_headers.go` — check if `renderPayloadWithHeadersType` can be removed (since it's ported to helpers.go)

**Important:** `response_headers.go` has `writePayloadWithHeadersFile` (called from `writeSchemaAuxFiles` as schema-level aux file) AND `renderPayloadWithHeadersType` (called by `client.go`'s `renderResponseStruct`). After deleting `client.go`, the `renderPayloadWithHeadersType` function in `response_headers.go` becomes unused — but `writePayloadWithHeadersFile` is still needed. Only remove `renderPayloadWithHeadersType` from `response_headers.go` if it's unused after `client.go` deletion.

Actually, `renderPayloadWithHeadersType` in `response_headers.go` is called from `renderResponseStruct` in `client.go`. Since we're replacing `client.go`, the legacy `renderPayloadWithHeadersType` becomes unused. But we ported it to `client_interface.go`. The legacy one in `response_headers.go` should be removed. However, `writePayloadWithHeadersFile` in the same file still uses `headerGoBaseType`, `payloadWithHeadersTypeName`, etc. — check what stays.

Let's verify by checking what else in `response_headers.go` is used after `client.go` is deleted.

- [ ] **Step 1: Write wire test**

Append to `internal/generator/generator_test.go`:

```go
func TestGenerate_ClientInterfaceFile_EmitsFile(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /items:
    get:
      operationId: listItems
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                type: array
                items: {type: string}
`)
	project := testProject(t, doc, "example.com/test")

	fw := &collectWriter{files: map[string][]byte{}}
	require.NoError(t, Generate(fw, project, nil))

	body := string(fw.files["interfaces/client/client.gen.go"])
	assert.Contains(t, body, "type Client interface {")
	assert.Contains(t, body, "ListItems(ctx context.Context, req *ListItemsRequest) (*ListItemsResponse, error)")
	assert.Contains(t, body, "type ListItemsResponse struct {")
}
```

- [ ] **Step 2: Run test to verify it passes against old code**

Run: `go test ./internal/generator/ -run TestGenerate_ClientInterfaceFile -v`
Expected: PASS (old `clientFile()` produces matching output).

- [ ] **Step 3: Add newOperationsRenderContext to generator.go**

In `internal/generator/generator.go`, after `newSchemaRenderContext` (line ~190), add:

```go
// newOperationsRenderContext строит RenderContext для operations-рендеринга.
// pkg — имя целевого пакета ("client" или "server").
func (g *Generator) newOperationsRenderContext(pkg string) *render.RenderContext {
	ctx := &render.RenderContext{
		Project:      g.project,
		SchemaIndex:  g.schemaIndex,
		Features:     g.project.Features,
		Splittable:   g.splittable,
		ModulePath:   g.project.ImportPrefix,
		ImportPrefix: g.project.ImportPrefix,
	}
	ctx.TypeMapper = g.newRenderTypeMapper(pkg, "", ctx)
	return ctx
}
```

- [ ] **Step 4: Rewrite writeOperationFiles in generator.go**

Replace the first 4 entries in `writeOperationFiles` (lines 518-532). The current code:

```go
func (g *Generator) writeOperationFiles(fw codegen.FileWriter) error {
	files := []struct {
		path string
		gen  func() codegen.File
	}{
		{"interfaces/client/client.gen.go", g.clientFile},
		{"interfaces/client/client_sugar.gen.go", g.clientSugarFile},
		{"interfaces/client/audit.gen.go", g.auditClientFile},
		{"interfaces/server/server.gen.go", g.serverFile},
		{"impl/httpclient/client.gen.go", g.implClientFile},
		{"impl/echoserver/server.gen.go", g.implServerFile},
		{"impl/mocks/client/mocks.gen.go", g.mockClientFile},
		{"impl/mocks/server/mocks.gen.go", g.mockServerFile},
		{"sdk/sdk.gen.go", g.sdkFile},
	}

	for _, f := range files {
		if err := fw.WriteFile(f.path, f.gen()); err != nil {
			return fmt.Errorf("write %s: %w", f.path, err)
		}
	}

	return nil
}
```

Replace with:

```go
func (g *Generator) writeOperationFiles(fw codegen.FileWriter) error {
	// Singleton-renderer'ы для интерфейсных файлов (Phase 3).
	singletonRenderers := []struct {
		path string
		r    render.SingletonRenderer
	}{
		{"interfaces/client/client.gen.go", opsrender.NewClientInterfaceRenderer()},
		{"interfaces/client/client_sugar.gen.go", opsrender.NewClientSugarRenderer()},
		{"interfaces/client/audit.gen.go", opsrender.NewAuditClientRenderer()},
		{"interfaces/server/server.gen.go", opsrender.NewServerInterfaceRenderer()},
	}

	ctx := g.newOperationsRenderContext("client")
	for _, sr := range singletonRenderers {
		// ServerInterfaceRenderer needs "server" mapper — rebuild ctx for it.
		if sr.path == "interfaces/server/server.gen.go" {
			ctx = g.newOperationsRenderContext("server")
		}
		file, err := g.composer.ComposeSingletonFile(sr.r, ctx)
		if err != nil {
			return fmt.Errorf("compose %s: %w", sr.path, err)
		}
		if err := fw.WriteFile(sr.path, file); err != nil {
			return fmt.Errorf("write %s: %w", sr.path, err)
		}
	}

	// Legacy (impl, mocks, sdk) — будут мигрированы в следующих фазах.
	legacyFiles := []struct {
		path string
		gen  func() codegen.File
	}{
		{"impl/httpclient/client.gen.go", g.implClientFile},
		{"impl/echoserver/server.gen.go", g.implServerFile},
		{"impl/mocks/client/mocks.gen.go", g.mockClientFile},
		{"impl/mocks/server/mocks.gen.go", g.mockServerFile},
		{"sdk/sdk.gen.go", g.sdkFile},
	}

	for _, f := range legacyFiles {
		if err := fw.WriteFile(f.path, f.gen()); err != nil {
			return fmt.Errorf("write %s: %w", f.path, err)
		}
	}

	return nil
}
```

Add import for `opsrender`:

```go
import (
	// ... existing imports ...
	opsrender "nschugorev/oapigenerator/internal/generator/render/operations"
)
```

- [ ] **Step 5: Clean up response_headers.go**

After this step, `client.go` is deleted. Check what in `response_headers.go` becomes unused:
- `renderPayloadWithHeadersType` — only called from `client.go:renderResponseStruct` → remove.
- `writePayloadWithHeadersFile` — still called from `writeSchemaAuxFiles` (schema aux renderer) → keep.
- `headerGoBaseType`, `payloadWithHeadersTypeName` — used by both `renderPayloadWithHeadersType` AND `writePayloadWithHeadersFile`. If we remove `renderPayloadWithHeadersType`, these are still used by `writePayloadWithHeadersFile` → keep.

To remove `renderPayloadWithHeadersType` from `response_headers.go`, find the function and delete it. But wait — step 5 is about deleting `client.go` and removing its function. Let me make this simpler: just delete `client.go`, and check if anything breaks. If `renderPayloadWithHeadersType` in `response_headers.go` becomes "unused", the compiler will flag it as an error (in Go, unused functions within a package are allowed, so no compiler error — only linters would flag it).

Actually, `response_headers.go` is in package `generator` — unused functions in the same package don't cause compiler errors. So we can safely remove `renderPayloadWithHeadersType` from `response_headers.go` or leave it — it won't break the build. Let's clean it up in Step 5.

In `internal/generator/response_headers.go`, locate and delete the function `renderPayloadWithHeadersType` (keep `writePayloadWithHeadersFile` and helper functions that it uses).

Read `response_headers.go` first to find the exact boundaries. The function signature is `func (g *Generator) renderPayloadWithHeadersType(...)` — it's a method on Generator. The function we ported in `client_interface.go` is a package-level function with the same name but different receiver. They don't conflict because the legacy one is `(g *Generator) renderPayloadWithHeadersType` and the new one is `func renderPayloadWithHeadersType`.

- [ ] **Step 6: Delete legacy client.go**

```bash
rm internal/generator/client.go
```

- [ ] **Step 7: Run build + tests**

Run: `go build ./... && go test ./internal/generator/ -v -run "TestGenerate_ClientInterfaceFile"`

Expected: build green, test PASS (renderer wired, file emitted).

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "T27 Phase 3 Task 2: wire ClientInterfaceRenderer, delete legacy client.go"
```

---

### Task 3: ServerInterfaceRenderer

**Files:**
- Create: `internal/generator/render/operations/server_interface.go`
- Create: `internal/generator/render/operations/server_interface_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/generator/render/operations/server_interface_test.go`:

```go
package operations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
)

func TestServerInterfaceRenderer_FilePath(t *testing.T) {
	t.Parallel()
	r := NewServerInterfaceRenderer()
	assert.Equal(t, "interfaces/server/server.gen.go", r.FilePath())
}

func TestServerInterfaceRenderer_EmptyProject_EmptyInterface(t *testing.T) {
	t.Parallel()
	r := NewServerInterfaceRenderer()
	ctx := &render.RenderContext{
		Project: &parser.Project{},
	}

	body, _, err := r.Render(ctx)
	require.NoError(t, err)
	got := string(body)
	assert.Contains(t, got, "type Server interface {")
	assert.Contains(t, got, "}")
}

func TestServerInterfaceRenderer_SingleOperation_RendersMethod(t *testing.T) {
	t.Parallel()

	ctx := &render.RenderContext{
		Project: &parser.Project{
			Paths: &parser.Paths{
				Services: []*parser.Service{
					{
						Name: "default",
						Methods: []*parser.Method{
							{OperationID: "listPets"},
						},
					},
				},
			},
		},
		TypeMapper: &mockTypeMapper{},
	}

	r := NewServerInterfaceRenderer()
	body, _, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)
	assert.Contains(t, got, "type Server interface {")
	assert.Contains(t, got, "ListPets(ctx context.Context, req *ListPetsRequest) (*ListPetsResponse, error)")
}

func TestServerInterfaceRenderer_DeprecatedOperation_HasComment(t *testing.T) {
	t.Parallel()

	ctx := &render.RenderContext{
		Project: &parser.Project{
			Paths: &parser.Paths{
				Services: []*parser.Service{
					{
						Name: "default",
						Methods: []*parser.Method{
							{OperationID: "oldMethod", Deprecated: true},
						},
					},
				},
			},
		},
		TypeMapper: &mockTypeMapper{},
	}

	r := NewServerInterfaceRenderer()
	body, _, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)
	assert.Contains(t, got, "// Deprecated: operation is marked as deprecated")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/generator/render/operations/ -run TestServerInterfaceRenderer -v`
Expected: FAIL with `undefined: NewServerInterfaceRenderer`.

- [ ] **Step 3: Write ServerInterfaceRenderer implementation**

Create `internal/generator/render/operations/server_interface.go`:

```go
package operations

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/generator/render"
)

// ServerInterfaceRenderer рендерит interfaces/server/server.gen.go: интерфейс
// Server. Переиспользует request/response-структуры из interfaces/client.
// Заменяет Generator.serverFile + Generator.renderServer (internal/generator/server.go).
type ServerInterfaceRenderer struct{}

func NewServerInterfaceRenderer() *ServerInterfaceRenderer { return &ServerInterfaceRenderer{} }

func (ServerInterfaceRenderer) FilePath() string { return "interfaces/server/server.gen.go" }

func (r *ServerInterfaceRenderer) Render(ctx *render.RenderContext) ([]byte, *render.ImportTracker, error) {
	imps := render.NewImportTracker()
	ctx.Imports = imps

	ops := allOperations(ctx.Project)
	modelImportPath := ""
	if ctx.Project != nil && ctx.Project.Paths != nil {
		modelImportPath = ctx.Project.Paths.Imports.ClientInterfaces.Path
	}

	w := codegen.NewBufferWriter()
	w.Print("type Server interface {\n")
	for _, op := range ops {
		name := operationMethodName(op)
		if op.Deprecated {
			w.Print("\t// Deprecated: operation is marked as deprecated\n")
		}
		w.Print("\t", name, "(ctx context.Context, req *", qualifyClient(name, "Request", modelImportPath), ") ")
		w.Print("(*", qualifyClient(name, "Response", modelImportPath), ", error)\n")
	}
	w.Print("}\n")

	return w.Content(), imps, nil
}

var _ render.SingletonRenderer = (*ServerInterfaceRenderer)(nil)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/generator/render/operations/ -run TestServerInterfaceRenderer -v`
Expected: PASS — all 4 tests green.

- [ ] **Step 5: Commit**

```bash
git add internal/generator/render/operations/server_interface.go internal/generator/render/operations/server_interface_test.go
git commit -m "T27 Phase 3 Task 3: ServerInterfaceRenderer"
```

---

### Task 4: Wire ServerInterfaceRenderer + delete legacy server.go

**Files:**
- Modify: `internal/generator/generator_test.go` — append wire test
- Delete: `internal/generator/server.go`

- [ ] **Step 1: Write wire test**

Append to `internal/generator/generator_test.go`:

```go
func TestGenerate_ServerInterfaceFile_EmitsFile(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /items:
    get:
      operationId: listItems
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                type: array
                items: {type: string}
`)
	project := testProject(t, doc, "example.com/test")

	fw := &collectWriter{files: map[string][]byte{}}
	require.NoError(t, Generate(fw, project, nil))

	body := string(fw.files["interfaces/server/server.gen.go"])
	assert.Contains(t, body, "type Server interface {")
	assert.Contains(t, body, "ListItems(ctx context.Context, req *client.ListItemsRequest) (*client.ListItemsResponse, error)")
}
```

- [ ] **Step 2: Run wire test — passes against old code**

Run: `go test ./internal/generator/ -run TestGenerate_ServerInterfaceFile -v`
Expected: PASS.

- [ ] **Step 3: Delete legacy server.go**

```bash
rm internal/generator/server.go
```

- [ ] **Step 4: Run build + tests**

Run: `go build ./... && go test ./internal/generator/ -v -run "TestGenerate_ServerInterfaceFile"`

Expected: build green, test PASS.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "T27 Phase 3 Task 4: wire ServerInterfaceRenderer, delete legacy server.go"
```

---

### Task 5: ClientSugarRenderer

**Files:**
- Create: `internal/generator/render/operations/client_sugar.go`
- Create: `internal/generator/render/operations/client_sugar_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/generator/render/operations/client_sugar_test.go`:

```go
package operations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
)

func TestClientSugarRenderer_FilePath(t *testing.T) {
	t.Parallel()
	r := NewClientSugarRenderer()
	assert.Equal(t, "interfaces/client/client_sugar.gen.go", r.FilePath())
}

func TestClientSugarRenderer_WithSuccessResponse_RendersReturnValue(t *testing.T) {
	t.Parallel()

	ctx := &render.RenderContext{
		Project: &parser.Project{
			Paths: &parser.Paths{
				Services: []*parser.Service{
					{
						Name: "default",
						Methods: []*parser.Method{
							{
								OperationID: "getItem",
								Responses: []*parser.Response{
									{StatusCode: "200", Content: map[string]*parser.MediaType{
										"application/json": {Schema: &parser.Schema{Type: "string"}},
									}},
								},
							},
						},
					},
				},
			},
		},
		TypeMapper: &mockTypeMapper{},
	}

	r := NewClientSugarRenderer()
	body, _, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)
	assert.Contains(t, got, "type ClientSugared struct {")
	assert.Contains(t, got, "func NewClientSugared(impl Client) *ClientSugared {")
	assert.Contains(t, got, "func (x *ClientSugared) GetItem(ctx context.Context, req *GetItemRequest) (*string, error) {")
	assert.Contains(t, got, "return resp.Response200, nil")
}

func TestClientSugarRenderer_NoBodyResponse_ReturnsOnlyError(t *testing.T) {
	t.Parallel()

	ctx := &render.RenderContext{
		Project: &parser.Project{
			Paths: &parser.Paths{
				Services: []*parser.Service{
					{
						Name: "default",
						Methods: []*parser.Method{
							{
								OperationID: "deleteItem",
								Responses: []*parser.Response{
									{StatusCode: "204"},
								},
							},
						},
					},
				},
			},
		},
		TypeMapper: &mockTypeMapper{},
	}

	r := NewClientSugarRenderer()
	body, _, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)
	assert.Contains(t, got, "func (x *ClientSugared) DeleteItem(ctx context.Context, req *DeleteItemRequest) error {")
	assert.NotContains(t, got, "DeleteItem(ctx context.Context, req *DeleteItemRequest) (")
}

func TestClientSugarRenderer_EmptyProject_NoMethods(t *testing.T) {
	t.Parallel()

	ctx := &render.RenderContext{
		Project: &parser.Project{},
	}
	r := NewClientSugarRenderer()
	body, _, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)
	assert.Contains(t, got, "type ClientSugared struct {")
	// After the boilerplate, no methods.
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/generator/render/operations/ -run TestClientSugarRenderer -v`
Expected: FAIL with `undefined: NewClientSugarRenderer`.

- [ ] **Step 3: Write ClientSugarRenderer implementation**

Create `internal/generator/render/operations/client_sugar.go`:

```go
package operations

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
)

// ClientSugarRenderer рендерит interfaces/client/client_sugar.gen.go:
// ClientSugared обёртка над Client. Заменяет Generator.clientSugarFile +
// Generator.renderClientSugar (internal/generator/client_sugar.go).
type ClientSugarRenderer struct{}

func NewClientSugarRenderer() *ClientSugarRenderer { return &ClientSugarRenderer{} }

func (ClientSugarRenderer) FilePath() string { return "interfaces/client/client_sugar.gen.go" }

func (r *ClientSugarRenderer) Render(ctx *render.RenderContext) ([]byte, *render.ImportTracker, error) {
	imps := render.NewImportTracker()
	ctx.Imports = imps

	ops := allOperations(ctx.Project)
	m := ctx.TypeMapper

	w := codegen.NewBufferWriter()
	w.Print("type ClientSugared struct {\n")
	w.Print("\timpl Client\n")
	w.Print("}\n\n")
	w.Print("func NewClientSugared(impl Client) *ClientSugared {\n")
	w.Print("\treturn &ClientSugared{impl: impl}\n")
	w.Print("}\n\n")

	for _, op := range ops {
		renderSugarMethod(w, op, m)
	}

	return w.Content(), imps, nil
}

func renderSugarMethod(w *codegen.BufferWriter, op *parser.Method, m render.TypeMapper) {
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
	w.WriteString("fmt.Errorf(\"unexpected status: %d\", resp.Code)\n")
	w.Print("}\n\n")
}

var _ render.SingletonRenderer = (*ClientSugarRenderer)(nil)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/generator/render/operations/ -run TestClientSugarRenderer -v`
Expected: PASS — all 3 tests green.

- [ ] **Step 5: Commit**

```bash
git add internal/generator/render/operations/client_sugar.go internal/generator/render/operations/client_sugar_test.go
git commit -m "T27 Phase 3 Task 5: ClientSugarRenderer"
```

---

### Task 6: Wire ClientSugarRenderer + delete legacy client_sugar.go

**Files:**
- Modify: `internal/generator/generator_test.go` — append wire test
- Delete: `internal/generator/client_sugar.go`

- [ ] **Step 1: Write wire test**

Append to `internal/generator/generator_test.go`:

```go
func TestGenerate_ClientSugarFile_EmitsFile(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /items:
    get:
      operationId: listItems
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                type: array
                items: {type: string}
`)
	project := testProject(t, doc, "example.com/test")

	fw := &collectWriter{files: map[string][]byte{}}
	require.NoError(t, Generate(fw, project, nil))

	body := string(fw.files["interfaces/client/client_sugar.gen.go"])
	assert.Contains(t, body, "type ClientSugared struct {")
	assert.Contains(t, body, "func (x *ClientSugared) ListItems")
}
```

- [ ] **Step 2: Run wire test — passes against old code**

Run: `go test ./internal/generator/ -run TestGenerate_ClientSugarFile -v`
Expected: PASS.

- [ ] **Step 3: Delete legacy client_sugar.go**

```bash
rm internal/generator/client_sugar.go
```

- [ ] **Step 4: Run build + tests**

Run: `go build ./... && go test ./internal/generator/ -v -run "TestGenerate_ClientSugarFile"`

Expected: build green, test PASS.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "T27 Phase 3 Task 6: wire ClientSugarRenderer, delete legacy client_sugar.go"
```

---

### Task 7: AuditClientRenderer

**Files:**
- Create: `internal/generator/render/operations/audit_client.go`
- Create: `internal/generator/render/operations/audit_client_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/generator/render/operations/audit_client_test.go`:

```go
package operations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
)

func TestAuditClientRenderer_FilePath(t *testing.T) {
	t.Parallel()
	r := NewAuditClientRenderer()
	assert.Equal(t, "interfaces/client/audit.gen.go", r.FilePath())
}

func TestAuditClientRenderer_OpWithBody_RendersAuditStruct(t *testing.T) {
	t.Parallel()

	ctx := &render.RenderContext{
		Project: &parser.Project{
			Paths: &parser.Paths{
				Services: []*parser.Service{
					{
						Name: "default",
						Methods: []*parser.Method{
							{
								OperationID: "createItem",
								Parameters: []*parser.Parameter{
									{Name: "id", In: "path", Required: true, Schema: &parser.Schema{Type: "string"}},
								},
								RequestBody: &parser.RequestBody{
									Required: true,
									Content: map[string]*parser.MediaType{
										"application/json": {Schema: &parser.Schema{Name: "ItemCreate", Ref: "ItemCreate"}},
									},
								},
							},
						},
					},
				},
			},
		},
		TypeMapper: &mockTypeMapper{},
	}

	r := NewAuditClientRenderer()
	body, _, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)
	assert.Contains(t, got, "type CreateItemRequestAuditData struct {")
	assert.Contains(t, got, "func (req *CreateItemRequest) GetAuditData() any {")
	assert.Contains(t, got, "am.Body = req.Body.GetAuditData()")
}

func TestAuditClientRenderer_OpWithRefResponse_RendersResponseAudit(t *testing.T) {
	t.Parallel()

	ctx := &render.RenderContext{
		Project: &parser.Project{
			Paths: &parser.Paths{
				Services: []*parser.Service{
					{
						Name: "default",
						Methods: []*parser.Method{
							{
								OperationID: "getItem",
								Responses: []*parser.Response{
									{
										StatusCode: "200",
										Content: map[string]*parser.MediaType{
											"application/json": {Schema: &parser.Schema{Name: "Item", Ref: "Item"}},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		TypeMapper: &mockTypeMapper{},
	}

	r := NewAuditClientRenderer()
	body, _, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)
	assert.Contains(t, got, "type GetItemResponse200AuditData struct {")
	assert.Contains(t, got, "func (resp *GetItemResponse) Response200AuditData() GetItemResponse200AuditData {")
}

func TestAuditClientRenderer_NoOperations_EmptyFile(t *testing.T) {
	t.Parallel()

	ctx := &render.RenderContext{
		Project: &parser.Project{},
	}

	r := NewAuditClientRenderer()
	body, _, err := r.Render(ctx)
	require.NoError(t, err)
	assert.Empty(t, body)
}

func TestAuditClientRenderer_InlineSchema_NoAudit(t *testing.T) {
	t.Parallel()

	ctx := &render.RenderContext{
		Project: &parser.Project{
			Paths: &parser.Paths{
				Services: []*parser.Service{
					{
						Name: "default",
						Methods: []*parser.Method{
							{
								OperationID: "getItem",
								Responses: []*parser.Response{
									{
										StatusCode: "200",
										Content: map[string]*parser.MediaType{
											"application/json": {Schema: &parser.Schema{Type: "string"}},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		TypeMapper: &mockTypeMapper{},
	}

	r := NewAuditClientRenderer()
	body, _, err := r.Render(ctx)
	require.NoError(t, err)
	assert.Empty(t, body)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/generator/render/operations/ -run TestAuditClientRenderer -v`
Expected: FAIL with `undefined: NewAuditClientRenderer`.

- [ ] **Step 3: Write AuditClientRenderer implementation**

Create `internal/generator/render/operations/audit_client.go`:

```go
package operations

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
	"strings"
)

// AuditClientRenderer рендерит interfaces/client/audit.gen.go с audit-версиями
// request/response-структур операций и методами GetAuditData.
// Заменяет Generator.auditClientFile (internal/generator/audit_server.go).
type AuditClientRenderer struct{}

func NewAuditClientRenderer() *AuditClientRenderer { return &AuditClientRenderer{} }

func (AuditClientRenderer) FilePath() string { return "interfaces/client/audit.gen.go" }

func (r *AuditClientRenderer) Render(ctx *render.RenderContext) ([]byte, *render.ImportTracker, error) {
	imps := render.NewImportTracker()
	ctx.Imports = imps

	ops := allOperations(ctx.Project)
	m := ctx.TypeMapper

	w := codegen.NewBufferWriter()

	for _, op := range ops {
		renderOpRequestAudit(w, op, m)
	}

	for _, op := range ops {
		renderOpResponseAudit(w, op)
	}

	return w.Content(), imps, nil
}

func renderOpRequestAudit(w *codegen.BufferWriter, op *parser.Method, m render.TypeMapper) {
	name := operationMethodName(op)
	auditName := name + "RequestAuditData"

	pathParams := filterParamsByIn(op.Parameters, "path")
	queryParams := filterParamsByIn(op.Parameters, "query")
	bodySchema := resolveBodySchema(op.RequestBody)
	hasBody := bodySchema != nil && bodySchema.Name != ""

	if len(pathParams) == 0 && len(queryParams) == 0 && !hasBody {
		return
	}

	renderAuditRequestStruct(w, auditName, pathParams, queryParams, hasBody, m)
	renderAuditRequestMethod(w, name, auditName, pathParams, queryParams, op.RequestBody, hasBody)
}

func resolveBodySchema(rb *parser.RequestBody) *parser.Schema {
	if rb == nil || rb.Content == nil {
		return nil
	}
	return firstContentSchema(rb.Content)
}

func renderAuditRequestStruct(w *codegen.BufferWriter, auditName string, pathParams, queryParams []*parser.Parameter, hasBody bool, m render.TypeMapper) {
	w.Print("type ", auditName, " struct {\n")
	for _, p := range pathParams {
		renderAuditParamField(w, p, m)
	}
	for _, p := range queryParams {
		renderAuditParamField(w, p, m)
	}
	if hasBody {
		w.Print("\tBody any\n")
	}
	w.Print("}\n\n")
}

func renderAuditParamField(w *codegen.BufferWriter, p *parser.Parameter, m render.TypeMapper) {
	fieldName := goName(p.Name)
	fieldType := m.GoType(p.Schema)
	required := p.Required || p.In == "path"
	if !required && !strings.HasPrefix(fieldType, "*") && !isInherentlyNilable(fieldType) {
		fieldType = "*" + fieldType
	}
	w.Print("\t", fieldName, " ", fieldType, "\n")
}

func renderAuditRequestMethod(w *codegen.BufferWriter, opName, auditName string, pathParams, queryParams []*parser.Parameter, rb *parser.RequestBody, hasBody bool) {
	w.Print("func (req *", opName, "Request) GetAuditData() any {\n")
	w.Print("\tam := ", auditName, "{\n")
	for _, p := range pathParams {
		f := goName(p.Name)
		w.Print("\t\t", f, ": req.", f, ",\n")
	}
	for _, p := range queryParams {
		f := goName(p.Name)
		w.Print("\t\t", f, ": req.", f, ",\n")
	}
	w.Print("\t}\n")
	if hasBody {
		required := rb != nil && rb.Required
		if required {
			w.Print("\tam.Body = req.Body.GetAuditData()\n")
		} else {
			w.Print("\tif req.Body != nil {\n")
			w.Print("\t\tam.Body = req.Body.GetAuditData()\n")
			w.Print("\t}\n")
		}
	}
	w.Print("\treturn am\n")
	w.Print("}\n\n")
}

func renderOpResponseAudit(w *codegen.BufferWriter, op *parser.Method) {
	opName := operationMethodName(op)
	for _, r := range op.Responses {
		if r.StatusCode == "default" {
			continue
		}
		schema := responseSchema(r)
		if schema == nil {
			continue
		}
		if schema.Ref == "" {
			continue
		}
		codeName := goName(r.StatusCode)
		auditName := opName + "Response" + codeName + "AuditData"
		methodName := "Response" + codeName + "AuditData"
		fieldName := responseFieldName(r.StatusCode)

		renderAuditResponseStruct(w, auditName)
		renderAuditResponseMethod(w, opName, auditName, methodName, fieldName, hasResponseHeaders(r))
	}
}

func renderAuditResponseStruct(w *codegen.BufferWriter, auditName string) {
	w.Print("type ", auditName, " struct {\n")
	w.Print("\tPayload any\n")
	w.Print("}\n\n")
}

func renderAuditResponseMethod(w *codegen.BufferWriter, opName, auditName, methodName, fieldName string, hasHeaders bool) {
	w.Print("func (resp *", opName, "Response) ", methodName, "() ", auditName, " {\n")
	w.Print("\tam := ", auditName, "{}\n")
	w.Print("\tif resp.", fieldName, " != nil {\n")
	if hasHeaders {
		w.Print("\t\tif resp.", fieldName, ".Payload != nil {\n")
		w.Print("\t\t\tam.Payload = resp.", fieldName, ".Payload.GetAuditData()\n")
		w.Print("\t\t}\n")
	} else {
		w.Print("\t\tam.Payload = resp.", fieldName, ".GetAuditData()\n")
	}
	w.Print("\t}\n")
	w.Print("\treturn am\n")
	w.Print("}\n\n")
}

var _ render.SingletonRenderer = (*AuditClientRenderer)(nil)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/generator/render/operations/ -run TestAuditClientRenderer -v`
Expected: PASS — all 4 tests green.

- [ ] **Step 5: Commit**

```bash
git add internal/generator/render/operations/audit_client.go internal/generator/render/operations/audit_client_test.go
git commit -m "T27 Phase 3 Task 7: AuditClientRenderer"
```

---

### Task 8: Wire AuditClientRenderer + delete legacy audit_server.go

**Files:**
- Modify: `internal/generator/generator_test.go` — append wire test
- Delete: `internal/generator/audit_server.go`

- [ ] **Step 1: Write wire test**

Append to `internal/generator/generator_test.go`:

```go
func TestGenerate_AuditClientFile_EmitsFile(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /items:
    post:
      operationId: createItem
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/ItemCreate'
      responses:
        '201':
          description: created
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Item'
components:
  schemas:
    ItemCreate:
      type: object
      properties:
        name: {type: string}
    Item:
      type: object
      properties:
        id: {type: string}
        name: {type: string}
`)
	project := testProject(t, doc, "example.com/test")

	fw := &collectWriter{files: map[string][]byte{}}
	require.NoError(t, Generate(fw, project, nil))

	body := string(fw.files["interfaces/client/audit.gen.go"])
	assert.Contains(t, body, "type CreateItemRequestAuditData struct {")
	assert.Contains(t, body, "func (req *CreateItemRequest) GetAuditData() any {")
}
```

- [ ] **Step 2: Run wire test — passes against old code**

Run: `go test ./internal/generator/ -run TestGenerate_AuditClientFile -v`
Expected: PASS.

- [ ] **Step 3: Delete legacy audit_server.go**

```bash
rm internal/generator/audit_server.go
```

- [ ] **Step 4: Run build + tests**

Run: `go build ./... && go test ./internal/generator/ -v -run "TestGenerate_AuditClientFile"`

Expected: build green, test PASS.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "T27 Phase 3 Task 8: wire AuditClientRenderer, delete legacy audit_server.go"
```

---

### Task 9: Verify Phase 3 acceptance criteria

- [ ] **Step 1: Build + test + vet**

```bash
go build ./...
go test ./...
go vet ./...
```

Expected: all green.

- [ ] **Step 2: Byte-for-byte identity vs golden**

```bash
go test ./cmd/oapigen/ -run TestE2E_Minimal -v
go test ./cmd/oapigen/ -run TestE2E_GoldenCompiles -v
```

Expected: both PASS. Golden files unchanged: `testdata/` working tree clean.

```bash
git status testdata/
```

Expected: clean.

- [ ] **Step 3: Verify legacy files deleted**

```bash
ls internal/generator/client.go internal/generator/server.go internal/generator/client_sugar.go internal/generator/audit_server.go 2>&1
```

Expected: all 4 report "No such file or directory".

- [ ] **Step 4: Verify renderer files exist**

```bash
ls internal/generator/render/operations/helpers.go \
   internal/generator/render/operations/client_interface.go \
   internal/generator/render/operations/client_interface_test.go \
   internal/generator/render/operations/server_interface.go \
   internal/generator/render/operations/server_interface_test.go \
   internal/generator/render/operations/client_sugar.go \
   internal/generator/render/operations/client_sugar_test.go \
   internal/generator/render/operations/audit_client.go \
   internal/generator/render/operations/audit_client_test.go
```

Expected: all 9 files exist.

- [ ] **Step 5: Verify generator wires the renderers**

```bash
grep -n "NewClientInterfaceRenderer\|NewServerInterfaceRenderer\|NewClientSugarRenderer\|NewAuditClientRenderer" internal/generator/generator.go
```

Expected: 4 matches — one per renderer.

- [ ] **Step 6: Verify no references to deleted legacy symbols in code**

```bash
grep -rn "g\.clientFile\|g\.serverFile\|g\.clientSugarFile\|g\.auditClientFile" --include="*.go" internal/generator/
```

Expected: no matches (only comments, if any).

- [ ] **Step 7: Commit final state (if any uncommitted changes)**

```bash
git status
# If clean: done. If not: commit remaining.
```

---

## Phase 3 Complete

Acceptance:
- [ ] `client.go`, `server.go`, `client_sugar.go`, `audit_server.go` deleted
- [ ] Four `SingletonRenderer` implementations in `internal/generator/render/operations/`
- [ ] Each has unit tests
- [ ] `writeOperationFiles` dispatches interface files via `ComposeSingletonFile`
- [ ] `go build ./...`, `go test ./...`, `go vet ./...` green
- [ ] `TestE2E_Minimal`, `TestE2E_GoldenCompiles` pass, `testdata/` clean

**Next:** Phase 4 (Implementations-side — `implClientFile`, `implServerFile`, `mockClientFile`, `mockServerFile`, `sdkFile`).

---

## Notes for the executing engineer

- **`writeOperationFiles` hybrid:** Phase 3 only migrates the 4 interface files. The remaining 5 (impl, mocks, sdk) stay as legacy `func() codegen.File` calls. Future phases will migrate those too.
- **`newOperationsRenderContext(pkg)`:** Creates a fresh `RenderContext` + `typeMapperAdapter` for the given package. Different from `newSchemaRenderContext()` which hardcodes "model" — operations need different packages ("client" vs "server").
- **`sugarReturnType` and `modeSettable`:** The `modeSettable` interface is internal to `helpers.go` — it's used to save/restore the type mapper's mode. The production `typeMapperAdapter` already has `Mode() string`. Test mocks must implement it too.
- **`response_headers.go` cleanup:** After Task 2, the `renderPayloadWithHeadersType` method on `*Generator` in `response_headers.go` becomes unused (port exists in `client_interface.go`). It won't cause a compiler error (same package), but `go vet` may flag it. Remove it in Task 2, Step 5.
- **`resolveBodySchema` vs `bodySchema`:** The old `audit_server.go` had `g.resolveBodySchema` which resolves `$ref` — but `bodySchema` from `helpers.go` just returns the first content schema. The renderer uses `bodySchema` (simpler) — confirm byte-for-byte matching against golden. If golden differs, fall back to the legacy logic.
- **Byte-for-byte golden:** `testdata/project/golden/minimal/interfaces/client/client.gen.go`, `client_sugar.gen.go`, `audit.gen.go`, `server.gen.go` must match byte-for-byte. Verify after each wire task.
