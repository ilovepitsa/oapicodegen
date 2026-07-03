package generator

import (
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/golden"
	oapiparser "nschugorev/oapigenerator/internal/parser"
)

var wsRe = regexp.MustCompile(`\s+`)

// collapseWS сворачивает последовательности пробелов в один пробел —
// позволяет не зависеть от выравнивания gofmt в assert.Contains.
func collapseWS(s string) string {
	return strings.TrimSpace(wsRe.ReplaceAllString(s, " "))
}

func containsCollapsed(got, want string) bool {
	return strings.Contains(collapseWS(got), collapseWS(want))
}

const testModulePath = "nschugorev/oapigenerator/internal/generator/testdata/golden/petstore"

func TestGenerate_PetstoreGolden(t *testing.T) {
	data := mustReadFile(t, "testdata/petstore.yaml")
	doc, err := oapiparser.Parse(data)
	require.NoError(t, err)

	dir := golden.NewDir(t, golden.WithPath("testdata/golden/petstore"), golden.WithRecreateOnUpdate())
	fw := golden.NewCodegenFS(t, dir)

	require.NoError(t, Generate(fw, doc, WithModulePath(testModulePath)))
}

func TestGenerate_SimpleStruct(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      required: [id, name]
      properties:
        id: {type: integer, format: int64}
        name: {type: string}
        tag: {type: string, nullable: true}
`)
	files := generateFiles(t, doc)
	got := string(files["model/pet.gen.go"])
	assert.Contains(t, got, "package model")
	assert.Contains(t, got, "type Pet struct {")
	assert.True(t, containsCollapsed(got, "ID int64 `json:\"id\""))
	assert.True(t, containsCollapsed(got, "Name string `json:\"name\""))
	assert.True(t, containsCollapsed(got, "Tag *string `json:\"tag,omitempty\""))
}

func TestGenerate_Enum(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Status:
      type: string
      enum: [active, inactive, archived]
      default: active
`)
	files := generateFiles(t, doc)
	got := string(files["model/status.gen.go"])
	assert.Contains(t, got, "type Status string")
	assert.True(t, containsCollapsed(got, "StatusActive Status = \"active\""))
	assert.True(t, containsCollapsed(got, "StatusInactive Status = \"inactive\""))
	assert.True(t, containsCollapsed(got, "StatusArchived Status = \"archived\""))
}

func TestGenerate_IntegerEnum(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Priority:
      type: integer
      enum: [1, 2, 3]
`)
	files := generateFiles(t, doc)
	got := string(files["model/priority.gen.go"])
	assert.Contains(t, got, "type Priority int")
	assert.True(t, containsCollapsed(got, "Priority1 Priority = 1"))
}

func TestGenerate_ArraySchema(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    PetCollection:
      type: array
      items: {$ref: '#/components/schemas/Pet'}
    Pet:
      type: object
      properties: {name: {type: string}}
`)
	files := generateFiles(t, doc)
	got := string(files["model/pet_collection.gen.go"])
	assert.Contains(t, got, "type PetCollection []Pet")
}

func TestGenerate_OneOf(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet: {type: object, properties: {name: {type: string}}}
    Error: {type: object, properties: {code: {type: integer}}}
    Either:
      oneOf:
        - {$ref: '#/components/schemas/Pet'}
        - {$ref: '#/components/schemas/Error'}
`)
	files := generateFiles(t, doc)
	require.Contains(t, files, "model/either.gen.go")
	require.Contains(t, files, "model/either_json.gen.go")

	got := string(files["model/either.gen.go"])
	assert.Contains(t, got, "type Either struct {")
	assert.True(t, containsCollapsed(got, "Pet *Pet `json:\"-,inline\"`"))
	assert.True(t, containsCollapsed(got, "Error *Error `json:\"-,inline\"`"))

	jgot := string(files["model/either_json.gen.go"])
	assert.Contains(t, jgot, "func (m *Either) UnmarshalJSON(data []byte) error {")
	assert.Contains(t, jgot, "var v_0 Pet")
	assert.Contains(t, jgot, "m.Pet = &v_0")
	assert.Contains(t, jgot, "func (m Either) MarshalJSON() ([]byte, error) {")
}

