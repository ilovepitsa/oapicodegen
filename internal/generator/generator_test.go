package generator

import (
	"go/parser"
	"go/token"
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/golden"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	assert.True(t, containsCollapsed(got, "Pet *Pet `json:\"-\"`"))
	assert.True(t, containsCollapsed(got, "Error *Error `json:\"-\"`"))

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

func TestGenerate_DateTime_UTCTimeFlag(t *testing.T) {
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
	pf := oapiparser.ProjectFeatures{UseUTCForDateTime: oapiparser.ProjectFeature{Value: true}}
	files := generateFilesWithFeatures(t, doc, pf)

	got := string(files["model/event.gen.go"])
	assert.True(t, containsCollapsed(got, "At *UTCTime"))
	assert.NotContains(t, got, `"time"`)

	utcFile, ok := files["model/utc_time.gen.go"]
	require.True(t, ok, "expected model/utc_time.gen.go to be generated")
	utc := string(utcFile)
	assert.Contains(t, utc, "type UTCTime time.Time")
	assert.Contains(t, utc, "func (u UTCTime) MarshalJSON")
	assert.Contains(t, utc, "func (u *UTCTime) UnmarshalJSON")
	assert.Contains(t, utc, ".UTC()")
}

func TestGenerate_OptionalFields_FlagOn(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      required: [id]
      properties:
        id: {type: integer, format: int64}
        name: {type: string, x-optional: true}
        tag: {type: string, x-optional: true}
        retries: {type: integer, format: int32, x-optional: true}
`)
	pf := oapiparser.ProjectFeatures{UseOptional: oapiparser.ProjectFeature{Value: true}}
	files := generateFilesWithFeatures(t, doc, pf)

	got := string(files["model/pet.gen.go"])
	assert.True(t, containsCollapsed(got, "Name optional.Optional[string]"))
	assert.True(t, containsCollapsed(got, "Tag optional.Optional[string]"))
	assert.True(t, containsCollapsed(got, "Retries optional.Optional[int32]"))
	// required поле не оборачивается в Optional.
	assert.True(t, containsCollapsed(got, "ID int64"))
	// import optional-пакета добавлен.
	assert.Contains(t, got, `optional "nschugorev/oapigenerator/pkg/optional"`)
	// x-optional поля без omitempty (struct value — omitempty no-op).
	assert.True(t, containsCollapsed(got, `json:"name"`))
	assert.True(t, containsCollapsed(got, `json:"tag"`))

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "pet.gen.go", files["model/pet.gen.go"], parser.AllErrors)
	require.NoError(t, err, "generated file should parse as valid Go")
}

func TestGenerate_OptionalFields_FlagOff_NoOptionalType(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      required: [id]
      properties:
        id: {type: integer, format: int64}
        name: {type: string, x-optional: true}
`)
	// Флаг выключен — x-optional игнорируется, поле рендерится как *T.
	files := generateFiles(t, doc)
	got := string(files["model/pet.gen.go"])

	assert.NotContains(t, got, "optional.Optional")
	assert.NotContains(t, got, "nschugorev/oapigenerator/pkg/optional")
	assert.True(t, containsCollapsed(got, "Name *string"))
}

func TestGenerate_OptionalFields_NoXOptional_NoEffect(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      required: [id]
      properties:
        id: {type: integer, format: int64}
        name: {type: string}
`)
	pf := oapiparser.ProjectFeatures{UseOptional: oapiparser.ProjectFeature{Value: true}}
	files := generateFilesWithFeatures(t, doc, pf)
	got := string(files["model/pet.gen.go"])

	// Флаг включён, но x-optional нет — обычная *T-семантика.
	assert.NotContains(t, got, "optional.Optional")
	assert.True(t, containsCollapsed(got, "Name *string"))
}

func TestGenerate_DateTime_UTCTimeFlagOff_NoUTCTimeFile(t *testing.T) {
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
	_, ok := files["model/utc_time.gen.go"]
	assert.False(t, ok, "utc_time.gen.go should not be generated when flag is off")
}

func TestGenerate_Date_NotAffectedByUTCTimeFlag(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Event:
      type: object
      properties: {on: {type: string, format: date}}
`)
	pf := oapiparser.ProjectFeatures{UseUTCForDateTime: oapiparser.ProjectFeature{Value: true}}
	files := generateFilesWithFeatures(t, doc, pf)

	got := string(files["model/event.gen.go"])
	assert.True(t, containsCollapsed(got, "On *time.Time"))
	assert.Contains(t, got, `"time"`)
}

func TestGenerate_SplitRequestResponse(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      required: [name]
      properties:
        id: {type: integer, format: int64, readOnly: true}
        name: {type: string}
        secret: {type: string, writeOnly: true}
`)
	pf := oapiparser.ProjectFeatures{
		SplitRequestResponse: oapiparser.ProjectFeature{Value: true},
	}
	files := generateFilesWithFeatures(t, doc, pf)
	got := string(files["model/pet.gen.go"])

	assert.Contains(t, got, "type PetRequest struct {")
	assert.Contains(t, got, "type PetResponse struct {")
	assert.NotContains(t, got, "type Pet struct {")

	// Request: no readOnly (id), has writeOnly (secret) + regular (name)
	reqSection := extractStruct(got, "PetRequest")
	assert.Contains(t, reqSection, "Name")
	assert.Contains(t, reqSection, "Secret")
	assert.NotContains(t, reqSection, "ID")

	// Response: no writeOnly (secret), has readOnly (id) + regular (name)
	respSection := extractStruct(got, "PetResponse")
	assert.Contains(t, respSection, "ID")
	assert.Contains(t, respSection, "Name")
	assert.NotContains(t, respSection, "Secret")
}

func TestGenerate_SplitFlagOff_SingleStruct(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      properties:
        id: {type: integer, readOnly: true}
        name: {type: string}
`)
	files := generateFiles(t, doc)
	got := string(files["model/pet.gen.go"])

	assert.Contains(t, got, "type Pet struct {")
	assert.NotContains(t, got, "PetRequest")
	assert.NotContains(t, got, "PetResponse")
}

