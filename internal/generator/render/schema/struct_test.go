package schema

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeCallbacks — тестовая реализация render.SchemaCallbacks. Записывает
// вызовы в calls для последующих assert'ов и опционально пишет заглушку в w,
// чтобы проверить порядок рендера (struct → SetDefaults → ValidateOwn).
// hasDefaults управляет возвратом SchemaTreeHasDefaults — от него зависит,
// вызовется ли RenderSetDefaults.
type fakeCallbacks struct {
	calls       []string
	hasDefaults bool
}

func (f *fakeCallbacks) RenderSetDefaults(
	w *codegen.BufferWriter,
	_ *parser.Schema,
	_, _ string,
	_ func(*parser.Property) bool,
) {
	f.calls = append(f.calls, "SetDefaults")
	w.Print("// SetDefaults placeholder\n")
}

func (f *fakeCallbacks) RenderValidateOwn(
	w *codegen.BufferWriter,
	_ *parser.Schema,
	_, _ string,
	_ bool,
	_ func(*parser.Property) bool,
) {
	f.calls = append(f.calls, "ValidateOwn")
	w.Print("// ValidateOwn placeholder\n")
}

func (f *fakeCallbacks) SchemaTreeHasDefaults(_ *parser.Schema, _ func(*parser.Property) bool) bool {
	return f.hasDefaults
}

// newStructRenderer строит StructRenderer с shared Buf/Imports и фейковым
// TypeMapper + Callbacks, привязанными к RenderContext.
func newStructRenderer(t *testing.T, tm render.TypeMapper, cb render.SchemaCallbacks) *StructRenderer {
	t.Helper()

	ctx := &render.RenderContext{TypeMapper: tm, Callbacks: cb}
	r := NewStructRenderer()
	r.Init(codegen.NewBufferWriter(), render.NewImportTracker(), ctx)

	return r
}

func TestStructRenderer_SimpleStruct(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newStructRenderer(t, tm, nil)

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name: "Pet",
		Type: "object",
		Properties: []*parser.Property{
			{Name: "name", Required: true, Schema: &parser.Schema{Type: "string"}},
		},
	}))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "type Pet struct {")
	assert.True(t, containsCollapsed(got, "Name string `json:\"name\" yaml:\"name\"`"))
}

func TestStructRenderer_OptionalFieldUsesPointer(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newStructRenderer(t, tm, nil)

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name: "Pet",
		Type: "object",
		Properties: []*parser.Property{
			{Name: "tag", Schema: &parser.Schema{Type: "string"}},
		},
	}))

	got := string(r.Buf.Content())
	assert.True(t, containsCollapsed(got, "Tag *string `json:\"tag,omitempty\" yaml:\"tag,omitempty\"`"))
}

func TestStructRenderer_CallbacksFiredInOrder(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	cb := &fakeCallbacks{hasDefaults: true}
	r := newStructRenderer(t, tm, cb)

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name: "Pet",
		Type: "object",
		Properties: []*parser.Property{
			{Name: "name", Schema: &parser.Schema{Type: "string"}},
		},
	}))

	assert.Equal(t, []string{"SetDefaults", "ValidateOwn"}, cb.calls)
}

func TestStructRenderer_SplitStructEmitsRequestAndResponse(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newStructRenderer(t, tm, nil)
	r.Ctx.Features = parser.ProjectFeatures{SplitRequestResponse: parser.ProjectFeature{Value: true}}

	require.NoError(t, r.OnSplitStruct(&parser.Schema{
		Name:    "Pet",
		Type:    "object",
		IsSplit: true,
		Properties: []*parser.Property{
			{Name: "name", Schema: &parser.Schema{Type: "string"}},
			{
				Name:   "id",
				Schema: &parser.Schema{Type: "string", ReadOnly: true},
			},
			{
				Name:   "w",
				Schema: &parser.Schema{Type: "string", WriteOnly: true},
			},
		},
	}))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "type PetRequest struct {")
	assert.Contains(t, got, "type PetResponse struct {")

	// Request: ReadOnly=false → name + w (но не id).
	// Response: WriteOnly=false → name + id (но не w).
	assert.True(t, containsCollapsed(got, "type PetRequest struct { Name *string `json:\"name,omitempty\""))
	assert.True(t, containsCollapsed(got, "type PetResponse struct { Name *string `json:\"name,omitempty\""))
}

func TestStructRenderer_UpdateStructEmittedWhenIsUsedInUpdate(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newStructRenderer(t, tm, nil)

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name:           "Pet",
		Type:           "object",
		IsUsedInUpdate: true,
		Properties: []*parser.Property{
			{Name: "name", Schema: &parser.Schema{Type: "string"}, IsUsedInUpdate: true},
		},
	}))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "type UpdatePet struct {")
	assert.True(t, containsCollapsed(got, "Name optional.Optional[string] `json:\"name\" yaml:\"name\"`"))
	assert.Contains(t, got, "func (u *UpdatePet) GetName() (*string, bool) {")
}

func TestStructRenderer_DescriptionEmitsDocComment(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newStructRenderer(t, tm, nil)

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name:        "Pet",
		Type:        "object",
		Description: "A pet.",
		Properties: []*parser.Property{
			{Name: "name", Schema: &parser.Schema{Type: "string"}},
		},
	}))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "// A pet.")
}

func TestStructRenderer_DeprecatedFieldEmitsMarker(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newStructRenderer(t, tm, nil)

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name: "Pet",
		Type: "object",
		Properties: []*parser.Property{
			{Name: "old", Schema: &parser.Schema{Type: "string", Deprecated: true}},
		},
	}))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "// Deprecated: schema marks this field as deprecated")
}

func TestStructRenderer_OptionalFeatureEmitsOptionalWrapper(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newStructRenderer(t, tm, nil)
	r.Ctx.Features = parser.ProjectFeatures{UseOptional: parser.ProjectFeature{Value: true}}

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name: "Pet",
		Type: "object",
		Properties: []*parser.Property{
			{Name: "tag", Schema: &parser.Schema{Type: "string"}, Optional: true},
		},
	}))

	got := string(r.Buf.Content())
	assert.True(t, containsCollapsed(got, "Tag optional.Optional[string] `json:\"tag\" yaml:\"tag\"`"))

	// optional-импорт должен быть в ImportTracker.
	imps := r.Imports.Imports()
	require.NotEmpty(t, imps)

	hasOptional := slices.ContainsFunc(imps, func(imp gogen.Import) bool {
		return imp.Path == optionalPkg && imp.Alias == "optional"
	})
	assert.True(t, hasOptional, "optional import must be tracked")
}

func TestStructRenderer_SkipReturnsTrue(t *testing.T) {
	t.Parallel()

	r := NewStructRenderer()
	assert.True(t, r.Skip(&parser.Schema{Name: "Pet"}))
}

// убеждаемся, что gogen импорт используется (он нужен для ImportTracker.Add
// в StructRenderer.renderField / renderUpdateField).
var _ = gogen.Import{}
