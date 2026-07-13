package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParse_RequiredExtensions проверяет, что парсер читает
// x-request-required / x-response-required из per-property расширений и
// заполняет Property.RequestRequired/ResponseRequired.
func TestParse_RequiredExtensions(t *testing.T) {
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
        id: {type: integer, format: int64, x-request-required: true, x-response-required: true}
        name: {type: string, x-request-required: true}
        label: {type: string}
`)

	pet := findSchema(t, doc, "Pet")

	idProp := findProperty(t, pet, "id")
	assert.True(t, idProp.Required, "id must be Required (standard OAS required)")
	assert.True(t, idProp.RequestRequired, "id must be RequestRequired")
	assert.True(t, idProp.ResponseRequired, "id must be ResponseRequired")

	nameProp := findProperty(t, pet, "name")
	assert.False(t, nameProp.Required, "name must NOT be Required (not in standard required)")
	assert.True(t, nameProp.RequestRequired, "name must be RequestRequired")
	assert.False(t, nameProp.ResponseRequired, "name must NOT be ResponseRequired")

	labelProp := findProperty(t, pet, "label")
	assert.False(t, labelProp.Required)
	assert.False(t, labelProp.RequestRequired)
	assert.False(t, labelProp.ResponseRequired)
}

// TestParse_RequiredExtensions_Absent проверяет, что без расширений
// RequestRequired/ResponseRequired на Property остаются false.
func TestParse_RequiredExtensions_Absent(t *testing.T) {
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
        id: {type: integer}
`)

	pet := findSchema(t, doc, "Pet")
	idProp := findProperty(t, pet, "id")
	assert.True(t, idProp.Required)
	assert.False(t, idProp.RequestRequired)
	assert.False(t, idProp.ResponseRequired)
}

// TestReadBoolExtension_NonScalar проверяет, что если значение
// расширения — не scalar-узел (например, sequence), оно игнорируется.
func TestReadBoolExtension_NonScalar(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      properties:
        id: {type: integer, x-optional: [not-a-bool]}
`)

	pet := findSchema(t, doc, "Pet")
	idProp := findProperty(t, pet, "id")
	assert.False(t, idProp.Optional, "non-scalar extension must be ignored")
}

// TestParse_OptionalExtension проверяет, что парсер читает x-optional: true
// из per-property расширений и заполняет Property.Optional.
func TestParse_OptionalExtension(t *testing.T) {
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
        id: {type: integer}
        name: {type: string, x-optional: true}
        label: {type: string, x-optional: true}
`)

	pet := findSchema(t, doc, "Pet")

	idProp := findProperty(t, pet, "id")
	assert.False(t, idProp.Optional, "id must NOT be Optional")

	nameProp := findProperty(t, pet, "name")
	assert.True(t, nameProp.Optional, "name must be Optional")

	labelProp := findProperty(t, pet, "label")
	assert.True(t, labelProp.Optional, "label must be Optional")
}

// TestParse_OptionalExtension_Absent проверяет, что без x-optional
// Property.Optional остаётся false.
func TestParse_OptionalExtension_Absent(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      properties:
        id: {type: integer}
`)

	pet := findSchema(t, doc, "Pet")
	idProp := findProperty(t, pet, "id")
	assert.False(t, idProp.Optional)
}

// TestParse_OptionalExtension_ExplicitFalse проверяет, что x-optional: false
// трактуется как absence (Optional=false).
func TestParse_OptionalExtension_ExplicitFalse(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      properties:
        id: {type: integer, x-optional: false}
`)

	pet := findSchema(t, doc, "Pet")
	idProp := findProperty(t, pet, "id")
	assert.False(t, idProp.Optional, "explicit x-optional: false must yield Optional=false")
}

// TestParse_SensitiveExtension проверяет, что парсер читает x-sensitive: true
// из per-property расширений и заполняет Property.Sensitive.
func TestParse_SensitiveExtension(t *testing.T) {
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
        id: {type: integer}
        plaintext: {type: string, format: binary, x-sensitive: true}
        secret: {type: string, x-sensitive: true}
`)

	pet := findSchema(t, doc, "Pet")

	idProp := findProperty(t, pet, "id")
	assert.False(t, idProp.Sensitive, "id must NOT be Sensitive")

	plaintextProp := findProperty(t, pet, "plaintext")
	assert.True(t, plaintextProp.Sensitive, "plaintext must be Sensitive")

	secretProp := findProperty(t, pet, "secret")
	assert.True(t, secretProp.Sensitive, "secret must be Sensitive")
}

// TestParse_SensitiveExtension_Absent проверяет, что без x-sensitive
// Property.Sensitive остаётся false.
func TestParse_SensitiveExtension_Absent(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      properties:
        id: {type: integer}
`)

	pet := findSchema(t, doc, "Pet")
	idProp := findProperty(t, pet, "id")
	assert.False(t, idProp.Sensitive)
}

func parseSpec(t *testing.T, spec string) *Document {
	t.Helper()
	doc, err := Parse([]byte(spec))
	require.NoError(t, err)

	return doc
}