func TestGenerate_Split_NestedSchemaRef(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      properties:
        id: {type: integer, readOnly: true}
        name: {type: string}
    Owner:
      type: object
      properties:
        pet: {$ref: '#/components/schemas/Pet'}
`)
	pf := oapiparser.ProjectFeatures{
		SplitRequestResponse: oapiparser.ProjectFeature{Value: true},
	}
	files := generateFilesWithFeatures(t, doc, pf)

	ownerGot := string(files["model/owner.gen.go"])
	// In OwnerRequest, pet field should be *PetRequest
	reqSection := extractStruct(ownerGot, "OwnerRequest")
	assert.True(t, containsCollapsed(reqSection, "Pet *PetRequest"))
	// In OwnerResponse, pet field should be *PetResponse
	respSection := extractStruct(ownerGot, "OwnerResponse")
	assert.True(t, containsCollapsed(respSection, "Pet *PetResponse"))
}

func extractStruct(source, structName string) string {
	startMarker := "type " + structName + " struct {"
	idx := strings.Index(source, startMarker)
	if idx < 0 {
		return ""
	}

	end := strings.Index(source[idx:], "\n}\n")
	if end < 0 {
		return ""
	}

	return source[idx : idx+end]
}

func TestGenerate_Split_OneOfRefExcludesFromSplit(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      properties:
        id: {type: integer, readOnly: true}
        name: {type: string}
    PetKind:
      oneOf:
        - {$ref: '#/components/schemas/Pet'}
`)
	pf := oapiparser.ProjectFeatures{
		SplitRequestResponse: oapiparser.ProjectFeature{Value: true},
	}
	files := generateFilesWithFeatures(t, doc, pf)

	petGot := string(files["model/pet.gen.go"])
	assert.Contains(t, petGot, "type Pet struct {", "Pet must NOT be split when referenced by oneOf")
	assert.NotContains(t, petGot, "type PetRequest struct {")
	assert.NotContains(t, petGot, "type PetResponse struct {")

	kindGot := string(files["model/pet_kind.gen.go"])
	assert.Contains(t, kindGot, "type PetKind struct {")
	assert.Contains(t, kindGot, "Pet *Pet")
}

func TestGenerate_Split_AllOfRefExcludesFromSplit(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      properties:
        id: {type: integer, readOnly: true}
        name: {type: string}
    Composite:
      allOf:
        - {$ref: '#/components/schemas/Pet'}
        - type: object
          properties:
            extra: {type: string}
`)
	pf := oapiparser.ProjectFeatures{
		SplitRequestResponse: oapiparser.ProjectFeature{Value: true},
	}
	files := generateFilesWithFeatures(t, doc, pf)

	petGot := string(files["model/pet.gen.go"])
	assert.Contains(t, petGot, "type Pet struct {", "Pet must NOT be split when referenced by allOf")
	assert.NotContains(t, petGot, "type PetRequest struct {")

	compositeGot := string(files["model/composite.gen.go"])
	assert.Contains(t, compositeGot, "type Composite struct {")
}

func TestGenerate_Split_ItemsRefExcludesFromSplit(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      properties:
        id: {type: integer, readOnly: true}
        name: {type: string}
    PetList:
      type: array
      items:
        $ref: '#/components/schemas/Pet'
`)
	pf := oapiparser.ProjectFeatures{
		SplitRequestResponse: oapiparser.ProjectFeature{Value: true},
	}
	files := generateFilesWithFeatures(t, doc, pf)

	petGot := string(files["model/pet.gen.go"])
	assert.Contains(t, petGot, "type Pet struct {", "Pet must NOT be split when used as array items")
	assert.NotContains(t, petGot, "type PetRequest struct {")

	listGot := string(files["model/pet_list.gen.go"])
	assert.Contains(t, listGot, "type PetList []Pet")
}

func TestGenerate_Split_OperationBodyAndResponseUseSuffix(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /pets:
    post:
      operationId: createPet
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Pet'
      responses:
        '201':
          description: created
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Pet'
components:
  schemas:
    Pet:
      type: object
      properties:
        id: {type: integer, readOnly: true}
        name: {type: string, writeOnly: true}
`)
	pf := oapiparser.ProjectFeatures{
		SplitRequestResponse: oapiparser.ProjectFeature{Value: true},
	}
	files := generateFilesWithFeatures(t, doc, pf)

	clientGot := string(files["interfaces/client/client.gen.go"])
	assert.Contains(t, clientGot, "Body model.PetRequest", "request body must use PetRequest")
	assert.Contains(t, clientGot, "*model.PetResponse", "response must use PetResponse")
	assert.NotContains(t, clientGot, "model.Pet ", "no bare Pet type reference")
	assert.NotContains(t, clientGot, "model.Pet)")
	assert.NotContains(t, clientGot, "model.Pet`")
}

func TestGenerate_Split_SugarAndImplUseResponseSuffix(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /pets/{id}:
    get:
      operationId: getPet
      parameters:
        - name: id
          in: path
          required: true
          schema: {type: integer}
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Pet'
components:
  schemas:
    Pet:
      type: object
      properties:
        id: {type: integer, readOnly: true}
        name: {type: string}
`)
	pf := oapiparser.ProjectFeatures{
		SplitRequestResponse: oapiparser.ProjectFeature{Value: true},
	}
	files := generateFilesWithFeatures(t, doc, pf)

	sugarGot := string(files["interfaces/client/client_sugar.gen.go"])
	assert.Contains(t, sugarGot, "*model.PetResponse", "sugar return type must use PetResponse")

	implGot := string(files["impl/httpclient/client.gen.go"])
	assert.Contains(t, implGot, "var v model.PetResponse", "impl decode target must use PetResponse")
}

func TestGenerate_Split_EmptyAfterFilter(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    WriteOnly:
      type: object
      properties:
        secret: {type: string, writeOnly: true}
`)
	pf := oapiparser.ProjectFeatures{
		SplitRequestResponse: oapiparser.ProjectFeature{Value: true},
	}
	files := generateFilesWithFeatures(t, doc, pf)
	got := string(files["model/write_only.gen.go"])

	assert.Contains(t, got, "type WriteOnlyRequest struct {")
	assert.Contains(t, got, "type WriteOnlyResponse struct {")
	// Response keeps `secret` (writeOnly allowed in response-filter? no: response excludes writeOnly)
	// Request keeps `secret` (writeOnly is allowed in request)
	reqSection := extractStruct(got, "WriteOnlyRequest")
	assert.Contains(t, reqSection, "Secret", "Request must include writeOnly field")
	respSection := extractStruct(got, "WriteOnlyResponse")
	assert.NotContains(t, respSection, "Secret", "Response must exclude writeOnly field")
}

