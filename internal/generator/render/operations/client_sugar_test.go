package operations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
)

func TestClientSugarRenderer_FilePath(t *testing.T) {
	t.Parallel()
	r := NewClientSugarRenderer()
	assert.Equal(t, "interfaces/client/client_sugar.gen.go", r.FilePath())
}

func TestClientSugarRenderer_WithSuccessResponse_RendersReturnValue(t *testing.T) {
	t.Parallel()

	ctx := &render.RenderContext{
		Project: &parser.Project{
			Paths: &parser.Paths{
				Services: []*parser.Service{
					{
						Name: "default",
						Methods: []*parser.Method{
							{
								OperationID: "getItem",
								Responses: []*parser.Response{
									{StatusCode: "200", Content: map[string]*parser.MediaType{
										"application/json": {Schema: &parser.Schema{Type: "string"}},
									}},
								},
							},
						},
					},
				},
			},
		},
		TypeMapper: &mockTypeMapper{},
	}

	r := NewClientSugarRenderer()
	body, _, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)
	assert.Contains(t, got, "type ClientSugared struct {")
	assert.Contains(t, got, "func NewClientSugared(impl Client) *ClientSugared {")
	assert.Contains(t, got, "func (x *ClientSugared) GetItem(ctx context.Context, req *GetItemRequest) (*string, error) {")
	assert.Contains(t, got, "return resp.Response200, nil")
}

func TestClientSugarRenderer_NoBodyResponse_ReturnsOnlyError(t *testing.T) {
	t.Parallel()

	ctx := &render.RenderContext{
		Project: &parser.Project{
			Paths: &parser.Paths{
				Services: []*parser.Service{
					{
						Name: "default",
						Methods: []*parser.Method{
							{
								OperationID: "deleteItem",
								Responses: []*parser.Response{
									{StatusCode: "204"},
								},
							},
						},
					},
				},
			},
		},
		TypeMapper: &mockTypeMapper{},
	}

	r := NewClientSugarRenderer()
	body, _, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)
	assert.Contains(t, got, "func (x *ClientSugared) DeleteItem(ctx context.Context, req *DeleteItemRequest) error {")
	assert.NotContains(t, got, "DeleteItem(ctx context.Context, req *DeleteItemRequest) (")
}

func TestClientSugarRenderer_EmptyProject_NoMethods(t *testing.T) {
	t.Parallel()

	ctx := &render.RenderContext{
		Project: &parser.Project{},
	}
	r := NewClientSugarRenderer()
	body, _, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)
	assert.Contains(t, got, "type ClientSugared struct {")
}
