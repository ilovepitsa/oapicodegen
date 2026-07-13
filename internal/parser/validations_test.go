package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Unit tests for parseValidationRule ---

func TestParseValidationRule_SimpleNumeric(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input  string
		target Target
		op     Operator
		value  float64
	}{
		{">0", TargetValue, OpGT, 0},
		{">=1", TargetValue, OpGE, 1},
		{"<100", TargetValue, OpLT, 100},
		{"<=50", TargetValue, OpLE, 50},
		{"==42", TargetValue, OpEQ, 42},
		{"!=0", TargetValue, OpNE, 0},
		{"-5", TargetValue, OpLT, -5}, // не валидный — нет оператора
	}
	_ = cases // -5 ниже проверим отдельно

	rule, ok := parseValidationRule(">0")
	require.True(t, ok)
	sr, ok := rule.(SimpleRule)
	require.True(t, ok)
	assert.Equal(t, TargetValue, sr.Target)
	assert.Equal(t, OpGT, sr.Op)
	assert.InDelta(t, 0.0, sr.Value, 0)
}

func TestParseValidationRule_AllOperators(t *testing.T) {
	t.Parallel()

	cases := map[string]Operator{
		">0":  OpGT,
		">=0": OpGE,
		"<0":  OpLT,
		"<=0": OpLE,
		"==0": OpEQ,
		"!=0": OpNE,
	}

	for input, wantOp := range cases {
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			rule, ok := parseValidationRule(input)
			require.True(t, ok)
			sr, ok := rule.(SimpleRule)
			require.True(t, ok)
			assert.Equal(t, wantOp, sr.Op)
			assert.Equal(t, TargetValue, sr.Target)
		})
	}
}

func TestParseValidationRule_SizeAndLength(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"Size >0", "Length >0"} {
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			rule, ok := parseValidationRule(input)
			require.True(t, ok)
			sr, ok := rule.(SimpleRule)
			require.True(t, ok)
			assert.Equal(t, TargetSize, sr.Target, "Size и Length нормализуются в TargetSize")
			assert.Equal(t, OpGT, sr.Op)
			assert.InDelta(t, 0.0, sr.Value, 0)
		})
	}
}

func TestParseValidationRule_FloatValue(t *testing.T) {
	t.Parallel()

	rule, ok := parseValidationRule(">=1.5")
	require.True(t, ok)
	sr, ok := rule.(SimpleRule)
	require.True(t, ok)
	assert.InDelta(t, 1.5, sr.Value, 0)
}

func TestParseValidationRule_NegativeValue(t *testing.T) {
	t.Parallel()

	rule, ok := parseValidationRule(">-10")
	require.True(t, ok)
	sr, ok := rule.(SimpleRule)
	require.True(t, ok)
	assert.InDelta(t, -10.0, sr.Value, 0)
}

func TestParseValidationRule_NamedValidator(t *testing.T) {
	t.Parallel()

	cases := []string{
		"cdn.EmailFormat",
		"cdn.subpkg.Validator",
		"pkg.Name",
		"a.B",
	}

	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			rule, ok := parseValidationRule(input)
			require.True(t, ok)
			nr, ok := rule.(NamedRule)
			require.True(t, ok)
			assert.Equal(t, input, nr.Name)
		})
	}
}

func TestParseValidationRule_Invalid(t *testing.T) {
	t.Parallel()

	cases := []string{
		"",             // пусто
		"  ",           // пробелы
		">>",           // двойной оператор
		">abc",         // не число
		"Size",         // только target без оператора
		"Size >",       // без числа
		"foo",          // no dot, not simple
		".Foo",         // пустая первая часть
		"Foo.",         // пустая последняя часть
		"cdn.123Bad",   // часть начинается с цифры
		"cdn.Bad-Name", // дефис не идентификатор
		"cdn..Bad",     // двойная точка
	}

	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			_, ok := parseValidationRule(input)
			assert.False(t, ok, "expected %q to be invalid", input)
		})
	}
}

func TestParseValidationRule_ImmutableNotRule(t *testing.T) {
	t.Parallel()

	// Immutable — маркер, не валидация. parseValidationRule его не
	// обрабатывает (вызов должен идти через parsePropertyValidations,
	// который фильтрует Immutable отдельно).
	_, ok := parseValidationRule("Immutable")
	assert.False(t, ok, "Immutable must not parse as validation rule")
}

// --- parsePropertyValidations tests (unit, with fake schema) ---

