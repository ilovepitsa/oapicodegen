package operations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
)

func TestSDKRenderer_FilePath(t *testing.T) {
	t.Parallel()
	r := NewSDKRenderer()
	assert.Equal(t, "sdk/sdk.gen.go", r.FilePath())
}

func TestSDKRenderer_RendersSDKStruct(t *testing.T) {
	t.Parallel()

	project := &parser.Project{}
	project.CreatePaths("example.com/test")
	// Set up operations after CreatePaths to avoid wipe.
	project.Paths.Services = []*parser.Service{
		{Name: "default", Methods: []*parser.Method{{OperationID: "listItems"}}},
	}

	ctx := &render.RenderContext{
		Project:      project,
		ImportPrefix: "example.com/test",
		TypeMapper:   &mockTypeMapper{},
	}

	r := NewSDKRenderer()
	body, imps, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)
	assert.Contains(t, got, "type SDK struct {")
	assert.Contains(t, got, "apiclient.Client")
	assert.Contains(t, got, "func NewSDK(baseURL string, opts ...httpclient.Option) (*SDK, error) {")
	assert.Contains(t, got, "implclient.NewClient(baseURL, opts...)")

	// Verify imports
	assert.NotNil(t, imps)
	imports := imps.Imports()
	assert.NotEmpty(t, imports)
}

func TestSDKRenderer_NoOperations_EmptyFile(t *testing.T) {
	t.Parallel()

	ctx := &render.RenderContext{
		Project: &parser.Project{},
	}

	r := NewSDKRenderer()
	body, _, err := r.Render(ctx)
	require.NoError(t, err)
	assert.Empty(t, body)
}
