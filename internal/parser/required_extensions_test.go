package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParse_RequiredExtensions проверяет, что парсер читает
// x-request-required / x-response-required из spec и заполняет
// Schema.RequestRequired/ResponseRequired и соответствующие поля Property.
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
      x-request-required: [id, name]
      x-response-required: [id]
      properties:
        id: {type: integer, format: int64}
        name: {type: string}
        label: {type: string}
`)

	pet := findSchema(t, doc, "Pet")
	assert.Equal(t, []string{"id", "name"}, pet.RequestRequired)
	assert.Equal(t, []string{"id"}, pet.ResponseRequired)

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
// RequestRequired/ResponseRequired на Schema и Property остаются nil/false.
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
	assert.Nil(t, pet.RequestRequired)
	assert.Nil(t, pet.ResponseRequired)

	idProp := findProperty(t, pet, "id")
	assert.True(t, idProp.Required)
	assert.False(t, idProp.RequestRequired)
	assert.False(t, idProp.ResponseRequired)
}

// TestReadRequiredExtension_NonSequence проверяет, что если значение
// расширения — не sequence-узел (например, scalar), возвращается nil.
func TestReadRequiredExtension_NonSequence(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      x-request-required: not-a-list
      properties:
        id: {type: integer}
`)

	pet := findSchema(t, doc, "Pet")
	assert.Nil(t, pet.RequestRequired, "non-sequence extension must be ignored")
}

// TestParse_OptionalExtension проверяет, что парсер читает x-optional
// из spec и заполняет Schema.Optional и Property.Optional.
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
      x-optional: [name, label]
      properties:
        id: {type: integer}
        name: {type: string}
        label: {type: string}
`)

	pet := findSchema(t, doc, "Pet")
	assert.Equal(t, []string{"name", "label"}, pet.Optional)

	idProp := findProperty(t, pet, "id")
	assert.False(t, idProp.Optional, "id must NOT be Optional")

	nameProp := findProperty(t, pet, "name")
	assert.True(t, nameProp.Optional, "name must be Optional")

	labelProp := findProperty(t, pet, "label")
	assert.True(t, labelProp.Optional, "label must be Optional")
}

// TestParse_OptionalExtension_Absent проверяет, что без x-optional
// Schema.Optional и Property.Optional остаются nil/false.
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
	assert.Nil(t, pet.Optional)

	idProp := findProperty(t, pet, "id")
	assert.False(t, idProp.Optional)
}

func parseSpec(t *testing.T, spec string) *Document {
	t.Helper()
	doc, err := Parse([]byte(spec))
	require.NoError(t, err)

	return doc
}