func TestParsePropertyValidations_MixedImmutableAndRules(t *testing.T) {
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
          x-validations: [Immutable, ">=2", "cdn.NotEmpty"]
`)

	pet := findSchema(t, doc, "Pet")
	nameProp := findProperty(t, pet, "name")

	assert.True(t, nameProp.Immutable, "Immutable marker must be set")
	require.Len(t, nameProp.Validations, 2, "Immutable filtered, 2 rules remain")

	sr, ok := nameProp.Validations[0].(SimpleRule)
	require.True(t, ok)
	assert.Equal(t, OpGE, sr.Op)
	assert.InDelta(t, 2.0, sr.Value, 0)

	nr, ok := nameProp.Validations[1].(NamedRule)
	require.True(t, ok)
	assert.Equal(t, "cdn.NotEmpty", nr.Name)
}

func TestParsePropertyValidations_OnlyImmutable(t *testing.T) {
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
          x-validations: [Immutable]
`)

	pet := findSchema(t, doc, "Pet")
	nameProp := findProperty(t, pet, "name")

	assert.True(t, nameProp.Immutable)
	assert.Empty(t, nameProp.Validations)
}

func TestParsePropertyValidations_OnlyRules(t *testing.T) {
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
          x-validations: [">0", "cdn.Email"]
`)

	pet := findSchema(t, doc, "Pet")
	nameProp := findProperty(t, pet, "name")

	assert.False(t, nameProp.Immutable)
	assert.Len(t, nameProp.Validations, 2)
}

func TestParsePropertyValidations_NoExtension(t *testing.T) {
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

	pet := findSchema(t, doc, "Pet")
	nameProp := findProperty(t, pet, "name")

	assert.False(t, nameProp.Immutable)
	assert.Empty(t, nameProp.Validations)
}

func TestParsePropertyValidations_InvalidEntrySkipped(t *testing.T) {
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
          x-validations: [">0", "invalid", "cdn.Valid"]
`)

	pet := findSchema(t, doc, "Pet")
	nameProp := findProperty(t, pet, "name")

	// "invalid" — no dot, not simple rule — silently skipped.
	require.Len(t, nameProp.Validations, 2)
	assert.IsType(t, SimpleRule{}, nameProp.Validations[0])
	assert.IsType(t, NamedRule{}, nameProp.Validations[1])
}

// --- parseSchemaValidations tests ---

func TestParseSchemaValidations_NamedOnly(t *testing.T) {
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

	pet := findSchema(t, doc, "Pet")
	require.Len(t, pet.Validations, 1)
	nr, ok := pet.Validations[0].(NamedRule)
	require.True(t, ok)
	assert.Equal(t, "cdn.PetConsistency", nr.Name)
}

func TestParseSchemaValidations_SimpleRuleSkipped(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      x-validations: [">0", "cdn.Valid"]
      properties:
        name: {type: string}
`)

	pet := findSchema(t, doc, "Pet")
	// Простые правила на schema-level пропускаются.
	require.Len(t, pet.Validations, 1)
	assert.IsType(t, NamedRule{}, pet.Validations[0])
}

func TestParseSchemaValidations_ImmutableSkipped(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      x-validations: [Immutable]
      properties:
        name: {type: string}
`)

	pet := findSchema(t, doc, "Pet")
	assert.Empty(t, pet.Validations)
}

func TestParseSchemaValidations_NoExtension(t *testing.T) {
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

	pet := findSchema(t, doc, "Pet")
	assert.Empty(t, pet.Validations)
}

// --- update_marker interaction ---

// TestUpdateMarker_WithValidations проверяет, что update-marker корректно
// помечает поле с x-validations: [Immutable] — оно пропускается (кроме
// name), но другие rules валидации остаются.
func TestUpdateMarker_WithValidations(t *testing.T) {
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
          x-validations: [Immutable, ">=1"]
        tag:
          type: string
          x-validations: [Immutable]
        label:
          type: string
          x-validations: [">0", "cdn.LabelCheck"]
`)

	pet := findSchema(t, doc, "Pet")

	nameProp := findProperty(t, pet, "name")
	assert.True(t, nameProp.Immutable)
	assert.True(t, nameProp.IsUsedInUpdate, "Immutable name должен быть в update")
	assert.Len(t, nameProp.Validations, 1)

	tagProp := findProperty(t, pet, "tag")
	assert.True(t, tagProp.Immutable)
	assert.False(t, tagProp.IsUsedInUpdate, "Immutable non-name пропускается")
	assert.Empty(t, tagProp.Validations)

	labelProp := findProperty(t, pet, "label")
	assert.False(t, labelProp.Immutable)
	assert.True(t, labelProp.IsUsedInUpdate)
	assert.Len(t, labelProp.Validations, 2)
}
