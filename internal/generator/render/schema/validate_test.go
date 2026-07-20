// Package schema: tests for ValidateOwnRenderer. После Task 4 renderer
// подключён к pack'у в writeStructFileViaComposer — но тесты вызывают
// OnStruct/OnSplitStruct напрямую, проверяя рендер ValidateOwn-метода без
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

// newValidateOwnTestRenderer строит ValidateOwnRenderer с shared Buf/Imports
// и фейковым TypeMapper, привязанным к RenderContext. Project в ctx — non-nil
// с пустой проиндексированной Model (для симметрии с SetDefaultsRenderer).
func newValidateOwnTestRenderer(t *testing.T, tm render.TypeMapper) *ValidateOwnRenderer {
	t.Helper()

	ctx := &render.RenderContext{
		TypeMapper: tm,
		Project:    newTestProjectWithEmptyModel(),
	}
	r := NewValidateOwnRenderer()
	r.Init(codegen.NewBufferWriter(), render.NewImportTracker(), ctx)

	return r
}

func TestValidateOwnRenderer_NoRules_NoOutput(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newValidateOwnTestRenderer(t, tm)

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name: "Pet",
		Type: "object",
		Properties: []*parser.Property{
			{Name: "Tag", Schema: &parser.Schema{Type: "string"}},
		},
	}))

	got := string(r.Buf.Content())
	assert.Empty(t, got, "schema without validations must produce no output")
}

func TestValidateOwnRenderer_SimpleRule_RendersIfBlock(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "int"}
	r := newValidateOwnTestRenderer(t, tm)

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name: "Pet",
		Type: "object",
		Properties: []*parser.Property{
			{
				Name:     "age",
				Required: true,
				Schema:   &parser.Schema{Type: "integer"},
				Validations: []parser.ValidationRule{
					parser.SimpleRule{Op: parser.OpGT, Target: parser.TargetValue, Value: 0},
				},
			},
		},
	}))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "func (x Pet) ValidateOwn(reg *validator.Registry) error {")
	// Required + int → no pointer, no guard. Rule ">0" inverts to "<=0".
	assert.Contains(t, got, "if x.Age <= 0 {")
	assert.Contains(t, got, `return fmt.Errorf("field Age: must be > 0")`)
}

func TestValidateOwnRenderer_NamedRuleProperty_RendersLookupAndCall(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newValidateOwnTestRenderer(t, tm)

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name: "Pet",
		Type: "object",
		Properties: []*parser.Property{
			{
				Name:     "email",
				Required: true,
				Schema:   &parser.Schema{Type: "string"},
				Validations: []parser.ValidationRule{
					parser.NamedRule{Name: "cdn.EmailFormat"},
				},
			},
		},
	}))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "func (x Pet) ValidateOwn(reg *validator.Registry) error {")
	// Required + string → no guard → no if-wrapper around block-scope.
	assert.Contains(t, got, `v, ok := reg.Get("cdn.EmailFormat")`)
	assert.Contains(t, got, `return fmt.Errorf("validator %q not registered", "cdn.EmailFormat")`)
	assert.Contains(t, got, "if err := v.Validate(x.Email); err != nil {")
	assert.Contains(t, got, `return fmt.Errorf("field Email: %w", err)`)
}

func TestValidateOwnRenderer_SchemaLevelNamedRule_RendersAtEnd(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newValidateOwnTestRenderer(t, tm)

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name: "Pet",
		Type: "object",
		Validations: []parser.ValidationRule{
			parser.NamedRule{Name: "cdn.PetConsistency"},
		},
		Properties: []*parser.Property{
			{Name: "name", Schema: &parser.Schema{Type: "string"}},
		},
	}))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "func (x Pet) ValidateOwn(reg *validator.Registry) error {")
	// Schema-level: validator called on receiver, no field-path wrap.
	assert.Contains(t, got, `v, ok := reg.Get("cdn.PetConsistency")`)
	assert.Contains(t, got, "if err := v.Validate(x); err != nil {")
	assert.Contains(t, got, "\t\t\treturn err\n")
}

