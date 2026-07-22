# T27 Phase 2 — Singleton-renderers: UTCTime + ExpectedValidators Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate `internal/generator/utc_time.go` and `internal/generator/expected_validators.go` by migrating them to `SingletonRenderer` implementations in `internal/generator/render/schema/`, dispatched via `compose.FileComposer.ComposeSingletonFile`.

**Architecture:** Two new `SingletonRenderer` types (`UTCTimeRenderer`, `ExpectedValidatorsRenderer`) implement `Render(ctx) → (body, imports, err)` + `FilePath()`. The generator calls `ComposeSingletonFile` per renderer instead of the legacy `utcTimeFile()`/`expectedValidatorsFile()` methods. After migration, the legacy files are deleted. The byte-for-byte output must match the existing golden files (`testdata/project/golden/minimal/model/expected_validators.gen.go` exists; `utc_time.gen.go` does not, because `minimal` project has `USE_UTC_FOR_DATE_TIME` off — UTC coverage relies on unit tests).

**Tech Stack:** Go 1.22+, `nschugorev/oapigenerator/internal/codegen`, `nschugorev/oapigenerator/internal/codegen/gogen`, `nschugorev/oapigenerator/internal/generator/render`, `nschugorev/oapigenerator/internal/generator/compose`, `nschugorev/oapigenerator/internal/parser`, `github.com/stretchr/testify/assert` + `require`.

---

## File Structure

**Create:**
- `internal/generator/render/schema/utc_time.go` — `UTCTimeRenderer` (`SingletonRenderer`).
- `internal/generator/render/schema/utc_time_test.go` — unit tests for `UTCTimeRenderer`.
- `internal/generator/render/schema/expected_validators.go` — `ExpectedValidatorsRenderer` (`SingletonRenderer`).
- `internal/generator/render/schema/expected_validators_test.go` — unit tests for `ExpectedValidatorsRenderer`.

**Modify:**
- `internal/generator/generator.go` — replace bodies of `writeUTCTimeFile`/`writeExpectedValidatorsFile` to call `ComposeSingletonFile`; remove `g.factory` use for those files.

**Delete:**
- `internal/generator/utc_time.go` — after Task 1 wires `UTCTimeRenderer`.
- `internal/generator/expected_validators.go` — after Task 3 wires `ExpectedValidatorsRenderer`. `collectExpectedValidatorNames` + `sortStrings` helpers migrate into `expected_validators.go` (render/schema package).

**Keep unchanged:**
- `internal/generator/compose/composer.go` — `ComposeSingletonFile` already exists.
- `internal/generator/render/base.go` — `SingletonRenderer` interface already exists.

---

### Task 1: UTCTimeRenderer — port body + FilePath

**Files:**
- Create: `internal/generator/render/schema/utc_time.go`

- [ ] **Step 1: Write the failing test**

Create `internal/generator/render/schema/utc_time_test.go`:

```go
package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nschugorev/oapigenerator/internal/generator/render"
)

func TestUTCTimeRenderer_FilePath(t *testing.T) {
	t.Parallel()

	r := NewUTCTimeRenderer()
	assert.Equal(t, "model/utc_time.gen.go", r.FilePath())
}

func TestUTCTimeRenderer_RenderBodyAndImports(t *testing.T) {
	t.Parallel()

	r := NewUTCTimeRenderer()
	ctx := &render.RenderContext{}

	body, imps, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)
	assert.Contains(t, got, "type UTCTime time.Time")
	assert.Contains(t, got, "func (u UTCTime) MarshalJSON() ([]byte, error) {")
	assert.Contains(t, got, "func (u *UTCTime) UnmarshalJSON(data []byte) error {")
	assert.Contains(t, got, "return json.Marshal(time.Time(u).UTC())")
	assert.Contains(t, got, "*u = UTCTime(t.UTC())")

	paths := importPaths(imps)
	assert.Contains(t, paths, "encoding/json")
	assert.Contains(t, paths, "time")
}

func TestUTCTimeRenderer_RenderIsDeterministic(t *testing.T) {
	t.Parallel()

	r := NewUTCTimeRenderer()
	ctx := &render.RenderContext{}

	body1, _, err := r.Render(ctx)
	require.NoError(t, err)

	body2, _, err := r.Render(ctx)
	require.NoError(t, err)

	assert.Equal(t, body1, body2)
}

// importPaths extracts .Path from each import for easy assertion.
func importPaths(imps *render.ImportTracker) []string {
	out := make([]string, 0, len(imps.Imports()))
	for _, imp := range imps.Imports() {
		out = append(out, imp.Path)
	}

	return out
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/generator/render/schema/ -run TestUTCTimeRenderer -v`
Expected: FAIL with `undefined: NewUTCTimeRenderer`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/generator/render/schema/utc_time.go`:

```go
// Package schema: UTCTimeRenderer — SingletonRenderer, рендерящий
// model/utc_time.gen.go (кастомный тип UTCTime). Заменяет
// Generator.utcTimeFile (internal/generator/utc_time.go). Тело портировано
// байт-в-байт; генерация происходит только когда вызывающая сторона
// (Generator.writeUTCTimeFile) подтвердила, что флаг USE_UTC_FOR_DATE_TIME
// включён — renderer не проверяет флаг сам.
package schema

import (
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/generator/render"
)

// utcTimeBody — дословное тело файла model/utc_time.gen.go. Хранится как
// единая константа, чтобы гарантировать байт-в-байт совпадение со старым
// выводом Generator.renderUTCTimeType.
const utcTimeBody = `// UTCTime — обёртка над time.Time, принудительно сериализующая
// значение в UTC. Используется для date-time полей, когда включён
// флаг USE_UTC_FOR_DATE_TIME.
type UTCTime time.Time

func (u UTCTime) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Time(u).UTC())
}

func (u *UTCTime) UnmarshalJSON(data []byte) error {
	var t time.Time
	if err := json.Unmarshal(data, &t); err != nil {
		return err
	}

	*u = UTCTime(t.UTC())

	return nil
}
`

// utcTimeImports — импорты для model/utc_time.gen.go. Порядок совпадает со
// старым utcTimeFile: encoding/json, time.
var utcTimeImports = []gogen.Import{
	{Path: "encoding/json"},
	{Path: "time"},
}

// UTCTimeRenderer рендерит model/utc_time.gen.go. Не зависит от RenderContext —
// тип фиксированный, без schema-специфичных данных.
type UTCTimeRenderer struct{}

// NewUTCTimeRenderer строит UTCTimeRenderer.
func NewUTCTimeRenderer() *UTCTimeRenderer { return &UTCTimeRenderer{} }

// Render возвращает тело файла и импорты. ctx не используется.
func (UTCTimeRenderer) Render(_ *render.RenderContext) ([]byte, *render.ImportTracker, error) {
	imps := render.NewImportTracker()
	for _, imp := range utcTimeImports {
		imps.Add(imp)
	}

	return []byte(utcTimeBody), imps, nil
}

// FilePath возвращает путь генерируемого файла.
func (UTCTimeRenderer) FilePath() string { return "model/utc_time.gen.go" }

