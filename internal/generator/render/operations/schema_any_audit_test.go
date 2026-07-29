package operations

import (
	"testing"

	"github.com/ilovepitsa/oapicodegen/internal/generator/render"
	"github.com/stretchr/testify/assert"
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
