// Package schema: tests for URLFormRenderer. Renderer НЕ подключён к pack'у
// (Task 6.3 делает wiring через writeURLFormAuxFile) — тесты вызывают
// OnStruct/OnSplitStruct напрямую, проверяя рендер MarshalURLForm/
// UnmarshalURLForm методов.
package schema

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
)

// newURLFormTestRenderer строит URLFormRenderer с shared Buf/Imports и фейковым
// TypeMapper, привязанным к RenderContext. Project в ctx — non-nil с пустой
// проиндексированной Model (для симметрии с SetDefaultsRenderer/ValidateOwnRenderer).
func newURLFormTestRenderer(t *testing.T, tm render.TypeMapper) *URLFormRenderer {
	t.Helper()

	ctx := &render.RenderContext{
		TypeMapper: tm,
		Project:    newTestProjectWithEmptyModel(),
	}
	r := NewURLFormRenderer()
	r.Init(codegen.NewBufferWriter(), render.NewImportTracker(), ctx)

	return r
}

func TestURLFormRenderer_PrimitiveFields_RendersMarshalAndUnmarshal(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newURLFormTestRenderer(t, tm)
	// Перекрываем тип mapper для integer-поля — fakeTypeMapper возвращает
	// одну и ту же строку для всех схем, поэтому используем int.
	tm.got = "int"

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name: "Pet",
		Type: "object",
		Properties: []*parser.Property{
			{Name: "name", Required: true, Schema: &parser.Schema{Type: "string"}},
			{Name: "age", Required: true, Schema: &parser.Schema{Type: "integer"}},
		},
	}))

	got := string(r.Buf.Content())
	// MarshalURLForm
	assert.Contains(t, got, "func (m Pet) MarshalURLForm() (url.Values, error) {")
	assert.Contains(t, got, `values.Set("name", m.Name)`)
	assert.Contains(t, got, `values.Set("age", strconv.FormatInt(int64(m.Age), 10))`)
	// UnmarshalURLForm
	assert.Contains(t, got, "func (m *Pet) UnmarshalURLForm(values url.Values) error {")
	assert.Contains(t, got, `m.Name = values.Get("name")`)
	assert.Contains(t, got, `parsed, err := strconv.Atoi(values.Get("age"))`)
}

func TestURLFormRenderer_UnsupportedField_RendersErrorReturn(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "[]string"}
	r := newURLFormTestRenderer(t, tm)

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name: "Pet",
		Type: "object",
		Properties: []*parser.Property{
			{Name: "tags", Required: true, Schema: &parser.Schema{Type: "array"}},
		},
	}))

	got := string(r.Buf.Content())
	// MarshalURLForm: unsupported → runtime error return.
	assert.Contains(t, got, "func (m Pet) MarshalURLForm() (url.Values, error) {")
	assert.Contains(t, got, `return nil, fmt.Errorf("field Tags: url-form encoding not supported")`)
	// UnmarshalURLForm: unsupported → runtime error return.
	assert.Contains(t, got, "func (m *Pet) UnmarshalURLForm(values url.Values) error {")
	assert.Contains(t, got, `return fmt.Errorf("field Tags: url-form decoding not supported")`)
}

func TestURLFormRenderer_OptionalField_RendersPointerGuard(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newURLFormTestRenderer(t, tm)

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name: "Pet",
		Type: "object",
		Properties: []*parser.Property{
			{Name: "tag", Schema: &parser.Schema{Type: "string"}},
		},
	}))

	got := string(r.Buf.Content())
	// Optional string → *string → nil-guard for marshal.
	assert.Contains(t, got, "if m.Tag != nil {")
	assert.Contains(t, got, `values.Set("tag", *m.Tag)`)
	// Optional string → pointer guard for unmarshal (empty → skip).
	assert.Contains(t, got, `if v := values.Get("tag"); v != "" {`)
	assert.Contains(t, got, "m.Tag = &v")
}

func TestURLFormRenderer_DateTimeField_RendersTimeParse(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "time.Time"}
	r := newURLFormTestRenderer(t, tm)

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name: "Event",
		Type: "object",
		Properties: []*parser.Property{
			{Name: "createdAt", Required: true, Schema: &parser.Schema{Type: "string", Format: "date-time"}},
		},
	}))

	got := string(r.Buf.Content())
	// MarshalURLForm: time.Time → .Format(time.RFC3339).
	assert.Contains(t, got, "func (m Event) MarshalURLForm() (url.Values, error) {")
	assert.Contains(t, got, `values.Set("createdAt", m.CreatedAt.Format(time.RFC3339))`)
	// UnmarshalURLForm: time.Parse(time.RFC3339, ...).
	assert.Contains(t, got, "func (m *Event) UnmarshalURLForm(values url.Values) error {")
	assert.Contains(t, got, `parsed, err := time.Parse(time.RFC3339, values.Get("createdAt"))`)
	assert.Contains(t, got, "m.CreatedAt = parsed")
}

func TestURLFormRenderer_ImportsAdded(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "int"}
	r := newURLFormTestRenderer(t, tm)
	// Зарегистрируем две схемы (string + integer) — нужны оба импорта strconv
	// и fmt (для unsupported-возврата, если бы был) и net/url (всегда).
	// Здесь integer + date-time — чтобы покрыть strconv + time.
	tm.got = "time.Time"

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name: "Event",
		Type: "object",
		Properties: []*parser.Property{
			{Name: "age", Required: true, Schema: &parser.Schema{Type: "integer"}},
			{Name: "createdAt", Required: true, Schema: &parser.Schema{Type: "string", Format: "date-time"}},
		},
	}))

	imps := r.Imports.Imports()
	require.NotEmpty(t, imps)

	hasPath := func(path string) bool {
		return slices.ContainsFunc(imps, func(imp gogen.Import) bool {
			return imp.Path == path
		})
	}

	assert.True(t, hasPath("net/url"), "net/url import must be tracked")
	assert.True(t, hasPath("fmt"), "fmt import must be tracked")
	assert.True(t, hasPath("strconv"), "strconv import must be tracked (integer field)")
	assert.True(t, hasPath("time"), "time import must be tracked (date-time field)")
}

func TestURLFormRenderer_OnSplitStruct_RendersForBaseName(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newURLFormTestRenderer(t, tm)

	require.NoError(t, r.OnSplitStruct(&parser.Schema{
		Name:    "Pet",
		Type:    "object",
		IsSplit: true,
		Properties: []*parser.Property{
			{Name: "name", Required: true, Schema: &parser.Schema{Type: "string"}},
		},
	}))

	got := string(r.Buf.Content())
	// URLForm рендерится на базовое имя Pet, а не PetRequest/PetResponse.
	assert.Contains(t, got, "func (m Pet) MarshalURLForm() (url.Values, error) {")
	assert.Contains(t, got, "func (m *Pet) UnmarshalURLForm(values url.Values) error {")
	assert.NotContains(t, got, "PetRequest")
	assert.NotContains(t, got, "PetResponse")
}
