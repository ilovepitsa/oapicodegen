package render

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBase_InitializesAllFields(t *testing.T) {
	t.Parallel()

	ctx := &RenderContext{ModulePath: "example.com/m"}

	b := NewBase(ctx)

	require.NotNil(t, b.Buf, "Buf must be initialized")
	require.NotNil(t, b.Imports, "Imports must be initialized")
	assert.Equal(t, ctx, b.Ctx, "Ctx must match the argument")

	var _ *codegen.BufferWriter = b.Buf
	assert.Empty(t, b.Imports.Imports(), "fresh tracker must have no imports")
	assert.Empty(t, b.Buf.Content(), "fresh BufferWriter must be empty")
}

func TestBase_Init_OverwritesAllFields(t *testing.T) {
	t.Parallel()

	origCtx := &RenderContext{ModulePath: "orig"}
	b := NewBase(origCtx)

	newBuf := codegen.NewBufferWriter()
	newBuf.WriteString("/* shared */")
	newImports := NewImportTracker()
	newImports.Add(gogen.Import{Path: "fmt"})
	newCtx := &RenderContext{ModulePath: "new"}

	b.Init(newBuf, newImports, newCtx)

	assert.Same(t, newBuf, b.Buf, "Buf must point to injected instance")
	assert.Same(t, newImports, b.Imports, "Imports must point to injected instance")
	assert.Same(t, newCtx, b.Ctx, "Ctx must point to injected instance")
	assert.Equal(t, "/* shared */", string(b.Buf.Content()))
}

// embeddingBase проверяет, что Init работает через embed — основной use-case
// compose-пакета из Task 6.
type embeddingBase struct {
	Base
}

func TestBase_Init_WorksWithEmbedding(t *testing.T) {
	t.Parallel()

	r := &embeddingBase{}
	ctx := &RenderContext{ModulePath: "m"}
	buf := codegen.NewBufferWriter()
	imports := NewImportTracker()

	// Init вызывается на embeddingBase, но должен установить поля встроенной Base.
	r.Init(buf, imports, ctx)

	assert.Same(t, buf, r.Buf)
	assert.Same(t, imports, r.Imports)
	assert.Same(t, ctx, r.Ctx)
}

func TestImportTracker_Add_DedupsByPathAndAlias(t *testing.T) {
	t.Parallel()

	tr := NewImportTracker()

	tr.Add(gogen.Import{Path: "fmt", Alias: ""})
	tr.Add(gogen.Import{Path: "fmt", Alias: ""})
	tr.Add(gogen.Import{Path: "strings", Alias: ""})
	tr.Add(gogen.Import{Path: "example.com/pkg", Alias: "mypkg"})
	tr.Add(gogen.Import{Path: "example.com/pkg", Alias: "other"}) // same path, diff alias → kept
	tr.Add(gogen.Import{Path: "example.com/pkg", Alias: "mypkg"}) // exact dup → dropped

	got := tr.Imports()
	// 5 distinct (Path,Alias) pairs; the last Add is an exact dup → dropped.
	require.Len(t, got, 4, "dedup by Path+Alias must keep 4 distinct entries")

	pairs := make(map[string]struct{}, len(got))
	for _, imp := range got {
		pairs[imp.Path+"|"+imp.Alias] = struct{}{}
	}

	assert.Contains(t, pairs, "fmt|")
	assert.Contains(t, pairs, "strings|")
	assert.Contains(t, pairs, "example.com/pkg|mypkg")
	assert.Contains(t, pairs, "example.com/pkg|other")
}

func TestImportTracker_Add_PreservesFirstOccurrence(t *testing.T) {
	t.Parallel()

	tr := NewImportTracker()

	first := gogen.Import{Path: "fmt", Alias: "", Package: "fmt"}
	second := gogen.Import{Path: "fmt", Alias: "", Package: "overridden"}
	tr.Add(first)
	tr.Add(second)

	got := tr.Imports()
	require.Len(t, got, 1)
	assert.Equal(t, "fmt", got[0].Package, "first occurrence must win")
}

func TestImportTracker_Imports_EmptyByDefault(t *testing.T) {
	t.Parallel()

	tr := NewImportTracker()
	assert.Empty(t, tr.Imports())
	assert.NotNil(t, tr.Imports(), "slice must be non-nil even when empty")
}
