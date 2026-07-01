package parser

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSchemaFromProxy_NilProxy(t *testing.T) {
	assert.Nil(t, schemaFromProxy(nil))
}

func TestRefToSchemaName_Empty(t *testing.T) {
	assert.Equal(t, "", refToSchemaName(""))
}

func TestRefToSchemaName_NoSlash(t *testing.T) {
	assert.Equal(t, "MySchema", refToSchemaName("MySchema"))
	assert.Equal(t, "Foo", refToSchemaName("#/components/schemas/Foo"))
}

func TestDecodeNode_Nil(t *testing.T) {
	assert.Nil(t, decodeNode(nil))
}

func TestDecodeNode_DecodeError(t *testing.T) {
	// Kind=0 — невалидный узел, yaml.v3 возвращает ошибку.
	n := &yaml.Node{Kind: 0, Tag: "!!str", Value: "x"}
	assert.Nil(t, decodeNode(n))
}

func TestExtractComponentsSchemas_NilMap(t *testing.T) {
	doc := &Document{}
	extractComponentsSchemas(doc, nil)
	assert.Empty(t, doc.Schemas)
}

func TestParseFile_NonexistentFile(t *testing.T) {
	fsys := fstest.MapFS{}
	_, err := ParseFile(fsys, "missing.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing.yaml")
}

func TestParse_BuildV3ModelErrors(t *testing.T) {
	// swagger 2.0 — валидный YAML, но BuildV3Model падает с "different version".
	_, err := Parse([]byte("swagger: '2.0'\ninfo:\n  title: x\n  version: '1'\npaths: {}\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "build v3 model")
}

func TestParse_DefaultResponse(t *testing.T) {
	spec := []byte(`
openapi: 3.0.3
info:
  title: t
  version: '1'
paths:
  /x:
    get:
      operationId: getX
      responses:
        default:
          description: any error
`)
	doc, err := Parse(spec)
	require.NoError(t, err)
	require.Len(t, doc.Operations, 1)
	require.Len(t, doc.Operations[0].Responses, 1)
	assert.Equal(t, "default", doc.Operations[0].Responses[0].StatusCode)
}

func TestParse_OperationWithoutResponses(t *testing.T) {
	spec := []byte(`
openapi: 3.0.3
info:
  title: t
  version: '1'
paths:
  /x:
    get:
      operationId: getX
`)
	doc, err := Parse(spec)
	require.NoError(t, err)
	require.Len(t, doc.Operations, 1)
	assert.Empty(t, doc.Operations[0].Responses)
}

func TestParse_OperationWithoutRequestBody(t *testing.T) {
	spec := []byte(`
openapi: 3.0.3
info:
  title: t
  version: '1'
paths:
  /x:
    post:
      operationId: postX
      responses:
        '200':
          description: ok
`)
	doc, err := Parse(spec)
	require.NoError(t, err)
	require.Len(t, doc.Operations, 1)
	assert.Nil(t, doc.Operations[0].RequestBody)
}

func TestParse_NoComponents(t *testing.T) {
	spec := []byte(`
openapi: 3.0.3
info:
  title: t
  version: '1'
paths: {}
`)
	doc, err := Parse(spec)
	require.NoError(t, err)
	assert.Empty(t, doc.Schemas)
}

func TestParse_WithLocation(t *testing.T) {
	// Покрывает ветку location != "" в parseBytes (BasePath).
	_, err := Parse([]byte("openapi: 3.0.3\ninfo:\n  title: t\n  version: '1'\npaths: {}\n"))
	// Вызовем parseBytes напрямую с location.
	_, err = parseBytes([]byte("openapi: 3.0.3\ninfo:\n  title: t\n  version: '1'\npaths: {}\n"), "spec.yaml")
	require.NoError(t, err)
}
