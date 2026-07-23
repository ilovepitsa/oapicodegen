package generator

import (
	"go/parser"
	"go/token"
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/generator/compose"
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

	require.NoError(t, Generate(fw, testProject(t, doc, testModulePath), nil))
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
	g := testGeneratorFromSchemas(inner, aSchema)

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
	g2 := testGeneratorFromSchemas(emptyA, emptyB)

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
	require.NoError(t, Generate(fw, testProject(t, doc, testModulePath), nil))
	assert.Empty(t, fw.files)
}

func TestGenerate_WithProject(t *testing.T) {
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
	assert.Contains(t, got, "if resp.Response204 {")
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

// testProject строит *parser.Project из распарсенного Document для тестов
// генератора. Аналог моста buildProject в cmd/oapigen/main.go.
func testProject(t *testing.T, doc *oapiparser.Document, modulePath string) *oapiparser.Project {
	t.Helper()

	project := &oapiparser.Project{
		Folder:       "test",
		ImportPrefix: modulePath,
	}
	project.CreateModel(gogen.Import{
		Path:  modulePath + "/model",
		Alias: "model",
	})
	project.CreatePaths(modulePath)
	project.Model.SetSchemas(doc.Schemas)

	for _, op := range doc.Operations {
		svcName, err := oapiparser.ServiceNameForMethod(op)
		require.NoError(t, err)

		project.Paths.AddMethod(svcName, op)
	}

	return project
}

// testGenerator строит Generator с Project из doc, но без modulePath
// (для unit-тестов рендереров, которым не нужен cross-package импорт).
func testGenerator(t *testing.T, doc *oapiparser.Document) *Generator {
	t.Helper()

	g := &Generator{
		project: testProject(t, doc, ""),
		factory: gogen.NewFileFactory("oapigen"),
	}
	g.composer = compose.NewFileComposer(g.factory)

	return g
}

// testGeneratorFromSchemas строит Generator с Project, содержащим только
// заданные схемы (без операций). Для unit-тестов resolution-логики
// (schemaTreeHasDefaults и т.п.).
func testGeneratorFromSchemas(schemas ...*oapiparser.Schema) *Generator {
	project := &oapiparser.Project{Folder: "test"}
	project.CreateModel(gogen.Import{Alias: "model"})
	project.CreatePaths("")
	project.Model.SetSchemas(schemas)

	return &Generator{
		project: project,
		factory: gogen.NewFileFactory("oapigen"),
	}
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
	require.NoError(t, Generate(fw, testProject(t, doc, testModulePath), nil))

	return fw.files
}

func generateFilesWithFeatures(
	t *testing.T,
	doc *oapiparser.Document,
	pf oapiparser.ProjectFeatures,
) map[string][]byte {
	t.Helper()
	fw := &collectWriter{files: map[string][]byte{}}
	project := testProject(t, doc, testModulePath)
	project.Features = pf
	require.NoError(t, Generate(fw, project, nil))

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

// TestGenerate_UpdateStruct_Getters проверяет, что для каждого поля
// Update<Name> генерируется Get<Field>() (*T, bool) с правильной
// сигнатурой и телом (три ветки: not-set / set-to-nil / set-to-value).
func TestGenerate_UpdateStruct_Getters(t *testing.T) {
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
        age: {type: integer, format: int32}
        tag: {$ref: '#/components/schemas/Tag'}
    Tag:
      type: object
      properties:
        color: {type: string}
`)
	files := generateFiles(t, doc)

	got := string(files["model/pet.gen.go"])

	// Сигнатуры геттеров для каждого поля.
	assert.True(t, containsCollapsed(got, "func (u *UpdatePet) GetName() (*string, bool)"))
	assert.True(t, containsCollapsed(got, "func (u *UpdatePet) GetAge() (*int32, bool)"))
	assert.True(t, containsCollapsed(got, "func (u *UpdatePet) GetTag() (*Tag, bool)"))

	// Три ветки тела: not-set → (nil, false); set-to-nil → (nil, true); value → (&v, true).
	assert.True(t, containsCollapsed(got, "if !u.Name.IsSet() {"))
	assert.True(t, containsCollapsed(got, "return nil, false"))
	assert.True(t, containsCollapsed(got, "if u.Name.IsNil() {"))
	assert.True(t, containsCollapsed(got, "return nil, true"))
	assert.True(t, containsCollapsed(got, "v := u.Name.Value()"))
	assert.True(t, containsCollapsed(got, "return &v, true"))

	// Валидный Go.
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "pet.gen.go", files["model/pet.gen.go"], parser.AllErrors)
	require.NoError(t, err, "pet.gen.go should parse as valid Go")
}

// TestGenerate_UpdateStruct_Getters_NullableField проверяет, что
// nullable-поле в Update использует baseType (без дополнительного *),
// потому что Optional уже различает null через IsNil.
func TestGenerate_UpdateStruct_Getters_NullableField(t *testing.T) {
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
        name: {type: string, nullable: true}
`)
	files := generateFiles(t, doc)

	got := string(files["model/pet.gen.go"])

	// Поле — Optional[string], а не Optional[*string].
	assert.True(t, containsCollapsed(got, "Name optional.Optional[string]"))
	// Геттер возвращает *string, а не **string.
	assert.True(t, containsCollapsed(got, "func (u *UpdatePet) GetName() (*string, bool)"))
}

// TestGenerate_UpdateStruct_Getters_SliceField проверяет геттер для
// slice-поля: Optional[[]string] → (*[]string, bool).
func TestGenerate_UpdateStruct_Getters_SliceField(t *testing.T) {
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
        tags:
          type: array
          items: {type: string}
`)
	files := generateFiles(t, doc)

	got := string(files["model/pet.gen.go"])
	assert.True(t, containsCollapsed(got, "Tags optional.Optional[[]string]"))
	assert.True(t, containsCollapsed(got, "func (u *UpdatePet) GetTags() (*[]string, bool)"))
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

// --- Validation generation tests (T-val.3) ---

func TestGenerate_ValidateOwn_SimpleRule(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      required: [age]
      properties:
        age:
          type: integer
          x-validations: [">0"]
`)
	files := generateFiles(t, doc)
	got := string(files["model/pet.gen.go"])

	assert.Contains(t, got, "func (x Pet) ValidateOwn(reg *validator.Registry) error {")
	assert.Contains(t, got, `if x.Age <= 0 {`)
	assert.Contains(t, got, `return fmt.Errorf("field Age: must be > 0")`)
	assert.Contains(t, got, `validator "nschugorev/oapigenerator/pkg/validator"`)

	// Generated file parses as valid Go.
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "pet.gen.go", files["model/pet.gen.go"], parser.AllErrors)
	require.NoError(t, err)
}

