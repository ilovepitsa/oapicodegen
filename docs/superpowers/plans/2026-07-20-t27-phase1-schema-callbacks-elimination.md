# T27 Phase 1 — Schema-side: Eliminate Callbacks Bridge
## Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate the `SchemaCallbacks`/`generatorCallbacks` bridge by migrating SetDefaults, ValidateOwn, UpdateStructs, URLForm, and Converters from `internal/generator/*.go` into full `walk.SchemaRenderer` implementations in `internal/generator/render/schema/`. After Phase 1, `render_callbacks_adapter.go` and `render/callbacks.go` are deleted, `RenderContext.Callbacks` is removed.

**Architecture:** One pack of renderers per output file, orchestrated by `compose.FileComposer`. Each new renderer embeds `render.Base` + `walk.NoopSchemaRenderer`, implements only the hooks it needs, and writes to `r.Buf`/`r.Imports` via `Base.Init`. Pure utility functions (e.g., `treeHasDefaults`) live in `render/schema/` as standalone functions. Byte-for-byte golden-test stability required at every step.

**Tech Stack:** Go 1.x, `testify/assert`, existing `internal/golden` golden-test framework, existing `walk`/`compose`/`render` infrastructure.

**Spec:** `docs/superpowers/specs/2026-07-20-t27-visitor-refactoring-design.md` (section 6.1)

---

## File Structure

### Create
- `internal/generator/render/schema/defaults.go` — `treeHasDefaults(s, mode)` pure function
- `internal/generator/render/schema/defaults_test.go` — unit test
- `internal/generator/render/schema/set_defaults.go` — `SetDefaultsRenderer`
- `internal/generator/render/schema/set_defaults_test.go` — unit test
- `internal/generator/render/schema/validate.go` — `ValidateOwnRenderer`
- `internal/generator/render/schema/validate_test.go` — unit test
- `internal/generator/render/schema/update_struct.go` — `UpdateStructRenderer`
- `internal/generator/render/schema/update_struct_test.go` — unit test
- `internal/generator/render/schema/url_form.go` — `URLFormRenderer`
- `internal/generator/render/schema/url_form_test.go` — unit test
- `internal/generator/render/schema/converters.go` — `ConvertersRenderer`
- `internal/generator/render/schema/converters_test.go` — unit test

### Modify
- `internal/generator/render/base.go` — remove `Callbacks` field from `RenderContext`
- `internal/generator/render/schema/struct.go` — remove `Callbacks` usage from `OnStruct`/`OnSplitStruct`; remove `renderUpdateStruct` (moves to `UpdateStructRenderer`)
- `internal/generator/generator.go` — update `writeStructFileViaComposer` pack composition; remove `writeSchemaAuxFiles` bridge for url_form/converters (uses composer); remove `ctx.Callbacks = &generatorCallbacks{...}` wiring
- `internal/generator/render/schema/struct_test.go` — remove `Callbacks` setup from test helpers
- `internal/generator/render/schema/alias_test.go` — remove `Callbacks` setup from test helpers (if present)

### Delete
- `internal/generator/render_callbacks_adapter.go` — entire file
- `internal/generator/render/callbacks.go` — entire file
- `internal/generator/set_defaults.go` — after logic moves to `render/schema/set_defaults.go`
- `internal/generator/validate.go` — after logic moves to `render/schema/validate.go`
- `internal/generator/url_form_methods.go` — after logic moves to `render/schema/url_form.go`
- `internal/generator/converter_methods.go` — after logic moves to `render/schema/converters.go`

---

## Pre-flight: capture golden baseline

### Task 0: Capture golden baseline before any changes

**Files:**
- Verify: `testdata/project/minimal/` exists with golden output

- [ ] **Step 1: Verify clean baseline**

```bash
go build ./...
go test ./...
```

Expected: all green. If any test fails before changes — STOP and fix first.

- [ ] **Step 2: Snapshot golden output for diff comparison**

```bash
rm -rf /tmp/oapigen-baseline && mkdir -p /tmp/oapigen-baseline
cp -r testdata/project/golden /tmp/oapigen-baseline/
```

This snapshot is used after each task to verify byte-for-byte identity.

- [ ] **Step 3: Run generator and capture output**

```bash
go run ./cmd/oapigen -input testdata/project/minimal -output /tmp/oapigen-baseline-out
diff -r /tmp/oapigen-baseline /tmp/oapigen-baseline-out
```

Expected: empty diff (or only path differences). If differences exist — golden files are stale; sync them before proceeding.

---

## Task 1: Migrate `schemaTreeHasDefaults` → `render/schema/defaults.go`

Pure function migration. No renderer yet — just the utility. The old method stays in place during this task (still called via Callbacks); the new function is added in parallel, verified to produce identical results.

**Files:**
- Create: `internal/generator/render/schema/defaults.go`
- Create: `internal/generator/render/schema/defaults_test.go`

- [ ] **Step 1: Read existing `schemaTreeHasDefaults` implementation**

```bash
grep -n "schemaTreeHasDefaults\|resolveRefSchema" internal/generator/set_defaults.go
```

Note the full body — it will be ported. `resolveRefSchema` is a helper used inside; check if it lives in `set_defaults.go` or elsewhere.

- [ ] **Step 2: Write failing unit test**

`internal/generator/render/schema/defaults_test.go`:

```go
package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"nschugorev/oapigenerator/internal/parser"
)

func TestTreeHasDefaults_NoProperties(t *testing.T) {
	s := &parser.Schema{Name: "Empty", Type: "object"}
	assert.False(t, treeHasDefaults(s, nil))
}

func TestTreeHasDefaults_WithDefault(t *testing.T) {
	s := &parser.Schema{
		Name: "Pet",
		Type: "object",
		Properties: []*parser.Property{
			{Name: "Tag", Schema: &parser.Schema{Type: "string"}, Default: "none"},
		},
	}
	assert.True(t, treeHasDefaults(s, nil))
}

func TestTreeHasDefaults_NoDefault_False(t *testing.T) {
	s := &parser.Schema{
		Name: "Pet",
		Type: "object",
		Properties: []*parser.Property{
			{Name: "Tag", Schema: &parser.Schema{Type: "string"}},
		},
	}
	assert.False(t, treeHasDefaults(s, nil))
}

func TestTreeHasDefaults_KeepFilter_ExcludesDefault(t *testing.T) {
	s := &parser.Schema{
		Name: "Pet",
		Type: "object",
		Properties: []*parser.Property{
			{Name: "ReadOnly", Schema: &parser.Schema{Type: "string", ReadOnly: true}, Default: "ro"},
		},
	}
	keep := func(p *parser.Property) bool { return !p.Schema.ReadOnly }
	assert.False(t, treeHasDefaults(s, keep))
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
go test ./internal/generator/render/schema/ -run TestTreeHasDefaults -v
```

Expected: FAIL — `treeHasDefaults` undefined.

- [ ] **Step 4: Implement `treeHasDefaults` in `render/schema/defaults.go`**

