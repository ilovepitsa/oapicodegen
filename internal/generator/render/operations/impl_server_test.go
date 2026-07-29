package operations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ilovepitsa/oapicodegen/internal/codegen"
	"github.com/ilovepitsa/oapicodegen/internal/generator/render"
	"github.com/ilovepitsa/oapicodegen/internal/parser"
)

func TestImplServerRenderer_FilePath(t *testing.T) {
	t.Parallel()
	r := NewImplServerRenderer()
	assert.Equal(t, "impl/echoserver/server.gen.go", r.FilePath())
}

func TestImplServerRenderer_PackageName(t *testing.T) {
	t.Parallel()
	r := NewImplServerRenderer()
	assert.Equal(t, "server", r.PackageName())
}

// TestImplServerRenderer_BindBodyUsesDisallowUnknownFields проверяет, что
// сгенерированный bindBody использует json.NewDecoder + DisallowUnknownFields
// и возвращает HTTP 400 с именем неизвестного поля через extractUnknownField.
func TestImplServerRenderer_BindBodyUsesDisallowUnknownFields(t *testing.T) {
	t.Parallel()

	project := &parser.Project{}
	project.CreatePaths("example.com/test")
	project.Paths.Services = []*parser.Service{
		{Name: "default", Methods: []*parser.Method{
			{OperationID: "createItem", RequestBody: &parser.RequestBody{
				Content: map[string]*parser.MediaType{
					"application/json": {Schema: &parser.Schema{Type: "object"}},
				},
			}},
		}},
	}

	ctx := &render.RenderContext{
		Project:      project,
		ImportPrefix: "example.com/test",
		TypeMapper:   &mockTypeMapper{},
	}

	r := NewImplServerRenderer()
	body, imps, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)

	// JSON branch: decoder + DisallowUnknownFields.
	assert.Contains(t, got, "func bindBody(c echo.Context, dst any) error {")
	assert.Contains(t, got, "dec := json.NewDecoder(bytes.NewReader(body))")
	assert.Contains(t, got, "dec.DisallowUnknownFields()")
	assert.Contains(t, got, "if err := dec.Decode(dst); err != nil {")
	assert.Contains(t, got, "field := extractUnknownField(err)")
	assert.Contains(t, got, `return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("unknown field %q", field))`)
	assert.Contains(t, got, "return echo.NewHTTPError(http.StatusBadRequest, err.Error())")

	// Old json.Unmarshal form must be gone.
	assert.NotContains(t, got, "json.Unmarshal(body, dst)")

	// Helper emitted alongside bindBody.
	assert.Contains(t, got, "func extractUnknownField(err error) string {")
	assert.Contains(t, got, `const prefix = `+"`"+`json: unknown field "`+"`")
	assert.Contains(t, got, "strings.Index(msg, prefix)")

	// Imports: fmt + strings added when needBody.
	assertImportHasPath(t, imps, "fmt")
	assertImportHasPath(t, imps, "strings")
}

// TestImplServerRenderer_URLFormBranchPreserved проверяет, что url-form ветка
// в bindBody сохранена и предшествует JSON-декодеру.
func TestImplServerRenderer_URLFormBranchPreserved(t *testing.T) {
	t.Parallel()

	project := &parser.Project{}
	project.CreatePaths("example.com/test")
	project.Paths.Services = []*parser.Service{
		{Name: "default", Methods: []*parser.Method{
			{OperationID: "uploadForm", RequestBody: &parser.RequestBody{
				Content: map[string]*parser.MediaType{
					"application/x-www-form-urlencoded": {Schema: &parser.Schema{Type: "object"}},
				},
			}},
		}},
	}

	ctx := &render.RenderContext{
		Project:      project,
		ImportPrefix: "example.com/test",
		TypeMapper:   &mockTypeMapper{},
	}

	r := NewImplServerRenderer()
	body, _, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)

	// URL-form branch present and BEFORE the decoder.
	urlIdx := strings.Index(got, "application/x-www-form-urlencoded")
	decIdx := strings.Index(got, "dec.DisallowUnknownFields()")
	require.GreaterOrEqual(t, urlIdx, 0)
	require.GreaterOrEqual(t, decIdx, 0)
	assert.Less(t, urlIdx, decIdx, "url-form branch must precede JSON decoder")

	// Both url-form + decoder paths coexist.
	assert.Contains(t, got, "c.Request().ParseForm()")
	assert.Contains(t, got, "u.UnmarshalURLForm(c.Request().PostForm)")
}

