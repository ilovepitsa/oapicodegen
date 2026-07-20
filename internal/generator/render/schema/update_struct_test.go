// Package schema: tests for UpdateStructRenderer. После Task 5 renderer
// подключён к pack'у в writeStructFileViaComposer — но тесты вызывают
// OnStruct/OnSplitStruct напрямую, проверяя рендер Update<Name>-варианта без
// зависимости от остальных renderer'ов.
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

// newUpdateStructTestRenderer строит UpdateStructRenderer с shared
// Buf/Imports и фейковым TypeMapper, привязанным к RenderContext. Project в
// ctx — non-nil с пустой проиндексированной Model (для симметрии с
// SetDefaultsRenderer/ValidateOwnRenderer).
func newUpdateStructTestRenderer(t *testing.T, tm render.TypeMapper) *UpdateStructRenderer {
	t.Helper()

	ctx := &render.RenderContext{
		TypeMapper: tm,
		Project:    newTestProjectWithEmptyModel(),
	}
	r := NewUpdateStructRenderer()
	r.Init(codegen.NewBufferWriter(), render.NewImportTracker(), ctx)

	return r
}

func TestUpdateStructRenderer_NotUsedInUpdate_NoOutput(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newUpdateStructTestRenderer(t, tm)

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name:           "Pet",
		Type:           "object",
		IsUsedInUpdate: false,
		Properties: []*parser.Property{
			{Name: "name", Schema: &parser.Schema{Type: "string"}, IsUsedInUpdate: true},
		},
	}))

	got := string(r.Buf.Content())
	assert.Empty(t, got, "schema without IsUsedInUpdate must produce no output")
}

func TestUpdateStructRenderer_UsedInUpdate_RendersStruct(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newUpdateStructTestRenderer(t, tm)

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
	// Все поля обёрнуты в optional.Optional[T] безусловно.
	assert.True(t, containsCollapsed(got, "Name optional.Optional[string] `json:\"name\" yaml:\"name\"`"))
}

func TestUpdateStructRenderer_RendersGetters(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newUpdateStructTestRenderer(t, tm)

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name:           "Pet",
		Type:           "object",
		IsUsedInUpdate: true,
		Properties: []*parser.Property{
			{Name: "name", Schema: &parser.Schema{Type: "string"}, IsUsedInUpdate: true},
		},
	}))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "func (u *UpdatePet) GetName() (*string, bool) {")
	// Семантика presence-флага.
	assert.Contains(t, got, "if !u.Name.IsSet() {")
	assert.Contains(t, got, "if u.Name.IsNil() {")
	assert.Contains(t, got, "v := u.Name.Value()")
	assert.Contains(t, got, "return &v, true")
}

// TestUpdateStructRenderer_RendersUpdateValidateOwn проверяет, что
// UpdateStructRenderer рендерит `func (x Update<Name>) ValidateOwn(...)` через
// общий renderValidateOwnInto с isUpdate=true. Guard для optional-полей:
// `x.Field.IsSet() && !x.Field.IsNil()`.
func TestUpdateStructRenderer_RendersUpdateValidateOwn(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newUpdateStructTestRenderer(t, tm)

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name:           "Pet",
		Type:           "object",
		IsUsedInUpdate: true,
		Properties: []*parser.Property{
			{
				Name:           "name",
				Schema:         &parser.Schema{Type: "string"},
				IsUsedInUpdate: true,
				Validations: []parser.ValidationRule{
					parser.SimpleRule{Op: parser.OpGE, Target: parser.TargetSize, Value: 1},
				},
			},
		},
	}))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "func (x UpdatePet) ValidateOwn(reg *validator.Registry) error {")
	// isUpdate=true → optional-wrapped accessor + guard.
	// Rule ">=1" with TargetSize → len(x.Name.Value()) < 1 with guard.
	assert.Contains(t, got, "if x.Name.IsSet() && !x.Name.IsNil() && len(x.Name.Value()) < 1 {")
}

