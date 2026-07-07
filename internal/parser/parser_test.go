package parser

import (
	"os"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_PetstoreInfo(t *testing.T) {
	doc := parsePetstore(t)

	assert.Equal(t, "3.0.3", doc.OpenAPI)
	assert.Equal(t, "Petstore", doc.Info.Title)
	assert.Equal(t, "1.0.0", doc.Info.Version)
	require.Len(t, doc.Servers, 1)
	assert.Equal(t, "https://api.example.com/v1", doc.Servers[0].URL)
}

func TestParse_Schemas(t *testing.T) {
	doc := parsePetstore(t)

	names := make([]string, 0, len(doc.Schemas))
	for _, s := range doc.Schemas {
		names = append(names, s.Name)
	}
	assert.ElementsMatch(
		t,
		[]string{"Pet", "Error", "PetCollection", "AnyPet", "OnePet", "MergedPet"},
		names,
	)
}

func TestParse_PetSchemaFields(t *testing.T) {
	doc := parsePetstore(t)

	pet := findSchema(t, doc, "Pet")
	assert.Equal(t, "object", pet.Type)
	assert.ElementsMatch(t, []string{"id", "name"}, pet.Required)

	propNames := make([]string, 0, len(pet.Properties))
	for _, p := range pet.Properties {
		propNames = append(propNames, p.Name)
	}
	assert.ElementsMatch(t, []string{"id", "name", "tag", "status"}, propNames)

	idProp := findProperty(t, pet, "id")
	assert.Equal(t, "integer", idProp.Schema.Type)
	assert.Equal(t, "int64", idProp.Schema.Format)
	assert.True(t, idProp.Schema.ReadOnly)
	assert.True(t, idProp.Required)

	statusProp := findProperty(t, pet, "status")
	assert.Equal(t, []any{"active", "inactive", "archived"}, statusProp.Schema.Enum)
	assert.Equal(t, "active", statusProp.Schema.Default)

	tagProp := findProperty(t, pet, "tag")
	assert.True(t, tagProp.Schema.Nullable)
}

func TestParse_ArraySchema(t *testing.T) {
	doc := parsePetstore(t)

	coll := findSchema(t, doc, "PetCollection")
	assert.Equal(t, "array", coll.Type)
	require.NotNil(t, coll.Items)
	assert.Equal(t, "Pet", coll.Items.Name)
	assert.Equal(t, "#/components/schemas/Pet", coll.Items.Ref)
}

func TestParse_CompositeSchemas(t *testing.T) {
	doc := parsePetstore(t)

	anyPet := findSchema(t, doc, "AnyPet")
	require.Len(t, anyPet.AnyOf, 2)
	assert.Equal(t, "Pet", anyPet.AnyOf[0].Name)

	onePet := findSchema(t, doc, "OnePet")
	require.Len(t, onePet.OneOf, 2)
	assert.Equal(t, "Pet", onePet.OneOf[0].Name)

	merged := findSchema(t, doc, "MergedPet")
	require.Len(t, merged.AllOf, 2)
	assert.Equal(t, "Pet", merged.AllOf[0].Name)
	assert.Equal(t, "object", merged.AllOf[1].Type)
}

func TestParse_PathsAndOperations(t *testing.T) {
	doc := parsePetstore(t)

	require.Len(t, doc.Paths, 2)
	assert.Equal(t, "/pets", doc.Paths[0].Path)
	assert.Equal(t, "/pets/{petId}", doc.Paths[1].Path)

	ops := doc.Paths[0].Operations
	require.Len(t, ops, 2)
	assert.Equal(t, "GET", ops[0].Method)
	assert.Equal(t, "listPets", ops[0].OperationID)
	assert.Equal(t, "POST", ops[1].Method)
	assert.Equal(t, "createPet", ops[1].OperationID)
}

func TestParse_OperationParameters(t *testing.T) {
	doc := parsePetstore(t)

	listPets := findOperation(t, doc, "listPets")
	require.Len(t, listPets.Parameters, 2)

	limit := listPets.Parameters[0]
	assert.Equal(t, "limit", limit.Name)
	assert.Equal(t, "query", limit.In)
	assert.False(t, limit.Required)
	assert.Equal(t, "integer", limit.Schema.Type)
	assert.Equal(t, "int32", limit.Schema.Format)
	assert.Equal(t, 20, limit.Schema.Default)

	headerParam := listPets.Parameters[1]
	assert.Equal(t, "X-Request-Id", headerParam.Name)
	assert.Equal(t, "header", headerParam.In)
}

func TestParse_OperationRequestBody(t *testing.T) {
	doc := parsePetstore(t)

	createPet := findOperation(t, doc, "createPet")
	require.NotNil(t, createPet.RequestBody)
	assert.True(t, createPet.RequestBody.Required)
	require.Contains(t, createPet.RequestBody.Content, "application/json")
	require.NotNil(t, createPet.RequestBody.Content["application/json"].Schema)
	assert.Equal(t, "Pet", createPet.RequestBody.Content["application/json"].Schema.Name)
}

func TestParse_OperationResponses(t *testing.T) {
	doc := parsePetstore(t)

	listPets := findOperation(t, doc, "listPets")
	require.Len(t, listPets.Responses, 2)

	var resp200 *Response
	var resp4xx *Response
	for _, r := range listPets.Responses {
		if r.StatusCode == "200" { //nolint:usestdlibvars // StatusCode — строковое поле, http.StatusOK не подходит (int)
			resp200 = r
		} else {
			resp4xx = r
		}
	}
	require.NotNil(t, resp200)
	require.NotNil(t, resp4xx)

	require.Contains(t, resp200.Content, "application/json")
	assert.Equal(t, "array", resp200.Content["application/json"].Schema.Type)
	assert.Equal(t, "Pet", resp200.Content["application/json"].Schema.Items.Name)

	require.Contains(t, resp200.Headers, "X-Total-Count")
	assert.Equal(t, "integer", resp200.Headers["X-Total-Count"].Schema.Type)

	require.Contains(t, resp4xx.Content, "application/json")
	assert.Equal(t, "Error", resp4xx.Content["application/json"].Schema.Name)
}

func TestParse_DeletedOperationDeprecated(t *testing.T) {
	doc := parsePetstore(t)

	del := findOperation(t, doc, "deletePet")
	assert.True(t, del.Deprecated)
	assert.Len(t, del.Responses, 1)
	assert.Equal(t, "204", del.Responses[0].StatusCode)
	assert.Empty(t, del.Responses[0].Content)
}

func TestParse_PathParameterRequired(t *testing.T) {
	doc := parsePetstore(t)

	show := findOperation(t, doc, "showPetById")
	require.Len(t, show.Parameters, 1)
	p := show.Parameters[0]
	assert.Equal(t, "petId", p.Name)
	assert.Equal(t, "path", p.In)
	assert.True(t, p.Required)
}

func TestParseFile_FromMapFS(t *testing.T) {
	data, err := os.ReadFile("testdata/petstore.yaml")
	require.NoError(t, err)

	fsys := fstest.MapFS{
		"spec.yaml": &fstest.MapFile{Data: data},
	}
	doc, err := ParseFile(fsys, "spec.yaml")
	require.NoError(t, err)
	assert.Equal(t, "Petstore", doc.Info.Title)
}

func TestParse_InvalidYAML_ReturnsError(t *testing.T) {
	_, err := Parse([]byte("openapi: 3.0.3\n  bad: indent: here\n"))
	require.Error(t, err)
}

func TestParse_EmptyDocument(t *testing.T) {
	doc, err := Parse([]byte("openapi: 3.0.3\ninfo:\n  title: x\n  version: '1'\n"))
	require.NoError(t, err)
	assert.Equal(t, "3.0.3", doc.OpenAPI)
	assert.Empty(t, doc.Paths)
	assert.Empty(t, doc.Schemas)
}

func parsePetstore(t *testing.T) *Document {
	t.Helper()
	data, err := os.ReadFile("testdata/petstore.yaml")
	require.NoError(t, err)
	doc, err := Parse(data)
	require.NoError(t, err)

	return doc
}

func findSchema(t *testing.T, doc *Document, name string) *Schema {
	t.Helper()
	for _, s := range doc.Schemas {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("schema %q not found", name)

	return nil
}

func findProperty(t *testing.T, s *Schema, name string) *Property {
	t.Helper()
	for _, p := range s.Properties {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("property %q not found", name)

	return nil
}

func findOperation(t *testing.T, doc *Document, id string) *Operation {
	t.Helper()
	for _, op := range doc.Operations {
		if op.OperationID == id {
			return op
		}
	}
	t.Fatalf("operation %q not found", id)

	return nil
}