func TestGenerate_AllOfSinglePrimitiveRendersAlias(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    StringWrapper:
      allOf:
        - type: string
`)
	files := generateFiles(t, doc)
	got := string(files["model/string_wrapper.gen.go"])
	assert.Contains(t, got, "type StringWrapper string")
}

func TestGenerate_AdditionalPropertiesFalse(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Closed:
      type: object
      additionalProperties: false
      properties:
        name: {type: string}
    Empty:
      type: object
      additionalProperties: false
`)
	files := generateFiles(t, doc)

	closedGot := string(files["model/closed.gen.go"])
	assert.Contains(t, closedGot, "type Closed struct {")
	assert.NotContains(t, closedGot, "map[string]", "closed struct must not be a map")

	emptyGot := string(files["model/empty.gen.go"])
	assert.Contains(t, emptyGot, "type Empty struct{}")
	assert.NotContains(t, emptyGot, "map[string]")
}

func TestGenerate_ResponseHeaders(t *testing.T) {
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
          headers:
            X-Total-Count:
              schema: {type: integer}
          content:
            application/json:
              schema:
                type: array
                items: {type: string}
components:
  schemas: {}
`)
	files := generateFiles(t, doc)

	clientGot := string(files["interfaces/client/client.gen.go"])
	assert.True(t, containsCollapsed(clientGot, "Response200 *ListPetsResponse200PayloadWithHeaders"))
	assert.True(t, containsCollapsed(clientGot, "Payload *[]string"))
	assert.True(t, containsCollapsed(clientGot, "XTotalCount int"))
	assert.Contains(t, clientGot, "func (m ListPetsResponse200PayloadWithHeaders) MarshalJSON() ([]byte, error) {")
	assert.True(t, containsCollapsed(clientGot,
		`func (m ListPetsResponse200PayloadWithHeaders) Headers() map[string]string {`))
	assert.True(t, containsCollapsed(clientGot, `"X-Total-Count": fmt.Sprintf("%v", m.XTotalCount)`))
	assert.Contains(t, clientGot, "\"encoding/json\"")
	assert.Contains(t, clientGot, "\"fmt\"")

	implGot := string(files["impl/httpclient/client.gen.go"])
	assert.Contains(t, implGot, "result.Response200 = &ListPetsResponse200PayloadWithHeaders{}")
	assert.Contains(t, implGot, "result.Response200.Payload = &v")
	assert.Contains(t, implGot, `strconv.Atoi(resp.Header.Get("X-Total-Count"))`)
	assert.Contains(t, implGot, "result.Response200.XTotalCount = raw")

	serverGot := string(files["impl/echoserver/server.gen.go"])
	assert.Contains(t, serverGot, "if resp.Response200 != nil {")
	assert.Contains(t, serverGot, "for k, v := range resp.Response200.Headers() {")
	assert.Contains(t, serverGot, "c.Response().Header().Set(k, v)")
	assert.Contains(t, serverGot, "return c.JSON(200, resp.Response200)")
}

func TestGenerate_SugarFallbackToDefault(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /pets/{id}:
    delete:
      operationId: deletePet
      parameters:
        - {name: id, in: path, required: true, schema: {type: integer}}
      responses:
        default:
          description: error
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Error'
components:
  schemas:
    Error: {type: object, properties: {code: {type: integer}}}
`)
	files := generateFiles(t, doc)
	got := string(files["interfaces/client/client_sugar.gen.go"])
	assert.Contains(t, got, "*model.Error, error)", "sugar must fall back to default schema")
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

func TestGenerate_SetDefaults(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Config:
      type: object
      required: [name]
      properties:
        name: {type: string, default: fallback}
        retries: {type: integer, format: int32, default: 3}
        enabled: {type: boolean, default: true}
        ratio: {type: number, format: float, default: 0.5}
        withoutDefault: {type: string}
`)
	files := generateFiles(t, doc)
	got := string(files["model/config.gen.go"])

	assert.Contains(t, got, "func (m *Config) SetDefaults() {")
	// required string default — zero-value check `== ""`
	assert.True(t, containsCollapsed(got, `if m.Name == "" {`))
	assert.True(t, containsCollapsed(got, `m.Name = "fallback"`))
	// optional int32 default — nil check (pointer) + `v := <literal>; m.Field = &v` pattern
	assert.True(t, containsCollapsed(got, "if m.Retries == nil {"))
	assert.True(t, containsCollapsed(got, "v := int32(3)"))
	assert.True(t, containsCollapsed(got, "m.Retries = &v"))
	// optional bool default — nil check
	assert.True(t, containsCollapsed(got, "if m.Enabled == nil {"))
	assert.True(t, containsCollapsed(got, "v := true"))
	assert.True(t, containsCollapsed(got, "m.Enabled = &v"))
	// optional float32 default — nil check + float32 cast
	assert.True(t, containsCollapsed(got, "if m.Ratio == nil {"))
	assert.True(t, containsCollapsed(got, "v := float32(0.5)"))
	assert.True(t, containsCollapsed(got, "m.Ratio = &v"))
	// field without default must not appear in SetDefaults
	setDefaultsBody := extractFunc(got, "Config) SetDefaults")
	assert.NotContains(t, setDefaultsBody, "WithoutDefault")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "config.gen.go", files["model/config.gen.go"], parser.AllErrors)
	require.NoError(t, err, "generated file should parse as valid Go")
}

func TestGenerate_SetDefaults_EnumField(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Status:
      type: string
      enum: [active, inactive]
      default: active
    Item:
      type: object
      properties:
        status: {$ref: '#/components/schemas/Status'}
`)
	files := generateFiles(t, doc)
	itemGot := string(files["model/item.gen.go"])
	assert.Contains(t, itemGot, "func (m *Item) SetDefaults() {")
	assert.True(t, containsCollapsed(itemGot, "if m.Status == nil {"))
	assert.True(t, containsCollapsed(itemGot, "v := StatusActive"))
	assert.True(t, containsCollapsed(itemGot, "m.Status = &v"))

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "item.gen.go", files["model/item.gen.go"], parser.AllErrors)
	require.NoError(t, err)
}

