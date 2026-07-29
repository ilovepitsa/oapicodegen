package schema

import (
	"testing"

	"github.com/ilovepitsa/oapicodegen/internal/generator/render"
	"github.com/ilovepitsa/oapicodegen/internal/parser"
	"github.com/stretchr/testify/assert"
)

func TestReportSchemaAny_AppendsOnAnyWithEmptySchema(t *testing.T) {
	t.Parallel()

	col := render.NewCollector()
	s := &parser.Schema{Type: "object"} // empty object, no AP
	reportSchemaAny(col, "components.schemas.P.properties.x", s, "map[string]any")
	got := col.Drain()
	assert.Len(t, got, 1)
	assert.Equal(t, "components.schemas.P.properties.x", got[0].Location)
	assert.Contains(t, got[0].Reason, "empty schema")
}

func TestReportSchemaAny_AppendsOnAnyWithNilSchema(t *testing.T) {
	t.Parallel()

	col := render.NewCollector()
	reportSchemaAny(col, "components.schemas.P.properties.x", nil, "any")
	got := col.Drain()
	assert.Len(t, got, 1)
	assert.Contains(t, got[0].Reason, "nil schema")
}

func TestReportSchemaAny_ExemptsExplicitAdditionalProperties(t *testing.T) {
	t.Parallel()

	col := render.NewCollector()
	s := &parser.Schema{Type: "object", AdditionalProperties: &parser.Schema{Type: "string"}}
	reportSchemaAny(col, "p", s, "map[string]any")
	assert.Empty(t, col.Drain(), "explicit additionalProperties is exempt even if type is any")
}

func TestReportSchemaAny_ExemptsAdditionalPropertiesTrue(t *testing.T) {
	t.Parallel()

	// additionalProperties: true парсится как AdditionalProperties: &Schema{} (non-nil).
	col := render.NewCollector()
	s := &parser.Schema{Type: "object", AdditionalProperties: &parser.Schema{}}
	reportSchemaAny(col, "p", s, "map[string]any")
	assert.Empty(t, col.Drain(), "additionalProperties: true is exempt")
}

func TestReportSchemaAny_ExemptsAdditionalPropertiesFalse(t *testing.T) {
	t.Parallel()

	col := render.NewCollector()
	s := &parser.Schema{Type: "object", AdditionalPropertiesFalse: true}
	reportSchemaAny(col, "p", s, "map[string]any")
	assert.Empty(t, col.Drain(), "additionalProperties: false (closed struct) is exempt")
}

func TestReportSchemaAny_NoOpOnConcreteType(t *testing.T) {
	t.Parallel()

	col := render.NewCollector()
	s := &parser.Schema{Type: "string"}
	reportSchemaAny(col, "p", s, "string")
	assert.Empty(t, col.Drain())
}

func TestReportSchemaAny_NilCollectorNoOp(t *testing.T) {
	t.Parallel()

	// не паникует при nil-коллекторе
	reportSchemaAny(nil, "p", &parser.Schema{Type: "object"}, "any")
}

func TestAnyReason_NilSchema(t *testing.T) {
	t.Parallel()

	r := anyReason(nil, "any")
	assert.Contains(t, r, "nil schema")
}

func TestAnyReason_ExternalRef(t *testing.T) {
	t.Parallel()

	s := &parser.Schema{ExternalRef: "/x.yaml#/components/schemas/Foo"}
	r := anyReason(s, "any")
	assert.Contains(t, r, "unresolved external $ref")
	assert.Contains(t, r, "/x.yaml#/components/schemas/Foo")
}

func TestAnyReason_EmptyObject(t *testing.T) {
	t.Parallel()

	s := &parser.Schema{Type: "object"}
	r := anyReason(s, "map[string]any")
	assert.Contains(t, r, "empty schema {}")
}

func TestAnyReason_UnknownFallback(t *testing.T) {
	t.Parallel()

	// non-empty schema without distinguishing markers → fallback reason
	s := &parser.Schema{Type: "string"}
	r := anyReason(s, "any")
	assert.Contains(t, r, "unknown/union fallback")
}

func TestSchemaFieldLocation_WithStructName(t *testing.T) {
	t.Parallel()

	p := &parser.Property{Name: "tag"}
	loc := schemaFieldLocation("Pet", p)
	assert.Equal(t, "components.schemas.Pet.properties.tag", loc)
}

func TestSchemaFieldLocation_WithoutStructName(t *testing.T) {
	t.Parallel()

	p := &parser.Property{Name: "tag"}
	loc := schemaFieldLocation("", p)
	assert.Equal(t, "schema.properties.tag", loc)
}

func TestSchemaFieldLocation_NilProperty(t *testing.T) {
	t.Parallel()

	loc := schemaFieldLocation("Pet", nil)
	assert.Equal(t, "schema.field", loc)
}
