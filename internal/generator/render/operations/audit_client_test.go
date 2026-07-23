package operations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
)

func TestAuditClientRenderer_FilePath(t *testing.T) {
	t.Parallel()
	r := NewAuditClientRenderer()
	assert.Equal(t, "interfaces/client/audit.gen.go", r.FilePath())
}

func TestAuditClientRenderer_OpWithBody_RendersAuditStruct(t *testing.T) {
	t.Parallel()

	project := &parser.Project{
		Paths: &parser.Paths{
			Services: []*parser.Service{
				{
					Name: "default",
					Methods: []*parser.Method{
						{
							OperationID: "createItem",
							Parameters: []*parser.Parameter{
								{Name: "id", In: "path", Required: true, Schema: &parser.Schema{Type: "string"}},
							},
							RequestBody: &parser.RequestBody{
								Required: true,
								Content: map[string]*parser.MediaType{
									"application/json": {Schema: &parser.Schema{Ref: "#/components/schemas/ItemCreate"}},
								},
							},
						},
					},
				},
			},
		},
	}
	// Set up Model with the resolved schema so resolveBodySchema can find it.
	project.Model = &parser.Model{}
	project.Model.SetSchemas([]*parser.Schema{{Name: "ItemCreate"}})

	ctx := &render.RenderContext{
		Project:    project,
		TypeMapper: &mockTypeMapper{},
	}

	r := NewAuditClientRenderer()
	body, _, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)
	assert.Contains(t, got, "type CreateItemRequestAuditData struct {")
	assert.Contains(t, got, "func (req *CreateItemRequest) GetAuditData() any {")
	assert.Contains(t, got, "am.Body = req.Body.GetAuditData()")
}

func TestAuditClientRenderer_OpWithRefResponse_RendersResponseAudit(t *testing.T) {
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
									{
										StatusCode: "200",
										Content: map[string]*parser.MediaType{
											"application/json": {Schema: &parser.Schema{Name: "Item", Ref: "Item"}},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		TypeMapper: &mockTypeMapper{},
	}

	r := NewAuditClientRenderer()
	body, _, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)
	assert.Contains(t, got, "type GetItemResponse200AuditData struct {")
	assert.Contains(t, got, "func (resp *GetItemResponse) Response200AuditData() GetItemResponse200AuditData {")
}

func TestAuditClientRenderer_NoOperations_EmptyFile(t *testing.T) {
	t.Parallel()

	ctx := &render.RenderContext{
		Project: &parser.Project{},
	}

	r := NewAuditClientRenderer()
	body, _, err := r.Render(ctx)
	require.NoError(t, err)
	assert.Empty(t, body)
}

func TestAuditClientRenderer_InlineSchema_NoAudit(t *testing.T) {
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
									{
										StatusCode: "200",
										Content: map[string]*parser.MediaType{
											"application/json": {Schema: &parser.Schema{Type: "string"}},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		TypeMapper: &mockTypeMapper{},
	}

	r := NewAuditClientRenderer()
	body, _, err := r.Render(ctx)
	require.NoError(t, err)
	assert.Empty(t, body)
}