func TestGenerate_SetDefaults_RequiredInt64(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Settings:
      type: object
      required: [timeout]
      properties:
        timeout: {type: integer, format: int64, default: 30}
`)
	files := generateFiles(t, doc)
	got := string(files["model/settings.gen.go"])
	assert.Contains(t, got, "func (m *Settings) SetDefaults() {")
	// required int64 — zero-value check `== 0`, no nil
	assert.True(t, containsCollapsed(got, "if m.Timeout == 0 {"))
	assert.True(t, containsCollapsed(got, "m.Timeout = int64(30)"))
}

func TestGenerate_SetDefaults_NoMethodWhenNoDefaults(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Plain:
      type: object
      properties:
        name: {type: string}
`)
	files := generateFiles(t, doc)
	got := string(files["model/plain.gen.go"])
	assert.NotContains(t, got, "SetDefaults")
}

func TestGenerate_SetDefaults_ServerAutoDefaultsFlag(t *testing.T) {
	spec := `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /config:
    post:
      operationId: createConfig
      requestBody:
        required: true
        content:
          application/json:
            schema: {$ref: '#/components/schemas/Config'}
      responses:
        '201': {description: created}
components:
  schemas:
    Config:
      type: object
      properties:
        name: {type: string, default: fallback}
`

	t.Run("ServerNoAutoDefaults=false calls SetDefaults", func(t *testing.T) {
		doc := parseSpec(t, spec)
		pf := oapiparser.ProjectFeatures{} // ServerNoAutoDefaults=false
		files := generateFilesWithFeatures(t, doc, pf)
		got := string(files["impl/echoserver/server.gen.go"])
		assert.Contains(t, got, "req.Body.SetDefaults()")
	})

	t.Run("ServerNoAutoDefaults=true skips SetDefaults", func(t *testing.T) {
		doc := parseSpec(t, spec)
		pf := oapiparser.ProjectFeatures{
			ServerNoAutoDefaults: oapiparser.ProjectFeature{Value: true},
		}
		files := generateFilesWithFeatures(t, doc, pf)
		got := string(files["impl/echoserver/server.gen.go"])
		assert.NotContains(t, got, "SetDefaults")
	})
}

func TestGenerate_SetDefaults_SplitRequestVariant(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      properties:
        id: {type: integer, format: int64, default: 42, readOnly: true}
        name: {type: string, default: unnamed}
`)
	pf := oapiparser.ProjectFeatures{
		SplitRequestResponse: oapiparser.ProjectFeature{Value: true},
	}
	files := generateFilesWithFeatures(t, doc, pf)
	got := string(files["model/pet.gen.go"])

	// PetRequest: id (readOnly) excluded, name kept → SetDefaults for name only.
	// name is optional (not in required) → pointer → `v := ...; m.Field = &v` pattern.
	assert.Contains(t, got, "func (m *PetRequest) SetDefaults() {")
	reqSection := extractFunc(got, "PetRequest) SetDefaults")
	assert.Contains(t, reqSection, `v := "unnamed"`)
	assert.Contains(t, reqSection, "m.Name = &v")
	assert.NotContains(t, reqSection, "ID")

	// PetResponse: both id and name kept → SetDefaults for both.
	// Both optional (not in required) → pointer → `v := ...; m.Field = &v` pattern.
	assert.Contains(t, got, "func (m *PetResponse) SetDefaults() {")
	respSection := extractFunc(got, "PetResponse) SetDefaults")
	assert.Contains(t, respSection, "v := int64(42)")
	assert.Contains(t, respSection, "m.ID = &v")
	assert.Contains(t, respSection, `v := "unnamed"`)
	assert.Contains(t, respSection, "m.Name = &v")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "pet.gen.go", files["model/pet.gen.go"], parser.AllErrors)
	require.NoError(t, err)
}

func TestGenerate_SetDefaults_RequiredEnumInt(t *testing.T) {
	// B1: required enum-поле с non-string base type (Code int32) не должно
	// генерировать `if m.Code == ""` — zero-value должен быть `0` по schema type.
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Code:
      type: integer
      format: int32
      enum: [1, 2, 3]
      default: 2
    Item:
      type: object
      required: [code]
      properties:
        code: {$ref: '#/components/schemas/Code'}
`)
	files := generateFiles(t, doc)
	got := string(files["model/item.gen.go"])

	assert.Contains(t, got, "func (m *Item) SetDefaults() {")
	// required integer-enum: zero-value `0` (untyped int constant совместим с Code).
	assert.True(t, containsCollapsed(got, "if m.Code == 0 {"))
	assert.True(t, containsCollapsed(got, "m.Code = Code2"))

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "item.gen.go", files["model/item.gen.go"], parser.AllErrors)
	require.NoError(t, err)
}