func TestGenerate_ValidateOwn_SizeRule(t *testing.T) {
	t.Parallel()

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
        name:
          type: string
          x-validations: ["Size >=2"]
`)
	files := generateFiles(t, doc)
	got := string(files["model/pet.gen.go"])

	// Required field — no pointer wrapping. Size-rules use len() without nil-guard.
	assert.Contains(t, got, `if len(x.Name) < 2 {`)
	assert.Contains(t, got, `return fmt.Errorf("field Name: must be >= 2")`)
}

func TestGenerate_ValidateOwn_SizeRule_PointerField(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      properties:
        name:
          type: string
          x-validations: ["Size >=2"]
`)
	files := generateFiles(t, doc)
	got := string(files["model/pet.gen.go"])

	// Non-required field → *string. Size rule needs nil-guard + deref.
	assert.Contains(t, got, `if x.Name != nil && len(*x.Name) < 2 {`)
}

func TestGenerate_ValidateOwn_PointerFieldNilGuard(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      properties:
        tag:
          type: string
          nullable: true
          x-validations: [">=1"]
`)
	files := generateFiles(t, doc)
	got := string(files["model/pet.gen.go"])

	// Nullable field becomes *string — guard + dereference. Rule ">=1" inverts to "< 1".
	assert.Contains(t, got, `if x.Tag != nil && *x.Tag < 1 {`)
}

func TestGenerate_ValidateOwn_NamedValidator_Property(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      required: [email]
      properties:
        email:
          type: string
          x-validations: ["cdn.EmailFormat"]
`)
	files := generateFiles(t, doc)
	got := string(files["model/pet.gen.go"])

	// Required field — no pointer wrapping, no nil-guard.
	assert.Contains(t, got, `v, ok := reg.Get("cdn.EmailFormat")`)
	assert.Contains(t, got, `return fmt.Errorf("validator %q not registered", "cdn.EmailFormat")`)
	assert.Contains(t, got, `if err := v.Validate(x.Email); err != nil {`)
	assert.Contains(t, got, `return fmt.Errorf("field Email: %w", err)`)
}

func TestGenerate_ValidateOwn_NamedValidator_PointerField(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      properties:
        email:
          type: string
          x-validations: ["cdn.EmailFormat"]
`)
	files := generateFiles(t, doc)
	got := string(files["model/pet.gen.go"])

	// Non-required → *string. Guard + deref for validator call.
	assert.Contains(t, got, `if x.Email != nil {`)
	assert.Contains(t, got, `if err := v.Validate(*x.Email); err != nil {`)
}

func TestGenerate_ValidateOwn_NamedValidator_SchemaLevel(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      x-validations: ["cdn.PetConsistency"]
      properties:
        name: {type: string}
`)
	files := generateFiles(t, doc)
	got := string(files["model/pet.gen.go"])

	// Schema-level: validator called on receiver, no field-path wrap.
	assert.Contains(t, got, `v, ok := reg.Get("cdn.PetConsistency")`)
	assert.Contains(t, got, `if err := v.Validate(x); err != nil {`)
	assert.Contains(t, got, "\t\treturn err\n")
}

func TestGenerate_ValidateOwn_NoRules_NotGenerated(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      properties:
        name: {type: string}
`)
	files := generateFiles(t, doc)
	got := string(files["model/pet.gen.go"])

	assert.NotContains(t, got, "ValidateOwn")
}

func TestGenerate_ValidateOwn_UpdateStruct(t *testing.T) {
	t.Parallel()

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
        name:
          type: string
          x-validations: [">=1"]
        tag:
          type: string
          x-validations: [Immutable]
`)
	files := generateFiles(t, doc)
	got := string(files["model/pet.gen.go"])

	// Update struct gets ValidateOwn with IsSet/IsNil guard + .Value() accessor.
	// Rule ">=1" inverts to "< 1".
	assert.Contains(t, got, "func (x UpdatePet) ValidateOwn(reg *validator.Registry) error {")
	assert.Contains(t, got, `if x.Name.IsSet() && !x.Name.IsNil() && x.Name.Value() < 1 {`)

	// Tag is Immutable non-name — skipped from UpdatePet, so no validation for it.
	updateBlock := extractStructBlock(t, got, "UpdatePet")
	assert.NotContains(t, updateBlock, "Tag")
}

func TestGenerate_ValidateOwn_SplitStruct(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      properties:
        name:
          type: string
          x-validations: [">=1"]
        id:
          type: integer
          format: int64
          readOnly: true
          x-validations: [">0"]
`)
	pf := oapiparser.ProjectFeatures{}
	pf.SplitRequestResponse.Value = true
	files := generateFilesWithFeatures(t, doc, pf)
	got := string(files["model/pet.gen.go"])

	// Request variant: name only (id is readOnly, filtered out). Rule ">=1" → "< 1".
	// name is non-required → *string, so guard + deref.
	assert.Contains(t, got, "func (x PetRequest) ValidateOwn(reg *validator.Registry) error {")
	assert.Contains(t, got, `if x.Name != nil && *x.Name < 1 {`)

	// Response variant: both name and id present. id → ID (initialism).
	assert.Contains(t, got, "func (x PetResponse) ValidateOwn(reg *validator.Registry) error {")
	assert.Contains(t, got, `if x.ID != nil && *x.ID <= 0 {`)
	assert.Contains(t, got, `if x.Name != nil && *x.Name < 1 {`)
}

func TestGenerate_ExpectedValidatorsFile(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      x-validations: ["cdn.PetConsistency"]
      properties:
        email:
          type: string
          x-validations: ["cdn.EmailFormat"]
        name:
          type: string
          x-validations: ["cdn.NotEmpty", "cdn.EmailFormat"]
    Owner:
      type: object
      properties:
        contact:
          type: string
          x-validations: ["cdn.EmailFormat"]
`)
	files := generateFiles(t, doc)
	got, ok := files["model/expected_validators.gen.go"]
	require.True(t, ok, "expected_validators.gen.go should be generated")

	src := string(got)
	assert.Contains(t, src, "func ExpectedValidatorNames() []string {")
	assert.Contains(t, src, `"cdn.EmailFormat",`)
	assert.Contains(t, src, `"cdn.NotEmpty",`)
	assert.Contains(t, src, `"cdn.PetConsistency",`)

	// Sorted: EmailFormat < NotEmpty < PetConsistency.
	emailIdx := strings.Index(src, `"cdn.EmailFormat"`)
	notEmptyIdx := strings.Index(src, `"cdn.NotEmpty"`)
	consistencyIdx := strings.Index(src, `"cdn.PetConsistency"`)
	assert.Greater(t, notEmptyIdx, emailIdx, "NotEmpty should come after EmailFormat")
	assert.Greater(t, consistencyIdx, notEmptyIdx, "PetConsistency should come after NotEmpty")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "expected_validators.gen.go", got, parser.AllErrors)
	require.NoError(t, err)
}

func TestGenerate_ExpectedValidatorsFile_NotGeneratedWhenNoNamed(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      properties:
        age:
          type: integer
          x-validations: [">0"]
`)
	files := generateFiles(t, doc)
	_, ok := files["model/expected_validators.gen.go"]
	assert.False(t, ok, "expected_validators.gen.go should not be generated when only simple rules exist")
}

// --- URLForm tests (T25c.1) ---

// generatePetURLFormFile рендерит url-form файл для схемы Pet из doc.
// Использует приватный Generator без Options — только для unit-тестов
// рендерера (T25c.1, T25c.2). Wire-up через Generate проверяется в T25c.3.
//
// Начиная с Task 6 (T27 Phase 1), рендер идёт через compose.FileComposer +
// render/schema.URLFormRenderer (см. Generator.writeURLFormAuxFile). Здесь
// дублируем composer-вызов напрямую, чтобы тесты проверяли только url_form-
// файл без запуска полного writeSchemaFiles.
func generatePetURLFormFile(t *testing.T, doc *oapiparser.Document) []byte {
	t.Helper()

	g := testGenerator(t, doc)

	for _, sh := range doc.Schemas {
		if sh.Name == "Pet" {
			fw := &collectWriter{files: map[string][]byte{}}
			require.NoError(t, g.writeURLFormAuxFile(fw, sh))

			return fw.files["model/pet_url_form.gen.go"]
		}
	}

	t.Fatal("schema Pet not found")

	return nil
}

func TestSchemeHasURLFormat_RefBody(t *testing.T) {
	t.Parallel()

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
          application/x-www-form-urlencoded:
            schema: {$ref: '#/components/schemas/Pet'}
      responses:
        '201': {description: ok}
components:
  schemas:
    Pet:
      type: object
      properties:
        name: {type: string}
    Other:
      type: object
      properties:
        id: {type: integer}
`)

	pet := findSchemaByName(t, doc, "Pet")
	other := findSchemaByName(t, doc, "Other")

	assert.True(t, schemeHasURLFormat(pet, doc.Operations), "Pet is referenced from form-urlencoded body")
	assert.False(t, schemeHasURLFormat(other, doc.Operations), "Other is not referenced from any form body")
}