func TestGenerate_AnyOf(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    A: {type: object, properties: {x: {type: string}}}
    B: {type: object, properties: {y: {type: integer}}}
    AB:
      anyOf:
        - {$ref: '#/components/schemas/A'}
        - {$ref: '#/components/schemas/B'}
`)
	files := generateFiles(t, doc)
	require.Contains(t, files, "model/ab_json.gen.go")
	assert.Contains(t, string(files["model/ab.gen.go"]), "type AB struct {")
}

func TestGenerate_AllOf(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Base:
      type: object
      required: [id]
      properties: {id: {type: integer}}
    Extended:
      allOf:
        - {$ref: '#/components/schemas/Base'}
        - type: object
          properties: {name: {type: string}}
`)
	files := generateFiles(t, doc)
	got := string(files["model/extended.gen.go"])
	assert.Contains(t, got, "type Extended struct {")
	assert.Contains(t, got, "Base")
	assert.True(t, containsCollapsed(got, "Name *string `json:\"name,omitempty\""))
}

func TestGenerate_DateTime(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Event:
      type: object
      properties: {at: {type: string, format: date-time}}
`)
	files := generateFiles(t, doc)
	got := string(files["model/event.gen.go"])
	assert.True(t, containsCollapsed(got, "At *time.Time"))
	assert.Contains(t, got, `"time"`)
}

func TestGenerate_MapObject(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Bag:
      type: object
      additionalProperties: {type: string}
`)
	files := generateFiles(t, doc)
	got := string(files["model/bag.gen.go"])
	assert.Contains(t, got, "type Bag map[string]string")
}

func TestGenerate_NestedArray(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Matrix:
      type: object
      properties:
        rows:
          type: array
          items:
            type: array
            items: {type: integer}
`)
	files := generateFiles(t, doc)
	got := string(files["model/matrix.gen.go"])
	assert.True(t, containsCollapsed(got, "Rows [][]int"))
}

func TestGenerate_RendersValidGo(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      required: [id, name]
      properties:
        id: {type: integer, format: int64}
        name: {type: string}
        tag: {type: string, nullable: true}
    Status:
      type: string
      enum: [active, inactive]
    Either:
      oneOf:
        - {$ref: '#/components/schemas/Pet'}
        - {$ref: '#/components/schemas/Status'}
`)
	files := generateFiles(t, doc)
	for name, content := range files {
		t.Run(name, func(t *testing.T) {
			fset := token.NewFileSet()
			_, err := parser.ParseFile(fset, name, content, parser.AllErrors)
			require.NoError(t, err, "generated file should parse as valid Go:\n%s", content)
		})
	}
}

func TestGenerate_EmptyDocument(t *testing.T) {
	doc := parseSpec(t, `openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
`)
	fw := &collectWriter{files: map[string][]byte{}}
	require.NoError(t, Generate(fw, doc, WithModulePath(testModulePath)))
	assert.Empty(t, fw.files)
}

