package operations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
)

func TestServerInterfaceRenderer_FilePath(t *testing.T) {
	t.Parallel()
	r := NewServerInterfaceRenderer()
	assert.Equal(t, "interfaces/server/server.gen.go", r.FilePath())
}

func TestServerInterfaceRenderer_EmptyProject_EmptyInterface(t *testing.T) {
	t.Parallel()
	r := NewServerInterfaceRenderer()
	ctx := &render.RenderContext{
		Project: &parser.Project{},
	}

	body, _, err := r.Render(ctx)
	require.NoError(t, err)
	got := string(body)
	assert.Contains(t, got, "type Server interface {")
	assert.Contains(t, got, "}")
}

func TestServerInterfaceRenderer_SingleOperation_RendersMethod(t *testing.T) {
	t.Parallel()

	ctx := &render.RenderContext{
		Project: &parser.Project{
			Paths: &parser.Paths{
				Services: []*parser.Service{
					{
						Name: "default",
						Methods: []*parser.Method{
							{OperationID: "listPets"},
						},
					},
				},
			},
		},
		TypeMapper: &mockTypeMapper{},
	}

	r := NewServerInterfaceRenderer()
	body, imps, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)
	assert.Contains(t, got, "type Server interface {")
	assert.Contains(t, got, "ListPets(ctx context.Context, req *ListPetsRequest) (*ListPetsResponse, error)")

	assertImportHasPath(t, imps, "context")
}

func TestServerInterfaceRenderer_DeprecatedOperation_HasComment(t *testing.T) {
	t.Parallel()

	ctx := &render.RenderContext{
		Project: &parser.Project{
			Paths: &parser.Paths{
				Services: []*parser.Service{
					{
						Name: "default",
						Methods: []*parser.Method{
							{OperationID: "oldMethod", Deprecated: true},
						},
					},
				},
			},
		},
		TypeMapper: &mockTypeMapper{},
	}

	r := NewServerInterfaceRenderer()
	body, _, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)
	assert.Contains(t, got, "// Deprecated: operation is marked as deprecated")
}

func assertImportHasPath(t *testing.T, imps *render.ImportTracker, path string) {
	t.Helper()
	for _, imp := range imps.Imports() {
		if imp.Path == path {
			return
		}
	}
	t.Errorf("expected import %q not found", path)
}
