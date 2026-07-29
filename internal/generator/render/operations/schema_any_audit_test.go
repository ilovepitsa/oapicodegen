package operations

import (
	"testing"

	"github.com/ilovepitsa/oapicodegen/internal/codegen"
	"github.com/ilovepitsa/oapicodegen/internal/generator/render"
	"github.com/ilovepitsa/oapicodegen/internal/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsAnyType(t *testing.T) {
	t.Parallel()

	cases := map[string]bool{
		"any":            true,
		"*any":           true,
		"[]any":          true,
		"map[string]any": true,
		"map[string]int": false,
		"[]int":          false,
		"string":         false,
		"*string":        false,
		"Pet":            false,
	}
	for typ, want := range cases {
		assert.Equal(t, want, isAnyType(typ), typ)
	}
}

func TestReportIfAny_AppendsDiagnostic(t *testing.T) {
	t.Parallel()

	col := render.NewCollector()
	reportIfAny(col, "schemas.P.tag", "any", "empty schema {}")
	got := col.Drain()
	assert.Len(t, got, 1)
	assert.Equal(t, "schemas.P.tag", got[0].Location)
	assert.Contains(t, got[0].Reason, "empty schema")
}

func TestReportIfAny_NoOpOnConcreteType(t *testing.T) {
	t.Parallel()

	col := render.NewCollector()
	reportIfAny(col, "schemas.P.tag", "string", "")
	assert.Empty(t, col.Drain())
}

func TestReportIfAny_NilCollectorNoOp(t *testing.T) {
	t.Parallel()

	// не паникует при nil-коллекторе
	reportIfAny(nil, "x", "any", "r")
}

func TestReportPayloadIfAny_ExemptsExplicitAdditionalProperties(t *testing.T) {
	t.Parallel()

	col := render.NewCollector()
	s := &parser.Schema{Type: "object", AdditionalProperties: &parser.Schema{Type: "string"}}
	reportPayloadIfAny(col, "p", s, "map[string]any")
	assert.Empty(t, col.Drain(), "explicit additionalProperties is exempt even if type is any")
}

func TestReportPayloadIfAny_ExemptsAdditionalPropertiesTrue(t *testing.T) {
	t.Parallel()

	// additionalProperties: true парсится как AdditionalProperties: &Schema{} (non-nil).
	col := render.NewCollector()
	s := &parser.Schema{Type: "object", AdditionalProperties: &parser.Schema{}}
	reportPayloadIfAny(col, "p", s, "map[string]any")
	assert.Empty(t, col.Drain(), "additionalProperties: true is exempt")
}

func TestReportPayloadIfAny_ExemptsAdditionalPropertiesFalse(t *testing.T) {
	t.Parallel()

	col := render.NewCollector()
	s := &parser.Schema{Type: "object", AdditionalPropertiesFalse: true}
	reportPayloadIfAny(col, "p", s, "map[string]any")
	assert.Empty(t, col.Drain(), "additionalProperties: false (closed struct) is exempt")
}

func TestReportPayloadIfAny_TriggersOnEmptySchema(t *testing.T) {
	t.Parallel()

	col := render.NewCollector()
	s := &parser.Schema{Type: "object"} // empty object, no AP
	reportPayloadIfAny(col, "p", s, "map[string]any")
	got := col.Drain()
	assert.Len(t, got, 1)
	assert.Contains(t, got[0].Reason, "empty schema")
}

func TestReportPayloadIfAny_TriggersOnNilSchema(t *testing.T) {
	t.Parallel()

	col := render.NewCollector()
	reportPayloadIfAny(col, "p", nil, "any")
	got := col.Drain()
	assert.Len(t, got, 1)
	assert.Contains(t, got[0].Reason, "nil schema")
}

func TestReportPayloadIfAny_TriggersOnExternalRef(t *testing.T) {
	t.Parallel()

	col := render.NewCollector()
	s := &parser.Schema{ExternalRef: "/x.yaml#/components/schemas/Foo"}
	reportPayloadIfAny(col, "p", s, "any")
	got := col.Drain()
	assert.Len(t, got, 1)
	assert.Contains(t, got[0].Reason, "unresolved external $ref")
}

func TestReportPayloadIfAny_NoOpOnConcreteType(t *testing.T) {
	t.Parallel()

	col := render.NewCollector()
	s := &parser.Schema{Type: "string"}
	reportPayloadIfAny(col, "p", s, "string")
	assert.Empty(t, col.Drain())
}

func TestReportPayloadIfAny_NilCollectorNoOp(t *testing.T) {
	t.Parallel()

	// не паникует при nil-коллекторе
	reportPayloadIfAny(nil, "p", &parser.Schema{Type: "object"}, "any")
}