// TestRenderImplServerStruct_Shape фиксирует форму struct + конструктора:
// ServerHTTP хранит *validator.Registry, NewServerHTTP принимает (impl, reg).
func TestRenderImplServerStruct_Shape(t *testing.T) {
	t.Parallel()

	w := codegen.NewBufferWriter()
	renderImplServerStruct(w, false, false)
	got := string(w.Content())

	assert.Contains(t, got, "type ServerHTTP struct {")
	assert.Contains(t, got, "\timpl apiserver.Server\n")
	assert.Contains(t, got, "\treg  *validator.Registry\n")

	assert.Contains(t, got, "func NewServerHTTP(impl apiserver.Server, reg *validator.Registry) *ServerHTTP {")
	assert.Contains(t, got, "\treturn &ServerHTTP{impl: impl, reg: reg}\n")

	// No body helpers when needBody=false.
	assert.NotContains(t, got, "func bindBody(")
	assert.NotContains(t, got, "func extractUnknownField(")
}

// TestRenderBindBody_OutputShape проверяет буфер напрямую, без полной сборки
// проекта — фиксирует форму JSON-ветки и helper'а.
func TestRenderBindBody_OutputShape(t *testing.T) {
	t.Parallel()

	w := codegen.NewBufferWriter()
	renderImplServerStruct(w, true, false)
	got := string(w.Content())

	assert.Contains(t, got, "dec := json.NewDecoder(bytes.NewReader(body))")
	assert.Contains(t, got, "dec.DisallowUnknownFields()")
	assert.Contains(t, got, "func extractUnknownField(err error) string {")
	assert.Contains(t, got, "return rest[:j]")
}

// TestImplServerRenderer_ValidatorImport проверяет, что рендер добавляет
// импорт pkg/validator с алиасом validator — он нужен каждому хендлеру
// для вызова validator.Validate(req, s.reg) (Task 7).
func TestImplServerRenderer_ValidatorImport(t *testing.T) {
	t.Parallel()

	project := &parser.Project{}
	project.CreatePaths("example.com/test")
	project.Paths.Services = []*parser.Service{
		{Name: "default", Methods: []*parser.Method{
			{OperationID: "createItem", RequestBody: &parser.RequestBody{
				Content: map[string]*parser.MediaType{
					"application/json": {Schema: &parser.Schema{Type: "object"}},
				},
			}},
		}},
	}

	ctx := &render.RenderContext{
		Project:      project,
		ImportPrefix: "example.com/test",
		TypeMapper:   &mockTypeMapper{},
	}

	r := NewImplServerRenderer()
	body, imps, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)
	assert.Contains(t, got, "reg  *validator.Registry")
	assert.Contains(t, got, "func NewServerHTTP(impl apiserver.Server, reg *validator.Registry) *ServerHTTP {")

	assertImportHasPath(t, imps, "github.com/ilovepitsa/oapicodegen/pkg/validator")
}