func TestGenerate_SetDefaults_RequiredEnumString(t *testing.T) {
	// B1 (string-enum): required string-enum — zero-value `""`.
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Status:
      type: string
      enum: [active, inactive]
      default: active
    Item:
      type: object
      required: [status]
      properties:
        status: {$ref: '#/components/schemas/Status'}
`)
	files := generateFiles(t, doc)
	got := string(files["model/item.gen.go"])

	assert.Contains(t, got, "func (m *Item) SetDefaults() {")
	assert.True(t, containsCollapsed(got, `if m.Status == "" {`))
	assert.True(t, containsCollapsed(got, "m.Status = StatusActive"))
}

func TestGenerate_SetDefaults_DateTimeSkipped(t *testing.T) {
	// B2: required date-time поле с default — SetDefaults-ветка НЕ генерируется
	// (присваивание строкового литерала time.Time не компилируется).
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Event:
      type: object
      required: [at]
      properties:
        at: {type: string, format: date-time, default: '2024-01-01T00:00:00Z'}
        name: {type: string, default: unnamed}
`)
	files := generateFiles(t, doc)
	got := string(files["model/event.gen.go"])

	// SetDefaults всё ещё генерируется (есть name с default).
	assert.Contains(t, got, "func (m *Event) SetDefaults() {")
	setDefaultsBody := extractFunc(got, "Event) SetDefaults")
	// date-time поле НЕ должно появляться в SetDefaults.
	assert.NotContains(t, setDefaultsBody, "m.At")
	// name с default должен присутствовать (optional → pointer → v-then-and-v pattern).
	assert.Contains(t, setDefaultsBody, `v := "unnamed"`)
	assert.Contains(t, setDefaultsBody, "m.Name = &v")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "event.gen.go", files["model/event.gen.go"], parser.AllErrors)
	require.NoError(t, err)
}

func TestGenerate_SetDefaults_NestedObjectRef(t *testing.T) {
	// M3: Outer.inner — $ref на Inner с defaults. Outer.SetDefaults()
	// должен вызывать m.Inner.SetDefaults().
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Inner:
      type: object
      properties:
        level: {type: integer, default: 5}
    Outer:
      type: object
      required: [inner]
      properties:
        inner: {$ref: '#/components/schemas/Inner'}
`)
	files := generateFiles(t, doc)
	got := string(files["model/outer.gen.go"])

	assert.Contains(t, got, "func (m *Outer) SetDefaults() {")
	// required nested object: прямой вызов без nil-check.
	assert.True(t, containsCollapsed(got, "m.Inner.SetDefaults()"))

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "outer.gen.go", files["model/outer.gen.go"], parser.AllErrors)
	require.NoError(t, err)
}

func TestGenerate_SetDefaults_NestedOptionalObjectRef(t *testing.T) {
	// M3 (optional nested): Outer.inner optional (pointer) — вызов под nil-check.
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Inner:
      type: object
      properties:
        level: {type: integer, default: 5}
    Outer:
      type: object
      properties:
        inner: {$ref: '#/components/schemas/Inner'}
`)
	files := generateFiles(t, doc)
	got := string(files["model/outer.gen.go"])

	assert.Contains(t, got, "func (m *Outer) SetDefaults() {")
	// optional nested object: nil-check перед вызовом.
	assert.True(t, containsCollapsed(got, "if m.Inner != nil {"))
	assert.True(t, containsCollapsed(got, "m.Inner.SetDefaults()"))

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "outer.gen.go", files["model/outer.gen.go"], parser.AllErrors)
	require.NoError(t, err)
}