// compile-time guard: UTCTimeRenderer удовлетворяет SingletonRenderer.
var _ render.SingletonRenderer = (*UTCTimeRenderer)(nil)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/generator/render/schema/ -run TestUTCTimeRenderer -v`
Expected: PASS — all 3 tests green.

- [ ] **Step 5: Commit**

```bash
git add internal/generator/render/schema/utc_time.go internal/generator/render/schema/utc_time_test.go
git commit -m "T27 Phase 2 Task 1: UTCTimeRenderer"
```

---

### Task 2: Wire UTCTimeRenderer into generator + delete legacy utc_time.go

**Files:**
- Modify: `internal/generator/generator.go` (function `writeUTCTimeFile`, locate by `func (g *Generator) writeUTCTimeFile`).
- Modify: `internal/generator/generator_test.go` (append new tests).
- Delete: `internal/generator/utc_time.go`.

- [ ] **Step 1: Write the failing test**

Append to `internal/generator/generator_test.go`. Use the existing `parseSpec` + `testProject` + `collectWriter` helpers (already defined in that file around lines 1946–1982):

```go
func TestGenerate_UTCTimeFile_WhenFeatureOn_EmitsFile(t *testing.T) {
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
	project := testProject(t, doc, "example.com/test")
	project.Features.UseUTCForDateTime.Value = true

	fw := &collectWriter{files: map[string][]byte{}}
	require.NoError(t, Generate(fw, project, nil))

	body := string(fw.files["model/utc_time.gen.go"])
	assert.Contains(t, body, "type UTCTime time.Time")
	assert.Contains(t, body, "func (u UTCTime) MarshalJSON() ([]byte, error) {")
	assert.Contains(t, body, "func (u *UTCTime) UnmarshalJSON(data []byte) error {")
}

