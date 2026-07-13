package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMarkUpdateSchemas_PUT помечает схему из PUT request body и её
// свойства. ReadOnly и Immutable (кроме name) пропускаются.
func TestMarkUpdateSchemas_PUT(t *testing.T) {
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
        kind: {type: string, x-validations: [Immutable]}
        label: {type: string}
`)

	pet := findSchema(t, doc, "Pet")
	assert.True(t, pet.IsUsedInUpdate, "Pet must be marked IsUsedInUpdate (PUT body)")

	// ReadOnly пропускается.
	idProp := findProperty(t, pet, "id")
	assert.False(t, idProp.IsUsedInUpdate, "readOnly id must be skipped")

	// Immutable без name — пропускается.
	tagProp := findProperty(t, pet, "tag")
	assert.False(t, tagProp.IsUsedInUpdate, "Immutable tag must be skipped")

	kindProp := findProperty(t, pet, "kind")
	assert.False(t, kindProp.IsUsedInUpdate, "Immutable kind must be skipped")

	// name сохраняется, даже если НЕ Immutable (это просто regular поле).
	nameProp := findProperty(t, pet, "name")
	assert.True(t, nameProp.IsUsedInUpdate, "name must be marked")

	// regular поле — помечается.
	labelProp := findProperty(t, pet, "label")
	assert.True(t, labelProp.IsUsedInUpdate, "label must be marked")
}

// TestMarkUpdateSchemas_PATCH помечает схему из PATCH request body.
func TestMarkUpdateSchemas_PATCH(t *testing.T) {
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

	pet := findSchema(t, doc, "Pet")
	assert.True(t, pet.IsUsedInUpdate, "Pet must be marked IsUsedInUpdate (PATCH body)")
	nameProp := findProperty(t, pet, "name")
	assert.True(t, nameProp.IsUsedInUpdate)
}

// TestMarkUpdateSchemas_ImmutableNameKept проверяет, что поле с именем "name"
// помечается, даже если оно Immutable.
func TestMarkUpdateSchemas_ImmutableNameKept(t *testing.T) {
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

	pet := findSchema(t, doc, "Pet")
	nameProp := findProperty(t, pet, "name")
	assert.True(t, nameProp.IsUsedInUpdate, "Immutable name must be kept (not skipped)")
	assert.True(t, nameProp.Immutable, "name must be Immutable")
}

// TestMarkUpdateSchemas_GETNotMarked проверяет, что GET/POST/DELETE не
// помечают схему.
func TestMarkUpdateSchemas_GETNotMarked(t *testing.T) {
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
    Pets:
      type: array
      items: {$ref: '#/components/schemas/Pet'}
`)

	pet := findSchema(t, doc, "Pet")
	assert.False(t, pet.IsUsedInUpdate, "Pet must NOT be marked (POST body, not PUT/PATCH)")

	pets := findSchema(t, doc, "Pets")
	assert.False(t, pets.IsUsedInUpdate, "Pets must NOT be marked (GET response)")

	for _, p := range pet.Properties {
		assert.False(t, p.IsUsedInUpdate, "Pet properties must NOT be marked")
	}
}

// TestMarkUpdateSchemas_NoRequestBody проверяет, что PUT без body ничего
// не ломает.
func TestMarkUpdateSchemas_NoRequestBody(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /pets/{id}:
    put:
      operationId: updatePetNoBody
      responses:
        '200': {description: ok}
components:
  schemas:
    Pet:
      type: object
      properties:
        name: {type: string}
`)

	pet := findSchema(t, doc, "Pet")
	assert.False(t, pet.IsUsedInUpdate, "Pet must NOT be marked (PUT without body)")
}

// TestMarkUpdateSchemas_BodyInlineSchema проверяет, что inline-схема в body
// (без $ref) не падает и ничего не помечает — v1 поддерживает только $ref.
func TestMarkUpdateSchemas_BodyInlineSchema(t *testing.T) {
	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /pets:
    put:
      operationId: updateInline
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                name: {type: string}
      responses:
        '200': {description: ok}
components:
  schemas:
    Pet:
      type: object
      properties:
        name: {type: string}
`)

	// Inline-схема не в doc.Schemas, поэтому не помечается. Pet тоже не
	// помечается — на него нет $ref из PUT body.
	pet := findSchema(t, doc, "Pet")
	assert.False(t, pet.IsUsedInUpdate, "Pet must NOT be marked (inline body, no $ref to Pet)")
}

func TestMarkUpdateSchemas_MultipleBodies(t *testing.T) {
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
  /owners/{id}:
    patch:
      operationId: patchOwner
      requestBody:
        required: true
        content:
          application/json:
            schema: {$ref: '#/components/schemas/Owner'}
      responses:
        '200': {description: ok}
components:
  schemas:
    Pet:
      type: object
      properties:
        name: {type: string}
    Owner:
      type: object
      properties:
        name: {type: string}
`)

	pet := findSchema(t, doc, "Pet")
	owner := findSchema(t, doc, "Owner")
	assert.True(t, pet.IsUsedInUpdate, "Pet must be marked (PUT body)")
	assert.True(t, owner.IsUsedInUpdate, "Owner must be marked (PATCH body)")
}
