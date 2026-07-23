package operations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
)

func TestMockClientRenderer_FilePath(t *testing.T) {
	t.Parallel()
	r := NewMockClientRenderer()
	assert.Equal(t, "impl/mocks/client/mocks.gen.go", r.FilePath())
}

func TestMockClientRenderer_PackageName(t *testing.T) {
	t.Parallel()
	r := NewMockClientRenderer()
	assert.Equal(t, "mock_client", r.PackageName())
}

func TestMockClientRenderer_RendersMockStruct(t *testing.T) {
	t.Parallel()

	project := &parser.Project{}
	project.CreatePaths("example.com/test")
	project.Paths.Services = []*parser.Service{
		{Name: "default", Methods: []*parser.Method{{OperationID: "listItems"}}},
	}

	ctx := &render.RenderContext{
		Project:    project,
		TypeMapper: &mockTypeMapper{},
	}

	r := NewMockClientRenderer()
	body, _, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)
	assert.Contains(t, got, "type MockClient struct {")
	assert.Contains(t, got, "func NewMockClient(ctrl *gomock.Controller) *MockClient {")
	assert.Contains(t, got, "func (m *MockClient) ListItems")
}

func TestMockServerRenderer_FilePath(t *testing.T) {
	t.Parallel()
	r := NewMockServerRenderer()
	assert.Equal(t, "impl/mocks/server/mocks.gen.go", r.FilePath())
}

func TestMockServerRenderer_PackageName(t *testing.T) {
	t.Parallel()
	r := NewMockServerRenderer()
	assert.Equal(t, "mock_server", r.PackageName())
}

func TestMockServerRenderer_RendersMockStruct(t *testing.T) {
	t.Parallel()

	project := &parser.Project{}
	project.CreatePaths("example.com/test")
	project.Paths.Services = []*parser.Service{
		{Name: "default", Methods: []*parser.Method{{OperationID: "listPets"}}},
	}

	ctx := &render.RenderContext{
		Project:    project,
		TypeMapper: &mockTypeMapper{},
	}

	r := NewMockServerRenderer()
	body, _, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)
	assert.Contains(t, got, "type MockServer struct {")
	assert.Contains(t, got, "func NewMockServer(ctrl *gomock.Controller) *MockServer {")
	assert.Contains(t, got, "func (m *MockServer) ListPets")
}