func TestSchemeHasURLFormat_JSONBody_False(t *testing.T) {
	t.Parallel()

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
        '201': {description: ok}
components:
  schemas:
    Pet:
      type: object
      properties:
        name: {type: string}
`)

	pet := findSchemaByName(t, doc, "Pet")
	assert.False(t, schemeHasURLFormat(pet, doc.Operations), "JSON body should not trigger URL form generation")
}

func TestMarshalURLForm_PrimitiveFields(t *testing.T) {
	t.Parallel()

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
          application/x-www-form-urlencoded:
            schema: {$ref: '#/components/schemas/Pet'}
      responses:
        '201': {description: ok}
components:
  schemas:
    Pet:
      type: object
      required: [name, age, active, score]
      properties:
        name: {type: string}
        age: {type: integer, format: int64}
        active: {type: boolean}
        score: {type: number, format: float}
`)
	got := string(generatePetURLFormFile(t, doc))

	assert.Contains(t, got, "func (m Pet) MarshalURLForm() (url.Values, error) {")
	assert.Contains(t, got, "values := url.Values{}")
	assert.Contains(t, got, `values.Set("name", m.Name)`)
	assert.Contains(t, got, `values.Set("age", strconv.FormatInt(int64(m.Age), 10))`)
	assert.Contains(t, got, `values.Set("active", strconv.FormatBool(m.Active))`)
	assert.Contains(t, got, `values.Set("score", strconv.FormatFloat(float64(m.Score), 'f', -1, 32))`)
	assert.Contains(t, got, "return values, nil")

	// Generated file parses as valid Go.
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "pet_url_form.gen.go", []byte(got), parser.AllErrors)
	require.NoError(t, err)
}

func TestMarshalURLForm_OptionalPointerField(t *testing.T) {
	t.Parallel()

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
          application/x-www-form-urlencoded:
            schema: {$ref: '#/components/schemas/Pet'}
      responses:
        '201': {description: ok}
components:
  schemas:
    Pet:
      type: object
      required: [name]
      properties:
        name: {type: string}
        tag: {type: string, nullable: true}
`)
	got := string(generatePetURLFormFile(t, doc))

	// Required field — direct set.
	assert.Contains(t, got, `values.Set("name", m.Name)`)
	// Optional nullable field — guard + dereference.
	assert.Contains(t, got, "if m.Tag != nil {")
	assert.Contains(t, got, `values.Set("tag", *m.Tag)`)
}

func TestMarshalURLForm_DateTimeField(t *testing.T) {
	t.Parallel()

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
          application/x-www-form-urlencoded:
            schema: {$ref: '#/components/schemas/Pet'}
      responses:
        '201': {description: ok}
components:
  schemas:
    Pet:
      type: object
      required: [createdAt]
      properties:
        createdAt: {type: string, format: date-time}
`)
	got := string(generatePetURLFormFile(t, doc))

	assert.Contains(t, got, `values.Set("createdAt", m.CreatedAt.Format(time.RFC3339))`)
}

func TestMarshalURLForm_UnsupportedArray(t *testing.T) {
	t.Parallel()

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
          application/x-www-form-urlencoded:
            schema: {$ref: '#/components/schemas/Pet'}
      responses:
        '201': {description: ok}
components:
  schemas:
    Pet:
      type: object
      required: [name, tags]
      properties:
        name: {type: string}
        tags:
          type: array
          items: {type: string}
`)
	got := string(generatePetURLFormFile(t, doc))

	// Array field unsupported → immediate error return, no values.Set calls.
	assert.Contains(t, got, "func (m Pet) MarshalURLForm() (url.Values, error) {")
	assert.Contains(t, got, `return nil, fmt.Errorf("field Tags: url-form encoding not supported")`)
	assert.NotContains(t, got, "values.Set")

	// Generated file parses as valid Go.
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "pet_url_form.gen.go", []byte(got), parser.AllErrors)
	require.NoError(t, err)
}

func TestMarshalURLForm_UnsupportedRef(t *testing.T) {
	t.Parallel()

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
          application/x-www-form-urlencoded:
            schema: {$ref: '#/components/schemas/Pet'}
      responses:
        '201': {description: ok}
components:
  schemas:
    Pet:
      type: object
      required: [name, owner]
      properties:
        name: {type: string}
        owner: {$ref: '#/components/schemas/Owner'}
    Owner:
      type: object
      properties:
        id: {type: integer}
`)
	got := string(generatePetURLFormFile(t, doc))

	// $ref field unsupported → error return.
	assert.Contains(t, got, `return nil, fmt.Errorf("field Owner: url-form encoding not supported")`)
}