func TestGenerate_UTCTimeFile_WhenFeatureOff_SkipsFile(t *testing.T) {
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
	project := testProject(t, doc, "example.com/test")
	// UseUTCForDateTime.Value остаётся false.

	fw := &collectWriter{files: map[string][]byte{}}
	require.NoError(t, Generate(fw, project, nil))

	_, ok := fw.files["model/utc_time.gen.go"]
	assert.False(t, ok, "utc_time.gen.go must not be emitted when USE_UTC_FOR_DATE_TIME is off")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/generator/ -run TestGenerate_UTCTimeFile -v`
Expected: `TestGenerate_UTCTimeFile_WhenFeatureOn_EmitsFile` PASSES (old `utcTimeFile` produces matching content), `TestGenerate_UTCTimeFile_WhenFeatureOff_SkipsFile` PASSES. Both passing means the wire-test is not yet discriminating between old and new code — that is acceptable. The real verification is Step 5 after deleting `utc_time.go`: if the renderer is not wired, `WhenFeatureOn_EmitsFile` will fail with "file not in map".

- [ ] **Step 3: Replace writeUTCTimeFile body**

In `internal/generator/generator.go`, locate `func (g *Generator) writeUTCTimeFile(fw codegen.FileWriter) error`. Replace its body to use `ComposeSingletonFile`:

```go
// writeUTCTimeFile пишет model/utc_time.gen.go, если включён флаг
// USE_UTC_FOR_DATE_TIME. Рендер идёт через UTCTimeRenderer (SingletonRenderer)
// + compose.FileComposer — замена устаревшему Generator.utcTimeFile.
func (g *Generator) writeUTCTimeFile(fw codegen.FileWriter) error {
	if !g.project.Features.UseUTCForDateTime.Value {
		return nil
	}

	ctx := g.newSchemaRenderContext()

	file, err := g.composer.ComposeSingletonFile(schemarender.NewUTCTimeRenderer(), ctx)
	if err != nil {
		return fmt.Errorf("compose utc_time: %w", err)
	}

	const fname = "model/utc_time.gen.go"
	if err := fw.WriteFile(fname, file); err != nil {
		return fmt.Errorf("write %s: %w", fname, err)
	}

	return nil
}
```

Note: `g.newSchemaRenderContext` already exists from Phase 1. If the helper signature differs, adapt — but the goal is: build a `*render.RenderContext` and pass it to `ComposeSingletonFile`.

- [ ] **Step 4: Delete legacy utc_time.go**

```bash
git rm internal/generator/utc_time.go
```

- [ ] **Step 5: Run build + tests to verify**

Run: `go build ./... && go test ./...`
Expected: all green. `TestGenerate_UTCTimeFile_WhenFeatureOn_EmitsFile` passes (renderer wired); `TestGenerate_UTCTimeFile_WhenFeatureOff_SkipsFile` passes (feature check still in place); `TestE2E_Minimal` and `TestE2E_GoldenCompiles` still pass (minimal project has feature off, so no `utc_time.gen.go` is emitted — golden unchanged).

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "T27 Phase 2 Task 2: wire UTCTimeRenderer, delete legacy utc_time.go"
```

---

### Task 3: ExpectedValidatorsRenderer — port body + helpers + FilePath

**Files:**
- Create: `internal/generator/render/schema/expected_validators.go`

- [ ] **Step 1: Write the failing test**

Create `internal/generator/render/schema/expected_validators_test.go`:

```go
package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
)

func TestExpectedValidatorsRenderer_FilePath(t *testing.T) {
	t.Parallel()

	r := NewExpectedValidatorsRenderer()
	assert.Equal(t, "model/expected_validators.gen.go", r.FilePath())
}

func TestExpectedValidatorsRenderer_NoNamedValidators_EmptyBody(t *testing.T) {
	t.Parallel()

	r := NewExpectedValidatorsRenderer()
	ctx := &render.RenderContext{
		Project: &parser.Project{
			Model: newModelWithSchemas([]*parser.Schema{
				{
					Name: "Item",
					Type: "object",
					Properties: []*parser.Property{
						{Name: "size", Schema: &parser.Schema{Type: "integer"}},
					},
				},
			}),
		},
	}

	body, _, err := r.Render(ctx)
	require.NoError(t, err)
	assert.Empty(t, body, "schema with only SimpleRule validations must produce no file body")
}

func TestExpectedValidatorsRenderer_NamedValidators_RendersSortedList(t *testing.T) {
	t.Parallel()

	r := NewExpectedValidatorsRenderer()
	ctx := &render.RenderContext{
		Project: &parser.Project{
			Model: newModelWithSchemas([]*parser.Schema{
				{
					Name: "Item",
					Type: "object",
					Validations: []parser.ValidationRule{
						parser.NamedRule{Name: "app.ItemConsistency"},
					},
					Properties: []*parser.Property{
						{
							Name: "name",
							Schema: &parser.Schema{Type: "string"},
							Validations: []parser.ValidationRule{
								parser.NamedRule{Name: "app.NonEmptyName"},
							},
						},
						{
							Name: "size",
							Schema: &parser.Schema{Type: "integer"},
							Validations: []parser.ValidationRule{
								parser.NamedRule{Name: "app.ItemConsistency"}, // дубликат — должен исчезнуть
							},
						},
					},
				},
				{
					Name: "Order",
					Type: "object",
					Validations: []parser.ValidationRule{
						parser.NamedRule{Name: "app.ItemCreateConsistency"},
					},
					Properties: []*parser.Property{},
				},
			}),
		},
	}

	body, imps, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)
	assert.Contains(t, got, "func ExpectedValidatorNames() []string {")
	assert.Contains(t, got, `"app.ItemConsistency",`)
	assert.Contains(t, got, `"app.ItemCreateConsistency",`)
	assert.Contains(t, got, `"app.NonEmptyName",`)

	// Порядок отсортированный: ItemConsistency < ItemCreateConsistency < NonEmptyName.
	idxConsistency := indexOf(t, got, `"app.ItemConsistency"`)
	idxCreate := indexOf(t, got, `"app.ItemCreateConsistency"`)
	idxNonEmpty := indexOf(t, got, `"app.NonEmptyName"`)
	assert.Less(t, idxConsistency, idxCreate)
	assert.Less(t, idxCreate, idxNonEmpty)

	// Импортов нет — файл только объявляет функцию, возвращающую []string.
	assert.Empty(t, imps.Imports())
}

func TestExpectedValidatorsRenderer_NilProject_EmptyBody(t *testing.T) {
	t.Parallel()

	r := NewExpectedValidatorsRenderer()
	ctx := &render.RenderContext{}

	body, _, err := r.Render(ctx)
	require.NoError(t, err)
	assert.Empty(t, body)
}

// newModelWithSchemas строит *parser.Model с заданными схемами (и пустым
// индексом). Используется тестами ExpectedValidatorsRenderer.
func newModelWithSchemas(schemas []*parser.Schema) *parser.Model {
	m := &parser.Model{}
	m.SetSchemas(schemas)
	return m
}

func indexOf(t *testing.T, s, substr string) int {
	t.Helper()
	i := indexOfString(s, substr)
	require.GreaterOrEqual(t, i, 0, "substring %q not found in %q", substr, s)
	return i
}

// indexOfString — обёртка без require, чтобы можно было использовать в
// декларативных assert-цепочках.
func indexOfString(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/generator/render/schema/ -run TestExpectedValidatorsRenderer -v`
Expected: FAIL with `undefined: NewExpectedValidatorsRenderer`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/generator/render/schema/expected_validators.go`:

```go
// Package schema: ExpectedValidatorsRenderer — SingletonRenderer, рендерящий
// model/expected_validators.gen.go. Заменяет Generator.expectedValidatorsFile
// (internal/generator/expected_validators.go). Тело и хелперы
// (collectExpectedValidatorNames, sortStrings) портированы байт-в-байт.
//
// Renderer смотрит на r.Ctx.Project.Model.Schemas() — все схемы документа.
// Имена валидаторов собираются с schema-level (sh.Validations) и property-level
// (p.Validations). SimpleRule игнорируются, учитываются только NamedRule.
// Файл генерируется только если есть хотя бы один named-валидатор — иначе
// Render возвращает пустое body, а вызывающая сторона пропускает запись файла.
package schema

import (
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
	"strconv"
)

// ExpectedValidatorsRenderer рендерит model/expected_validators.gen.go.
type ExpectedValidatorsRenderer struct{}

// NewExpectedValidatorsRenderer строит ExpectedValidatorsRenderer.
func NewExpectedValidatorsRenderer() *ExpectedValidatorsRenderer {
	return &ExpectedValidatorsRenderer{}
}

// FilePath возвращает путь генерируемого файла.
func (ExpectedValidatorsRenderer) FilePath() string {
	return "model/expected_validators.gen.go"
}

// Render возвращает тело файла и (пустой) трекер импортов. Если ни одного
// named-валидатора нет — возвращает пустое body и nil-трекер; вызывающая
// сторона (Generator.writeExpectedValidatorsFile) пропускает запись файла.
func (ExpectedValidatorsRenderer) Render(ctx *render.RenderContext) ([]byte, *render.ImportTracker, error) {
	names := collectExpectedValidatorNames(schemasOf(ctx))
	if len(names) == 0 {
		return nil, nil, nil
	}

	var buf []byte
	buf = append(buf, []byte("// ExpectedValidatorNames возвращает отсортированный уникальный список\n")...)
	buf = append(buf, []byte("// имён named-валидаторов, на которые ссылаются x-validations схем.\n")...)
	buf = append(buf, []byte("// Используется с validator.Registry.AssertExact при старте приложения\n")...)
	buf = append(buf, []byte("// для проверки, что все валидаторы зарегистрированы.\n")...)
	buf = append(buf, []byte("func ExpectedValidatorNames() []string {\n")...)
	buf = append(buf, []byte("\treturn []string{\n")...)

	for _, n := range names {
		buf = append(buf, []byte("\t\t")...)
		buf = append(buf, []byte(strconv.Quote(n))...)
		buf = append(buf, ',', '\n')
	}

	buf = append(buf, []byte("\t}\n")...)
	buf = append(buf, []byte("}\n")...)

	return buf, render.NewImportTracker(), nil
}

// schemasOf безопасно достаёт схемы проекта из контекста. Возвращает nil при
// отсутствии Project или Model — покрывает тестовые ctx без проекта.
func schemasOf(ctx *render.RenderContext) []*parser.Schema {
	if ctx == nil || ctx.Project == nil || ctx.Project.Model == nil {
		return nil
	}

	return ctx.Project.Model.Schemas()
}

// collectExpectedValidatorNames собирает уникальные имена именованных
// валидаторов со всех схем документа (property-level + schema-level).
// Портировано из internal/generator/expected_validators.go.
func collectExpectedValidatorNames(schemas []*parser.Schema) []string {
	seen := make(map[string]bool)

	for _, sh := range schemas {
		for _, rule := range sh.Validations {
			if nr, ok := rule.(parser.NamedRule); ok {
				seen[nr.Name] = true
			}
		}

		for _, p := range sh.Properties {
			for _, rule := range p.Validations {
				if nr, ok := rule.(parser.NamedRule); ok {
					seen[nr.Name] = true
				}
			}
		}
	}

	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}

	return sortStrings(out)
}

