// Package schema: tests for ConvertersRenderer. После Task 7 renderer
// подключён к aux-файлу через Generator.writeConvertersAuxFile — но тесты
// вызывают OnSplitStruct напрямую, проверяя рендер <Name>RequestToResponse
// без зависимости от composer'а.
package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ilovepitsa/oapicodegen/internal/codegen"
	"github.com/ilovepitsa/oapicodegen/internal/generator/render"
	"github.com/ilovepitsa/oapicodegen/internal/parser"
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

// TestConvertersRenderer_SplittableField_RendersConverter — shared-поле,
// резолвящееся в разные Request/Response Go-типы (splittable $ref), должно
// конвертироваться через <Type>RequestToResponse, а не прямым copy.
func TestConvertersRenderer_SplittableField_RendersConverter(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{byMode: map[string]string{
		modeRequest:  "UserRequest",
		modeResponse: "UserResponse",
	}}
	r := newConvertersTestRenderer(t, tm)

	require.NoError(t, r.OnSplitStruct(&parser.Schema{
		Name:    "Container",
		Type:    "object",
		IsSplit: true,
		Properties: []*parser.Property{
			{
				Name:     "user",
				Required: true,
				Schema:   &parser.Schema{Ref: "#/components/schemas/User"},
			},
		},
	}))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "resp.User = UserRequestToResponse(req.User)")
	assert.NotContains(t, got, "resp.User = req.User")
}

// TestConvertersRenderer_UnresolvedAnyField_DirectCopy — regression-тест бага
// v1.3.0: splittable-по-имени поле, чей тип НЕ резолвится (GoType = any как в
// Request, так и в Response — следствие дефекта #2 rolodex cross-file $ref),
// должно копироваться напрямую, а не через несуществующую anyToResponse.
//
// До фикса isSplittableField возвращал true (refToName(ref)="User" ∈ Splittable),
// а renderSplittableFieldConvert строил converterCall = "any"+"ToResponse" →
// undefined: anyToResponse → go build падал.
func TestConvertersRenderer_UnresolvedAnyField_DirectCopy(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: goTypeAny} // любой mode → any (тип не резолвится)
	r := newConvertersTestRenderer(t, tm)
	// Имитируем имя-в-Splittable: именно это вводило в заблуждение старый gate.
	r.Ctx.Splittable = map[string]bool{"User": true}

	require.NoError(t, r.OnSplitStruct(&parser.Schema{
		Name:    "LoginResponse",
		Type:    "object",
		IsSplit: true,
		Properties: []*parser.Property{
			{
				Name:     "user",
				Required: true,
				Schema:   &parser.Schema{Ref: "../models/User.yaml"},
			},
		},
	}))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "resp.User = req.User")
	assert.NotContains(t, got, "anyToResponse",
		"unresolved any field must direct-copy, not emit undefined anyToResponse")
}