func TestUnmarshalURLForm_PrimitiveFields(t *testing.T) {
	t.Parallel()

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
          application/x-www-form-urlencoded:
            schema: {$ref: '#/components/schemas/Pet'}
      responses:
        '201': {description: ok}
components:
  schemas:
    Pet:
      type: object
      required: [name, age, active, score]
      properties:
        name: {type: string}
        age: {type: integer, format: int64}
        active: {type: boolean}
        score: {type: number, format: float}
`)
	got := string(generatePetURLFormFile(t, doc))

	assert.Contains(t, got, "func (m *Pet) UnmarshalURLForm(values url.Values) error {")
	// Plain string → direct assign.
	assert.Contains(t, got, `m.Name = values.Get("name")`)
	// int64 → ParseInt + direct assign (no cast).
	assert.Contains(t, got, `parsed, err := strconv.ParseInt(values.Get("age"), 10, 64)`)
	assert.Contains(t, got, `m.Age = parsed`)
	// bool → ParseBool + direct assign.
	assert.Contains(t, got, `parsed, err := strconv.ParseBool(values.Get("active"))`)
	assert.Contains(t, got, `m.Active = parsed`)
	// float32 → ParseFloat + cast.
	assert.Contains(t, got, `parsed, err := strconv.ParseFloat(values.Get("score"), 32)`)
	assert.Contains(t, got, `m.Score = float32(parsed)`)
	assert.Contains(t, got, "return nil")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "pet_url_form.gen.go", []byte(got), parser.AllErrors)
	require.NoError(t, err)
}

func TestUnmarshalURLForm_Int32Field(t *testing.T) {
	t.Parallel()

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
          application/x-www-form-urlencoded:
            schema: {$ref: '#/components/schemas/Pet'}
      responses:
        '201': {description: ok}
components:
  schemas:
    Pet:
      type: object
      required: [age]
      properties:
        age: {type: integer, format: int32}
`)
	got := string(generatePetURLFormFile(t, doc))

	assert.Contains(t, got, `parsed, err := strconv.ParseInt(values.Get("age"), 10, 32)`)
	assert.Contains(t, got, `m.Age = int32(parsed)`)
}

func TestUnmarshalURLForm_OptionalPointerField(t *testing.T) {
	t.Parallel()

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
          application/x-www-form-urlencoded:
            schema: {$ref: '#/components/schemas/Pet'}
      responses:
        '201': {description: ok}
components:
  schemas:
    Pet:
      type: object
      required: [name]
      properties:
        name: {type: string}
        tag: {type: string, nullable: true}
        age: {type: integer, format: int64, nullable: true}
`)
	got := string(generatePetURLFormFile(t, doc))

	// Required string → direct assign.
	assert.Contains(t, got, `m.Name = values.Get("name")`)
	// Nullable string → guard + &v.
	assert.Contains(t, got, `if v := values.Get("tag"); v != "" {`)
	assert.Contains(t, got, `m.Tag = &v`)
	// Nullable int64 → guard + parse + &parsed.
	assert.Contains(t, got, `if v := values.Get("age"); v != "" {`)
	assert.Contains(t, got, `parsed, err := strconv.ParseInt(v, 10, 64)`)
	assert.Contains(t, got, `m.Age = &parsed`)

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "pet_url_form.gen.go", []byte(got), parser.AllErrors)
	require.NoError(t, err)
}

func TestUnmarshalURLForm_PointerWithCast(t *testing.T) {
	t.Parallel()

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
          application/x-www-form-urlencoded:
            schema: {$ref: '#/components/schemas/Pet'}
      responses:
        '201': {description: ok}
components:
  schemas:
    Pet:
      type: object
      required: [name]
      properties:
        name: {type: string}
        score: {type: number, format: float, nullable: true}
`)
	got := string(generatePetURLFormFile(t, doc))

	// Nullable float32 → guard + parse + converted temp + &converted.
	assert.Contains(t, got, `if v := values.Get("score"); v != "" {`)
	assert.Contains(t, got, `parsed, err := strconv.ParseFloat(v, 32)`)
	assert.Contains(t, got, `converted := float32(parsed)`)
	assert.Contains(t, got, `m.Score = &converted`)

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "pet_url_form.gen.go", []byte(got), parser.AllErrors)
	require.NoError(t, err)
}

func TestUnmarshalURLForm_DateTimeField(t *testing.T) {
	t.Parallel()

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
          application/x-www-form-urlencoded:
            schema: {$ref: '#/components/schemas/Pet'}
      responses:
        '201': {description: ok}
components:
  schemas:
    Pet:
      type: object
      required: [createdAt]
      properties:
        createdAt: {type: string, format: date-time}
`)
	got := string(generatePetURLFormFile(t, doc))

	assert.Contains(t, got, `parsed, err := time.Parse(time.RFC3339, values.Get("createdAt"))`)
	assert.Contains(t, got, `m.CreatedAt = parsed`)
}

func TestUnmarshalURLForm_DateField(t *testing.T) {
	t.Parallel()

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
          application/x-www-form-urlencoded:
            schema: {$ref: '#/components/schemas/Pet'}
      responses:
        '201': {description: ok}
components:
  schemas:
    Pet:
      type: object
      required: [birthday]
      properties:
        birthday: {type: string, format: date}
`)
	got := string(generatePetURLFormFile(t, doc))

	assert.Contains(t, got, `parsed, err := time.Parse(time.DateOnly, values.Get("birthday"))`)
	assert.Contains(t, got, `m.Birthday = parsed`)
}

func TestUnmarshalURLForm_UnsupportedArray(t *testing.T) {
	t.Parallel()

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
          application/x-www-form-urlencoded:
            schema: {$ref: '#/components/schemas/Pet'}
      responses:
        '201': {description: ok}
components:
  schemas:
    Pet:
      type: object
      required: [name, tags]
      properties:
        name: {type: string}
        tags:
          type: array
          items: {type: string}
`)
	got := string(generatePetURLFormFile(t, doc))

	assert.Contains(t, got, "func (m *Pet) UnmarshalURLForm(values url.Values) error {")
	assert.Contains(t, got, `return fmt.Errorf("field Tags: url-form decoding not supported")`)
	assert.NotContains(t, got, "values.Get")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "pet_url_form.gen.go", []byte(got), parser.AllErrors)
	require.NoError(t, err)
}

func findSchemaByName(t *testing.T, doc *oapiparser.Document, name string) *oapiparser.Schema {
	t.Helper()

	for _, sh := range doc.Schemas {
		if sh.Name == name {
			return sh
		}
	}

	t.Fatalf("schema %s not found", name)

	return nil
}

// --- T25c.3 integration tests ---

func TestGenerate_URLFormFile_Generated(t *testing.T) {
	t.Parallel()

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
          application/x-www-form-urlencoded:
            schema: {$ref: '#/components/schemas/Pet'}
      responses:
        '201': {description: ok}
components:
  schemas:
    Pet:
      type: object
      required: [name, age]
      properties:
        name: {type: string}
        age: {type: integer, format: int64}
`)
	files := generateFiles(t, doc)

	got, ok := files["model/pet_url_form.gen.go"]
	require.True(t, ok, "pet_url_form.gen.go should be generated for URL-form body")

	src := string(got)
	assert.Contains(t, src, "func (m Pet) MarshalURLForm() (url.Values, error) {")
	assert.Contains(t, src, "func (m *Pet) UnmarshalURLForm(values url.Values) error {")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "pet_url_form.gen.go", got, parser.AllErrors)
	require.NoError(t, err)
}