func TestGenerate_WithModulePath(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Foo: {type: object, properties: {x: {type: string}}}
`)
	files := generateFiles(t, doc)
	assert.Contains(t, string(files["model/foo.gen.go"]), "package model")
}

func TestGenerate_ClientInterface(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /pets:
    get:
      operationId: listPets
      parameters:
        - {name: limit, in: query, schema: {type: integer}}
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema: {$ref: '#/components/schemas/Pets'}
        default:
          description: error
          content:
            application/json:
              schema: {$ref: '#/components/schemas/Error'}
    post:
      operationId: createPet
      requestBody:
        required: true
        content:
          application/json:
            schema: {$ref: '#/components/schemas/Pet'}
      responses:
        '201':
          description: created
          content:
            application/json:
              schema: {$ref: '#/components/schemas/Pet'}
components:
  schemas:
    Pet: {type: object, properties: {name: {type: string}}}
    Pets: {type: array, items: {$ref: '#/components/schemas/Pet'}}
    Error: {type: object, properties: {code: {type: integer}}}
`)
	files := generateFiles(t, doc)
	got := string(files["interfaces/client/client.gen.go"])
	assert.Contains(t, got, "package client")
	assert.Contains(t, got, "type Client interface {")
	assert.True(t, containsCollapsed(got, "ListPets(ctx context.Context, req *ListPetsRequest) (*ListPetsResponse, error)"))
	assert.True(t, containsCollapsed(got, "CreatePet(ctx context.Context, req *CreatePetRequest) (*CreatePetResponse, error)"))

	assert.Contains(t, got, "type ListPetsRequest struct {")
	assert.True(t, containsCollapsed(got, "Limit *int `query:\"limit\"`"))

	assert.Contains(t, got, "type CreatePetRequest struct {")
	assert.True(t, containsCollapsed(got, "Body model.Pet `json:\"-\"`"))

	assert.Contains(t, got, "type ListPetsResponse struct {")
	assert.True(t, containsCollapsed(got, "Code int"))
	assert.True(t, containsCollapsed(got, "Response200 *model.Pets"))
	assert.True(t, containsCollapsed(got, "ResponseDefault *model.Error"))
}

func TestGenerate_ClientInterface_NoOperationId(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /pets/{id}:
    get:
      parameters:
        - {name: id, in: path, required: true, schema: {type: string}}
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema: {$ref: '#/components/schemas/Pet'}
components:
  schemas:
    Pet: {type: object, properties: {name: {type: string}}}
`)
	files := generateFiles(t, doc)
	got := string(files["interfaces/client/client.gen.go"])
	assert.True(t, containsCollapsed(got, "GetPetsByID(ctx context.Context, req *GetPetsByIDRequest) (*GetPetsByIDResponse, error)"))
	assert.True(t, containsCollapsed(got, "ID string `param:\"id\"`"))
}

func TestGenerate_ClientSugar(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /pets:
    get:
      operationId: listPets
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema: {$ref: '#/components/schemas/Pets'}
  /pets/{id}:
    delete:
      operationId: deletePet
      parameters:
        - {name: id, in: path, required: true, schema: {type: string}}
      responses:
        '204':
          description: deleted
components:
  schemas:
    Pet: {type: object, properties: {name: {type: string}}}
    Pets: {type: array, items: {$ref: '#/components/schemas/Pet'}}
`)
	files := generateFiles(t, doc)
	got := string(files["interfaces/client/client_sugar.gen.go"])
	assert.Contains(t, got, "package client")
	assert.Contains(t, got, "type ClientSugared struct {")
	assert.Contains(t, got, "func NewClientSugared(impl Client) *ClientSugared {")
	assert.True(t, containsCollapsed(got, "func (x *ClientSugared) ListPets(ctx context.Context, req *ListPetsRequest) (*model.Pets, error) {"))
	assert.True(t, containsCollapsed(got, "func (x *ClientSugared) DeletePet(ctx context.Context, req *DeletePetRequest) error {"))
	assert.Contains(t, got, "resp, err := x.impl.ListPets(ctx, req)")
	assert.Contains(t, got, "return resp.Response200, nil")
	assert.Contains(t, got, "if resp.Response204 != nil {")
	assert.Contains(t, got, "return nil")
}

func TestGenerate_ClientNoOperations(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Foo: {type: object, properties: {x: {type: string}}}
`)
	files := generateFiles(t, doc)
	assert.NotContains(t, files, "interfaces/client/client.gen.go")
	assert.NotContains(t, files, "interfaces/client/client_sugar.gen.go")
	assert.NotContains(t, files, "interfaces/server/server.gen.go")
	assert.Contains(t, files, "model/foo.gen.go")
}

