package operations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
)

func TestClientInterfaceRenderer_FilePath(t *testing.T) {
	t.Parallel()
	r := NewClientInterfaceRenderer()
	assert.Equal(t, "interfaces/client/client.gen.go", r.FilePath())
}

func TestClientInterfaceRenderer_EmptyProject_EmptyInterface(t *testing.T) {
	t.Parallel()
	r := NewClientInterfaceRenderer()
	ctx := &render.RenderContext{
		Project: &parser.Project{},
	}

	body, _, err := r.Render(ctx)
	require.NoError(t, err)
	got := string(body)
	assert.Contains(t, got, "type Client interface {")
	assert.Contains(t, got, "}")
}

func TestClientInterfaceRenderer_SingleOperation_RendersInterfaceAndStructs(t *testing.T) {
	t.Parallel()

	ctx := newClientTestCtx(t, []*parser.Method{
		{
			OperationID: "listPets",
			Parameters: []*parser.Parameter{
				{Name: "limit", In: "query", Schema: &parser.Schema{Type: "integer"}},
			},
		},
	})

	r := NewClientInterfaceRenderer()
	body, _, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)
	assert.Contains(t, got, "type Client interface {")
	assert.Contains(t, got, "ListPets(ctx context.Context, req *ListPetsRequest) (*ListPetsResponse, error)")
	assert.Contains(t, got, "type ListPetsRequest struct {")
	assert.Contains(t, got, "Limit *int `query:\"limit\"`")
	assert.Contains(t, got, "type ListPetsResponse struct {")
	assert.Contains(t, got, "Code int")
}

func TestClientInterfaceRenderer_DeprecatedOperation_HasComment(t *testing.T) {
	t.Parallel()

	ctx := newClientTestCtx(t, []*parser.Method{
		{
			OperationID: "oldMethod",
			Deprecated:  true,
		},
	})

	r := NewClientInterfaceRenderer()
	body, _, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)
	assert.Contains(t, got, "// Deprecated: operation is marked as deprecated")
}

func TestClientInterfaceRenderer_PathParam_RendersWithoutPointer(t *testing.T) {
	t.Parallel()

	ctx := newClientTestCtx(t, []*parser.Method{
		{
			OperationID: "getItem",
			Parameters: []*parser.Parameter{
				{Name: "id", In: "path", Required: true, Schema: &parser.Schema{Type: "string"}},
			},
		},
	})

	r := NewClientInterfaceRenderer()
	body, _, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)
	assert.Contains(t, got, "ID string `param:\"id\"`")
}

// newClientTestCtx строит RenderContext с project, содержащим один сервис
// с переданными методами. TypeMapper — mock.
func newClientTestCtx(t *testing.T, methods []*parser.Method) *render.RenderContext {
	t.Helper()
	return &render.RenderContext{
		Project: &parser.Project{
			Paths: &parser.Paths{
				Services: []*parser.Service{
					{Name: "default", Methods: methods},
				},
			},
		},
		TypeMapper: &mockTypeMapper{},
	}
}

// mockTypeMapper реализует render.TypeMapper и modeSettable для тестов.
type mockTypeMapper struct{ mode string }

func (m *mockTypeMapper) GoType(s *parser.Schema) string {
	if s == nil {
		return "any"
	}
	switch s.Type {
	case "integer":
		return "int"
	case "number":
		return "float64"
	case "boolean":
		return "bool"
	default:
		if s.Ref != "" {
			return "model." + goName(s.Ref)
		}
		return s.Type
	}
}

func (m *mockTypeMapper) SetMode(mode string) { m.mode = mode }
func (m *mockTypeMapper) Mode() string        { return m.mode }