func TestGenerate_SetDefaults_NestedObjectNoDefaultsSkipped(t *testing.T) {
	// M3: если nested object не имеет defaults (ни прямых, ни вложенных),
	// Outer.SetDefaults не должен генерировать пустой вызов.
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Inner:
      type: object
      properties:
        level: {type: integer}
    Outer:
      type: object
      properties:
        inner: {$ref: '#/components/schemas/Inner'}
`)
	files := generateFiles(t, doc)
	got := string(files["model/outer.gen.go"])
	// Outer сам не имеет defaults и Inner тоже — SetDefaults не нужен.
	assert.NotContains(t, got, "SetDefaults")
}

func TestSchemaTreeHasDefaults_CyclicRefTerminates(t *testing.T) {
	// M3 (cyclic, unit): schemaTreeHasDefaults должен завершиться на циклических
	// $ref (A → B → A) благодаря visited-set. Парсер libopenapi с
	// ExtractRefsSequentially не может распарсить циклические spec, поэтому
	// тестируем логику напрямую, минуя парсинг.
	inner := &oapiparser.Schema{
		Name: "Inner",
		Type: oapiTypeObject,
		Properties: []*oapiparser.Property{
			{Name: "level", Schema: &oapiparser.Schema{Type: oapiTypeInteger, Default: 5}},
		},
	}
	// A.a → A (self-ref via $ref на себя же).
	aSchema := &oapiparser.Schema{
		Name: "A",
		Type: oapiTypeObject,
		Properties: []*oapiparser.Property{
			{Name: "inner", Schema: &oapiparser.Schema{Ref: "#/components/schemas/Inner"}},
		},
	}
	g := &Generator{doc: &oapiparser.Document{Schemas: []*oapiparser.Schema{inner, aSchema}}}

	// Цикл: aSchema → inner ($ref Inner) → Inner.level (default). Должно вернуться true.
	assert.True(t, g.schemaTreeHasDefaults(aSchema, nil, map[string]bool{aSchema.Name: true}))

	// Без defaults — visited-set предотвращает бесконечную рекурсию.
	emptyA := &oapiparser.Schema{Name: "EmptyA", Type: oapiTypeObject}
	emptyB := &oapiparser.Schema{Name: "EmptyB", Type: oapiTypeObject}
	emptyA.Properties = []*oapiparser.Property{
		{Name: "b", Schema: &oapiparser.Schema{Ref: "#/components/schemas/EmptyB"}},
	}
	emptyB.Properties = []*oapiparser.Property{
		{Name: "a", Schema: &oapiparser.Schema{Ref: "#/components/schemas/EmptyA"}},
	}
	g2 := &Generator{doc: &oapiparser.Document{Schemas: []*oapiparser.Schema{emptyA, emptyB}}}

	assert.False(t, g2.schemaTreeHasDefaults(emptyA, nil, map[string]bool{emptyA.Name: true}))
}

// requiredV2Spec — общий spec для тестов USE_REQUIRED_V2:
// required: [id]; id и name помечены x-request-required, id — x-response-required.
// id readOnly, name writeOnly, label regular.
const requiredV2Spec = `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      required: [id]
      properties:
        id: {type: integer, format: int64, readOnly: true, x-request-required: true, x-response-required: true}
        name: {type: string, writeOnly: true, x-request-required: true}
        label: {type: string}
`

func TestGenerate_UseRequiredV2_SplitRequest(t *testing.T) {
	doc := parseSpec(t, requiredV2Spec)
	pf := oapiparser.ProjectFeatures{
		SplitRequestResponse: oapiparser.ProjectFeature{Value: true},
		UseRequiredV2:        oapiparser.ProjectFeature{Value: true},
	}
	files := generateFilesWithFeatures(t, doc, pf)
	got := string(files["model/pet.gen.go"])

	// Request: id excluded (readOnly), name required (x-request-required), label optional.
	reqSection := extractStruct(got, "PetRequest")
	assert.NotContains(t, reqSection, "ID")
	assert.True(t, containsCollapsed(reqSection, `Name string`), "name must be required (no pointer, no omitempty)")
	assert.True(t, containsCollapsed(reqSection, `Label *string`), "label must be optional (pointer)")

	// Response: name excluded (writeOnly), id required (x-response-required), label optional.
	respSection := extractStruct(got, "PetResponse")
	assert.NotContains(t, respSection, "Name")
	assert.True(t, containsCollapsed(respSection, `ID int64`), "id must be required (no pointer, no omitempty)")
	assert.True(t, containsCollapsed(respSection, `Label *string`), "label must be optional (pointer)")
}

func TestGenerate_UseRequiredV2_Off(t *testing.T) {
	doc := parseSpec(t, requiredV2Spec)
	pf := oapiparser.ProjectFeatures{
		SplitRequestResponse: oapiparser.ProjectFeature{Value: true},
		// UseRequiredV2 off — standard OAS required applies.
	}
	files := generateFilesWithFeatures(t, doc, pf)
	got := string(files["model/pet.gen.go"])

	// Request: id excluded (readOnly), name optional (not in standard required), label optional.
	reqSection := extractStruct(got, "PetRequest")
	assert.NotContains(t, reqSection, "ID")
	assert.True(t, containsCollapsed(reqSection, `Name *string`), "name must be optional (not in required)")
	assert.True(t, containsCollapsed(reqSection, `Label *string`))

	// Response: name excluded (writeOnly), id required (in standard required), label optional.
	respSection := extractStruct(got, "PetResponse")
	assert.NotContains(t, respSection, "Name")
	assert.True(t, containsCollapsed(respSection, `ID int64`), "id must be required (in standard required)")
	assert.True(t, containsCollapsed(respSection, `Label *string`))
}

func TestGenerate_UseRequiredV2_Mono(t *testing.T) {
	// Без split, UseRequiredV2=true. Поле с обоими x-* маркерами → required.
	// Поле только с одним → optional.
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      properties:
        id: {type: integer, x-request-required: true, x-response-required: true}
        name: {type: string, x-request-required: true}
`)
	pf := oapiparser.ProjectFeatures{
		UseRequiredV2: oapiparser.ProjectFeature{Value: true},
	}
	files := generateFilesWithFeatures(t, doc, pf)
	got := string(files["model/pet.gen.go"])

	// id с обоими x-* маркерами → required (no pointer, no omitempty).
	assert.True(t, containsCollapsed(got, `ID int`), "id (both markers) must be required")
	// name только с x-request-required → optional (pointer).
	assert.True(t, containsCollapsed(got, `Name *string`), "name (only request marker) must be optional")
}

// TestGenerate_UseRequiredV2_Compiles — end-to-end проверка, что
// сгенерированный код с v2 + split компилируется Go-компилятором.
// Доказывает, что required/optional логика корректна типами
// (pointer vs value поля).
func TestGenerate_UseRequiredV2_Compiles(t *testing.T) {
	if testing.Short() {
		t.Skip("compile test skipped in short mode")
	}

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go binary not in PATH, skipping compile test")
	}

	doc := parseSpec(t, requiredV2Spec)
	pf := oapiparser.ProjectFeatures{
		SplitRequestResponse: oapiparser.ProjectFeature{Value: true},
		UseRequiredV2:        oapiparser.ProjectFeature{Value: true},
	}
	files := generateFilesWithFeatures(t, doc, pf)

	dir := t.TempDir()
	modelDir := filepath.Join(dir, "model")
	require.NoError(t, os.MkdirAll(modelDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module compiletest\n\ngo 1.26\n"), 0o644))

	for name, content := range files {
		if !strings.HasPrefix(name, "model/") {
			continue
		}

		require.NoError(t, os.WriteFile(filepath.Join(modelDir, filepath.Base(name)), content, 0o644))
	}

	cmd := exec.Command("go", "build", "./model")
	cmd.Dir = dir

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated model package did not compile: %v\n--- output ---\n%s", err, out)
	}
}

