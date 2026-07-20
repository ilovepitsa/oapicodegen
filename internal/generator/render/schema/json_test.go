package schema

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/generator/walk"
	"nschugorev/oapigenerator/internal/parser"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newJSONRenderer строит JSONRenderer с shared Buf/Imports и фейковым
// TypeMapper, привязанным к RenderContext.
func newJSONRenderer(t *testing.T, tm render.TypeMapper) *JSONRenderer {
	t.Helper()

	ctx := &render.RenderContext{TypeMapper: tm}
	r := NewJSONRenderer()
	r.Init(codegen.NewBufferWriter(), render.NewImportTracker(), ctx)

	return r
}

func TestJSONRenderer_OneOfEmitsMarshalUnmarshal(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "CreatedEvent"}
	r := newJSONRenderer(t, tm)

	require.NoError(t, r.OnUnion(&parser.Schema{
		Name: "Event",
		OneOf: []*parser.Schema{
			{Ref: "#/components/schemas/CreatedEvent"},
			{Ref: "#/components/schemas/DeletedEvent"},
		},
	}, walk.UnionOneOf))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "func (m *Event) UnmarshalJSON(data []byte) error {")
	assert.Contains(t, got, "func (m Event) MarshalJSON() ([]byte, error) {")
	assert.Contains(t, got, "return fmt.Errorf(\"Event: no variant matched\")")
}

func TestJSONRenderer_AnyOfDispatched(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "CreatedEvent"}
	r := newJSONRenderer(t, tm)

	require.NoError(t, r.OnUnion(&parser.Schema{
		Name: "Event",
		AnyOf: []*parser.Schema{
			{Ref: "#/components/schemas/CreatedEvent"},
		},
	}, walk.UnionAnyOf))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "func (m *Event) UnmarshalJSON(data []byte) error {")
}

func TestJSONRenderer_TracksEncodingJSONImport(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "CreatedEvent"}
	r := newJSONRenderer(t, tm)

	require.NoError(t, r.OnUnion(&parser.Schema{
		Name: "Event",
		OneOf: []*parser.Schema{
			{Ref: "#/components/schemas/CreatedEvent"},
		},
	}, walk.UnionOneOf))

	imps := r.Imports.Imports()
	require.NotEmpty(t, imps)

	hasJSON := slices.ContainsFunc(imps, func(imp gogen.Import) bool {
		return imp.Path == "encoding/json"
	})
	hasFmt := slices.ContainsFunc(imps, func(imp gogen.Import) bool {
		return imp.Path == "fmt"
	})

	assert.True(t, hasJSON, "encoding/json import must be tracked")
	assert.True(t, hasFmt, "fmt import must be tracked")
}

func TestJSONRenderer_SkipsAnyVariants(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "any"}
	r := newJSONRenderer(t, tm)

	require.NoError(t, r.OnUnion(&parser.Schema{
		Name: "Event",
		OneOf: []*parser.Schema{
			{Type: "string"}, // variantType == "any" → skipped
		},
	}, walk.UnionOneOf))

	got := string(r.Buf.Content())
	assert.NotContains(t, got, "if m. ")
}

func TestJSONRenderer_InlineVariantGetsGeneratedFieldName(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "[]string"}
	r := newJSONRenderer(t, tm)

	require.NoError(t, r.OnUnion(&parser.Schema{
		Name: "Event",
		OneOf: []*parser.Schema{
			{Type: "array", Items: &parser.Schema{Type: "string"}},
		},
	}, walk.UnionOneOf))

	got := string(r.Buf.Content())
	// inlineVariantName("[]string") = "SliceString"
	assert.Contains(t, got, "m.SliceString")
}

func TestJSONRenderer_SkipReturnsTrue(t *testing.T) {
	t.Parallel()

	r := NewJSONRenderer()
	assert.True(t, r.Skip(&parser.Schema{Name: "Event"}))
}

// убеждаемся, что gogen импорт используется (он нужен для ImportTracker.Add
// в JSONRenderer.OnUnion).
var _ = gogen.Import{}