func TestGenerate_URLFormFile_NotGeneratedForJSONBody(t *testing.T) {
	t.Parallel()

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
        '201': {description: ok}
components:
  schemas:
    Pet:
      type: object
      properties:
        name: {type: string}
`)
	files := generateFiles(t, doc)

	_, ok := files["model/pet_url_form.gen.go"]
	assert.False(t, ok, "pet_url_form.gen.go should not be generated for JSON body")
}

func TestGenerate_URLFormClient_UsesMarshalURLForm(t *testing.T) {
	t.Parallel()

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
          application/x-www-form-urlencoded:
            schema: {$ref: '#/components/schemas/Pet'}
      responses:
        '201': {description: ok}
components:
  schemas:
    Pet:
      type: object
      required: [name]
      properties:
        name: {type: string}
`)
	files := generateFiles(t, doc)

	got, ok := files["impl/httpclient/client.gen.go"]
	require.True(t, ok, "client.gen.go should be generated")

	src := string(got)
	assert.Contains(t, src, "values, err := req.Body.MarshalURLForm()")
	assert.Contains(t, src, "body := []byte(values.Encode())")
	assert.NotContains(t, src, "json.Marshal(req.Body)")
	assert.Contains(t, src, `httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")`)
	assert.NotContains(t, src, `httpReq.Header.Set("Content-Type", "application/json")`)

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "client.gen.go", got, parser.AllErrors)
	require.NoError(t, err)
}

func TestGenerate_URLFormServer_UsesUnmarshalURLForm(t *testing.T) {
	t.Parallel()

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
          application/x-www-form-urlencoded:
            schema: {$ref: '#/components/schemas/Pet'}
      responses:
        '201': {description: ok}
components:
  schemas:
    Pet:
      type: object
      required: [name]
      properties:
        name: {type: string}
`)
	files := generateFiles(t, doc)

	got, ok := files["impl/echoserver/server.gen.go"]
	require.True(t, ok, "server.gen.go should be generated")

	src := string(got)
	assert.Contains(t, src, `strings.HasPrefix(ct, "application/x-www-form-urlencoded")`)
	assert.Contains(t, src, "interface{ UnmarshalURLForm(url.Values) error }")
	assert.Contains(t, src, "u.UnmarshalURLForm(c.Request().PostForm)")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "server.gen.go", got, parser.AllErrors)
	require.NoError(t, err)
}

func TestGenerate_URLFormServer_NoURLFormBranchWhenOnlyJSON(t *testing.T) {
	t.Parallel()

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
        '201': {description: ok}
components:
  schemas:
    Pet:
      type: object
      properties:
        name: {type: string}
`)
	files := generateFiles(t, doc)

	got, ok := files["impl/echoserver/server.gen.go"]
	require.True(t, ok, "server.gen.go should be generated")

	src := string(got)
	assert.NotContains(t, src, "application/x-www-form-urlencoded")
	assert.NotContains(t, src, "UnmarshalURLForm")
}

// --- T26.1 converter tests ---

func generateConverterFile(t *testing.T, doc *oapiparser.Document, schemaName string) []byte {
	t.Helper()

	g := testGenerator(t, doc)

	for _, sh := range doc.Schemas {
		if sh.Name == schemaName {
			sh.IsSplit = true
			fw := &collectWriter{files: map[string][]byte{}}
			require.NoError(t, g.writeConvertersAuxFile(fw, sh))

			return fw.files["model/"+fileName(sh.Name)+"_converters.gen.go"]
		}
	}

	t.Fatalf("schema %s not found", schemaName)

	return nil
}

func TestConverter_RequestToResponse_SharedFields(t *testing.T) {
	t.Parallel()

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
        tag: {type: string}
`)
	got := string(generateConverterFile(t, doc, "Pet"))

	// Function signature.
	assert.Contains(t, got, "func PetRequestToResponse(req PetRequest) PetResponse {")
	assert.Contains(t, got, "var resp PetResponse")

	// Shared fields (name, tag) copied.
	assert.Contains(t, got, "resp.Name = req.Name")
	assert.Contains(t, got, "resp.Tag = req.Tag")

	// readOnly (id) and writeOnly (secret) NOT copied.
	assert.NotContains(t, got, "resp.ID = req.ID")
	assert.NotContains(t, got, "resp.Secret = req.Secret")

	assert.Contains(t, got, "return resp")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "pet_converters.gen.go", []byte(got), parser.AllErrors)
	require.NoError(t, err)
}

func TestConverter_NoSharedFields_NotGenerated(t *testing.T) {
	t.Parallel()

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
        secret: {type: string, writeOnly: true}
`)
	assert.False(t, schemaHasSharedFields(findSchemaByName(t, doc, "Pet")))
}

func TestConverter_HasSharedFields(t *testing.T) {
	t.Parallel()

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
	assert.True(t, schemaHasSharedFields(findSchemaByName(t, doc, "Pet")))
}

func TestConverter_AllFieldsShared(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      required: [name, tag]
      properties:
        name: {type: string}
        tag: {type: string}
`)
	got := string(generateConverterFile(t, doc, "Pet"))

	assert.Contains(t, got, "resp.Name = req.Name")
	assert.Contains(t, got, "resp.Tag = req.Tag")
	assert.NotContains(t, got, "resp.ID")
}

func TestConverter_PointerFieldCopied(t *testing.T) {
	t.Parallel()

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
        name: {type: string}
        tag: {type: string, nullable: true}
`)
	got := string(generateConverterFile(t, doc, "Pet"))

	// Nullable (pointer) field copied directly — shallow pointer copy.
	assert.Contains(t, got, "resp.Tag = req.Tag")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "pet_converters.gen.go", []byte(got), parser.AllErrors)
	require.NoError(t, err)
}

// --- T26.2 integration tests ---

func TestGenerate_Converters_GeneratedWithSplit(t *testing.T) {
	t.Parallel()

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

	got, ok := files["model/pet_converters.gen.go"]
	require.True(t, ok, "pet_converters.gen.go should be generated with split on")

	src := string(got)
	assert.Contains(t, src, "func PetRequestToResponse(req PetRequest) PetResponse {")
	assert.Contains(t, src, "resp.Name = req.Name")
	assert.NotContains(t, src, "resp.ID")
	assert.NotContains(t, src, "resp.Secret")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "pet_converters.gen.go", got, parser.AllErrors)
	require.NoError(t, err)
}

func TestGenerate_Converters_NotGeneratedWithoutSplit(t *testing.T) {
	t.Parallel()

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

	_, ok := files["model/pet_converters.gen.go"]
	assert.False(t, ok, "pet_converters.gen.go should not be generated without split")
}