// TestGenerate_SetDefaults_Compiles является end-to-end проверкой того, что
// сгенерированный SetDefaults-код компилируется Go-компилятором (типы), а не
// только проходит go/parser (синтаксис). Покрывает регрессии B1 (int/string
// enum zero-value), B2 (date-time skip), M3 (nested SetDefaults call) и
// optional-pointer-default (BLOCKER: `m.Field = &v` pattern для *int/*string/
// *bool/*float64/*<Enum>).
//
// Код записывается в изолированный временный модуль (t.TempDir + go.mod),
// затем запускается `go build ./model`. Тест требует установленного go в PATH.
func TestGenerate_SetDefaults_Compiles(t *testing.T) {
	if testing.Short() {
		t.Skip("compile test skipped in short mode")
	}

	// Пропускаем, если go недоступен в PATH.
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go binary not in PATH, skipping compile test")
	}

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Code:
      type: integer
      format: int32
      enum: [1, 2, 3]
      default: 2
    Status:
      type: string
      enum: [active, inactive]
      default: active
    Inner:
      type: object
      required: [level]
      properties:
        level: {type: integer, default: 5}
    Outer:
      type: object
      required: [code, status, inner, at, name]
      properties:
        code: {$ref: '#/components/schemas/Code'}
        status: {$ref: '#/components/schemas/Status'}
        inner: {$ref: '#/components/schemas/Inner'}
        at: {type: string, format: date-time, default: '2024-01-01T00:00:00Z'}
        name: {type: string, default: unnamed}
        # Optional pointer-fields with defaults — BLOCKER regression coverage.
        # All must be NON-required (pointer), so SetDefaults uses the
        # v-then-and-v pattern (v := literal; m.Field = andv).
        count: {type: integer, default: 5}
        label: {type: string, default: hello}
        active: {type: boolean, default: true}
        rate: {type: number, format: double, default: 3.14}
        statusCode: {$ref: '#/components/schemas/Code'}
`)
	files := generateFiles(t, doc)

	dir := t.TempDir()
	modelDir := filepath.Join(dir, "model")
	require.NoError(t, os.MkdirAll(modelDir, 0o755))

	// go.mod для изолированного модуля.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module compiletest\n\ngo 1.26\n"), 0o644))

	for name, content := range files {
		// Нас интересует только model-пакет (там SetDefaults).
		if !strings.HasPrefix(name, "model/") {
			continue
		}

		require.NoError(t, os.WriteFile(filepath.Join(modelDir, filepath.Base(name)), content, 0o644))
	}

	// go build ./model — компиляция с проверкой типов.
	cmd := exec.Command("go", "build", "./model")
	cmd.Dir = dir

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated model package did not compile: %v\n--- output ---\n%s", err, out)
	}
}

func extractFunc(source, funcName string) string {
	idx := strings.Index(source, funcName)
	if idx < 0 {
		return ""
	}

	end := strings.Index(source[idx:], "\n}\n")
	if end < 0 {
		return ""
	}

	return source[idx : idx+end]
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
	got := string(files["impl/httpclient/client.gen.go"])
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
	_, err := parser.ParseFile(fset, "client.gen.go", files["impl/httpclient/client.gen.go"], parser.AllErrors)
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
	got := string(files["impl/echoserver/server.gen.go"])
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
	_, err := parser.ParseFile(fset, "server.gen.go", files["impl/echoserver/server.gen.go"], parser.AllErrors)
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

func TestWithProjectFeatures_DefaultsToZero(t *testing.T) {
	g := &Generator{}
	WithProjectFeatures(oapiparser.ProjectFeatures{
		ServerNoAutoDefaults: oapiparser.ProjectFeature{Value: true},
		SplitRequestResponse: oapiparser.ProjectFeature{Value: true},
		UseRequiredV2:        oapiparser.ProjectFeature{Value: true},
		UseUTCForDateTime:    oapiparser.ProjectFeature{Value: true},
	})(g)

	assert.True(t, g.features.ServerNoAutoDefaults.Value)
	assert.True(t, g.features.SplitRequestResponse.Value)
	assert.True(t, g.features.UseRequiredV2.Value)
	assert.True(t, g.features.UseUTCForDateTime.Value)
}

func TestWithProjectFeatures_NotCalled_AllFalse(t *testing.T) {
	g := &Generator{}
	assert.False(t, g.features.ServerNoAutoDefaults.Value)
	assert.False(t, g.features.SplitRequestResponse.Value)
	assert.False(t, g.features.UseRequiredV2.Value)
	assert.False(t, g.features.UseUTCForDateTime.Value)
}

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

func generateFilesWithFeatures(
	t *testing.T,
	doc *oapiparser.Document,
	pf oapiparser.ProjectFeatures,
) map[string][]byte {
	t.Helper()
	fw := &collectWriter{files: map[string][]byte{}}
	require.NoError(t, Generate(fw, doc, WithModulePath(testModulePath), WithProjectFeatures(pf)))

	return fw.files
}

func mustReadFile(t *testing.T, p string) []byte {
	t.Helper()
	data, err := os.ReadFile(p)
	require.NoError(t, err)

	return data
}

// TestGenerate_UpdateStruct проверяет, что для схемы, использованной в
// PUT/PATCH request body, генерируется Update<Name> с полями, помеченными
// IsUsedInUpdate. ReadOnly и Immutable (кроме name) пропускаются.
// Каждое поле обёрнуто в optional.Optional[T], теги без omitempty.
func TestGenerate_UpdateStruct(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /pets/{id}:
    put:
      operationId: updatePet
      requestBody:
        required: true
        content:
          application/json:
            schema: {$ref: '#/components/schemas/Pet'}
      responses:
        '200': {description: ok}
components:
  schemas:
    Pet:
      type: object
      properties:
        id: {type: integer, format: int64, readOnly: true}
        name: {type: string}
        tag: {type: string, x-validations: [Immutable]}
        label: {type: string}
`)
	files := generateFiles(t, doc)

	got := string(files["model/pet.gen.go"])

	// UpdatePet существует.
	assert.True(t, containsCollapsed(got, "type UpdatePet struct {"))

	// Поля: name и label (IsUsedInUpdate=true).
	assert.True(t, containsCollapsed(got, "Name optional.Optional[string] `json:\"name\" yaml:\"name\"`"))
	assert.True(t, containsCollapsed(got, "Label optional.Optional[string] `json:\"label\" yaml:\"label\"`"))

	// ReadOnly id и Immutable tag не входят в UpdatePet.
	// Проверяем, что UpdatePet-блок не содержит Id/Tag. Извлечём блок.
	updateBlock := extractStructBlock(t, got, "UpdatePet")
	assert.NotContains(t, updateBlock, "Id")
	assert.NotContains(t, updateBlock, "Tag")

	// import optional-пакета добавлен.
	assert.Contains(t, got, `optional "nschugorev/oapigenerator/pkg/optional"`)

	// Сгенерированный файл валиден как Go.
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "pet.gen.go", files["model/pet.gen.go"], parser.AllErrors)
	require.NoError(t, err, "pet.gen.go should parse as valid Go")
}