// sortStrings сортирует срез строк in-place вставками. Портировано из
// internal/generator/expected_validators.go — сохранён алгоритм, чтобы
// гарантироать байт-в-байт совпадение вывода (стандартный sort.Strings может
// отличаться по стабильности для дубликатов, хотя здесь дубликатов нет).
func sortStrings(s []string) []string {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}

	return s
}

// compile-time guard: ExpectedValidatorsRenderer удовлетворяет SingletonRenderer.
var _ render.SingletonRenderer = (*ExpectedValidatorsRenderer)(nil)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/generator/render/schema/ -run TestExpectedValidatorsRenderer -v`
Expected: PASS — all 4 tests green.

- [ ] **Step 5: Commit**

```bash
git add internal/generator/render/schema/expected_validators.go internal/generator/render/schema/expected_validators_test.go
git commit -m "T27 Phase 2 Task 3: ExpectedValidatorsRenderer"
```

---

### Task 4: Wire ExpectedValidatorsRenderer + delete legacy expected_validators.go

**Files:**
- Modify: `internal/generator/generator.go` (function `writeExpectedValidatorsFile`).
- Modify: `internal/generator/generator_test.go` (append new tests).
- Delete: `internal/generator/expected_validators.go`.

- [ ] **Step 1: Write the failing test**

Append to `internal/generator/generator_test.go`:

```go
func TestGenerate_ExpectedValidatorsFile_WithNamedValidators_EmitsFile(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Item:
      type: object
      x-validations: ["app.ItemConsistency"]
      properties:
        name: {type: string}
`)
	project := testProject(t, doc, "example.com/test")

	fw := &collectWriter{files: map[string][]byte{}}
	require.NoError(t, Generate(fw, project, nil))

	body := string(fw.files["model/expected_validators.gen.go"])
	assert.Contains(t, body, "func ExpectedValidatorNames() []string {")
	assert.Contains(t, body, `"app.ItemConsistency"`)
}

func TestGenerate_ExpectedValidatorsFile_NoNamedValidators_SkipsFile(t *testing.T) {
	t.Parallel()

	doc := parseSpec(t, `
openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    Item:
      type: object
      properties:
        name: {type: string}
`)
	project := testProject(t, doc, "example.com/test")

	fw := &collectWriter{files: map[string][]byte{}}
	require.NoError(t, Generate(fw, project, nil))

	_, ok := fw.files["model/expected_validators.gen.go"]
	assert.False(t, ok, "expected_validators.gen.go must not be emitted when no named validators exist")
}
```

Note: the spec uses `x-validations: ["app.ItemConsistency"]` at schema level. Verify by reading `internal/parser/validations_test.go` for the exact YAML key — if the key is different (e.g., `x-validator`), adapt the test spec.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/generator/ -run TestGenerate_ExpectedValidatorsFile -v`
Expected: both tests PASS against the old `expectedValidatorsFile` code (it already produces matching output). The real verification is Step 5 after deleting `expected_validators.go`: if the renderer is not wired, `WithNamedValidators_EmitsFile` will fail with "file not in map".

- [ ] **Step 3: Replace writeExpectedValidatorsFile body**

In `internal/generator/generator.go`, locate `func (g *Generator) writeExpectedValidatorsFile(fw codegen.FileWriter) error`. Replace its body. Use the direct-`Render` variant (not `ComposeSingletonFile`) because `ExpectedValidatorsRenderer.Render` returns `(nil, nil, nil)` when there are no named validators, and we need to skip writing in that case:

```go
// writeExpectedValidatorsFile пишет model/expected_validators.gen.go, если
// в документе есть хотя бы один named-валидатор. Рендер идёт через
// ExpectedValidatorsRenderer (SingletonRenderer) — замена устаревшему
// Generator.expectedValidatorsFile. Direct-Render (без ComposeSingletonFile)
// позволяет пропустить запись файла, когда именованных валидаторов нет.
func (g *Generator) writeExpectedValidatorsFile(fw codegen.FileWriter) error {
	ctx := g.newSchemaRenderContext()
	r := schemarender.NewExpectedValidatorsRenderer()

	body, imps, err := r.Render(ctx)
	if err != nil {
		return fmt.Errorf("render expected_validators: %w", err)
	}

	if len(body) == 0 {
		return nil
	}

	file := g.factory.Create(&gogen.File{
		Package: "model",
		Imports: imps.Imports(),
		Body:    body,
	})

	const fname = "model/expected_validators.gen.go"
	if err := fw.WriteFile(fname, file); err != nil {
		return fmt.Errorf("write %s: %w", fname, err)
	}

	return nil
}
```

- [ ] **Step 4: Delete legacy expected_validators.go**

```bash
git rm internal/generator/expected_validators.go
```

- [ ] **Step 5: Run build + tests to verify**

Run: `go build ./... && go test ./...`
Expected: all green. `TestGenerate_ExpectedValidatorsFile_*` pass; `TestE2E_Minimal` and `TestE2E_GoldenCompiles` must pass — the minimal project's golden file `testdata/project/golden/minimal/model/expected_validators.gen.go` must match byte-for-byte. Verify with:

```bash
go test ./cmd/oapigen/ -run TestE2E_Minimal -v
git status testdata/
```

Expected: test PASS; `testdata/` working tree clean (no modified golden files).

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "T27 Phase 2 Task 4: wire ExpectedValidatorsRenderer, delete legacy expected_validators.go"
```

---

### Task 5: Verify Phase 2 acceptance criteria

Final verification that Phase 2 is complete.

- [ ] **Step 1: Build + test + vet**

```bash
go build ./...
go test ./...
go vet ./...
```

Expected: all green.

- [ ] **Step 2: Byte-for-byte identity vs golden**

```bash
go test ./cmd/oapigen/ -run TestE2E_Minimal -v
go test ./cmd/oapigen/ -run TestE2E_GoldenCompiles -v
```

Expected: both PASS. Golden file `testdata/project/golden/minimal/model/expected_validators.gen.go` unchanged; `testdata/project/` working tree clean (no modifications to golden files).

Verify working tree status:

```bash
git status testdata/
```

Expected: clean (no modified golden files).

- [ ] **Step 3: Verify legacy files deleted**

```bash
ls internal/generator/utc_time.go internal/generator/expected_validators.go 2>&1
```

Expected: both files report "No such file or directory".

- [ ] **Step 4: Verify renderer files exist**

```bash
ls internal/generator/render/schema/utc_time.go \
   internal/generator/render/schema/utc_time_test.go \
   internal/generator/render/schema/expected_validators.go \
   internal/generator/render/schema/expected_validators_test.go
```

Expected: all 4 files exist.

- [ ] **Step 5: Verify generator wires the renderers**

```bash
grep -n "NewUTCTimeRenderer\|NewExpectedValidatorsRenderer" internal/generator/generator.go
```

Expected: 2 matches — one per renderer.

- [ ] **Step 6: Verify no references to deleted legacy symbols**

```bash
grep -rn "utcTimeFile\|expectedValidatorsFile\|g\.utcTimeFile\|g\.expectedValidatorsFile" --include="*.go" internal/generator/
```

Expected: no matches (only references in comments describing the migration, if any).

- [ ] **Step 7: Commit final state (if any uncommitted changes remain)**

If all tasks were committed individually, nothing to commit here. Otherwise:

```bash
git status
# If clean: done. If not: commit remaining.
```

---

## Phase 2 Complete

Phase 2 acceptance:
- [ ] `internal/generator/utc_time.go` deleted
- [ ] `internal/generator/expected_validators.go` deleted
- [ ] `UTCTimeRenderer` and `ExpectedValidatorsRenderer` are `SingletonRenderer` implementations in `internal/generator/render/schema/`
- [ ] Each renderer has a unit test
- [ ] Generator dispatches both via `compose.FileComposer.ComposeSingletonFile` (or direct `Render` + `factory.Create` for the skip-when-empty case)
- [ ] Golden tests byte-for-byte identical to baseline (`TestE2E_Minimal`, `TestE2E_GoldenCompiles` pass; `testdata/` working tree clean)
- [ ] `go build ./...`, `go test ./...`, `go vet ./...` all green

**Next:** Phase 3 plan (Operations-side interfaces — ClientInterfaceRenderer, ClientSugarRenderer, AuditClientRenderer, ServerInterfaceRenderer) will be written after Phase 2 is merged.

---

## Notes for the executing engineer

- **`USE_UTC_FOR_DATE_TIME` coverage gap:** The `testdata/project/minimal` project has this feature OFF — so `model/utc_time.gen.go` is never emitted in the golden test. `UTCTimeRenderer` is covered by unit tests + the `TestGenerate_UTCTimeFile_WhenFeatureOn_EmitsFile` wire-test. Do NOT turn the feature on in minimal just to get golden coverage — that would change `testdata/` and break byte-for-byte baseline for other fields. If broader coverage is desired, add a separate testdata project in a future task (out of Phase 2 scope).
- **Direct-`Render` vs `ComposeSingletonFile`:** Task 2 uses `ComposeSingletonFile` for `UTCTimeRenderer` (file always emitted when feature is on — no skip case). Task 4 uses direct `r.Render(ctx)` + `g.factory.Create` for `ExpectedValidatorsRenderer` because the renderer returns `(nil, nil, nil)` when there are no named validators, and we need to skip file writing in that case. Both patterns are valid — `ComposeSingletonFile` is a thin wrapper around `Render` + `assembleBytes` (see `internal/generator/compose/composer.go:82`).
- **Byte-for-byte fidelity:** `UTCTimeRenderer` stores the file body as a single string constant — do NOT reflow, reformat, or "improve" the text. `ExpectedValidatorsRenderer` builds the body via `append` to preserve exact spacing (leading tabs `\t\t`, trailing `,\n`). Compare output to `testdata/project/golden/minimal/model/expected_validators.gen.go` after wiring — they must match byte-for-byte.
- **`parser.NamedRule{Name: "..."}`:** Field name is `Name` (confirmed at `internal/parser/document.go:223`). Tests in Task 3 use this literal directly.
- **`collectWriter` + `parseSpec` + `testProject`:** All three helpers already exist in `internal/generator/generator_test.go` (around lines 1946, 2013, 1960). Use them verbatim — do not invent new helpers.
- **`Generate(fw, project, nil)`:** Third arg is `*parser.SchemaIndex`, and `nil` is acceptable for unit tests (matches existing pattern at `generator_test.go:45`). Production code in `cmd/oapigen/main.go` builds the index via `&parser.SchemaIndex{Schemas: map[string]*SchemaEntry{}}` + `buildSchemaIndex` — not needed for these tests.
- **`g.newSchemaRenderContext` helper:** Already exists from Phase 1. If it was renamed or moved, grep for `newSchemaRenderContext` in `internal/generator/generator.go`. The helper returns `*render.RenderContext` with Project, SchemaIndex, Features, Splittable, ModulePath, ImportPrefix, TypeMapper all set — sufficient for both singleton renderers.
- **No Callbacks bridge:** Phase 1 deleted `RenderContext.Callbacks`. Singleton renderers do not need it — `Render(ctx)` receives the context and reads `ctx.Project` directly. Do not re-introduce Callbacks.
- **`x-validations` YAML key:** Confirmed at `testdata/project/minimal/src/openapi/openapi.yaml:70` — schema-level `x-validations: ["app.ItemConsistency"]` and property-level `x-validations: ["Size >=1", "app.NonEmptyName"]` both work. Task 4 wire-test uses schema-level form.