func TestGenerate_Converters_NotGeneratedWhenNoSharedFields(t *testing.T) {
	t.Parallel()

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
        secret: {type: string, writeOnly: true}
`)
	pf := oapiparser.ProjectFeatures{
		SplitRequestResponse: oapiparser.ProjectFeature{Value: true},
	}
	files := generateFilesWithFeatures(t, doc, pf)

	_, ok := files["model/pet_converters.gen.go"]
	assert.False(t, ok, "converters should not be generated when there are no shared fields")
}

func TestGenerate_Converters_Compiles(t *testing.T) {
	if testing.Short() {
		t.Skip("compile test skipped in short mode")
	}

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go binary not in PATH, skipping compile test")
	}

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
        tag: {type: string, nullable: true}
        secret: {type: string, writeOnly: true}
`)
	pf := oapiparser.ProjectFeatures{
		SplitRequestResponse: oapiparser.ProjectFeature{Value: true},
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

// --- T27.3 model-layer audit-data tests ---

func generatePetAuditModelFile(t *testing.T, doc *oapiparser.Document) []byte {
	t.Helper()

	g := testGenerator(t, doc)

	for _, sh := range doc.Schemas {
		if sh.Name == "Pet" {
			return g.auditModelFile(sh).Content()
		}
	}

	t.Fatal("schema Pet not found")

	return nil
}

func TestAuditModel_StructAndMethod(t *testing.T) {
	t.Parallel()

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
        name: {type: string}
        age: {type: integer, format: int64}
`)
	got := string(generatePetAuditModelFile(t, doc))

	assert.Contains(t, got, "type PetAuditData struct {")
	assert.Contains(t, got, "func (m Pet) GetAuditData() any {")
	assert.Contains(t, got, "var am PetAuditData")
	assert.Contains(t, got, "return am")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "pet_audit_data.gen.go", []byte(got), parser.AllErrors)
	require.NoError(t, err)
}

func TestAuditModel_SensitiveFieldMasked(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      required: [name, secret]
      properties:
        name: {type: string}
        secret: {type: string, x-sensitive: true}
`)
	got := string(generatePetAuditModelFile(t, doc))

	// Sensitive field gets sensitive.Sensitive[T] type.
	assert.Regexp(t, `Secret\s+sensitive\.Sensitive\[string\]`, got)
	// Non-sensitive field keeps original type.
	assert.Regexp(t, `Name\s+string`, got)

	// Method: non-sensitive copied directly.
	assert.Contains(t, got, "am.Name = m.Name")
	// Sensitive wrapped in sensitive.New.
	assert.Contains(t, got, "am.Secret = sensitive.New(m.Secret)")
}

func TestAuditModel_SensitivePointerField(t *testing.T) {
	t.Parallel()

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
        name: {type: string}
        secret: {type: string, nullable: true, x-sensitive: true}
`)
	got := string(generatePetAuditModelFile(t, doc))

	// Sensitive pointer field → *sensitive.Sensitive[string].
	assert.Regexp(t, `Secret\s+\*sensitive\.Sensitive\[string\]`, got)

	// Method: nil-guard + dereference + sensitive.New.
	assert.Contains(t, got, "if m.Secret != nil {")
	assert.Contains(t, got, "v := sensitive.New(*m.Secret)")
	assert.Contains(t, got, "am.Secret = &v")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "pet_audit_data.gen.go", []byte(got), parser.AllErrors)
	require.NoError(t, err)
}

func TestAuditModel_NonSensitivePointerField(t *testing.T) {
	t.Parallel()

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
        name: {type: string}
        tag: {type: string, nullable: true}
`)
	got := string(generatePetAuditModelFile(t, doc))

	// Non-sensitive nullable field → *string, copied directly.
	assert.Regexp(t, `Tag\s+\*string`, got)
	assert.Contains(t, got, "am.Tag = m.Tag")
}

func TestSchemaReferencedByOperation_RequestBody(t *testing.T) {
	t.Parallel()

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
        '201': {description: ok}
components:
  schemas:
    Pet:
      type: object
      properties:
        name: {type: string}
    Other:
      type: object
      properties:
        id: {type: integer}
`)
	pet := findSchemaByName(t, doc, "Pet")
	other := findSchemaByName(t, doc, "Other")

	assert.True(t, schemaReferencedByOperation(pet, doc.Operations), "Pet is referenced by request body")
	assert.False(t, schemaReferencedByOperation(other, doc.Operations), "Other is not referenced")
}

func TestSchemaReferencedByOperation_Response(t *testing.T) {
	t.Parallel()

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
              schema: {$ref: '#/components/schemas/Pet'}
components:
  schemas:
    Pet:
      type: object
      properties:
        name: {type: string}
`)
	pet := findSchemaByName(t, doc, "Pet")
	assert.True(t, schemaReferencedByOperation(pet, doc.Operations), "Pet is referenced by response")
}

func TestSchemaReferencedByOperation_NotReferenced(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      properties:
        name: {type: string}
`)
	pet := findSchemaByName(t, doc, "Pet")
	assert.False(t, schemaReferencedByOperation(pet, doc.Operations), "Pet is not referenced by any operation")
}

// --- T27.4 server-layer audit-data tests ---

func generateAuditClientFile(t *testing.T, doc *oapiparser.Document) []byte {
	t.Helper()

	g := &Generator{
		project: testProject(t, doc, testModulePath),
		factory: gogen.NewFileFactory("oapigen"),
	}

	return g.auditClientFile().Content()
}

func TestAuditServer_RequestWithPathParamAndBody(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /pets/{petId}:
    post:
      operationId: updatePet
      parameters:
        - {name: petId, in: path, required: true, schema: {type: string}}
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
	got := string(generateAuditClientFile(t, doc))

	assert.Contains(t, got, "type UpdatePetRequestAuditData struct {")
	assert.Regexp(t, `PetID\s+string`, got)
	assert.Regexp(t, `Body\s+any`, got)
	assert.Contains(t, got, "func (req *UpdatePetRequest) GetAuditData() any {")
	assert.Contains(t, got, "PetID: req.PetID,")
	// Required body → no nil-check.
	assert.Contains(t, got, "am.Body = req.Body.GetAuditData()")
	assert.NotContains(t, got, "if req.Body != nil")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, "audit.gen.go", []byte(got), parser.AllErrors)
	require.NoError(t, err)
}

func TestAuditServer_RequestWithOptionalBody_NilCheck(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /pets:
    post:
      operationId: createPet
      requestBody:
        required: false
        content:
          application/json:
            schema: {$ref: '#/components/schemas/Pet'}
      responses:
        '201': {description: created}
components:
  schemas:
    Pet:
      type: object
      properties:
        name: {type: string}
`)
	got := string(generateAuditClientFile(t, doc))

	assert.Contains(t, got, "if req.Body != nil {")
	assert.Contains(t, got, "am.Body = req.Body.GetAuditData()")
}

func TestAuditServer_RequestWithQueryParamOnly(t *testing.T) {
	t.Parallel()

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
        '200': {description: ok}
components:
  schemas: {}
`)
	got := string(generateAuditClientFile(t, doc))

	assert.Contains(t, got, "type ListPetsRequestAuditData struct {")
	assert.Regexp(t, `Limit\s+\*int`, got)
	assert.NotContains(t, got, "Body any")
}

func TestAuditServer_RequestSkipped_NoParamsNoBody(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /health:
    get:
      operationId: health
      responses:
        '200': {description: ok}
components:
  schemas: {}
`)
	got := string(generateAuditClientFile(t, doc))

	assert.NotContains(t, got, "HealthRequestAuditData")
}

