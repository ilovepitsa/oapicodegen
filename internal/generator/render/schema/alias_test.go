package schema

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTypeMapper — тестовая реализация render.TypeMapper, возвращающая
// предзаготовленную строку. Используется в тестах, где не нужна полная
// логика generator.typeMapper (resolution $ref, imports, nullable и т.п.).
type fakeTypeMapper struct{ got string }

func (f *fakeTypeMapper) GoType(_ *parser.Schema) string { return f.got }

// newAliasTestRenderer строит AliasRenderer с shared Buf/Imports и фейковым
// TypeMapper, привязанным к RenderContext.
func newAliasTestRenderer(t *testing.T, tm render.TypeMapper) *AliasRenderer {
	t.Helper()

	ctx := &render.RenderContext{TypeMapper: tm}
	r := NewAliasRenderer()
	r.Init(codegen.NewBufferWriter(), render.NewImportTracker(), ctx)

	return r
}

func TestAliasRenderer_StringAlias(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newAliasTestRenderer(t, tm)

	require.NoError(t, r.OnAlias(&parser.Schema{Name: "PetID", Type: "string"}))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "type PetID string")
}

func TestAliasRenderer_IntegerAliasInt64(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "int64"}
	r := newAliasTestRenderer(t, tm)

	require.NoError(t, r.OnAlias(&parser.Schema{Name: "Age", Type: "integer", Format: "int64"}))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "type Age int64")
}

func TestAliasRenderer_DescriptionEmitsDocComment(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newAliasTestRenderer(t, tm)

	require.NoError(t, r.OnAlias(&parser.Schema{
		Name:        "Label",
		Type:        "string",
		Description: "Short label",
	}))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "// Short label")
	assert.Contains(t, got, "type Label string")
}

func TestAliasRenderer_MapAliasWithStringValues(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newAliasTestRenderer(t, tm)

	require.NoError(t, r.OnMap(&parser.Schema{
		Name:                 "StringMap",
		Type:                 "object",
		AdditionalProperties: &parser.Schema{Type: "string"},
	}))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "type StringMap map[string]string")
}

func TestAliasRenderer_MapAliasWithAdditionalPropertiesFalse(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "any"}
	r := newAliasTestRenderer(t, tm)

	require.NoError(t, r.OnMap(&parser.Schema{
		Name:                      "Empty",
		Type:                      "object",
		AdditionalPropertiesFalse: true,
	}))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "type Empty struct{}")
}

func TestAliasRenderer_MapAliasWithoutAdditionalPropertiesDefaultsToAny(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "any"}
	r := newAliasTestRenderer(t, tm)

	require.NoError(t, r.OnMap(&parser.Schema{Name: "AnyMap", Type: "object"}))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "type AnyMap map[string]any")
}