func TestOpLocation(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "paths.get./pets.responses.200", opLocation("get", "/pets", "200"))
	assert.Equal(t, "paths.post./pets/{id}.responses.default", opLocation("post", "/pets/{id}", "default"))
}

// bodyAnyTypeMapper — TypeMapper, эмулирующий поведение реального typeMapper
// для request body: пустой объект (schema {}, Type=="object" без Properties и
// AdditionalProperties) → "map[string]any"; явный additionalProperties →
// "map[string]any" (но аудит различает их по *parser.Schema); прочее — как в
// mockTypeMapper. Используется для проверки, что renderBodyField реально зовёт
// reportPayloadIfAny при разрешении body в any-тип.
type bodyAnyTypeMapper struct{ mode string }

func (m *bodyAnyTypeMapper) GoType(s *parser.Schema) string {
	if s == nil {
		return "any"
	}
	if s.Type == "object" && len(s.Properties) == 0 {
		if s.AdditionalPropertiesFalse {
			return "struct{}"
		}
		if s.AdditionalProperties != nil {
			return "map[string]any"
		}
		return "map[string]any"
	}
	switch s.Type {
	case "integer":
		return "int"
	case "number":
		return "float64"
	case "boolean":
		return "bool"
	case "string":
		return "string"
	}
	return "any"
}

func (m *bodyAnyTypeMapper) SetMode(mode string) { m.mode = mode }
func (m *bodyAnyTypeMapper) Mode() string        { return m.mode }

// TestRenderBodyField_AuditsEmptyObjectBody проверяет, что renderBodyField
// вызывает reportPayloadIfAny для request body со схемой {} (пустой объект без
// Properties и AdditionalProperties), разрешающейся в map[string]any.
func TestRenderBodyField_AuditsEmptyObjectBody(t *testing.T) {
	t.Parallel()

	w := codegen.NewBufferWriter()
	col := render.NewCollector()
	op := &parser.Method{Method: "post", Path: "/items"}
	rb := &parser.RequestBody{
		Required: true,
		Content: map[string]*parser.MediaType{
			"application/json": {Schema: &parser.Schema{Type: "object"}},
		},
	}

	renderBodyField(w, op, rb, &bodyAnyTypeMapper{}, col)

	got := col.Drain()
	require.Len(t, got, 1, "empty object body schema must produce a diagnostic")
	assert.Equal(t, "paths.post./items.requestBody", got[0].Location)
	assert.Contains(t, got[0].Reason, "empty schema")
}

// TestRenderBodyField_ExemptsExplicitAdditionalProperties проверяет, что
// явный additionalProperties не триггерит аудит, даже если GoType —
// map[string]any.
func TestRenderBodyField_ExemptsExplicitAdditionalProperties(t *testing.T) {
	t.Parallel()

	w := codegen.NewBufferWriter()
	col := render.NewCollector()
	op := &parser.Method{Method: "post", Path: "/items"}
	rb := &parser.RequestBody{
		Required: true,
		Content: map[string]*parser.MediaType{
			"application/json": {Schema: &parser.Schema{
				Type:                 "object",
				AdditionalProperties: &parser.Schema{Type: "string"},
			}},
		},
	}

	renderBodyField(w, op, rb, &bodyAnyTypeMapper{}, col)
	assert.Empty(t, col.Drain(), "explicit additionalProperties body must be exempt")
}

// TestRenderBodyField_NoAuditOnConcreteBody проверяет, что конкретный тип body
// не порождает diagnostic.
func TestRenderBodyField_NoAuditOnConcreteBody(t *testing.T) {
	t.Parallel()

	w := codegen.NewBufferWriter()
	col := render.NewCollector()
	op := &parser.Method{Method: "post", Path: "/items"}
	rb := &parser.RequestBody{
		Required: true,
		Content: map[string]*parser.MediaType{
			"application/json": {Schema: &parser.Schema{Type: "string"}},
		},
	}

	renderBodyField(w, op, rb, &bodyAnyTypeMapper{}, col)
	assert.Empty(t, col.Drain(), "concrete body type must not produce a diagnostic")
}

// TestRenderBodyField_NilCollectorNoOp проверяет, что nil-коллектор не паникует.
func TestRenderBodyField_NilCollectorNoOp(t *testing.T) {
	t.Parallel()

	w := codegen.NewBufferWriter()
	op := &parser.Method{Method: "post", Path: "/items"}
	rb := &parser.RequestBody{
		Required: true,
		Content: map[string]*parser.MediaType{
			"application/json": {Schema: &parser.Schema{Type: "object"}},
		},
	}

	assert.NotPanics(t, func() {
		renderBodyField(w, op, rb, &bodyAnyTypeMapper{}, nil)
	})
}