// TestUpdateStructRenderer_OptionalImportAdded проверяет, что после OnStruct
// импорт optional добавлен в ImportTracker (нужен для Optional[T]-обёртки).
func TestUpdateStructRenderer_OptionalImportAdded(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newUpdateStructTestRenderer(t, tm)

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name:           "Pet",
		Type:           "object",
		IsUsedInUpdate: true,
		Properties: []*parser.Property{
			{Name: "name", Schema: &parser.Schema{Type: "string"}, IsUsedInUpdate: true},
		},
	}))

	imps := r.Imports.Imports()
	require.NotEmpty(t, imps)

	hasOptional := slices.ContainsFunc(imps, func(imp gogen.Import) bool {
		return imp.Path == optionalPkg && imp.Alias == "optional"
	})
	assert.True(t, hasOptional, "optional import must be tracked")
}

// TestUpdateStructRenderer_OnlyIsUsedInUpdateFieldsRendered проверяет, что
// поля без IsUsedInUpdate не попадают в Update<Name>-структуру.
func TestUpdateStructRenderer_OnlyIsUsedInUpdateFieldsRendered(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newUpdateStructTestRenderer(t, tm)

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name:           "Pet",
		Type:           "object",
		IsUsedInUpdate: true,
		Properties: []*parser.Property{
			{Name: "name", Schema: &parser.Schema{Type: "string"}, IsUsedInUpdate: true},
			{Name: "id", Schema: &parser.Schema{Type: "string"}, IsUsedInUpdate: false},
		},
	}))

	got := string(r.Buf.Content())
	assert.True(t, containsCollapsed(got, "Name optional.Optional[string]"))
	assert.NotContains(t, got, "ID optional.Optional")
	assert.NotContains(t, got, "GetID()")
}

// TestUpdateStructRenderer_SplitStruct_RendersWithRequestMode проверяет, что
// OnSplitStruct рендерит Update<Name> с mode=modeRequest (для разрешения $ref
// на splittable-схемы в <Name>Request).
func TestUpdateStructRenderer_SplitStruct_RendersWithRequestMode(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newUpdateStructTestRenderer(t, tm)
	r.Ctx.Features = parser.ProjectFeatures{SplitRequestResponse: parser.ProjectFeature{Value: true}}

	require.NoError(t, r.OnSplitStruct(&parser.Schema{
		Name:           "Pet",
		Type:           "object",
		IsSplit:        true,
		IsUsedInUpdate: true,
		Properties: []*parser.Property{
			{Name: "name", Schema: &parser.Schema{Type: "string"}, IsUsedInUpdate: true},
		},
	}))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "type UpdatePet struct {")
	assert.Contains(t, got, "func (u *UpdatePet) GetName() (*string, bool) {")
}

// TestUpdateStructRenderer_DescriptionEmitsDocComment проверяет, что
// description схемы рендерится как doc-comment перед type Update<Name>.
func TestUpdateStructRenderer_DescriptionEmitsDocComment(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newUpdateStructTestRenderer(t, tm)

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name:           "Pet",
		Type:           "object",
		Description:    "A pet.",
		IsUsedInUpdate: true,
		Properties: []*parser.Property{
			{Name: "name", Schema: &parser.Schema{Type: "string"}, IsUsedInUpdate: true},
		},
	}))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "// UpdatePet — PATCH/PUT variant of Pet.")
}

// TestUpdateStructRenderer_NoValidations_NoValidateOwn проверяет, что при
// отсутствии x-validations Update<Name>-структура рендерится (type + getters),
// но метод ValidateOwn не рендерится.
func TestUpdateStructRenderer_NoValidations_NoValidateOwn(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newUpdateStructTestRenderer(t, tm)

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
	assert.Contains(t, got, "func (u *UpdatePet) GetName()")
	assert.NotContains(t, got, "func (x UpdatePet) ValidateOwn")
}