// TestImplServerRenderer_HandlerCallsValidate проверяет, что каждый хендлер
// после bind (и SetDefaults, если есть) вызывает validator.Validate(req, s.reg)
// и при ошибке возвращает writeValidationError(c, err) (Task 7).
func TestImplServerRenderer_HandlerCallsValidate(t *testing.T) {
	t.Parallel()

	project := &parser.Project{}
	project.CreatePaths("example.com/test")
	project.Paths.Services = []*parser.Service{
		{Name: "default", Methods: []*parser.Method{
			{OperationID: "createItem", RequestBody: &parser.RequestBody{
				Content: map[string]*parser.MediaType{
					"application/json": {Schema: &parser.Schema{Type: "object"}},
				},
			}},
		}},
	}

	ctx := &render.RenderContext{
		Project:      project,
		ImportPrefix: "example.com/test",
		TypeMapper:   &mockTypeMapper{},
	}

	r := NewImplServerRenderer()
	body, _, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)

	// Validate call after bind, before s.impl.
	bindIdx := strings.Index(got, "if err := c.Bind(req); err != nil {")
	validateIdx := strings.Index(got, "if err := validator.Validate(req, s.reg); err != nil {")
	implIdx := strings.Index(got, "resp, err := s.impl.")
	require.GreaterOrEqual(t, bindIdx, 0)
	require.GreaterOrEqual(t, validateIdx, 0)
	require.GreaterOrEqual(t, implIdx, 0)
	assert.Less(t, bindIdx, validateIdx, "Validate must come after c.Bind")
	assert.Less(t, validateIdx, implIdx, "Validate must come before s.impl call")

	assert.Contains(t, got, "if err := validator.Validate(req, s.reg); err != nil {")
	assert.Contains(t, got, "\t\treturn writeValidationError(c, err)\n")
}

// TestImplServerRenderer_ValidationHelpers фиксирует тела двух package-level
// helper'ов: writeValidationError (400 + structured JSON {error,field,message})
// и splitValidationErr (режет строку ошибки walker'а по первому ": ").
func TestImplServerRenderer_ValidationHelpers(t *testing.T) {
	t.Parallel()

	project := &parser.Project{}
	project.CreatePaths("example.com/test")
	project.Paths.Services = []*parser.Service{
		{Name: "default", Methods: []*parser.Method{
			{OperationID: "createItem", RequestBody: &parser.RequestBody{
				Content: map[string]*parser.MediaType{
					"application/json": {Schema: &parser.Schema{Type: "object"}},
				},
			}},
		}},
	}

	ctx := &render.RenderContext{
		Project:      project,
		ImportPrefix: "example.com/test",
		TypeMapper:   &mockTypeMapper{},
	}

	r := NewImplServerRenderer()
	body, imps, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)

	// writeValidationError: 400 + structured JSON body.
	assert.Contains(t, got, "func writeValidationError(c echo.Context, err error) error {")
	assert.Contains(t, got, "\tfield, msg := splitValidationErr(err.Error())\n")
	assert.Contains(t, got, "\treturn c.JSON(http.StatusBadRequest, map[string]any{\n")
	assert.Contains(t, got, "\t\t\"error\":   \"validation_error\",\n")
	assert.Contains(t, got, "\t\t\"field\":   field,\n")
	assert.Contains(t, got, "\t\t\"message\": msg,\n")
	assert.Contains(t, got, "\t})\n")
	assert.Contains(t, got, "}\n\n")

	// splitValidationErr: cuts walker error string by first ": ".
	assert.Contains(t, got, "func splitValidationErr(s string) (field, msg string) {")
	assert.Contains(t, got, "\ti := strings.Index(s, \": \")\n")
	assert.Contains(t, got, "\tif i < 0 {\n\t\treturn \"\", s\n\t}\n")
	assert.Contains(t, got, "\treturn s[:i], s[i+2:]\n")

	// strings import present (splitValidationErr uses strings.Index).
	assertImportHasPath(t, imps, "strings")
}

// TestRenderImplServerStruct_HelpersAlwaysEmitted проверяет, что helper'ы
// рендерятся безусловно (даже при needBody=false), т.к. каждый хендлер их
// использует. bindBody/extractUnknownField при этом не рендерятся.
func TestRenderImplServerStruct_HelpersAlwaysEmitted(t *testing.T) {
	t.Parallel()

	w := codegen.NewBufferWriter()
	renderImplServerStruct(w, false, false)
	got := string(w.Content())

	assert.Contains(t, got, "func writeValidationError(c echo.Context, err error) error {")
	assert.Contains(t, got, "func splitValidationErr(s string) (field, msg string) {")

	// Body helpers still gated by needBody.
	assert.NotContains(t, got, "func bindBody(")
	assert.NotContains(t, got, "func extractUnknownField(")
}