func TestValidateOwnRenderer_PointerField_RendersNilGuard(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newValidateOwnTestRenderer(t, tm)

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name: "Pet",
		Type: "object",
		Properties: []*parser.Property{
			{
				Name:   "tag",
				Schema: &parser.Schema{Type: "string"},
				Validations: []parser.ValidationRule{
					parser.SimpleRule{Op: parser.OpGE, Target: parser.TargetValue, Value: 1},
				},
			},
		},
	}))

	got := string(r.Buf.Content())
	// Non-required + non-nilable string → *string → guard + deref.
	// Rule ">=1" inverts to "< 1".
	assert.Contains(t, got, "if x.Tag != nil && *x.Tag < 1 {")
	assert.Contains(t, got, `return fmt.Errorf("field Tag: must be >= 1")`)
}

func TestValidateOwnRenderer_ValidatorImportAdded(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "int"}
	r := newValidateOwnTestRenderer(t, tm)

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name: "Pet",
		Type: "object",
		Properties: []*parser.Property{
			{
				Name:     "age",
				Required: true,
				Schema:   &parser.Schema{Type: "integer"},
				Validations: []parser.ValidationRule{
					parser.SimpleRule{Op: parser.OpGT, Target: parser.TargetValue, Value: 0},
				},
			},
		},
	}))

	imps := r.Imports.Imports()
	require.NotEmpty(t, imps)

	hasValidator := slices.ContainsFunc(imps, func(imp gogen.Import) bool {
		return imp.Path == validatorPkg && imp.Alias == "validator"
	})
	assert.True(t, hasValidator, "validator import must be tracked")

	hasFmt := slices.ContainsFunc(imps, func(imp gogen.Import) bool {
		return imp.Path == "fmt" && imp.Alias == ""
	})
	assert.True(t, hasFmt, "fmt import must be tracked")
}

func TestValidateOwnRenderer_SizeRule_RendersLenWithoutGuard(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newValidateOwnTestRenderer(t, tm)

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name: "Pet",
		Type: "object",
		Properties: []*parser.Property{
			{
				Name:     "name",
				Required: true,
				Schema:   &parser.Schema{Type: "string"},
				Validations: []parser.ValidationRule{
					parser.SimpleRule{Op: parser.OpGE, Target: parser.TargetSize, Value: 2},
				},
			},
		},
	}))

	got := string(r.Buf.Content())
	// Required + string + TargetSize → len() without nil-guard.
	// Rule ">=2" inverts to "< 2".
	assert.Contains(t, got, "if len(x.Name) < 2 {")
}

func TestValidateOwnRenderer_SplitStruct_RendersRequestAndResponse(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newValidateOwnTestRenderer(t, tm)

	require.NoError(t, r.OnSplitStruct(&parser.Schema{
		Name:    "Pet",
		Type:    "object",
		IsSplit: true,
		Properties: []*parser.Property{
			{
				Name:   "name",
				Schema: &parser.Schema{Type: "string"},
				Validations: []parser.ValidationRule{
					parser.SimpleRule{Op: parser.OpGE, Target: parser.TargetValue, Value: 1},
				},
			},
			{
				Name:   "id",
				Schema: &parser.Schema{Type: "integer", ReadOnly: true},
				Validations: []parser.ValidationRule{
					parser.SimpleRule{Op: parser.OpGT, Target: parser.TargetValue, Value: 0},
				},
			},
		},
	}))

	got := string(r.Buf.Content())
	// Request variant: name only (id is readOnly, filtered out).
	assert.Contains(t, got, "func (x PetRequest) ValidateOwn(reg *validator.Registry) error {")
	// Response variant: both name and id present.
	assert.Contains(t, got, "func (x PetResponse) ValidateOwn(reg *validator.Registry) error {")
	// Both variants have their rule rendered.
	assert.Contains(t, got, "*x.Name < 1")
	assert.Contains(t, got, "*x.ID <= 0")
}

// TestValidateOwnRenderer_NamedRulePointerField_RendersGuardBlock проверяет,
// что NamedRule на optional (pointer) поле обёрнут в if-guard с
// deref-accessor — регрессия на Tag-сценарий из minimal spec.
func TestValidateOwnRenderer_NamedRulePointerField_RendersGuardBlock(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newValidateOwnTestRenderer(t, tm)

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name: "Pet",
		Type: "object",
		Properties: []*parser.Property{
			{
				Name:   "email",
				Schema: &parser.Schema{Type: "string"},
				Validations: []parser.ValidationRule{
					parser.NamedRule{Name: "cdn.EmailFormat"},
				},
			},
		},
	}))

	got := string(r.Buf.Content())
	// Non-required → *string → guard + deref for validator call.
	assert.Contains(t, got, "if x.Email != nil {")
	assert.Contains(t, got, "if err := v.Validate(*x.Email); err != nil {")
}