func TestGenerate_ServerInterface(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /pets:
    get:
      operationId: listPets
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema: {$ref: '#/components/schemas/Pets'}
  /pets/{id}:
    delete:
      operationId: deletePet
      parameters:
        - {name: id, in: path, required: true, schema: {type: string}}
      responses:
        '204': {description: deleted}
components:
  schemas:
    Pet: {type: object, properties: {name: {type: string}}}
    Pets: {type: array, items: {$ref: '#/components/schemas/Pet'}}
`)
	files := generateFiles(t, doc)
	got := string(files["interfaces/server/server.gen.go"])
	assert.Contains(t, got, "package server")
	assert.Contains(t, got, "type Server interface {")
	assert.True(t, containsCollapsed(got, "ListPets(ctx context.Context, req *client.ListPetsRequest) (*client.ListPetsResponse, error)"))
	assert.True(t, containsCollapsed(got, "DeletePet(ctx context.Context, req *client.DeletePetRequest) (*client.DeletePetResponse, error)"))
}

func TestGenerate_ImplClient(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /pets:
    get:
      operationId: listPets
      parameters:
        - {name: limit, in: query, schema: {type: integer}}
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema: {$ref: '#/components/schemas/Pets'}
        default:
          description: error
          content:
            application/json:
              schema: {$ref: '#/components/schemas/Error'}
    post:
      operationId: createPet
      requestBody:
        required: true
        content:
          application/json:
            schema: {$ref: '#/components/schemas/Pet'}
      responses:
        '201':
          description: created
          content:
            application/json:
              schema: {$ref: '#/components/schemas/Pet'}
  /pets/{id}:
    delete:
      operationId: deletePet
      parameters:
        - {name: id, in: path, required: true, schema: {type: string}}
      responses:
        '204': {description: deleted}
components:
  schemas:
    Pet: {type: object, properties: {name: {type: string}}}
    Pets: {type: array, items: {$ref: '#/components/schemas/Pet'}}
    Error: {type: object, properties: {code: {type: integer}}}
`)
	files := generateFiles(t, doc)
	got := string(files["impl/vanillahttp/client/client.gen.go"])
	assert.Contains(t, got, "package client")
	assert.Contains(t, got, "nschugorev/oapigenerator/pkg/httpclient")
	assert.Contains(t, got, "var _ apiclient.Client = (*Client)(nil)")
	assert.Contains(t, got, "type Client struct {")
	assert.Contains(t, got, "func NewClient(baseURL string, opts ...httpclient.Option) (*Client, error) {")
	assert.True(t, containsCollapsed(got, "func (c *Client) ListPets(ctx context.Context, req *apiclient.ListPetsRequest) (*apiclient.ListPetsResponse, error) {"))
	assert.Contains(t, got, "q.Set(\"limit\", fmt.Sprint(*req.Limit))")
	assert.Contains(t, got, "case 200:")
	assert.Contains(t, got, "result.Response200 = &v")
	assert.Contains(t, got, "result.ResponseDefault = &v")

	assert.True(t, containsCollapsed(got, "func (c *Client) CreatePet(ctx context.Context, req *apiclient.CreatePetRequest) (*apiclient.CreatePetResponse, error) {"))
	assert.Contains(t, got, "body, err := json.Marshal(req.Body)")
	assert.Contains(t, got, "bytes.NewReader(body)")
	assert.Contains(t, got, "httpReq.Header.Set(\"Content-Type\", \"application/json\")")

	assert.True(t, containsCollapsed(got, "func (c *Client) DeletePet(ctx context.Context, req *apiclient.DeletePetRequest) (*apiclient.DeletePetResponse, error) {"))
	assert.Contains(t, got, "strings.Replace(path, \"{id}\", url.PathEscape(fmt.Sprint(req.ID)), 1)")
	assert.Contains(t, got, "result.Response204 = true")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "client.gen.go", files["impl/vanillahttp/client/client.gen.go"], parser.AllErrors)
	require.NoError(t, err, "impl client should parse as valid Go")
}

// helpers