func TestAuditServer_ResponseWithSchema(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /pets/{petId}:
    get:
      operationId: getPet
      parameters:
        - {name: petId, in: path, required: true, schema: {type: string}}
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema: {$ref: '#/components/schemas/Pet'}
        '404': {description: not found}
components:
  schemas:
    Pet:
      type: object
      properties:
        name: {type: string}
`)
	got := string(generateAuditClientFile(t, doc))

	assert.Contains(t, got, "type GetPetResponse200AuditData struct {")
	assert.Contains(t, got, "Payload any")
	assert.Contains(t, got, "func (resp *GetPetResponse) Response200AuditData() GetPetResponse200AuditData {")
	assert.Contains(t, got, "if resp.Response200 != nil {")
	assert.Contains(t, got, "am.Payload = resp.Response200.GetAuditData()")
	// 404 has no schema → no audit struct.
	assert.NotContains(t, got, "GetPetResponse404AuditData")
}

func TestAuditServer_ResponseWithHeaders(t *testing.T) {
	t.Parallel()

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
          headers:
            X-Total:
              schema: {type: integer}
          content:
            application/json:
              schema: {$ref: '#/components/schemas/Pet'}
components:
  schemas:
    Pet:
      type: object
      properties:
        name: {type: string}
`)
	got := string(generateAuditClientFile(t, doc))

	// Headers case → access via .Payload.
	assert.Contains(t, got, "if resp.Response200.Payload != nil {")
	assert.Contains(t, got, "am.Payload = resp.Response200.Payload.GetAuditData()")
}

func TestAuditServer_ResponseDefaultSkipped(t *testing.T) {
	t.Parallel()

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
        default:
          description: err
          content:
            application/json:
              schema: {$ref: '#/components/schemas/Error'}
components:
  schemas:
    Error:
      type: object
      properties:
        msg: {type: string}
`)
	got := string(generateAuditClientFile(t, doc))

	// default response → no audit struct.
	assert.NotContains(t, got, "ResponseDefaultAuditData")
}

// TestGenerate_AuditData_Compiles — end-to-end проверка, что сгенерированные
// audit-data файлы (model + interfaces/client) компилируются вместе.
func TestGenerate_AuditData_Compiles(t *testing.T) {
	if testing.Short() {
		t.Skip("compile test skipped in short mode")
	}

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go binary not in PATH, skipping compile test")
	}

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /pets/{petId}:
    get:
      operationId: getPet
      parameters:
        - {name: petId, in: path, required: true, schema: {type: string}}
        - {name: verbose, in: query, schema: {type: boolean}}
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema: {$ref: '#/components/schemas/Pet'}
        '404': {description: not found}
    put:
      operationId: updatePet
      parameters:
        - {name: petId, in: path, required: true, schema: {type: string}}
      requestBody:
        required: true
        content:
          application/json:
            schema: {$ref: '#/components/schemas/Pet'}
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema: {$ref: '#/components/schemas/Pet'}
components:
  schemas:
    Pet:
      type: object
      required: [name]
      properties:
        name: {type: string}
        secret: {type: string, x-sensitive: true}
        tag: {type: string, nullable: true}
`)
	files := generateFiles(t, doc)

	dir := t.TempDir()
	modelDir := filepath.Join(dir, "model")
	sensitiveDir := filepath.Join(dir, "pkg", "sensitive")
	optionalDir := filepath.Join(dir, "pkg", "optional")
	require.NoError(t, os.MkdirAll(modelDir, 0o755))
	require.NoError(t, os.MkdirAll(sensitiveDir, 0o755))
	require.NoError(t, os.MkdirAll(optionalDir, 0o755))

	// Generated audit-data code imports nschugorev/oapigenerator/pkg/sensitive
	// (hardcoded sensitivePkg in audit_model.go). Update<Name> structs use
	// pkg/optional. Temp module must match the real module path.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module nschugorev/oapigenerator\n\ngo 1.26\n"), 0o644))

	sensitiveSrc, err := os.ReadFile("../../pkg/sensitive/sensitive.go")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(sensitiveDir, "sensitive.go"),
		sensitiveSrc, 0o644))

	optionalSrc, err := os.ReadFile("../../pkg/optional/optional.go")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(optionalDir, "optional.go"),
		optionalSrc, 0o644))

	for name, content := range files {
		if !strings.HasPrefix(name, "model/") {
			continue
		}

		require.NoError(t, os.WriteFile(filepath.Join(modelDir, filepath.Base(name)), content, 0o644))
	}

	cmd := exec.Command("go", "build", "./model")
	cmd.Dir = dir

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated audit-data model package did not compile: %v\n--- output ---\n%s", err, out)
	}
}

func TestCrossServiceRef_GeneratesImport(t *testing.T) {
	const commonSpec = "/input/common/src/openapi/openapi.yaml"
	commonProject := &oapiparser.Project{
		Folder:       "common",
		ImportPrefix: "github.com/foo/bar/go/common",
	}

	userSchema := &oapiparser.Schema{
		Name:       "User",
		Type:       "object",
		SourceFile: commonSpec,
	}
	userSchema.Properties = []*oapiparser.Property{
		{Name: "id", Schema: &oapiparser.Schema{Type: "string"}},
	}

	commonProject.CreateModel(gogen.Import{
		Path:  "github.com/foo/bar/go/common/model",
		Alias: "model",
	})
	commonProject.CreatePaths("github.com/foo/bar/go/common")
	commonProject.Model.SetSchemas([]*oapiparser.Schema{userSchema})

	extRef := commonSpec + "#/components/schemas/User"
	wrapperSchema := &oapiparser.Schema{
		Name: "Wrapper",
		Type: "object",
		Properties: []*oapiparser.Property{
			{Name: "user", Schema: &oapiparser.Schema{ExternalRef: extRef}},
		},
	}

	userBackendProject := &oapiparser.Project{
		Folder:       "userBackend",
		ImportPrefix: "github.com/foo/bar/go/userBackend",
	}
	userBackendProject.CreateModel(gogen.Import{
		Path:  "github.com/foo/bar/go/userBackend/model",
		Alias: "model",
	})
	userBackendProject.CreatePaths("github.com/foo/bar/go/userBackend")
	userBackendProject.Model.SetSchemas([]*oapiparser.Schema{wrapperSchema})

	si := &oapiparser.SchemaIndex{
		Schemas: map[string]*oapiparser.SchemaEntry{
			commonSpec + "#/components/schemas/User": {
				Project:    commonProject,
				SchemaName: "User",
				GoImport:   "github.com/foo/bar/go/common",
				GoType:     "User",
			},
		},
	}

	g := &Generator{
		project:     userBackendProject,
		schemaIndex: si,
		factory:     gogen.NewFileFactory("oapigen"),
	}

	m := g.newTypeMapper("model")
	got := m.baseType(&oapiparser.Schema{ExternalRef: extRef})

	assert.Equal(t, "common.User", got)

	var foundImport bool
	for _, imp := range m.imports {
		if imp.Path == "github.com/foo/bar/go/common/model" && imp.Alias == "common" {
			foundImport = true

			break
		}
	}
	assert.True(t, foundImport, "must add import for common/model with alias 'common'")
}