// TestGenerate_UpdateStruct_PATCH проверяет, что PATCH тоже триггерит
// генерацию Update<Name>.
func TestGenerate_UpdateStruct_PATCH(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /pets/{id}:
    patch:
      operationId: patchPet
      requestBody:
        required: true
        content:
          application/json:
            schema: {$ref: '#/components/schemas/Pet'}
      responses:
        '200': {description: ok}
components:
  schemas:
    Pet:
      type: object
      properties:
        name: {type: string}
`)
	files := generateFiles(t, doc)

	got := string(files["model/pet.gen.go"])
	assert.True(t, containsCollapsed(got, "type UpdatePet struct {"))
	assert.True(t, containsCollapsed(got, "Name optional.Optional[string]"))
}

// TestGenerate_UpdateStruct_NotMarked проверяет, что схема, не
// используемая в PUT/PATCH, не получает Update-вариант.
func TestGenerate_UpdateStruct_NotMarked(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /pets:
    post:
      operationId: createPet
      requestBody:
        required: true
        content:
          application/json:
            schema: {$ref: '#/components/schemas/Pet'}
      responses:
        '200': {description: ok}
components:
  schemas:
    Pet:
      type: object
      properties:
        name: {type: string}
`)
	files := generateFiles(t, doc)

	got := string(files["model/pet.gen.go"])
	assert.NotContains(t, got, "UpdatePet")
}

// TestGenerate_UpdateStruct_ImmutableNameKept проверяет, что поле с
// именем "name" входит в Update<Name>, даже если оно Immutable.
func TestGenerate_UpdateStruct_ImmutableNameKept(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /pets/{id}:
    put:
      operationId: updatePet
      requestBody:
        required: true
        content:
          application/json:
            schema: {$ref: '#/components/schemas/Pet'}
      responses:
        '200': {description: ok}
components:
  schemas:
    Pet:
      type: object
      properties:
        name: {type: string, x-validations: [Immutable]}
`)
	files := generateFiles(t, doc)

	got := string(files["model/pet.gen.go"])
	assert.True(t, containsCollapsed(got, "Name optional.Optional[string]"))
}

// TestGenerate_UpdateStruct_NoOptionalFlag проверяет, что поля Update<Name>
// оборачиваются в optional.Optional[T] даже при выключенном флаге
// GOLANG_USE_OPTIONAL — update-режим требует Optional безусловно.
func TestGenerate_UpdateStruct_NoOptionalFlag(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /pets/{id}:
    put:
      operationId: updatePet
      requestBody:
        required: true
        content:
          application/json:
            schema: {$ref: '#/components/schemas/Pet'}
      responses:
        '200': {description: ok}
components:
  schemas:
    Pet:
      type: object
      properties:
        name: {type: string}
`)
	// Флаги не заданы — UseOptional по умолчанию false.
	files := generateFiles(t, doc)

	got := string(files["model/pet.gen.go"])
	assert.True(t, containsCollapsed(got, "Name optional.Optional[string]"))
	assert.Contains(t, got, `optional "nschugorev/oapigenerator/pkg/optional"`)
}

// TestGenerate_UpdateStruct_RefToOtherSchema проверяет, что $ref на
// другую схему в update-поле разрешается базовым именем (без Update
// суффикса — marker v1 не помечает nested).
func TestGenerate_UpdateStruct_RefToOtherSchema(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /pets/{id}:
    put:
      operationId: updatePet
      requestBody:
        required: true
        content:
          application/json:
            schema: {$ref: '#/components/schemas/Pet'}
      responses:
        '200': {description: ok}
components:
  schemas:
    Pet:
      type: object
      properties:
        name: {type: string}
        owner: {$ref: '#/components/schemas/Owner'}
    Owner:
      type: object
      properties:
        name: {type: string}
`)
	files := generateFiles(t, doc)

	got := string(files["model/pet.gen.go"])
	// $ref на Owner разрешается базовым именем (не UpdateOwner).
	assert.True(t, containsCollapsed(got, "Owner optional.Optional[Owner]"))
}

// TestGenerate_UpdateStruct_SplitMode проверяет, что при включённом
// GOLANG_SPLIT_REQUEST_RESPONSE $ref из Update<Name> на splittable-схему
// разрешается в <Name>Request.
func TestGenerate_UpdateStruct_SplitMode(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /pets/{id}:
    put:
      operationId: updatePet
      requestBody:
        required: true
        content:
          application/json:
            schema: {$ref: '#/components/schemas/Pet'}
      responses:
        '200': {description: ok}
components:
  schemas:
    Pet:
      type: object
      properties:
        name: {type: string}
        tag: {$ref: '#/components/schemas/Tag'}
    Tag:
      type: object
      properties:
        color: {type: string}
`)
	pf := oapiparser.ProjectFeatures{SplitRequestResponse: oapiparser.ProjectFeature{Value: true}}
	files := generateFilesWithFeatures(t, doc, pf)

	got := string(files["model/pet.gen.go"])
	// $ref на Tag (splittable) в update-контексте → TagRequest.
	assert.True(t, containsCollapsed(got, "Tag optional.Optional[TagRequest]"))
}

// extractStructBlock возвращает содержимое структуры с заданным именем
// (между "type <name> struct {" и следующей "}" на отдельной строке).
func extractStructBlock(t *testing.T, src, structName string) string {
	t.Helper()

	startMarker := "type " + structName + " struct {"
	startIdx := strings.Index(src, startMarker)
	require.GreaterOrEqualf(t, startIdx, 0, "struct %s not found in source", structName)

	bodyStart := startIdx + len(startMarker)
	depth := 1
	i := bodyStart

	for i < len(src) && depth > 0 {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
		}
		i++
	}

	return src[startIdx:i]
}
