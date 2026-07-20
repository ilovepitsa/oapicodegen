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

// newStructRenderer строит StructRenderer с shared Buf/Imports и фейковым
// TypeMapper, привязанным к RenderContext.
func newStructRenderer(t *testing.T, tm render.TypeMapper) *StructRenderer {
	t.Helper()

	ctx := &render.RenderContext{TypeMapper: tm}
	r := NewStructRenderer()
	r.Init(codegen.NewBufferWriter(), render.NewImportTracker(), ctx)

	return r
}

func TestStructRenderer_SimpleStruct(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newStructRenderer(t, tm)

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
	r := newStructRenderer(t, tm)

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

func TestStructRenderer_SplitStructEmitsRequestAndResponse(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newStructRenderer(t, tm)
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

func TestStructRenderer_DescriptionEmitsDocComment(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newStructRenderer(t, tm)

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
	r := newStructRenderer(t, tm)

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
	r := newStructRenderer(t, tm)
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
// в StructRenderer.renderField, а также для slices.ContainsFunc-проверок
// импортов в тестах).
var _ = gogen.Import{}
