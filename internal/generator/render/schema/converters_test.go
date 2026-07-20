// Package schema: tests for ConvertersRenderer. После Task 7 renderer
// подключён к aux-файлу через Generator.writeConvertersAuxFile — но тесты
// вызывают OnSplitStruct напрямую, проверяя рендер <Name>RequestToResponse
// без зависимости от composer'а.
package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
)

func newConvertersTestRenderer(t *testing.T, tm render.TypeMapper) *ConvertersRenderer {
	t.Helper()

	ctx := &render.RenderContext{
		TypeMapper: tm,
		Project:    newTestProjectWithEmptyModel(),
	}
	r := NewConvertersRenderer()
	r.Init(codegen.NewBufferWriter(), render.NewImportTracker(), ctx)

	return r
}

func TestConvertersRenderer_NoSharedFields_NoOutput(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newConvertersTestRenderer(t, tm)

	require.NoError(t, r.OnSplitStruct(&parser.Schema{
		Name:    "Pet",
		Type:    "object",
		IsSplit: true,
		Properties: []*parser.Property{
			{Name: "id", Schema: &parser.Schema{Type: "integer", ReadOnly: true}},
			{Name: "secret", Schema: &parser.Schema{Type: "string", WriteOnly: true}},
		},
	}))

	assert.Empty(t, r.Buf.Content())
}

func TestConvertersRenderer_OnStruct_NoOutput(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newConvertersTestRenderer(t, tm)

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name: "Pet",
		Type: "object",
		Properties: []*parser.Property{
			{Name: "name", Schema: &parser.Schema{Type: "string"}},
		},
	}))

	assert.Empty(t, r.Buf.Content())
}

func TestConvertersRenderer_SharedFields_RendersRequestToResponse(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newConvertersTestRenderer(t, tm)

	require.NoError(t, r.OnSplitStruct(&parser.Schema{
		Name:    "Pet",
		Type:    "object",
		IsSplit: true,
		Properties: []*parser.Property{
			{Name: "id", Schema: &parser.Schema{Type: "integer", ReadOnly: true}},
			{Name: "name", Schema: &parser.Schema{Type: "string"}},
			{Name: "secret", Schema: &parser.Schema{Type: "string", WriteOnly: true}},
			{Name: "tag", Schema: &parser.Schema{Type: "string"}},
		},
	}))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "func PetRequestToResponse(req PetRequest) PetResponse {")
	assert.Contains(t, got, "var resp PetResponse")
	assert.Contains(t, got, "resp.Name = req.Name")
	assert.Contains(t, got, "resp.Tag = req.Tag")
	assert.NotContains(t, got, "resp.ID = req.ID")
	assert.NotContains(t, got, "resp.Secret = req.Secret")
	assert.Contains(t, got, "return resp")
}