func TestGenerate_ImplServer(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /pets:
    get:
      operationId: listPets
      parameters:
        - {name: limit, in: query, schema: {type: integer}}
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema: {$ref: '#/components/schemas/Pets'}
        default:
          description: error
          content:
            application/json:
              schema: {$ref: '#/components/schemas/Error'}
    post:
      operationId: createPet
      requestBody:
        required: true
        content:
          application/json:
            schema: {$ref: '#/components/schemas/Pet'}
      responses:
        '201':
          description: created
          content:
            application/json:
              schema: {$ref: '#/components/schemas/Pet'}
  /pets/{id}:
    delete:
      operationId: deletePet
      parameters:
        - {name: id, in: path, required: true, schema: {type: string}}
      responses:
        '204': {description: deleted}
components:
  schemas:
    Pet: {type: object, properties: {name: {type: string}}}
    Pets: {type: array, items: {$ref: '#/components/schemas/Pet'}}
    Error: {type: object, properties: {code: {type: integer}}}
`)
	files := generateFiles(t, doc)
	got := string(files["impl/vanillahttp/server/server.gen.go"])
	assert.Contains(t, got, "package server")
	assert.Contains(t, got, "github.com/labstack/echo/v4")
	assert.Contains(t, got, "type ServerHTTP struct {")
	assert.Contains(t, got, "func NewServerHTTP(impl apiserver.Server) *ServerHTTP {")
	assert.Contains(t, got, "func (s *ServerHTTP) Register(e *echo.Echo) {")
	assert.Contains(t, got, `e.GET("/pets", s.listPets)`)
	assert.Contains(t, got, `e.POST("/pets", s.createPet)`)
	assert.Contains(t, got, `e.DELETE("/pets/:id", s.deletePet)`)

	assert.Contains(t, got, "func (s *ServerHTTP) listPets(c echo.Context) error {")
	assert.Contains(t, got, "if err := c.Bind(req); err != nil {")
	assert.Contains(t, got, "resp, err := s.impl.ListPets(c.Request().Context(), req)")
	assert.Contains(t, got, "return c.JSON(200, resp.Response200)")
	assert.Contains(t, got, "return c.JSON(resp.Code, resp.ResponseDefault)")

	assert.Contains(t, got, "func (s *ServerHTTP) createPet(c echo.Context) error {")
	assert.Contains(t, got, "if err := bindBody(c, &req.Body); err != nil {")
	assert.Contains(t, got, "func bindBody(c echo.Context, dst any) error {")
	assert.Contains(t, got, "return c.JSON(201, resp.Response201)")

	assert.Contains(t, got, "func (s *ServerHTTP) deletePet(c echo.Context) error {")
	assert.Contains(t, got, "if resp.Response204 {")
	assert.Contains(t, got, "return c.NoContent(204)")

	assert.Contains(t, got, "return c.NoContent(resp.Code)")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "server.gen.go", files["impl/vanillahttp/server/server.gen.go"], parser.AllErrors)
	require.NoError(t, err, "impl server should parse as valid Go")
}

func TestGenerate_Mocks(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /pets:
    get:
      operationId: listPets
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema: {$ref: '#/components/schemas/Pets'}
  /pets/{id}:
    delete:
      operationId: deletePet
      parameters:
        - {name: id, in: path, required: true, schema: {type: string}}
      responses:
        '204': {description: deleted}
components:
  schemas:
    Pet: {type: object, properties: {name: {type: string}}}
    Pets: {type: array, items: {$ref: '#/components/schemas/Pet'}}
`)
	files := generateFiles(t, doc)

	clientGot := string(files["impl/mocks/client/mocks.gen.go"])
	assert.Contains(t, clientGot, "package mock_client")
	assert.Contains(t, clientGot, "var _ apiclient.Client = (*MockClient)(nil)")
	assert.Contains(t, clientGot, "type MockClient struct {")
	assert.Contains(t, clientGot, "ctrl     *gomock.Controller")
	assert.Contains(t, clientGot, "recorder *MockClientMockRecorder")
	assert.Contains(t, clientGot, "func NewMockClient(ctrl *gomock.Controller) *MockClient {")
	assert.Contains(t, clientGot, "func (m *MockClient) EXPECT() *MockClientMockRecorder {")
	assert.True(t, containsCollapsed(clientGot, "func (m *MockClient) ListPets(arg0 context.Context, arg1 *apiclient.ListPetsRequest) (*apiclient.ListPetsResponse, error) {"))
	assert.Contains(t, clientGot, `ret := m.ctrl.Call(m, "ListPets", arg0, arg1)`)
	assert.Contains(t, clientGot, "func (mr *MockClientMockRecorder) ListPets(arg0, arg1 any) *gomock.Call {")
	assert.Contains(t, clientGot, `mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListPets", reflect.TypeOf((*MockClient)(nil).ListPets), arg0, arg1)`)

	serverGot := string(files["impl/mocks/server/mocks.gen.go"])
	assert.Contains(t, serverGot, "package mock_server")
	assert.Contains(t, serverGot, "var _ apiserver.Server = (*MockServer)(nil)")
	assert.Contains(t, serverGot, "type MockServer struct {")
	assert.True(t, containsCollapsed(serverGot, "func (m *MockServer) DeletePet(arg0 context.Context, arg1 *apiclient.DeletePetRequest) (*apiclient.DeletePetResponse, error) {"))
	assert.Contains(t, serverGot, "func (mr *MockServerMockRecorder) DeletePet(arg0, arg1 any) *gomock.Call {")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "client_mocks.gen.go", files["impl/mocks/client/mocks.gen.go"], parser.AllErrors)
	require.NoError(t, err, "client mocks should parse as valid Go")
	_, err = parser.ParseFile(fset, "server_mocks.gen.go", files["impl/mocks/server/mocks.gen.go"], parser.AllErrors)
	require.NoError(t, err, "server mocks should parse as valid Go")
}