Copy the body of `Generator.schemaTreeHasDefaults` from `internal/generator/set_defaults.go` verbatim, converted to a free function. Also copy `resolveRefSchema` helper (if it's only used by `schemaTreeHasDefaults`; otherwise leave the original in place for now and add a TODO to migrate later).

```go
// Package schema: treeHasDefaults и сопутствующие утилиты для SetDefaultsRenderer.
package schema

import (
	"nschugorev/oapigenerator/internal/parser"
)

// treeHasDefaults рекурсивно (через $ref на object-схемы) проверяет, есть ли в
// дереве схемы хотя бы одно property с Default != nil, проходящее фильтр keep.
// keep nil = все свойства проходят.
//
// visited защищает от циклов по $ref (A → B → A). Имя текущей схемы
// добавляется в visited перед рекурсивным обходом.
func treeHasDefaults(s *parser.Schema, keep func(*parser.Property) bool) bool {
	if s == nil {
		return false
	}
	return treeHasDefaultsWithVisited(s, keep, map[string]bool{s.Name: true})
}

func treeHasDefaultsWithVisited(
	s *parser.Schema,
	keep func(*parser.Property) bool,
	visited map[string]bool,
) bool {
	if s == nil {
		return false
	}

	for _, p := range s.Properties {
		if keep != nil && !keep(p) {
			continue
		}

		if p.Schema != nil && p.Schema.Default != nil {
			return true
		}

		target := resolveRefSchema(p.Schema)
		if target == nil || target.Name == "" {
			continue
		}

		if visited[target.Name] {
			continue
		}

		visited[target.Name] = true
		if treeHasDefaultsWithVisited(target, keep, visited) {
			return true
		}
		delete(visited, target.Name)
	}

	return false
}
```

Note: `resolveRefSchema` must be migrated too — copy it from `set_defaults.go` to `defaults.go` if it's only used by `schemaTreeHasDefaults`. If used elsewhere — leave in original file as a temporary duplicate, deduplicate in Task 7 cleanup.

- [ ] **Step 5: Run unit test to verify it passes**

```bash
go test ./internal/generator/render/schema/ -run TestTreeHasDefaults -v
```

Expected: PASS.

- [ ] **Step 6: Verify golden tests still pass (no behavior change — old path still active)**

```bash
go test ./...
```

Expected: all green.

- [ ] **Step 7: Commit**

```bash
git add internal/generator/render/schema/defaults.go internal/generator/render/schema/defaults_test.go
git commit -m "T27-1: migrate schemaTreeHasDefaults to render/schema/defaults.go"
```

---

## Task 2: Migrate `SetDefaults` → `SetDefaultsRenderer`

`SetDefaultsRenderer` implements `OnStruct` and `OnSplitStruct`. For each hook: check `treeHasDefaults`; if true, render `func (m *<Name>) SetDefaults()` body (port from `renderSetDefaultsMethod`). Skip if no defaults. Uses `r.Ctx.TypeMapper` directly (no fresh typeMapper per call — unlike the bridge).

**Files:**
- Create: `internal/generator/render/schema/set_defaults.go`
- Create: `internal/generator/render/schema/set_defaults_test.go`

- [ ] **Step 1: Write failing unit test**

`internal/generator/render/schema/set_defaults_test.go`:

```go
package schema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
)

func TestSetDefaultsRenderer_NoDefaults_NoOutput(t *testing.T) {
	r := newSetDefaultsTestRenderer()
	s := &parser.Schema{
		Name: "Pet",
		Type: "object",
		Properties: []*parser.Property{
			{Name: "Tag", Schema: &parser.Schema{Type: "string"}},
		},
	}

	err := r.OnStruct(s)
	assert.NoError(t, err)
	assert.Empty(t, r.Buf.Content())
}

func TestSetDefaultsRenderer_WithDefault_RendersMethod(t *testing.T) {
	r := newSetDefaultsTestRenderer()
	s := &parser.Schema{
		Name: "Pet",
		Type: "object",
		Properties: []*parser.Property{
			{Name: "tag", Schema: &parser.Schema{Type: "string"}, Default: "none"},
		},
	}

	err := r.OnStruct(s)
	assert.NoError(t, err)

	out := string(r.Buf.Content())
	assert.Contains(t, out, "func (m *Pet) SetDefaults()")
	assert.Contains(t, out, `m.Tag = "none"`)
}

func newSetDefaultsTestRenderer() *SetDefaultsRenderer {
	r := NewSetDefaultsRenderer()
	buf := codegen.NewBufferWriter()
	imports := render.NewImportTracker()
	ctx := &render.RenderContext{}
	r.Init(buf, imports, ctx)
	return r
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/generator/render/schema/ -run TestSetDefaultsRenderer -v
```

Expected: FAIL — `SetDefaultsRenderer` undefined.

- [ ] **Step 3: Implement `SetDefaultsRenderer`**

`internal/generator/render/schema/set_defaults.go`:

Port `renderSetDefaultsMethod` body from `internal/generator/set_defaults.go`. Replace `g.resolveRefSchema` with `resolveRefSchema` (now in `defaults.go`). Replace `m.addImport(...)` with `r.Imports.Add(gogen.Import{Path: ..., Alias: ...})`. Replace `m.goType(...)` with `r.Ctx.TypeMapper.GoType(...)`. Replace `m.mode` with `r.currentMode()` (same pattern as StructRenderer).

```go
// Package schema: SetDefaultsRenderer рендерит func (m *<Name>) SetDefaults()
// для object-схем с default-полями в дереве.
package schema

import (
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/generator/walk"
	"nschugorev/oapigenerator/internal/parser"
)

// SetDefaultsRenderer рендерит SetDefaults() методы для object-схем.
// Срабатывает на OnStruct/OnSplitStruct. Если treeHasDefaults(s) = false —
// метод не рендерится (noop). Для split-схем рендерятся два метода:
// SetDefaults() на <Name>Request и SetDefaults() на <Name>Response.
type SetDefaultsRenderer struct {
	render.Base
	walk.NoopSchemaRenderer
}

func NewSetDefaultsRenderer() *SetDefaultsRenderer { return &SetDefaultsRenderer{} }

func (r *SetDefaultsRenderer) OnStruct(s *parser.Schema) error {
	if !treeHasDefaults(s, nil) {
		return nil
	}
	r.renderSetDefaults(s, goName(s.Name), nil)
	return nil
}

func (r *SetDefaultsRenderer) OnSplitStruct(s *parser.Schema) error {
	name := goName(s.Name)
	reqKeep := func(p *parser.Property) bool { return p.Schema == nil || !p.Schema.ReadOnly }
	respKeep := func(p *parser.Property) bool { return p.Schema == nil || !p.Schema.WriteOnly }

	if treeHasDefaults(s, reqKeep) {
		r.renderSetDefaults(s, name+"Request", reqKeep)
	}
	if treeHasDefaults(s, respKeep) {
		r.renderSetDefaults(s, name+"Response", respKeep)
	}
	return nil
}

// renderSetDefaults ports renderSetDefaultsMethod body verbatim.
// Replace m.* with r.Ctx.TypeMapper.* and r.Imports.Add.
func (r *SetDefaultsRenderer) renderSetDefaults(
	s *parser.Schema,
	name string,
	keep func(*parser.Property) bool,
) {
	// ... port body of Generator.renderSetDefaultsMethod ...
}
```

Implementation note: the body of `renderSetDefaultsMethod` must be copied line-for-line, with these substitutions:
- `m.addImport(path, alias)` → `r.Imports.Add(gogen.Import{Path: path, Alias: alias})`
- `m.goType(s)` → `r.Ctx.TypeMapper.GoType(s)`
- `m.mode` → `r.currentMode()`
- `g.requiredForMode(p, m.mode)` → `r.Ctx.TypeMapper.RequiredForMode(p, r.currentMode())` (or whatever the existing `render.TypeMapper` interface exposes — verify in `render/typemap.go`)
- `g.resolveRefSchema(p.Schema)` → `resolveRefSchema(p.Schema)` (now in same package)
- `g.schemaTreeHasDefaults(target, keep, visited)` → `treeHasDefaultsWithVisited(target, keep, visited)`

If `render.TypeMapper` interface lacks `RequiredForMode` — add it as a method on the interface and on the adapter. (Check `internal/generator/render/typemap.go` for current interface surface.)

- [ ] **Step 4: Run unit test to verify it passes**

```bash
go test ./internal/generator/render/schema/ -run TestSetDefaultsRenderer -v
```

Expected: PASS.

- [ ] **Step 5: Verify golden tests still pass (old path still active — new renderer not wired yet)**

```bash
go test ./...
```

Expected: all green.

- [ ] **Step 6: Commit**

```bash
git add internal/generator/render/schema/set_defaults.go internal/generator/render/schema/set_defaults_test.go
git commit -m "T27-1: add SetDefaultsRenderer (not wired yet)"
```

---

## Task 3: Wire `SetDefaultsRenderer` into struct pack; remove Callbacks path for SetDefaults

Wire the new renderer into `writeStructFileViaComposer` pack. Remove `cb.RenderSetDefaults` call from `StructRenderer.OnStruct`. Old `renderSetDefaultsMethod` stays in place (will be deleted in Task 7 cleanup).

**Files:**
- Modify: `internal/generator/generator.go:260-280` (`writeStructFileViaComposer`)
- Modify: `internal/generator/render/schema/struct.go:58-81` (`OnStruct`) and `OnSplitStruct`
- Modify: `internal/generator/render/schema/struct_test.go` (remove `Callbacks` setup)

- [ ] **Step 1: Update `writeStructFileViaComposer` pack**

```go
func (g *Generator) writeStructFileViaComposer(sh *parser.Schema) (codegen.File, error) {
	ctx := &render.RenderContext{
		Project:      g.project,
		SchemaIndex:  g.schemaIndex,
		Features:     g.project.Features,
		Splittable:   g.splittable,
		ModulePath:   g.project.ImportPrefix,
		ImportPrefix: g.project.ImportPrefix,
	}
	ctx.TypeMapper = g.newRenderTypeMapper("model", "", ctx)

	renderers := []walk.SchemaRenderer{
		schemarender.NewStructRenderer(),
		schemarender.NewSetDefaultsRenderer(),
	}

	cf, err := g.composer.ComposeSchemaFile(sh, renderers, ctx) //nolint:lll // renderer-list literal
	if err != nil {
		return nil, fmt.Errorf("compose struct %q: %w", sh.Name, err)
	}
	return cf, nil
}
```

Note: `ctx.Callbacks = &generatorCallbacks{...}` line removed.

- [ ] **Step 2: Remove `cb.RenderSetDefaults` call from `StructRenderer.OnStruct`**

In `internal/generator/render/schema/struct.go`, the `OnStruct` body currently:

```go
cb := r.Ctx.Callbacks
if cb != nil {
	if cb.SchemaTreeHasDefaults(s, nil) {
		cb.RenderSetDefaults(r.Buf, s, r.currentMode(), name, nil)
	}
	cb.RenderValidateOwn(r.Buf, s, r.currentMode(), name, false, nil)
}
```

Replace with:

```go
// SetDefaults rendered by SetDefaultsRenderer in the same pack.
// ValidateOwn rendered by ValidateOwnRenderer in the same pack (Task 4).
```

(For now leave `cb.RenderValidateOwn` call — it's removed in Task 4 when ValidateOwnRenderer is wired.) So the interim state:

```go
cb := r.Ctx.Callbacks
if cb != nil {
	cb.RenderValidateOwn(r.Buf, s, r.currentMode(), name, false, nil)
}
```

- [ ] **Step 3: Same for `OnSplitStruct`** — remove `cb.RenderSetDefaults` calls.

- [ ] **Step 4: Update `struct_test.go` test helpers**

Remove `Callbacks` setup from any test renderer construction. If tests rely on `Callbacks` for ValidateOwn — leave the Callbacks setup for now (will be removed in Task 4).

- [ ] **Step 5: Run golden tests**

```bash
go test ./...
```

Expected: all green. If golden fails — investigate; the new `SetDefaultsRenderer` output must match the old `renderSetDefaultsMethod` byte-for-byte. Common causes:
- TypeMapper mode not set correctly (verify `r.currentMode()` returns same value as `m.mode` did)
- Imports drained differently (verify `r.Imports.Add` is called with same path/alias as `m.addImport`)
- Order of properties differs (shouldn't — both iterate `s.Properties`)

- [ ] **Step 6: Run generator and verify byte-for-byte identity**

```bash
go run ./cmd/oapigen -input testdata/project/minimal -output /tmp/oapigen-phase1-task3
diff -r /tmp/oapigen-baseline-out /tmp/oapigen-phase1-task3
```

Expected: empty diff.

- [ ] **Step 7: Commit**

```bash
git add internal/generator/generator.go internal/generator/render/schema/struct.go internal/generator/render/schema/struct_test.go
git commit -m "T27-1: wire SetDefaultsRenderer, drop Callbacks path for SetDefaults"
```

---

## Task 4: Migrate `ValidateOwn` → `ValidateOwnRenderer`; wire; remove Callbacks path for ValidateOwn

Same pattern as Tasks 2–3, for ValidateOwn. After this task, `StructRenderer` no longer touches `Callbacks` at all.

**Files:**
- Create: `internal/generator/render/schema/validate.go`
- Create: `internal/generator/render/schema/validate_test.go`
- Modify: `internal/generator/generator.go` — add `schemarender.NewValidateOwnRenderer()` to pack
- Modify: `internal/generator/render/schema/struct.go` — remove remaining `cb.RenderValidateOwn` calls
- Modify: `internal/generator/render/schema/struct_test.go` — remove `Callbacks` setup entirely

- [ ] **Step 1: Write failing unit test**

`internal/generator/render/schema/validate_test.go`:

```go
package schema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
)

func TestValidateOwnRenderer_NoRules_NoOutput(t *testing.T) {
	r := newValidateOwnTestRenderer()
	s := &parser.Schema{Name: "Pet", Type: "object"}
	err := r.OnStruct(s)
	assert.NoError(t, err)
	assert.Empty(t, r.Buf.Content())
}

func TestValidateOwnRenderer_WithSimpleRule_RendersMethod(t *testing.T) {
	r := newValidateOwnTestRenderer()
	s := &parser.Schema{
		Name: "Pet",
		Type: "object",
		Properties: []*parser.Property{
			{
				Name: "id",
				Schema: &parser.Schema{Type: "integer"},
				Validations: []parser.ValidationRule{
					parser.SimpleRule{Op: ">", Target: parser.TargetValue, Value: 0},
				},
			},
		},
	}
	err := r.OnStruct(s)
	assert.NoError(t, err)
	out := string(r.Buf.Content())
	assert.Contains(t, out, "func (x Pet) ValidateOwn(reg *validator.Registry) error")
	assert.Contains(t, out, "if x.ID <= 0")
}

func newValidateOwnTestRenderer() *ValidateOwnRenderer {
	r := NewValidateOwnRenderer()
	buf := codegen.NewBufferWriter()
	imports := render.NewImportTracker()
	ctx := &render.RenderContext{}
	r.Init(buf, imports, ctx)
	return r
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/generator/render/schema/ -run TestValidateOwnRenderer -v
```

Expected: FAIL — `ValidateOwnRenderer` undefined.

- [ ] **Step 3: Implement `ValidateOwnRenderer`**

Port `renderValidateOwn` body from `internal/generator/validate.go`. Same substitutions as SetDefaults. Also port helpers `renderPropertyValidations`, `renderSimpleRule`, `fieldAccessor`, `renderNamedValidatorCall`, `renderNamedValidatorCallIndented`, `hasValidationRules`, `inverseOperator`, `opSymbol`, `formatValueLiteral` — these need to move to `render/schema/` too (verify they're only used by `renderValidateOwn`; if used elsewhere, leave for now).

```go
// Package schema: ValidateOwnRenderer рендерит ValidateOwn(reg *validator.Registry)
// методы для object-схем с x-validations.
package schema

import (
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/generator/walk"
	"nschugorev/oapigenerator/internal/parser"
)

type ValidateOwnRenderer struct {
	render.Base
	walk.NoopSchemaRenderer
}

func NewValidateOwnRenderer() *ValidateOwnRenderer { return &ValidateOwnRenderer{} }

func (r *ValidateOwnRenderer) OnStruct(s *parser.Schema) error {
	if !hasValidationRules(s) {
		return nil
	}
	r.renderValidateOwn(s, goName(s.Name), false, nil)
	return nil
}

func (r *ValidateOwnRenderer) OnSplitStruct(s *parser.Schema) error {
	if !hasValidationRules(s) {
		return nil
	}
	name := goName(s.Name)
	reqKeep := func(p *parser.Property) bool { return p.Schema == nil || !p.Schema.ReadOnly }
	respKeep := func(p *parser.Property) bool { return p.Schema == nil || !p.Schema.WriteOnly }
	r.renderValidateOwn(s, name+"Request", false, reqKeep)
	r.renderValidateOwn(s, name+"Response", false, respKeep)
	return nil
}

// renderValidateOwn ports Generator.renderValidateOwn body.
func (r *ValidateOwnRenderer) renderValidateOwn(
	s *parser.Schema,
	name string,
	isUpdate bool,
	keep func(*parser.Property) bool,
) {
	// ... port body ...
}
```

- [ ] **Step 4: Run unit test to verify it passes**

```bash
go test ./internal/generator/render/schema/ -run TestValidateOwnRenderer -v
```

Expected: PASS.

- [ ] **Step 5: Wire into pack in `writeStructFileViaComposer`**

```go
renderers := []walk.SchemaRenderer{
	schemarender.NewStructRenderer(),
	schemarender.NewSetDefaultsRenderer(),
	schemarender.NewValidateOwnRenderer(),
}
```

- [ ] **Step 6: Remove `cb.RenderValidateOwn` call from `StructRenderer.OnStruct` and `OnSplitStruct`**

Remove the entire `cb := r.Ctx.Callbacks` block — StructRenderer no longer touches Callbacks.

- [ ] **Step 7: Remove `Callbacks` setup from `struct_test.go` and `alias_test.go`**

- [ ] **Step 8: Run golden tests + byte-for-byte diff**

```bash
go test ./...
go run ./cmd/oapigen -input testdata/project/minimal -output /tmp/oapigen-phase1-task4
diff -r /tmp/oapigen-baseline-out /tmp/oapigen-phase1-task4
```

Expected: all green, empty diff.

- [ ] **Step 9: Commit**

```bash
git add internal/generator/render/schema/validate.go internal/generator/render/schema/validate_test.go internal/generator/generator.go internal/generator/render/schema/struct.go internal/generator/render/schema/struct_test.go internal/generator/render/schema/alias_test.go
git commit -m "T27-1: add ValidateOwnRenderer, wire, drop Callbacks path for ValidateOwn"
```

---

## Task 5: Migrate `UpdateStruct` → `UpdateStructRenderer`; wire

`renderUpdateStruct` currently lives inside `StructRenderer` (in `struct.go`). Move it to its own renderer.

**Files:**
- Create: `internal/generator/render/schema/update_struct.go`
- Create: `internal/generator/render/schema/update_struct_test.go`
- Modify: `internal/generator/generator.go` — add `NewUpdateStructRenderer()` to pack
- Modify: `internal/generator/render/schema/struct.go` — remove `renderUpdateStruct`, `renderUpdateGetters`, `renderUpdateGetter`, `updateFieldType`

- [ ] **Step 1: Write failing unit test**

`internal/generator/render/schema/update_struct_test.go`:

```go
package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
)

func TestUpdateStructRenderer_NotUsedInUpdate_NoOutput(t *testing.T) {
	r := newUpdateStructTestRenderer()
	s := &parser.Schema{Name: "Pet", Type: "object"}
	err := r.OnStruct(s)
	assert.NoError(t, err)
	assert.Empty(t, r.Buf.Content())
}

func TestUpdateStructRenderer_UsedInUpdate_RendersStruct(t *testing.T) {
	r := newUpdateStructTestRenderer()
	s := &parser.Schema{
		Name:           "Pet",
		Type:           "object",
		IsUsedInUpdate: true,
		Properties: []*parser.Property{
			{Name: "tag", Schema: &parser.Schema{Type: "string"}, IsUsedInUpdate: true},
		},
	}
	err := r.OnStruct(s)
	assert.NoError(t, err)
	out := string(r.Buf.Content())
	assert.Contains(t, out, "type UpdatePet struct")
	assert.Contains(t, out, "Tag optional.Optional[string]")
	assert.Contains(t, out, "func (u *UpdatePet) GetTag() (*string, bool)")
}

func newUpdateStructTestRenderer() *UpdateStructRenderer {
	r := NewUpdateStructRenderer()
	buf := codegen.NewBufferWriter()
	imports := render.NewImportTracker()
	ctx := &render.RenderContext{}
	r.Init(buf, imports, ctx)
	return r
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/generator/render/schema/ -run TestUpdateStructRenderer -v
```

Expected: FAIL — `UpdateStructRenderer` undefined.

- [ ] **Step 3: Implement `UpdateStructRenderer`**

Move `renderUpdateStruct`, `renderUpdateGetters`, `renderUpdateGetter`, `updateFieldType` from `struct.go` to `update_struct.go`. Wrap in renderer:

```go
package schema

import (
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/generator/walk"
	"nschugorev/oapigenerator/internal/parser"
)

type UpdateStructRenderer struct {
	render.Base
	walk.NoopSchemaRenderer
}

func NewUpdateStructRenderer() *UpdateStructRenderer { return &UpdateStructRenderer{} }

func (r *UpdateStructRenderer) OnStruct(s *parser.Schema) error {
	if !s.IsUsedInUpdate {
		return nil
	}
	r.renderUpdateStruct(s, goName(s.Name))
	return nil
}

func (r *UpdateStructRenderer) OnSplitStruct(s *parser.Schema) error {
	if !s.IsUsedInUpdate {
		return nil
	}
	r.renderUpdateStruct(s, goName(s.Name))
	return nil
}

// renderUpdateStruct ports the body verbatim from struct.go.
func (r *UpdateStructRenderer) renderUpdateStruct(s *parser.Schema, name string) {
	// ... moved from StructRenderer ...
}
```

- [ ] **Step 4: Run unit test to verify it passes**

```bash
go test ./internal/generator/render/schema/ -run TestUpdateStructRenderer -v
```

Expected: PASS.

- [ ] **Step 5: Wire into pack**

```go
renderers := []walk.SchemaRenderer{
	schemarender.NewStructRenderer(),
	schemarender.NewSetDefaultsRenderer(),
	schemarender.NewValidateOwnRenderer(),
	schemarender.NewUpdateStructRenderer(),
}
```

- [ ] **Step 6: Remove `renderUpdateStruct` and helpers from `struct.go`; remove `IsUsedInUpdate` check from `StructRenderer.OnStruct`/`OnSplitStruct`**

- [ ] **Step 7: Run golden tests + byte-for-byte diff**

```bash
go test ./...
go run ./cmd/oapigen -input testdata/project/minimal -output /tmp/oapigen-phase1-task5
diff -r /tmp/oapigen-baseline-out /tmp/oapigen-phase1-task5
```

Expected: all green, empty diff.

- [ ] **Step 8: Commit**

```bash
git add internal/generator/render/schema/update_struct.go internal/generator/render/schema/update_struct_test.go internal/generator/generator.go internal/generator/render/schema/struct.go
git commit -m "T27-1: extract UpdateStructRenderer from StructRenderer"
```

---

## Task 6: Migrate `URLForm` → `URLFormRenderer`; wire as aux file

URLForm methods are written to a separate `<name>_url_form.gen.go` aux file. Different pack, different file.

**Files:**
- Create: `internal/generator/render/schema/url_form.go`
- Create: `internal/generator/render/schema/url_form_test.go`
- Modify: `internal/generator/generator.go` — replace `writeSchemaAuxFile(... url_form ...)` path with composer call

- [ ] **Step 1: Read current URLForm aux file wiring**

```bash
grep -n "url_form\|URLForm\|writeSchemaAuxFile" internal/generator/generator.go internal/generator/url_form_methods.go
```

- [ ] **Step 2: Write failing unit test**

`internal/generator/render/schema/url_form_test.go`:

```go
package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
)

func TestURLFormRenderer_NotFormEncoded_NoOutput(t *testing.T) {
	r := newURLFormTestRenderer()
	s := &parser.Schema{Name: "Pet", Type: "object"}
	err := r.OnStruct(s)
	assert.NoError(t, err)
	assert.Empty(t, r.Buf.Content())
}

func newURLFormTestRenderer() *URLFormRenderer {
	r := NewURLFormRenderer()
	buf := codegen.NewBufferWriter()
	imports := render.NewImportTracker()
	ctx := &render.RenderContext{}
	r.Init(buf, imports, ctx)
	return r
}
```

(The "NotFormEncoded" case requires knowing the trigger condition — read `url_form_methods.go` to determine what gates URLForm rendering. Likely a check on the schema being referenced from a form-encoded operation body. If the trigger is complex, the unit test may need to mock that condition. Consult existing `url_form_methods.go` and adapt.)

- [ ] **Step 3: Run test to verify it fails**

```bash
go test ./internal/generator/render/schema/ -run TestURLFormRenderer -v
```

Expected: FAIL.

- [ ] **Step 4: Implement `URLFormRenderer`**

Port `MarshalURLForm`/`UnmarshalURLForm` rendering from `url_form_methods.go` into `OnStruct`/`OnSplitStruct` of `URLFormRenderer`. Same substitution pattern.

```go
package schema

import (
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/generator/walk"
	"nschugorev/oapigenerator/internal/parser"
)

type URLFormRenderer struct {
	render.Base
	walk.NoopSchemaRenderer
}

func NewURLFormRenderer() *URLFormRenderer { return &URLFormRenderer{} }

func (r *URLFormRenderer) OnStruct(s *parser.Schema) error {
	// Port condition + body from url_form_methods.go
	return nil
}

func (r *URLFormRenderer) OnSplitStruct(s *parser.Schema) error {
	// Port split-mode URLForm rendering if applicable
	return nil
}
```

- [ ] **Step 5: Run unit test to verify it passes**

```bash
go test ./internal/generator/render/schema/ -run TestURLFormRenderer -v
```

Expected: PASS.

- [ ] **Step 6: Wire URLFormRenderer as aux file via composer**

In `writeSchemaAuxFiles` (or equivalent), replace the URLForm branch:

```go
// Before:
if g.shouldRenderURLForm(sh) {
	body := g.urlFormFile(sh)
	if err := fw.WriteFile("model/"+sh.Name+"_url_form.gen.go", body); err != nil {
		return err
	}
}

// After:
if g.shouldRenderURLForm(sh) {
	ctx := g.newSchemaRenderContext(sh)
	renderers := []walk.SchemaRenderer{schemarender.NewURLFormRenderer()}
	cf, err := g.composer.ComposeSchemaFile(sh, renderers, ctx)
	if err != nil {
		return fmt.Errorf("compose url_form %q: %w", sh.Name, err)
	}
	if err := fw.WriteFile("model/"+sh.Name+"_url_form.gen.go", cf); err != nil {
		return err
	}
}
```

Extract a helper `g.newSchemaRenderContext(sh *parser.Schema) *render.RenderContext` to avoid duplicating RenderContext construction across `writeStructFileViaComposer` and aux files.

- [ ] **Step 7: Run golden tests + byte-for-byte diff**

```bash
go test ./...
go run ./cmd/oapigen -input testdata/project/minimal -output /tmp/oapigen-phase1-task6
diff -r /tmp/oapigen-baseline-out /tmp/oapigen-phase1-task6
```

Expected: all green, empty diff.

- [ ] **Step 8: Commit**

```bash
git add internal/generator/render/schema/url_form.go internal/generator/render/schema/url_form_test.go internal/generator/generator.go
git commit -m "T27-1: migrate URLForm to URLFormRenderer (aux file)"
```

---

## Task 7: Migrate `Converters` → `ConvertersRenderer`; wire as aux file

Same pattern as Task 6, for `<name>_converters.gen.go` (split-mode + shared fields).

**Files:**
- Create: `internal/generator/render/schema/converters.go`
- Create: `internal/generator/render/schema/converters_test.go`
- Modify: `internal/generator/generator.go` — replace converters aux path with composer call

- [ ] **Step 1: Read current converters wiring**

```bash
grep -n "converter\|Converter\|shouldGenerateConverters" internal/generator/generator.go internal/generator/converter_methods.go
```

- [ ] **Step 2: Write failing unit test**

`internal/generator/render/schema/converters_test.go` — test that `ConvertersRenderer` produces expected `<Name>RequestToResponse` / `<Name>ResponseToRequest` functions for a split + shared-field schema.

- [ ] **Step 3: Run test to verify it fails**

```bash
go test ./internal/generator/render/schema/ -run TestConvertersRenderer -v
```

Expected: FAIL.

- [ ] **Step 4: Implement `ConvertersRenderer`**

Port from `converter_methods.go`. Only renders when `s.IsSplit` and `schemaHasSharedFields(s)`. Gate logic lives in `OnStruct`/`OnSplitStruct` (the renderer decides itself whether to render).

- [ ] **Step 5: Run unit test to verify it passes**

Expected: PASS.

- [ ] **Step 6: Wire via composer as aux file** — same pattern as Task 6.

- [ ] **Step 7: Run golden tests + byte-for-byte diff**

```bash
go test ./...
go run ./cmd/oapigen -input testdata/project/minimal -output /tmp/oapigen-phase1-task7
diff -r /tmp/oapigen-baseline-out /tmp/oapigen-phase1-task7
```

Expected: all green, empty diff. (Note: minimal testdata may not exercise converters — verify with a split-mode testdata project if one exists; if not, this task's golden coverage may be limited. Add unit test coverage to compensate.)

- [ ] **Step 8: Commit**

```bash
git add internal/generator/render/schema/converters.go internal/generator/render/schema/converters_test.go internal/generator/generator.go
git commit -m "T27-1: migrate Converters to ConvertersRenderer (aux file)"
```

---

## Task 8: Delete `Callbacks` bridge and old generator files

After Tasks 1–7, the Callbacks bridge is dead code. Old generator files (`set_defaults.go`, `validate.go`, `url_form_methods.go`, `converter_methods.go`) are also dead. Delete them.

**Files:**
- Delete: `internal/generator/render_callbacks_adapter.go`
- Delete: `internal/generator/render/callbacks.go`
- Delete: `internal/generator/set_defaults.go`
- Delete: `internal/generator/validate.go`
- Delete: `internal/generator/url_form_methods.go`
- Delete: `internal/generator/converter_methods.go`
- Modify: `internal/generator/render/base.go` — remove `Callbacks` field from `RenderContext`

- [ ] **Step 1: Verify no remaining references to `SchemaCallbacks`**

```bash
grep -rn "SchemaCallbacks\|generatorCallbacks\|Callbacks:" internal/generator/
```

Expected: only references in files about to be deleted (and the `RenderContext.Callbacks` field).

- [ ] **Step 2: Remove `Callbacks` field from `RenderContext`**

In `internal/generator/render/base.go`:

```go
type RenderContext struct {
	Project      *parser.Project
	SchemaIndex  *parser.SchemaIndex
	Features     parser.ProjectFeatures
	Splittable   map[string]bool
	ModulePath   string
	ImportPrefix string
	TypeMapper   TypeMapper
	Imports      *ImportTracker
	// Callbacks field removed.
}
```

- [ ] **Step 3: Delete the six files**

```bash
rm internal/generator/render_callbacks_adapter.go
rm internal/generator/render/callbacks.go
rm internal/generator/set_defaults.go
rm internal/generator/validate.go
rm internal/generator/url_form_methods.go
rm internal/generator/converter_methods.go
```

- [ ] **Step 4: Build and test**

```bash
go build ./...
go test ./...
```

Expected: all green. If build fails — there's a remaining reference. Fix the caller (likely a test helper or a now-dead import in `generator.go`).

- [ ] **Step 5: Run golangci-lint**

```bash
golangci-lint run
```

Expected: green. Fix any new warnings (likely unused imports in `generator.go`).

- [ ] **Step 6: Byte-for-byte diff**

```bash
go run ./cmd/oapigen -input testdata/project/minimal -output /tmp/oapigen-phase1-task8
diff -r /tmp/oapigen-baseline-out /tmp/oapigen-phase1-task8
```

Expected: empty diff.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "T27-1: delete Callbacks bridge and old schema generator files"
```

---

## Task 9: Verify Phase 1 acceptance criteria

Final verification that Phase 1 is complete.

- [ ] **Step 1: Build + test + lint**

```bash
go build ./...
go test ./...
golangci-lint run
```

Expected: all green.

- [ ] **Step 2: Byte-for-byte identity vs baseline**

```bash
go run ./cmd/oapigen -input testdata/project/minimal -output /tmp/oapigen-phase1-final
diff -r /tmp/oapigen-baseline-out /tmp/oapigen-phase1-final
```

Expected: empty diff.

- [ ] **Step 3: Verify Callbacks bridge fully removed**

```bash
grep -rn "SchemaCallbacks\|generatorCallbacks\|Callbacks\b" internal/generator/
```

Expected: no matches (or only matches in unrelated contexts like `MethodRenderer` callbacks, which are not part of this bridge).

- [ ] **Step 4: Verify renderer pack composition in `writeStructFileViaComposer`**

```bash
grep -A 5 "renderers := \[\]walk.SchemaRenderer" internal/generator/generator.go
```

Expected: pack contains `StructRenderer`, `SetDefaultsRenderer`, `ValidateOwnRenderer`, `UpdateStructRenderer` (and `JSONRenderer` if applicable).

- [ ] **Step 5: Verify each new renderer has a unit test**

```bash
ls internal/generator/render/schema/*_test.go
```

Expected: `defaults_test.go`, `set_defaults_test.go`, `validate_test.go`, `update_struct_test.go`, `url_form_test.go`, `converters_test.go` (plus existing `struct_test.go`, `alias_test.go`, `json_test.go`).

- [ ] **Step 6: Commit final state (if any uncommitted changes remain)**

If all tasks were committed individually, nothing to commit here. Otherwise:

```bash
git status
# If clean: done. If not: commit remaining.
```

---

## Phase 1 Complete

Phase 1 acceptance:
- [x] Callbacks bridge (`render_callbacks_adapter.go`, `render/callbacks.go`) deleted
- [x] `RenderContext.Callbacks` field removed
- [x] SetDefaults, ValidateOwn, UpdateStruct, URLForm, Converters are full `SchemaRenderer` implementations
- [x] Each new renderer has a unit test
- [x] Golden tests byte-for-byte identical to baseline
- [x] `go build ./...`, `go test ./...`, `golangci-lint run` all green

**Next:** Phase 2 plan (Singleton-рендеры — UTCTime, ExpectedValidators) will be written after Phase 1 is merged.

---

## Notes for the executing engineer

- **TypeMapper interface:** Verify `render.TypeMapper` (in `internal/generator/render/typemap.go`) exposes all methods the new renderers need: `GoType(s)`, `RequiredForMode(p, mode)`, `SetMode(mode)`, etc. If any are missing — add them to the interface and the adapter in `typemap_adapter.go`. The adapter itself stays until Phase 6 (TypeMapper migration), but the interface can be extended now.
- **`currentMode()` helper:** Look at `StructRenderer.currentMode()` in `struct.go` — it tracks the active mode (Request/Response) for split-struct rendering. New renderers (`SetDefaultsRenderer`, `ValidateOwnRenderer`) need the same helper. Either duplicate it (small) or extract to a shared helper in `render/schema/`.
- **`gogen.Import` vs `addImport`:** The bridge used `m.addImport(path, alias)`. The render path uses `r.Imports.Add(gogen.Import{Path: path, Alias: alias})`. Verify the dedup behavior is identical (it should be — `ImportTracker.Add` dedups by Path+Alias).
- **Ordering of properties:** Both old and new code iterate `s.Properties` in spec order. No sorting needed. If a golden diff shows ordering issues — investigate whether the old path sorted somewhere that the new path doesn't.
- **Test data coverage:** `testdata/project/minimal/` may not exercise all code paths (e.g., split-mode + shared fields for converters). If a renderer's golden coverage is thin — rely on unit tests for that renderer. Consider adding a richer testdata project in a future task (not in Phase 1 scope).
