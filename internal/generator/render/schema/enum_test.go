package schema

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newEnumRenderer строит EnumRenderer с shared Buf/Imports и фейковым TypeMapper.
func newEnumRenderer(t *testing.T, tm render.TypeMapper) *EnumRenderer {
	t.Helper()

	ctx := &render.RenderContext{TypeMapper: tm}
	r := NewEnumRenderer()
	r.Init(codegen.NewBufferWriter(), render.NewImportTracker(), ctx)

	return r
}

func TestEnumRenderer_StringEnum(t *testing.T) {
	t.Parallel()

	r := newEnumRenderer(t, &fakeTypeMapper{got: "string"})

	require.NoError(t, r.OnEnum(&parser.Schema{
		Name: "Status",
		Type: "string",
		Enum: []any{"active", "inactive", "archived"},
	}))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "type Status string")
	assert.True(t, containsCollapsed(got, "StatusActive Status = \"active\""))
	assert.True(t, containsCollapsed(got, "StatusInactive Status = \"inactive\""))
	assert.True(t, containsCollapsed(got, "StatusArchived Status = \"archived\""))
}

func TestEnumRenderer_IntegerEnumInt64(t *testing.T) {
	t.Parallel()

	r := newEnumRenderer(t, &fakeTypeMapper{got: "int64"})

	require.NoError(t, r.OnEnum(&parser.Schema{
		Name:   "Priority",
		Type:   "integer",
		Format: "int64",
		Enum:   []any{1, 2, 3},
	}))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "type Priority int64")
	assert.True(t, containsCollapsed(got, "Priority1 Priority = 1"))
	assert.True(t, containsCollapsed(got, "Priority2 Priority = 2"))
	assert.True(t, containsCollapsed(got, "Priority3 Priority = 3"))
}

func TestEnumRenderer_DedupSameValue(t *testing.T) {
	t.Parallel()

	r := newEnumRenderer(t, &fakeTypeMapper{got: "string"})

	require.NoError(t, r.OnEnum(&parser.Schema{
		Name: "Color",
		Type: "string",
		Enum: []any{"red", "red", "blue"},
	}))

	got := string(r.Buf.Content())

	// "red" встречается дважды → константа ColorRed должна быть одна.
	count := strings.Count(got, "ColorRed")
	assert.Equal(t, 1, count, "duplicate enum value must be deduplicated")
	assert.True(t, containsCollapsed(got, "ColorBlue Color = \"blue\""))
}

func TestEnumRenderer_EmptyValueGetsEmptySuffix(t *testing.T) {
	t.Parallel()

	r := newEnumRenderer(t, &fakeTypeMapper{got: "string"})

	require.NoError(t, r.OnEnum(&parser.Schema{
		Name: "Tag",
		Type: "string",
		Enum: []any{"", "x"},
	}))

	got := string(r.Buf.Content())
	assert.True(t, containsCollapsed(got, "TagEmpty Tag = \"\""))
	assert.True(t, containsCollapsed(got, "TagX Tag = \"x\""))
}

// containsCollapsed — компактная проверка подстроки без учёта выравнивания
// gofmt. Локальная копия того же хелпера из generator_test.go (пакет
// render/schema не может импортировать generator).
func containsCollapsed(got, want string) bool {
	return strings.Contains(collapseWS(got), collapseWS(want))
}

func collapseWS(s string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}
