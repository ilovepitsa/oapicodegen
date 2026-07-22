package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
)

func TestExpectedValidatorsRenderer_FilePath(t *testing.T) {
	t.Parallel()

	r := NewExpectedValidatorsRenderer()
	assert.Equal(t, "model/expected_validators.gen.go", r.FilePath())
}

func TestExpectedValidatorsRenderer_NoNamedValidators_EmptyBody(t *testing.T) {
	t.Parallel()

	r := NewExpectedValidatorsRenderer()
	ctx := &render.RenderContext{
		Project: &parser.Project{
			Model: newModelWithSchemas([]*parser.Schema{
				{
					Name: "Item",
					Type: "object",
					Properties: []*parser.Property{
						{Name: "size", Schema: &parser.Schema{Type: "integer"}},
					},
				},
			}),
		},
	}

	body, _, err := r.Render(ctx)
	require.NoError(t, err)
	assert.Empty(t, body, "schema with only SimpleRule validations must produce no file body")
}

func TestExpectedValidatorsRenderer_NamedValidators_RendersSortedList(t *testing.T) {
	t.Parallel()

	r := NewExpectedValidatorsRenderer()
	ctx := &render.RenderContext{
		Project: &parser.Project{
			Model: newModelWithSchemas([]*parser.Schema{
				{
					Name: "Item",
					Type: "object",
					Validations: []parser.ValidationRule{
						parser.NamedRule{Name: "app.ItemConsistency"},
					},
					Properties: []*parser.Property{
						{
							Name:   "name",
							Schema: &parser.Schema{Type: "string"},
							Validations: []parser.ValidationRule{
								parser.NamedRule{Name: "app.NonEmptyName"},
							},
						},
						{
							Name:   "size",
							Schema: &parser.Schema{Type: "integer"},
							Validations: []parser.ValidationRule{
								parser.NamedRule{Name: "app.ItemConsistency"}, // дубликат — должен исчезнуть
							},
						},
					},
				},
				{
					Name: "Order",
					Type: "object",
					Validations: []parser.ValidationRule{
						parser.NamedRule{Name: "app.ItemCreateConsistency"},
					},
					Properties: []*parser.Property{},
				},
			}),
		},
	}

	body, imps, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)
	assert.Contains(t, got, "func ExpectedValidatorNames() []string {")
	assert.Contains(t, got, `"app.ItemConsistency",`)
	assert.Contains(t, got, `"app.ItemCreateConsistency",`)
	assert.Contains(t, got, `"app.NonEmptyName",`)

	// Порядок отсортированный: ItemConsistency < ItemCreateConsistency < NonEmptyName.
	idxConsistency := indexOf(t, got, `"app.ItemConsistency"`)
	idxCreate := indexOf(t, got, `"app.ItemCreateConsistency"`)
	idxNonEmpty := indexOf(t, got, `"app.NonEmptyName"`)
	assert.Less(t, idxConsistency, idxCreate)
	assert.Less(t, idxCreate, idxNonEmpty)

	// Импортов нет — файл только объявляет функцию, возвращающую []string.
	assert.Empty(t, imps.Imports())
}

func TestExpectedValidatorsRenderer_NilProject_EmptyBody(t *testing.T) {
	t.Parallel()

	r := NewExpectedValidatorsRenderer()
	ctx := &render.RenderContext{}

	body, _, err := r.Render(ctx)
	require.NoError(t, err)
	assert.Empty(t, body)
}

// newModelWithSchemas строит *parser.Model с заданными схемами (и пустым
// индексом). Используется тестами ExpectedValidatorsRenderer.
func newModelWithSchemas(schemas []*parser.Schema) *parser.Model {
	m := &parser.Model{}
	m.SetSchemas(schemas)
	return m
}

func indexOf(t *testing.T, s, substr string) int {
	t.Helper()
	i := indexOfString(s, substr)
	require.GreaterOrEqual(t, i, 0, "substring %q not found in %q", substr, s)
	return i
}

// indexOfString — обёртка без require, чтобы можно было использовать в
// декларативных assert-цепочках.
func indexOfString(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