func TestCrossServiceRef_NoSchemaIndex_FallbackToAny(t *testing.T) {
	project := &oapiparser.Project{Folder: "test"}
	project.CreateModel(gogen.Import{Alias: "model"})
	project.CreatePaths("")

	g := &Generator{
		project: project,
		factory: gogen.NewFileFactory("oapigen"),
	}

	m := g.newTypeMapper("model")
	got := m.baseType(&oapiparser.Schema{
		ExternalRef: "/input/common/src/openapi/openapi.yaml#/components/schemas/User",
	})

	assert.Equal(t, goTypeAny, got, "without SchemaIndex, external ref falls back to any")
}

func TestCrossServiceRef_SplitModeAddsSuffix(t *testing.T) {
	const commonSpec = "/input/common/src/openapi/openapi.yaml"
	commonProject := &oapiparser.Project{
		Folder:       "common",
		ImportPrefix: "github.com/foo/bar/go/common",
		Features: oapiparser.ProjectFeatures{
			SplitRequestResponse: oapiparser.ProjectFeature{Value: true},
		},
	}

	userSchema := &oapiparser.Schema{
		Name:       "User",
		Type:       "object",
		SourceFile: commonSpec,
	}
	userSchema.Properties = []*oapiparser.Property{
		{Name: "id", Schema: &oapiparser.Schema{Type: "string"}},
	}

	commonProject.CreateModel(gogen.Import{Alias: "model"})
	commonProject.CreatePaths("github.com/foo/bar/go/common")
	commonProject.Model.SetSchemas([]*oapiparser.Schema{userSchema})

	extRef := commonSpec + "#/components/schemas/User"

	userBackendProject := &oapiparser.Project{
		Folder:       "userBackend",
		ImportPrefix: "github.com/foo/bar/go/userBackend",
	}
	userBackendProject.CreateModel(gogen.Import{Alias: "model"})
	userBackendProject.CreatePaths("github.com/foo/bar/go/userBackend")

	si := &oapiparser.SchemaIndex{
		Schemas: map[string]*oapiparser.SchemaEntry{
			commonSpec + "#/components/schemas/User": {
				Project:    commonProject,
				SchemaName: "User",
				GoImport:   "github.com/foo/bar/go/common",
				GoType:     "User",
			},
		},
	}

	g := &Generator{
		project:     userBackendProject,
		schemaIndex: si,
		factory:     gogen.NewFileFactory("oapigen"),
	}

	m := g.newTypeMapper("model")
	m.mode = oapiparser.ModeRequest

	got := m.baseType(&oapiparser.Schema{ExternalRef: extRef})
	assert.Equal(t, "common.UserRequest", got,
		"owner project has split enabled → suffix added by LookupForMode")
}

func TestGenerate_UTCTimeFile_WhenFeatureOn_EmitsFile(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      properties:
        name: {type: string}
`)
	project := testProject(t, doc, "example.com/test")
	project.Features.UseUTCForDateTime.Value = true

	fw := &collectWriter{files: map[string][]byte{}}
	require.NoError(t, Generate(fw, project, nil))

	body := string(fw.files["model/utc_time.gen.go"])
	assert.Contains(t, body, "type UTCTime time.Time")
	assert.Contains(t, body, "func (u UTCTime) MarshalJSON() ([]byte, error) {")
	assert.Contains(t, body, "func (u *UTCTime) UnmarshalJSON(data []byte) error {")
}

func TestGenerate_UTCTimeFile_WhenFeatureOff_SkipsFile(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      properties:
        name: {type: string}
`)
	project := testProject(t, doc, "example.com/test")
	// UseUTCForDateTime.Value остаётся false.

	fw := &collectWriter{files: map[string][]byte{}}
	require.NoError(t, Generate(fw, project, nil))

	_, ok := fw.files["model/utc_time.gen.go"]
	assert.False(t, ok, "utc_time.gen.go must not be emitted when USE_UTC_FOR_DATE_TIME is off")
}

func TestGenerate_ExpectedValidatorsFile_WithNamedValidators_EmitsFile(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Item:
      type: object
      x-validations: ["app.ItemConsistency"]
      properties:
        name: {type: string}
`)
	project := testProject(t, doc, "example.com/test")

	fw := &collectWriter{files: map[string][]byte{}}
	require.NoError(t, Generate(fw, project, nil))

	body := string(fw.files["model/expected_validators.gen.go"])
	assert.Contains(t, body, "func ExpectedValidatorNames() []string {")
	assert.Contains(t, body, `"app.ItemConsistency"`)
}

func TestGenerate_ExpectedValidatorsFile_NoNamedValidators_SkipsFile(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Item:
      type: object
      properties:
        name: {type: string}
`)
	project := testProject(t, doc, "example.com/test")

	fw := &collectWriter{files: map[string][]byte{}}
	require.NoError(t, Generate(fw, project, nil))

	_, ok := fw.files["model/expected_validators.gen.go"]
	assert.False(t, ok, "expected_validators.gen.go must not be emitted when no named validators exist")
}

func TestGenerate_ClientInterfaceFile_EmitsFile(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /items:
    get:
      operationId: listItems
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                type: array
                items: {type: string}
`)
	project := testProject(t, doc, "example.com/test")

	fw := &collectWriter{files: map[string][]byte{}}
	require.NoError(t, Generate(fw, project, nil))

	body := string(fw.files["interfaces/client/client.gen.go"])
	assert.Contains(t, body, "type Client interface {")
	assert.Contains(t, body, "ListItems(ctx context.Context, req *ListItemsRequest) (*ListItemsResponse, error)")
	assert.Contains(t, body, "type ListItemsResponse struct {")
}

func TestGenerate_ServerInterfaceFile_EmitsFile(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /items:
    get:
      operationId: listItems
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                type: array
                items: {type: string}
`)
	project := testProject(t, doc, "example.com/test")

	fw := &collectWriter{files: map[string][]byte{}}
	require.NoError(t, Generate(fw, project, nil))

	body := string(fw.files["interfaces/server/server.gen.go"])
	assert.Contains(t, body, "type Server interface {")
	assert.Contains(t, body, "ListItems(ctx context.Context, req *client.ListItemsRequest) (*client.ListItemsResponse, error)")
}

func TestGenerate_ClientSugarFile_EmitsFile(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /items:
    get:
      operationId: listItems
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                type: array
                items: {type: string}
`)
	project := testProject(t, doc, "example.com/test")

	fw := &collectWriter{files: map[string][]byte{}}
	require.NoError(t, Generate(fw, project, nil))

	body := string(fw.files["interfaces/client/client_sugar.gen.go"])
	assert.Contains(t, body, "type ClientSugared struct {")
	assert.Contains(t, body, "func (x *ClientSugared) ListItems")
}