func TestGenerate_SDK(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /pets:
    get:
      operationId: listPets
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema: {$ref: '#/components/schemas/Pets'}
  /pets/{id}:
    delete:
      operationId: deletePet
      parameters:
        - {name: id, in: path, required: true, schema: {type: string}}
      responses:
        '204': {description: deleted}
components:
  schemas:
    Pet: {type: object, properties: {name: {type: string}}}
    Pets: {type: array, items: {$ref: '#/components/schemas/Pet'}}
`)
	files := generateFiles(t, doc)
	got := string(files["sdk/sdk.gen.go"])
	assert.Contains(t, got, "package sdk")
	assert.Contains(t, got, "type SDK struct {")
	assert.Contains(t, got, "apiclient.Client")
	assert.Contains(t, got, "func NewSDK(baseURL string, opts ...httpclient.Option) (*SDK, error) {")
	assert.Contains(t, got, "c, err := implclient.NewClient(baseURL, opts...)")
	assert.Contains(t, got, `return nil, fmt.Errorf("init sdk client: %w", err)`)
	assert.Contains(t, got, "return &SDK{Client: c}, nil")
	assert.Contains(t, got, "func NewSDKFromClient(c apiclient.Client) *SDK {")
	assert.Contains(t, got, "return &SDK{Client: c}")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "sdk.gen.go", files["sdk/sdk.gen.go"], parser.AllErrors)
	require.NoError(t, err, "sdk should parse as valid Go")
}

type collectWriter struct {
	files map[string][]byte
}

func (c *collectWriter) WriteFile(name string, f codegen.File) error {
	c.files[name] = f.Content()
	return nil
}

func (c *collectWriter) Close() error { return nil }

func parseSpec(t *testing.T, spec string) *oapiparser.Document {
	t.Helper()
	doc, err := oapiparser.Parse([]byte(spec))
	require.NoError(t, err)
	return doc
}

func generateFiles(t *testing.T, doc *oapiparser.Document) map[string][]byte {
	t.Helper()
	fw := &collectWriter{files: map[string][]byte{}}
	require.NoError(t, Generate(fw, doc, WithModulePath(testModulePath)))
	return fw.files
}

func mustReadFile(t *testing.T, p string) []byte {
	t.Helper()
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	return data
}
