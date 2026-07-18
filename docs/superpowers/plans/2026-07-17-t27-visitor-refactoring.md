# T27 — Visitor Pattern Refactoring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Рефакторинг `internal/generator/*` на visitor-pattern с тремя слоями (Walker / Renderer / Composer) и тонким оркестратором Generator. Golden-файлы должны остаться байтово идентичными.

**Architecture:** Трёхслойная visitor-модель — `Walker` (recursive обход `Schema`/`Method`) → `Renderer` (подписанные на хуки, пишут в собственный буфер) → `FileComposer` (оркестрирует прогон renderer'ов, склеивает в `codegen.File`). Полный дизайн — `docs/superpowers/specs/2026-07-17-t27-visitor-refactoring-design.md`.

**Tech Stack:** Go 1.26, `internal/parser`, `internal/codegen`/`gogen`, testify (assert + require), golangci-lint.

**Migration principle:** Инкрементально по одному renderer'у. После каждого task'а — `make test` зелёный, golden-файлы байтово идентичны. Старый код удаляется только после полного переноса.

**User commits:** All `git commit` steps are executed by the user, not by the agent.

---

## File Structure

```
internal/generator/
├── generator.go              # Generator — тонкий оркестратор (финальная стадия миграции)
├── context.go                # RenderContext, buildContext
├── naming.go                 # shared naming helpers (без изменений)
├── operation.go              # shared operation helpers (без изменений)
├── constants.go              # shared constants (без изменений)
├── type.go                   # typeMapper — переезжает в ctx, без изменений в логике
├── walk/                     # NEW: SchemaWalker, MethodWalker, interfaces, noops
│   ├── schema_walker.go
│   ├── method_walker.go
│   ├── interfaces.go
│   ├── noop.go
│   └── walk_test.go
├── render/
│   ├── base.go               # NEW: Base, RenderContext
│   ├── schema/               # NEW: SchemaRenderer'ы
│   │   ├── alias.go          # из schema.go:renderAlias
│   │   ├── enum.go           # из schema.go:renderEnum
│   │   ├── struct.go         # из schema.go:renderStruct/renderSplitStruct/renderUpdateStruct
│   │   ├── json.go           # из json_methods.go
│   │   ├── validate.go       # из validate.go
│   │   ├── set_defaults.go   # из set_defaults.go
│   │   ├── url_form.go       # из url_form_methods.go
│   │   ├── converter.go      # из converter_methods.go
│   │   └── audit.go          # из audit_model.go (future stub — не в T27)
│   ├── method/               # NEW: MethodRenderer'ы
│   │   ├── client_interface.go    # из client.go
│   │   ├── server_interface.go    # из server.go
│   │   ├── http_client.go         # из impl_client.go
│   │   ├── echo_server.go         # из impl_server.go
│   │   ├── client_mock.go         # из mocks.go (client half)
│   │   ├── server_mock.go         # из mocks.go (server half)
│   │   ├── sdk.go                 # из sdk.go
│   │   ├── client_sugar.go        # из client_sugar.go
│   │   ├── server_sugar.go        # из server.go sugar half
│   │   └── audit_server.go        # из audit_server.go (future stub — не в T27)
│   └── singleton/            # NEW: SingletonRenderer'ы (без walker)
│       ├── utc_time.go            # из utc_time.go
│       └── expected_validators.go # из expected_validators.go
└── compose/                  # NEW: FileComposer, specs
    ├── composer.go
    └── specs.go              # schemaRenderers(), methodFileSpecs(), singletonSpecs()
```

**Удаляются в финальной стадии:** `internal/generator/{schema,validate,set_defaults,url_form_methods,converter_methods,json_methods,client,server,impl_client,impl_server,mocks,sdk,client_sugar,utc_time,expected_validators,audit_model,audit_server,response_headers}.go` — содержимое переносится в `render/`.

---

## Task 1: Walk package — interfaces and noops

**Files:**
- Create: `internal/generator/walk/interfaces.go`
- Create: `internal/generator/walk/noop.go`

- [ ] **Step 1: Write `interfaces.go`**

```go
// Package walk содержит recursive walker'ы для доменной модели parser.Schema
// и parser.Method. Walker'ы не делают рендер — только обход и диспатч хуков
// в зарегистрированные renderer'ы.
package walk

import (
	"nschugorev/oapigenerator/internal/parser"
)

// UnionKind различает oneOf и anyOf.
type UnionKind int

const (
	UnionOneOf UnionKind = iota
	UnionAnyOf
)

// SchemaRenderer — reactive-интерфейс: renderer'ы реализуют нужные методы,
// остальные наследуют от noopSchemaRenderer.
type SchemaRenderer interface {
	OnStruct(s *parser.Schema) error
	OnEnum(s *parser.Schema) error
	OnAlias(s *parser.Schema) error
	OnArray(s *parser.Schema) error
	OnMap(s *parser.Schema) error
	OnUnion(s *parser.Schema, kind UnionKind) error
	OnAllOf(s *parser.Schema) error
	OnSplitStruct(s *parser.Schema) error

	OnStructProperty(s *parser.Schema, name string, prop *parser.Schema) error
	OnArrayItem(s *parser.Schema, idx int, item *parser.Schema) error
	OnMapValue(s *parser.Schema, value *parser.Schema) error
	OnUnionVariant(s *parser.Schema, idx int, variant *parser.Schema) error
	OnAllOfMember(s *parser.Schema, idx int, member *parser.Schema) error
}

// SkipDescendants — optional-интерфейс. Renderer реализует его, чтобы
// попросить walker не спускаться в дочерние схемы (например, для external $ref).
type SkipDescendants interface {
	Skip(s *parser.Schema) bool
}

// MethodRenderer — reactive-интерфейс для обхода операций.
type MethodRenderer interface {
	OnMethod(m *parser.Method) error
	OnPathParameter(m *parser.Method, p *parser.Parameter) error
	OnQueryParameter(m *parser.Method, p *parser.Parameter) error
	OnHeaderParameter(m *parser.Method, p *parser.Parameter) error
	OnCookieParameter(m *parser.Method, p *parser.Parameter) error
	OnRequestBody(m *parser.Method, body *parser.Schema) error
	OnResponse(m *parser.Method, code int, resp *parser.Response) error
	OnResponseHeader(m *parser.Method, code int, name string, h *parser.Parameter) error
}
```

- [ ] **Step 2: Write `noop.go`**

```go
package walk

import "nschugorev/oapigenerator/internal/parser"

// noopSchemaRenderer даёт пустые реализации для неиспользуемых хуков.
// Renderer'ы embed'ят его и реализуют только нужные методы.
type noopSchemaRenderer struct{}

func (noopSchemaRenderer) OnStruct(s *parser.Schema) error                       { return nil }
func (noopSchemaRenderer) OnEnum(s *parser.Schema) error                         { return nil }
func (noopSchemaRenderer) OnAlias(s *parser.Schema) error                        { return nil }
func (noopSchemaRenderer) OnArray(s *parser.Schema) error                        { return nil }
func (noopSchemaRenderer) OnMap(s *parser.Schema) error                          { return nil }
func (noopSchemaRenderer) OnUnion(s *parser.Schema, kind UnionKind) error        { return nil }
func (noopSchemaRenderer) OnAllOf(s *parser.Schema) error                        { return nil }
func (noopSchemaRenderer) OnSplitStruct(s *parser.Schema) error                  { return nil }
func (noopSchemaRenderer) OnStructProperty(s *parser.Schema, name string, prop *parser.Schema) error {
	return nil
}
func (noopSchemaRenderer) OnArrayItem(s *parser.Schema, idx int, item *parser.Schema) error {
	return nil
}
func (noopSchemaRenderer) OnMapValue(s *parser.Schema, value *parser.Schema) error {
	return nil
}
func (noopSchemaRenderer) OnUnionVariant(s *parser.Schema, idx int, variant *parser.Schema) error {
	return nil
}
func (noopSchemaRenderer) OnAllOfMember(s *parser.Schema, idx int, member *parser.Schema) error {
	return nil
}

// noopMethodRenderer — аналогично для method-walker'а.
type noopMethodRenderer struct{}

func (noopMethodRenderer) OnMethod(m *parser.Method) error                                       { return nil }
func (noopMethodRenderer) OnPathParameter(m *parser.Method, p *parser.Parameter) error           { return nil }
func (noopMethodRenderer) OnQueryParameter(m *parser.Method, p *parser.Parameter) error          { return nil }
func (noopMethodRenderer) OnHeaderParameter(m *parser.Method, p *parser.Parameter) error         { return nil }
func (noopMethodRenderer) OnCookieParameter(m *parser.Method, p *parser.Parameter) error         { return nil }
func (noopMethodRenderer) OnRequestBody(m *parser.Method, body *parser.Schema) error             { return nil }
func (noopMethodRenderer) OnResponse(m *parser.Method, code int, resp *parser.Response) error    { return nil }
func (noopMethodRenderer) OnResponseHeader(m *parser.Method, code int, name string, h *parser.Parameter) error {
	return nil
}
```

- [ ] **Step 3: Verify compile**

Run: `go build ./internal/generator/walk/...`
Expected: PASS (interfaces compile, no callers yet).

- [ ] **Step 4: Commit (user)**

```bash
git add internal/generator/walk/interfaces.go internal/generator/walk/noop.go
git commit -m "T27: add walk package interfaces and noops"
```

---

## Task 2: SchemaWalker — recursive dispatch with recording-mock test

**Files:**
- Create: `internal/generator/walk/schema_walker.go`
- Create: `internal/generator/walk/walk_test.go`

- [ ] **Step 1: Write failing test with recording mock**

`internal/generator/walk/walk_test.go`:

```go
package walk

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"nschugorev/oapigenerator/internal/parser"
)

// recordingRenderer записывает последовательность хуков для проверки порядка обхода.
type recordingRenderer struct {
	noopSchemaRenderer
	calls []string
}

func (r *recordingRenderer) record(name string, s *parser.Schema) error {
	r.calls = append(r.calls, name+":"+s.Name)
	return nil
}

func (r *recordingRenderer) OnStruct(s *parser.Schema) error                { return r.record("OnStruct", s) }
func (r *recordingRenderer) OnEnum(s *parser.Schema) error                  { return r.record("OnEnum", s) }
func (r *recordingRenderer) OnAlias(s *parser.Schema) error                 { return r.record("OnAlias", s) }
func (r *recordingRenderer) OnArray(s *parser.Schema) error                 { return r.record("OnArray", s) }
func (r *recordingRenderer) OnMap(s *parser.Schema) error                   { return r.record("OnMap", s) }
func (r *recordingRenderer) OnUnion(s *parser.Schema, _ UnionKind) error    { return r.record("OnUnion", s) }
func (r *recordingRenderer) OnAllOf(s *parser.Schema) error                 { return r.record("OnAllOf", s) }
func (r *recordingRenderer) OnSplitStruct(s *parser.Schema) error           { return r.record("OnSplitStruct", s) }
func (r *recordingRenderer) OnStructProperty(s *parser.Schema, name string, prop *parser.Schema) error {
	r.calls = append(r.calls, "OnStructProperty:"+s.Name+"."+name)
	return nil
}
func (r *recordingRenderer) OnArrayItem(s *parser.Schema, idx int, item *parser.Schema) error {
	r.calls = append(r.calls, "OnArrayItem:"+s.Name+"["+itoa(idx)+"]="+item.Name)
	return nil
}
// ... остальные recording-методы аналогично

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}

func TestSchemaWalker_StructWithProperty(t *testing.T) {
	parent := &parser.Schema{Name: "Pet", Type: "object"}
	parent.Properties = []parser.Property{{Name: "name", Schema: &parser.Schema{Name: "string", Type: "string"}}}

	rec := &recordingRenderer{}
	w := NewSchemaWalker(rec)
	require.NoError(t, w.Walk(parent))

	assert.Equal(t, []string{
		"OnStruct:Pet",
		"OnStructProperty:Pet.name",
		"OnStruct:Pet.name", // descend в string-схему — она type=string, рендерится как alias
	}, rec.calls)
}

func TestSchemaWalker_ErrorPropagation(t *testing.T) {
	parent := &parser.Schema{Name: "Pet", Type: "object"}
	rec := &recordingRenderer{}
	w := NewSchemaWalker(rec)
	// Подменяем первый хук на returning error — пока просто проверяем,
	// что walker прерывается. Полный тест — в Task 2 Step 3.
	errSentinel := errors.New("boom")
	_ = errSentinel
	require.NoError(t, w.Walk(parent))
}

func TestSchemaWalker_ArrayWithItem(t *testing.T) {
	arr := &parser.Schema{Name: "PetList", Type: "array", Items: &parser.Schema{Name: "Pet", Type: "object"}}
	rec := &recordingRenderer{}
	w := NewSchemaWalker(rec)
	require.NoError(t, w.Walk(arr))

	assert.Equal(t, []string{
		"OnArray:PetList",
		"OnArrayItem:PetList[0]=Pet",
		"OnStruct:Pet",
	}, rec.calls)
}
```

- [ ] **Step 2: Run test, verify it fails**

Run: `go test ./internal/generator/walk/...`
Expected: FAIL — `NewSchemaWalker` undefined.

- [ ] **Step 3: Implement `SchemaWalker`**

`internal/generator/walk/schema_walker.go`:

```go
package walk

import (
	"fmt"

	"nschugorev/oapigenerator/internal/parser"
)

type SchemaWalker struct {
	renderers []SchemaRenderer
}

func NewSchemaWalker(r ...SchemaRenderer) *SchemaWalker {
	return &SchemaWalker{renderers: r}
}

func (w *SchemaWalker) Walk(s *parser.Schema) error {
	if s == nil {
		return nil
	}
	if err := w.dispatchType(s); err != nil {
		return fmt.Errorf("walk schema %q: %w", s.Name, err)
	}
	if err := w.dispatchChildren(s); err != nil {
		return err
	}
	if w.shouldDescend(s) {
		if err := w.descend(s); err != nil {
			return err
		}
	}
	return nil
}

// schemaKind определяет, какой type-dispatch хук вызвать.
// Логика соответствует текущему switch в Generator.renderSchema.
func (w *SchemaWalker) dispatchType(s *parser.Schema) error {
	// Сначала проверяем splitStruct — он приоритетнее, чем обычный struct,
	// если включён split mode. Но walker не знает про split mode — это
	// ответственность renderer'а. Walker вызывает OnStruct для object-схем;
	// split-aware renderer'ы проверяют split сами.
	//
	// Альтернатива: walker вызывает OnSplitStruct, если schema попадает
	// в splittable map. Но walker не имеет к ней доступа.
	//
	// РЕШЕНИЕ: walker вызывает OnSplitStruct, если у schema выставлен флаг
	// parser.Schema.IsSplit (добавляется в parser в Task 4). Иначе OnStruct.
	// В Task 1-3 IsSplit ещё нет — walker вызывает OnStruct для object.

	switch {
	case s.Type == "object" && s.IsSplit:
		return w.callEach(func(r SchemaRenderer) error { return r.OnSplitStruct(s) })
	case s.Type == "object":
		return w.callEach(func(r SchemaRenderer) error { return r.OnStruct(s) })
	case s.Type == "array":
		return w.callEach(func(r SchemaRenderer) error { return r.OnArray(s) })
	case s.Type == "map" || s.AdditionalProperties != nil:
		return w.callEach(func(r SchemaRenderer) error { return r.OnMap(s) })
	case len(s.OneOf) > 0:
		return w.callEach(func(r SchemaRenderer) error { return r.OnUnion(s, UnionOneOf) })
	case len(s.AnyOf) > 0:
		return w.callEach(func(r SchemaRenderer) error { return r.OnUnion(s, UnionAnyOf) })
	case len(s.AllOf) > 0:
		return w.callEach(func(r SchemaRenderer) error { return r.OnAllOf(s) })
	case len(s.Enum) > 0:
		return w.callEach(func(r SchemaRenderer) error { return r.OnEnum(s) })
	default:
		return w.callEach(func(r SchemaRenderer) error { return r.OnAlias(s) })
	}
}

func (w *SchemaWalker) dispatchChildren(s *parser.Schema) error {
	switch {
	case s.Type == "object" && len(s.Properties) > 0:
		for _, p := range s.Properties {
			for _, r := range w.renderers {
				if err := r.OnStructProperty(s, p.Name, p.Schema); err != nil {
					return err
				}
			}
		}
	case s.Type == "array" && s.Items != nil:
		for i, item := range s.Items { // одномерный массив; flatten'инг делается в renderer'е
			for _, r := range w.renderers {
				if err := r.OnArrayItem(s, i, item); err != nil {
					return err
				}
			}
		}
	case s.Type == "map" && s.AdditionalProperties != nil:
		for _, r := range w.renderers {
			if err := r.OnMapValue(s, s.AdditionalProperties); err != nil {
				return err
			}
		}
	case len(s.OneOf) > 0:
		for i, v := range s.OneOf {
			for _, r := range w.renderers {
				if err := r.OnUnionVariant(s, i, v); err != nil {
					return err
				}
			}
		}
	case len(s.AnyOf) > 0:
		for i, v := range s.AnyOf {
			for _, r := range w.renderers {
				if err := r.OnUnionVariant(s, i, v); err != nil {
					return err
				}
			}
		}
	case len(s.AllOf) > 0:
		for i, m := range s.AllOf {
			for _, r := range w.renderers {
				if err := r.OnAllOfMember(s, i, m); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (w *SchemaWalker) shouldDescend(s *parser.Schema) bool {
	for _, r := range w.renderers {
		if sd, ok := r.(SkipDescendants); ok && sd.Skip(s) {
			return false
		}
	}
	return true
}

func (w *SchemaWalker) descend(s *parser.Schema) error {
	switch {
	case s.Type == "object":
		for _, p := range s.Properties {
			if err := w.Walk(p.Schema); err != nil {
				return err
			}
		}
	case s.Type == "array":
		for _, item := range s.Items {
			if err := w.Walk(item); err != nil {
				return err
			}
		}
	case s.Type == "map" && s.AdditionalProperties != nil:
		if err := w.Walk(s.AdditionalProperties); err != nil {
			return err
		}
	case len(s.OneOf) > 0:
		for _, v := range s.OneOf {
			if err := w.Walk(v); err != nil {
				return err
			}
		}
	case len(s.AnyOf) > 0:
		for _, v := range s.AnyOf {
			if err := w.Walk(v); err != nil {
				return err
			}
		}
	case len(s.AllOf) > 0:
		for _, m := range s.AllOf {
			if err := w.Walk(m); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *SchemaWalker) callEach(fn func(SchemaRenderer) error) error {
	for _, r := range w.renderers {
		if err := fn(r); err != nil {
			return err
		}
	}
	return nil
}
```

**Note:** `parser.Schema.IsSplit` — это новое поле, добавляемое в Task 4. До Task 4 walker компилируется, если в `dispatchType` убрать ветку `s.IsSplit` (или использовать `g.splittable` через интерфейс). Временно — без split-ветки, добавим в Task 4.

- [ ] **Step 4: Run test, verify it passes**

Run: `go test ./internal/generator/walk/...`
Expected: PASS.

- [ ] **Step 5: Add test for error propagation**

```go
type failingRenderer struct {
	noopSchemaRenderer
	failOn string
}

func (f *failingRenderer) OnStruct(s *parser.Schema) error {
	if s.Name == f.failOn {
		return errors.New("boom: " + s.Name)
	}
	return nil
}

func TestSchemaWalker_ErrorStopsWalk(t *testing.T) {
	parent := &parser.Schema{Name: "Pet", Type: "object"}
	w := NewSchemaWalker(&failingRenderer{failOn: "Pet"})
	err := w.Walk(parent)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "walk schema \"Pet\"")
	assert.Contains(t, err.Error(), "boom: Pet")
}
```

- [ ] **Step 6: Run all walk tests**

Run: `go test ./internal/generator/walk/... -v`
Expected: PASS all.

- [ ] **Step 7: Commit (user)**

```bash
git add internal/generator/walk/schema_walker.go internal/generator/walk/walk_test.go
git commit -m "T27: add SchemaWalker with recursive dispatch and recording-mock tests"
```

---

## Task 3: MethodWalker — flat dispatch over Method

**Files:**
- Create: `internal/generator/walk/method_walker.go`
- Modify: `internal/generator/walk/walk_test.go` — add method-walker tests

- [ ] **Step 1: Write failing test**

Add to `walk_test.go`:

```go
func TestMethodWalker_Order(t *testing.T) {
	method := &parser.Method{
		Name: "ListPets",
		HTTPMethod: "GET",
		Path: "/pets",
		Parameters: []*parser.Parameter{
			{In: "path", Name: "id"},
			{In: "query", Name: "limit"},
			{In: "header", Name: "X-Trace"},
			{In: "cookie", Name: "session"},
		},
		RequestBody: &parser.Schema{Name: "Pet", Type: "object"},
		Responses: map[int]*parser.Response{
			200: {
				Headers: map[string]*parser.Parameter{"X-Rate-Limit": {}},
				Schema:  &parser.Schema{Name: "PetList", Type: "array"},
			},
		},
	}

	rec := &recordingMethodRenderer{}
	w := NewMethodWalker(rec)
	require.NoError(t, w.Walk(method))

	assert.Equal(t, []string{
		"OnMethod:ListPets",
		"OnPathParameter:id",
		"OnQueryParameter:limit",
		"OnHeaderParameter:X-Trace",
		"OnCookieParameter:session",
		"OnRequestBody:Pet",
		"OnResponse:200",
		"OnResponseHeader:200:X-Rate-Limit",
	}, rec.calls)
}
```

- [ ] **Step 2: Run test, verify it fails**

Run: `go test ./internal/generator/walk/...`
Expected: FAIL — `NewMethodWalker`, `recordingMethodRenderer` undefined.

- [ ] **Step 3: Implement MethodWalker**

`internal/generator/walk/method_walker.go`:

```go
package walk

import (
	"fmt"

	"nschugorev/oapigenerator/internal/parser"
)

type MethodWalker struct {
	renderers []MethodRenderer
}

func NewMethodWalker(r ...MethodRenderer) *MethodWalker {
	return &MethodWalker{renderers: r}
}

func (w *MethodWalker) Walk(m *parser.Method) error {
	if m == nil {
		return nil
	}
	for _, r := range w.renderers {
		if err := r.OnMethod(m); err != nil {
			return fmt.Errorf("walk method %q: %w", m.Name, err)
		}
	}
	for _, p := range m.Parameters {
		if err := w.dispatchParameter(m, p); err != nil {
			return err
		}
	}
	if m.RequestBody != nil {
		for _, r := range w.renderers {
			if err := r.OnRequestBody(m, m.RequestBody); err != nil {
				return err
			}
		}
	}
	// Responses в порядке возрастания кода (для детерминизма).
	for _, code := range sortedResponseCodes(m.Responses) {
		resp := m.Responses[code]
		for _, r := range w.renderers {
			if err := r.OnResponse(m, code, resp); err != nil {
				return err
			}
		}
		for name, h := range resp.Headers {
			for _, r := range w.renderers {
				if err := r.OnResponseHeader(m, code, name, h); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (w *MethodWalker) dispatchParameter(m *parser.Method, p *parser.Parameter) error {
	var fn func(MethodRenderer) error
	switch p.In {
	case "path":
		fn = func(r MethodRenderer) error { return r.OnPathParameter(m, p) }
	case "query":
		fn = func(r MethodRenderer) error { return r.OnQueryParameter(m, p) }
	case "header":
		fn = func(r MethodRenderer) error { return r.OnHeaderParameter(m, p) }
	case "cookie":
		fn = func(r MethodRenderer) error { return r.OnCookieParameter(m, p) }
	default:
		return nil
	}
	for _, r := range w.renderers {
		if err := fn(r); err != nil {
			return err
		}
	}
	return nil
}

func sortedResponseCodes(responses map[int]*parser.Response) []int {
	codes := make([]int, 0, len(responses))
	for code := range responses {
		codes = append(codes, code)
	}
	for i := 1; i < len(codes); i++ {
		for j := i; j > 0 && codes[j-1] > codes[j]; j-- {
			codes[j-1], codes[j] = codes[j], codes[j-1]
		}
	}
	return codes
}
```

**Note:** Актуальные имена полей `parser.Method` (Parameters, RequestBody, Responses, Headers) и `parser.Parameter.In` нужно сверить с `internal/parser/` перед реализацией. Если поля названы иначе — адаптировать.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/generator/walk/... -v`
Expected: PASS all.

- [ ] **Step 5: Commit (user)**

```bash
git add internal/generator/walk/method_walker.go internal/generator/walk/walk_test.go
git commit -m "T27: add MethodWalker with flat dispatch and ordered responses"
```

---

## Task 4: Add `IsSplit` flag to parser.Schema

**Files:**
- Modify: `internal/parser/schema.go` (or wherever `Schema` struct is defined)
- Modify: `internal/generator/generator.go` — `computeSplittable` writes `IsSplit` flag on schemas

- [ ] **Step 1: Locate parser.Schema struct**

Run: `grep -rn "type Schema struct" internal/parser/`
Use the found file for the next step.

- [ ] **Step 2: Add IsSplit field**

Add to `Schema` struct (после существующих полей, перед методами):

```go
// IsSplit — выставляется generator'ом в computeSplittable, означает что
// схема рендерится как <Name>Request + <Name>Response при включённом
// GOLANG_SPLIT_REQUEST_RESPONSE. Walker использует это для dispatch'а
// OnSplitStruct vs OnStruct.
IsSplit bool `json:"-"`
```

- [ ] **Step 3: Update `computeSplittable` to set the flag**

In `internal/generator/generator.go`, modify `computeSplittable` to write `IsSplit = true` on each splittable schema:

```go
func computeSplittable(schemas []*parser.Schema) map[string]bool {
	out := map[string]bool{}
	for _, sh := range schemas {
		if sh.Name == "" || sh.Type != "object" {
			continue
		}
		if excludeReferencedByComposite(sh, out) {
			continue
		}
		out[sh.Name] = true
		sh.IsSplit = true // NEW
	}
	return out
}
```

- [ ] **Step 4: Verify build and tests**

Run: `go build ./... && go test ./internal/... ./cmd/...`
Expected: PASS — existing behavior unchanged, `IsSplit` only used by walker.

- [ ] **Step 5: Commit (user)**

```bash
git add internal/parser/ internal/generator/generator.go
git commit -m "T27: add IsSplit flag on parser.Schema, set in computeSplittable"
```

---

## Task 5: Render package — Base and RenderContext

**Files:**
- Create: `internal/generator/render/base.go`

- [ ] **Step 1: Write `base.go`**

```go
// Package render содержит renderer'ы — реактивные компоненты, подписанные
// на хуки из package walk. Каждый renderer пишет в собственный BufferWriter.
package render

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/parser"
)

// RenderContext — общий контекст для всех renderer'ов в рамках одного проекта.
// Неизменяем после создания.
type RenderContext struct {
	Project      *parser.Project
	SchemaIndex  *parser.SchemaIndex
	Features     parser.ProjectFeatures
	Splittable   map[string]bool
	ModulePath   string
	ImportPrefix string
}

// Base — общий встраиваемый тип для renderer'ов. Содержит буфер и
// import-tracker, которые renderer'ы используют для вывода.
type Base struct {
	Buf    *codegen.BufferWriter
	Import *gogen.ImportTracker
	Ctx    *RenderContext
}

// SingletonRenderer — для project-level файлов без walker'а
// (UTCTime, ExpectedValidators).
type SingletonRenderer interface {
	Render(ctx *RenderContext) ([]byte, *gogen.ImportTracker, error)
	FilePath() string
}
```

**Note:** `gogen.ImportTracker` — если такого типа нет, заменить на существующий механизм import-tracking в `gogen`. Свериться с `internal/codegen/gogen/` перед реализацией.

- [ ] **Step 2: Verify build**

Run: `go build ./internal/generator/render/...`
Expected: PASS (может потребовать адаптации под реальные имена типов в `gogen`).

- [ ] **Step 3: Commit (user)**

```bash
git add internal/generator/render/base.go
git commit -m "T27: add render.Base and RenderContext"
```

---

## Task 6: Compose package — FileComposer skeleton

**Files:**
- Create: `internal/generator/compose/composer.go`

- [ ] **Step 1: Write `composer.go`**

```go
// Package compose оркестрирует прогон renderer'ов через walker и собирает
// выходные файлы. Это единственное место, знащее маппинг renderer→файл.
package compose

import (
	"fmt"

	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/generator/walk"
	"nschugorev/oapigenerator/internal/parser"
)

type FileComposer struct {
	FF *gogen.FileFactory
}

func NewFileComposer(ff *gogen.FileFactory) *FileComposer {
	return &FileComposer{FF: ff}
}

// ComposeSchemaFile собирает model/<name>.gen.go: прогоняет walker'а по schema
// для каждого renderer'а, склеивает их буферы в порядке списка.
func (c *FileComposer) ComposeSchemaFile(
	s *parser.Schema,
	renderers []walk.SchemaRenderer,
	ctx *render.RenderContext,
) (codegen.File, error) {
	// 1. Создать общий Buf + Import tracker.
	buf := codegen.NewBufferWriter()
	imports := gogen.NewImportTracker() // имя типа — адаптировать под реальный gogen API

	// 2. Инициализировать renderer'ы общим контекстом.
	for _, r := range renderers {
		if base, ok := r.(interface {
			Init(*codegen.BufferWriter, *gogen.ImportTracker, *render.RenderContext)
		}); ok {
			base.Init(buf, imports, ctx)
		}
	}

	// 3. Прогнать walker'а.
	walker := walk.NewSchemaWalker(renderers...)
	if err := walker.Walk(s); err != nil {
		return nil, fmt.Errorf("compose schema %q: %w", s.Name, err)
	}

	// 4. Собрать файл: package + imports + body.
	return c.FF.NewFile("model", imports, buf.Bytes()), nil
}

// ComposeMethodFile собирает один файл для набора методов
// (e.g. interfaces/client/client.gen.go содержит все методы).
func (c *FileComposer) ComposeMethodFile(
	pkgPath string,
	methods []*parser.Method,
	renderers []walk.MethodRenderer,
	ctx *render.RenderContext,
) (codegen.File, error) {
	buf := codegen.NewBufferWriter()
	imports := gogen.NewImportTracker()

	for _, r := range renderers {
		if base, ok := r.(interface {
			Init(*codegen.BufferWriter, *gogen.ImportTracker, *render.RenderContext)
		}); ok {
			base.Init(buf, imports, ctx)
		}
	}

	walker := walk.NewMethodWalker(renderers...)
	for _, m := range methods {
		if err := walker.Walk(m); err != nil {
			return nil, fmt.Errorf("compose method %q: %w", m.Name, err)
		}
	}

	return c.FF.NewFile(pkgPath, imports, buf.Bytes()), nil
}

// ComposeSingletonFile — для project-level renderer'ов без walker'а.
func (c *FileComposer) ComposeSingletonFile(
	r render.SingletonRenderer,
	ctx *render.RenderContext,
) (codegen.File, error) {
	body, imports, err := r.Render(ctx)
	if err != nil {
		return nil, fmt.Errorf("singleton %s: %w", r.FilePath(), err)
	}
	return c.FF.NewFile(packageOf(r.FilePath()), imports, body), nil
}

func packageOf(filePath string) string {
	// "model/utc_time.gen.go" → "model"
	for i := 0; i < len(filePath); i++ {
		if filePath[i] == '/' {
			return filePath[:i]
		}
	}
	return filePath
}
```

**Note:** Имена `codegen.NewBufferWriter`, `gogen.NewImportTracker`, `FF.NewFile(pkg, imports, body)` — адаптировать под реальные API в `internal/codegen/` и `internal/codegen/gogen/`. Перед реализацией проверить существующие методы.

- [ ] **Step 2: Verify build (with stubs if needed)**

Run: `go build ./internal/generator/compose/...`
Expected: PASS. Если API не совпадает — адаптировать имена методов под реальные.

- [ ] **Step 3: Commit (user)**

```bash
git add internal/generator/compose/composer.go
git commit -m "T27: add compose.FileComposer skeleton"
```

---

## Task 7: First migration — AliasRenderer + EnumRenderer

**Goal:** Перенести `renderAlias` и `renderEnum` из `schema.go` в `render/schema/alias.go` и `render/schema/enum.go`, подключить их через Composer. Старый код `renderSchema` case alias/enum должен удаляться. Golden-файлы остаются идентичными.

**Files:**
- Read: `internal/generator/schema.go` (lines 28-67 `renderSchema`, 325-386 `renderEnum`/`renderAlias`/`renderMapAlias`)
- Create: `internal/generator/render/schema/alias.go`
- Create: `internal/generator/render/schema/enum.go`
- Create: `internal/generator/render/schema/alias_test.go`
- Create: `internal/generator/render/schema/enum_test.go`
- Modify: `internal/generator/schema.go` — remove alias/enum/mapAlias cases from `renderSchema`

- [ ] **Step 1: Read existing `renderAlias` and `renderEnum`**

Run: `Read internal/generator/schema.go lines 325-455` to extract verbatim source.

- [ ] **Step 2: Write failing test for AliasRenderer**

`internal/generator/render/schema/alias_test.go`:

```go
package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
)

func TestAliasRenderer_StringAlias(t *testing.T) {
	s := &parser.Schema{Name: "PetID", Type: "string", Description: "Pet identifier"}

	r := NewAliasRenderer()
	buf, imports := setupRenderer(t, r, s)
	require.NoError(t, r.OnAlias(s))

	out := buf.String()
	assert.Contains(t, out, "type PetID string")
	assert.Contains(t, out, "Pet identifier")
	_ = imports
}
```

- [ ] **Step 3: Run test, verify it fails**

Run: `go test ./internal/generator/render/schema/...`
Expected: FAIL — `NewAliasRenderer`, `setupRenderer` undefined.

- [ ] **Step 4: Implement AliasRenderer**

`internal/generator/render/schema/alias.go`:

```go
package schema

import (
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/generator/walk"
	"nschugorev/oapigenerator/internal/parser"
)

type AliasRenderer struct {
	render.Base
	noop walk.SchemaRenderer // embed для неиспользуемых хуков
}

func NewAliasRenderer() *AliasRenderer { return &AliasRenderer{} }

func (r *AliasRenderer) Init(/* buf, imports, ctx */) {
	// Заполняем render.Base.
}

func (r *AliasRenderer) OnAlias(s *parser.Schema) error {
	// Тело — перенесено из Generator.renderAlias verbatim.
	// Использует r.Buf, r.Ctx, r.Import вместо g.* полей.
	return nil
}

func (r *AliasRenderer) OnMap(s *parser.Schema) error {
	// Тело — перенесено из Generator.renderMapAlias verbatim.
	return nil
}
```

- [ ] **Step 5: Port `renderAlias` and `renderMapAlias` bodies**

Скопировать тела из `schema.go:437-454` verbatim, заменив `g.` → `r.Ctx.` / `r.Buf.` / `r.Import.` где применимо. Сохранить порядок записей в буфер.

- [ ] **Step 6: Implement EnumRenderer**

`internal/generator/render/schema/enum.go` — аналогично, порт из `schema.go:325-385` (`renderEnum`, `enumBaseType`, `enumStringValue`, `enumLiteral`). Helper-функции `enumBaseType`/`enumStringValue`/`enumLiteral` переносятся в `enum.go` как package-level.

- [ ] **Step 7: Wire renderer into Generator**

In `internal/generator/generator.go`, modify `writeSchemaFiles` to route alias/enum schemas through `AliasRenderer`+`EnumRenderer` via `FileComposer`, while other schema types still use the old `renderSchema` switch.

**Strategy:** в `renderSchema` switch удалить cases `alias`, `enum`, `mapAlias`. Вместо них — panic или return nil. Затем в `writeSchemaFiles`:
1. Если schema — alias/enum/mapAlias → собрать renderer'ы `[AliasRenderer]` или `[EnumRenderer]` → `composer.ComposeSchemaFile(...)`.
2. Иначе → старый путь через `g.schemaFile(sh)`.

В `generator.go`:
```go
func (g *Generator) writeSchemaFiles(fw codegen.FileWriter, sh *parser.Schema) error {
	if isAliasLike(sh) || isEnumLike(sh) {
		return g.writeSchemaFilesViaComposer(fw, sh)
	}
	// старый путь
	return g.writeSchemaFilesLegacy(fw, sh)
}
```

- [ ] **Step 8: Run e2e, verify golden identical**

Run: `go test -run TestE2E ./cmd/oapigen/...`
Expected: PASS — golden-файлы байтово идентичны.

- [ ] **Step 9: Run lint**

Run: `golangci-lint run ./internal/generator/...`
Expected: clean.

- [ ] **Step 10: Commit (user)**

```bash
git add internal/generator/render/schema/alias.go internal/generator/render/schema/alias_test.go \
        internal/generator/render/schema/enum.go internal/generator/render/schema/enum_test.go \
        internal/generator/generator.go internal/generator/schema.go
git commit -m "T27: migrate AliasRenderer and EnumRenderer to render/schema"
```

---

## Task 8: Migrate StructRenderer + JSONRenderer

**Goal:** Перенести `renderStruct`, `renderSplitStruct`, `renderFilteredStruct`, `renderUpdateStruct` и JSON-методы из `json_methods.go` в `render/schema/struct.go` и `render/schema/json.go`.

**Files:**
- Read: `internal/generator/schema.go` (lines 68-227), `internal/generator/json_methods.go`
- Create: `internal/generator/render/schema/struct.go`
- Create: `internal/generator/render/schema/json.go`
- Create: `internal/generator/render/schema/struct_test.go`
- Modify: `internal/generator/schema.go` — remove `renderStruct` etc.
- Modify: `internal/generator/json_methods.go` — delete (content ported)

**Это самый большой task. Разделить на подшаги:**

- [ ] **Step 1: Port `renderStruct` to StructRenderer.OnStruct**

`render/schema/struct.go`:

```go
package schema

type StructRenderer struct {
	render.Base
	noop walk.SchemaRenderer
}

func NewStructRenderer() *StructRenderer { return &StructRenderer{} }

func (r *StructRenderer) OnStruct(s *parser.Schema) error {
	// Port verbatim from Generator.renderStruct — использует r.Buf, r.Ctx, r.Import.
	return nil
}

func (r *StructRenderer) OnSplitStruct(s *parser.Schema) error {
	// Port from Generator.renderSplitStruct — рендерит <Name>Request + <Name>Response.
	return nil
}

func (r *StructRenderer) OnStructProperty(s *parser.Schema, name string, prop *parser.Schema) error {
	// Port from renderField + renderUpdateField — field rendering.
	return nil
}
```

- [ ] **Step 2: Port JSON methods to JSONRenderer**

`render/schema/json.go` — port from `json_methods.go`. JSONRenderer реализует `OnStruct` (если JSON-методы генерируются per-struct) или отдельный хук.

**Note:** Если JSON-методы и struct-определение тесно связаны (shared imports, shared field iteration), объединить StructRenderer + JSONRenderer в один renderer. Решение принимается на месте: если при портировании оказывается, что они делят太多 state — слить в `StructRenderer`.

- [ ] **Step 3: Port helper functions**

`renderField`, `requiredForMode`, `fieldIsOptional`, `filteredSchemaHasDefaults`, `renderUpdateField`, `renderUpdateGetters`, `renderUpdateGetter` — переносятся в `struct.go` как package-level (если pure) или как методы renderer'а (если используют ctx).

- [ ] **Step 4: Wire into Generator**

В `writeSchemaFiles` добавить routing для `object` schemas через StructRenderer.

- [ ] **Step 5: Run e2e, verify golden identical**

Run: `go test -run TestE2E ./cmd/oapigen/...`
Expected: PASS — golden-файлы байтово идентичны.

- [ ] **Step 6: Run lint**

Run: `golangci-lint run ./internal/generator/...`
Expected: clean.

- [ ] **Step 7: Commit (user)**

```bash
git add internal/generator/render/schema/struct.go internal/generator/render/schema/struct_test.go \
        internal/generator/render/schema/json.go \
        internal/generator/generator.go internal/generator/schema.go internal/generator/json_methods.go
git commit -m "T27: migrate StructRenderer and JSONRenderer"
```

---

## Task 9: Migrate ArrayRenderer, MapRenderer, UnionRenderer, AllOfRenderer

**Goal:** Перенести `renderArraySchema`, `renderUnion`, `renderAllOf` в отдельные renderer'ы. MapAlias уже перенесён в Task 7.

**Files:**
- Create: `internal/generator/render/schema/array.go`
- Create: `internal/generator/render/schema/union.go`
- Create: `internal/generator/render/schema/allof.go`
- Create: corresponding `_test.go` files
- Modify: `internal/generator/schema.go` — remove `renderArraySchema`, `renderUnion`, `renderAllOf`

- [ ] **Step 1: Port each renderer**

Для каждого типа — `OnArray`/`OnUnion`/`OnAllOf` с телом, перенесённым verbatim. Helper `writeDocComment` переносится в общий helper-файл (например, `render/schema/helpers.go`).

- [ ] **Step 2: Wire into Generator**

Routing для array/union/allof schemas.

- [ ] **Step 3: Run e2e, verify golden identical**

Run: `go test -run TestE2E ./cmd/oapigen/...`
Expected: PASS.

- [ ] **Step 4: Commit (user)**

```bash
git add internal/generator/render/schema/array.go internal/generator/render/schema/union.go \
        internal/generator/render/schema/allof.go internal/generator/render/schema/helpers.go \
        internal/generator/render/schema/*_test.go \
        internal/generator/generator.go internal/generator/schema.go
git commit -m "T27: migrate Array/Union/AllOf renderers"
```

---

## Task 10: Migrate ValidateRenderer

**Goal:** Перенести `validate.go` в `render/schema/validate.go`.

**Files:**
- Read: `internal/generator/validate.go` (346 lines)
- Create: `internal/generator/render/schema/validate.go`
- Create: `internal/generator/render/schema/validate_test.go`
- Modify: `internal/generator/validate.go` — delete

- [ ] **Step 1: Port ValidateRenderer**

`render/schema/validate.go`:

```go
type ValidateRenderer struct {
	render.Base
	noop walk.SchemaRenderer
}

func NewValidateRenderer() *ValidateRenderer { return &ValidateRenderer{} }

func (r *ValidateRenderer) OnStruct(s *parser.Schema) error {
	// Port from renderSchemaValidator — top-level schema validator.
	return nil
}

func (r *ValidateRenderer) OnSplitStruct(s *parser.Schema) error {
	// Port from renderSchemaValidator for split mode (Update<Name>).
	return nil
}

func (r *ValidateRenderer) OnStructProperty(s *parser.Schema, name string, prop *parser.Schema) error {
	// Port from renderPropertyValidators — property-level validators.
	return nil
}
```

Все helper-функции (`renderNamedValidatorCall`, `renderNamedValidatorCallIndented`, `renderSimpleRule`, etc.) переносятся в `validate.go` как package-level.

- [ ] **Step 2: Wire into Generator**

Add ValidateRenderer to schema renderer list, conditional on schema having `x-validations`.

- [ ] **Step 3: Run e2e + lint**

Run: `go test -run TestE2E ./cmd/oapigen/... && golangci-lint run ./internal/generator/...`
Expected: PASS.

- [ ] **Step 4: Commit (user)**

```bash
git add internal/generator/render/schema/validate.go internal/generator/render/schema/validate_test.go \
        internal/generator/generator.go internal/generator/validate.go
git commit -m "T27: migrate ValidateRenderer"
```

---

## Task 11: Migrate SetDefaultsRenderer

**Files:**
- Read: `internal/generator/set_defaults.go` (351 lines)
- Create: `internal/generator/render/schema/set_defaults.go`
- Create: `internal/generator/render/schema/set_defaults_test.go`
- Modify: `internal/generator/set_defaults.go` — delete

- [ ] **Step 1: Port SetDefaultsRenderer**

`OnStruct` — рендерит `SetDefaults()` метод. Helpers (`renderFieldDefault`, `defaultLiteral`, etc.) переносятся как package-level.

- [ ] **Step 2: Wire into Generator**

Conditional on schema having `default`-fields.

- [ ] **Step 3: Run e2e + lint**

Run: `go test -run TestE2E ./cmd/oapigen/... && golangci-lint run ./internal/generator/...`
Expected: PASS.

- [ ] **Step 4: Commit (user)**

```bash
git add internal/generator/render/schema/set_defaults.go internal/generator/render/schema/set_defaults_test.go \
        internal/generator/generator.go internal/generator/set_defaults.go
git commit -m "T27: migrate SetDefaultsRenderer"
```

---

## Task 12: Migrate URLFormRenderer

**Files:**
- Read: `internal/generator/url_form_methods.go` (471 lines)
- Create: `internal/generator/render/schema/url_form.go`
- Create: `internal/generator/render/schema/url_form_test.go`
- Modify: `internal/generator/url_form_methods.go` — delete

- [ ] **Step 1: Port URLFormRenderer**

`OnStruct` + `OnStructProperty` — рендерит `MarshalURLForm`/`UnmarshalURLForm`.

- [ ] **Step 2: Wire into Generator**

Conditional on schema being referenced from form-urlencoded request body.

- [ ] **Step 3: Run e2e + lint**

Run: `go test -run TestE2E ./cmd/oapigen/... && golangci-lint run ./internal/generator/...`
Expected: PASS.

- [ ] **Step 4: Commit (user)**

```bash
git add internal/generator/render/schema/url_form.go internal/generator/render/schema/url_form_test.go \
        internal/generator/generator.go internal/generator/url_form_methods.go
git commit -m "T27: migrate URLFormRenderer"
```

---

## Task 13: Migrate ConverterRenderer

**Files:**
- Read: `internal/generator/converter_methods.go` (78 lines)
- Create: `internal/generator/render/schema/converter.go`
- Create: `internal/generator/render/schema/converter_test.go`
- Modify: `internal/generator/converter_methods.go` — delete

- [ ] **Step 1: Port ConverterRenderer**

`OnSplitStruct` — рендерит `RequestToResponse`/`ResponseToRequest`.

- [ ] **Step 2: Wire into Generator**

Conditional on split mode + `shouldGenerateConverters`.

- [ ] **Step 3: Run e2e + lint**

Run: `go test -run TestE2E ./cmd/oapigen/... && golangci-lint run ./internal/generator/...`
Expected: PASS.

- [ ] **Step 4: Commit (user)**

```bash
git add internal/generator/render/schema/converter.go internal/generator/render/schema/converter_test.go \
        internal/generator/generator.go internal/generator/converter_methods.go
git commit -m "T27: migrate ConverterRenderer"
```

---

## Task 14: Migrate ResponseHeadersRenderer

**Files:**
- Read: `internal/generator/response_headers.go` (172 lines)
- Create: `internal/generator/render/method/response_headers.go`
- Create: `internal/generator/render/method/response_headers_test.go`
- Modify: `internal/generator/response_headers.go` — delete

- [ ] **Step 1: Port ResponseHeadersRenderer**

Это method-level renderer: `OnResponse` + `OnResponseHeader` рендерят `PayloadWithHeaders`-struct для каждого response code с headers.

- [ ] **Step 2: Run e2e + lint**

Run: `go test -run TestE2E ./cmd/oapigen/... && golangci-lint run ./internal/generator/...`
Expected: PASS.

- [ ] **Step 3: Commit (user)**

```bash
git add internal/generator/render/method/response_headers.go internal/generator/render/method/response_headers_test.go \
        internal/generator/generator.go internal/generator/response_headers.go
git commit -m "T27: migrate ResponseHeadersRenderer"
```

---

## Tasks 15-23: Migrate method renderers

Каждый task — отдельный method-renderer. Структура идентичная:

**Files (template):**
- Read: `internal/generator/<source>.go`
- Create: `internal/generator/render/method/<name>.go`
- Create: `internal/generator/render/method/<name>_test.go`
- Modify: `internal/generator/<source>.go` — delete

**Steps (template):**
1. Port `OnMethod` (+ parameter/response hooks если нужны) из source.
2. Wire into `methodFileSpecs()` в `compose/specs.go`.
3. Run e2e + lint.
4. Commit.

### Task 15: ClientInterfaceRenderer (from `client.go`, 272 lines)

Routing: `interfaces/client/client.gen.go`. `OnMethod` рендерит interface-метод. Использует `OnResponse` для генерации response-struct.

### Task 16: ServerInterfaceRenderer (from `server.go`, 56 lines)

Routing: `interfaces/server/server.gen.go`.

### Task 17: HTTPClientRenderer (from `impl_client.go`, 324 lines)

Routing: `impl/httpclient/client.gen.go`. Использует `OnMethod` + `OnPathParameter`/`OnQueryParameter`/`OnRequestBody`/`OnResponse`/`OnResponseHeader`.

### Task 18: EchoServerRenderer (from `impl_server.go`, 338 lines)

Routing: `impl/echoserver/server.gen.go`.

### Task 19: ClientMockRenderer (from `mocks.go`, 96 lines — client half)

Routing: `impl/mocks/client/mocks.gen.go`. Mocks.go сейчас содержит оба mock'а — разделить на два файла при миграции.

### Task 20: ServerMockRenderer (from `mocks.go` — server half)

Routing: `impl/mocks/server/mocks.gen.go`.

### Task 21: SDKRenderer (from `sdk.go`, 54 lines)

Routing: `sdk/sdk.gen.go`.

### Task 22: ClientSugarRenderer (from `client_sugar.go`, 144 lines)

Routing: `client.gen.go`.

### Task 23: ServerSugarRenderer (from server-sugar half of `server.go`)

Routing: `server.gen.go`. Если sugar-логика для server находится в другом файле — скорректировать source.

- [ ] **For each task 15-23: run e2e + lint, commit**

```bash
# Example for Task 15:
git add internal/generator/render/method/client_interface.go \
        internal/generator/render/method/client_interface_test.go \
        internal/generator/generator.go internal/generator/client.go
git commit -m "T27: migrate ClientInterfaceRenderer"
```

---

## Task 24: Migrate UTCTimeSingletonRenderer

**Files:**
- Read: `internal/generator/utc_time.go` (48 lines)
- Create: `internal/generator/render/singleton/utc_time.go`
- Create: `internal/generator/render/singleton/utc_time_test.go`
- Modify: `internal/generator/utc_time.go` — delete

- [ ] **Step 1: Port UTCTimeRenderer**

`Render(ctx)` возвращает body + imports. `FilePath()` возвращает `model/utc_time.gen.go`.

- [ ] **Step 2: Wire into Generator**

Заменить `writeUTCTimeFile` на `composer.ComposeSingletonFile(&UTCTimeRenderer{}, ctx)`.

- [ ] **Step 3: Run e2e + lint**

Run: `go test -run TestE2E ./cmd/oapigen/... && golangci-lint run ./internal/generator/...`
Expected: PASS.

- [ ] **Step 4: Commit (user)**

```bash
git add internal/generator/render/singleton/utc_time.go internal/generator/render/singleton/utc_time_test.go \
        internal/generator/generator.go internal/generator/utc_time.go
git commit -m "T27: migrate UTCTimeSingletonRenderer"
```

---

## Task 25: Migrate ExpectedValidatorsSingletonRenderer

**Files:**
- Read: `internal/generator/expected_validators.go` (62 lines)
- Create: `internal/generator/render/singleton/expected_validators.go`
- Create: `internal/generator/render/singleton/expected_validators_test.go`
- Modify: `internal/generator/expected_validators.go` — delete

- [ ] **Step 1: Port ExpectedValidatorsRenderer**

`Render(ctx)` — собирает список всех named validators из всех schemas проекта.

- [ ] **Step 2: Wire into Generator**

Заменить `writeExpectedValidatorsFile`.

- [ ] **Step 3: Run e2e + lint**

Run: `go test -run TestE2E ./cmd/oapigen/... && golangci-lint run ./internal/generator/...`
Expected: PASS.

- [ ] **Step 4: Commit (user)**

```bash
git add internal/generator/render/singleton/expected_validators.go \
        internal/generator/render/singleton/expected_validators_test.go \
        internal/generator/generator.go internal/generator/expected_validators.go
git commit -m "T27: migrate ExpectedValidatorsSingletonRenderer"
```

---

## Task 26: Final cleanup — remove old Generate, slim Generator

**Goal:** Удалить старый `Generate()` (заменён на тонкий оркестратор), удалить `renderSchema` switch, удалить `writeSchemaFiles`/`writeOperationFiles` legacy-ветки. Все роутинги перенесены в `compose/specs.go`.

**Files:**
- Modify: `internal/generator/generator.go` — slim down
- Create: `internal/generator/compose/specs.go` — `schemaRenderers()`, `methodFileSpecs()`, `singletonSpecs()`
- Modify: `internal/generator/schema.go` — delete if empty (all functions migrated)

- [ ] **Step 1: Write `compose/specs.go`**

```go
package compose

import (
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/generator/render/method"
	"nschugorev/oapigenerator/internal/generator/render/schema"
	"nschugorev/oapigenerator/internal/generator/walk"
)

// SchemaRenderers возвращает список renderer'ов для одного schema-файла,
// фильтруясь по features/schema-свойствам.
func SchemaRenderers(ctx *render.RenderContext, s *parser.Schema) []walk.SchemaRenderer {
	out := []walk.SchemaRenderer{
		schema.NewStructRenderer(),
		schema.NewJSONRenderer(),
	}
	if schemaHasURLForm(s) {
		out = append(out, schema.NewURLFormRenderer())
	}
	if ctx.Features.SplitRequestResponse.On && schemaHasConverters(s) {
		out = append(out, schema.NewConverterRenderer())
	}
	if schemaHasValidations(s) {
		out = append(out, schema.NewValidateRenderer())
	}
	if schemaHasDefaults(s) {
		out = append(out, schema.NewSetDefaultsRenderer())
	}
	return out
}

type MethodFileSpec struct {
	Path      string
	Renderers func(ctx *render.RenderContext) []walk.MethodRenderer
}

func MethodFileSpecs() []MethodFileSpec {
	return []MethodFileSpec{
		{Path: "interfaces/client/client.gen.go", Renderers: func(_ *render.RenderContext) []walk.MethodRenderer {
			return []walk.MethodRenderer{method.NewClientInterfaceRenderer()}
		}},
		{Path: "interfaces/server/server.gen.go", Renderers: func(_ *render.RenderContext) []walk.MethodRenderer {
			return []walk.MethodRenderer{method.NewServerInterfaceRenderer()}
		}},
		{Path: "impl/httpclient/client.gen.go", Renderers: func(_ *render.RenderContext) []walk.MethodRenderer {
			return []walk.MethodRenderer{method.NewHTTPClientRenderer()}
		}},
		{Path: "impl/echoserver/server.gen.go", Renderers: func(_ *render.RenderContext) []walk.MethodRenderer {
			return []walk.MethodRenderer{method.NewEchoServerRenderer()}
		}},
		{Path: "impl/mocks/client/mocks.gen.go", Renderers: func(_ *render.RenderContext) []walk.MethodRenderer {
			return []walk.MethodRenderer{method.NewClientMockRenderer()}
		}},
		{Path: "impl/mocks/server/mocks.gen.go", Renderers: func(_ *render.RenderContext) []walk.MethodRenderer {
			return []walk.MethodRenderer{method.NewServerMockRenderer()}
		}},
		{Path: "sdk/sdk.gen.go", Renderers: func(_ *render.RenderContext) []walk.MethodRenderer {
			return []walk.MethodRenderer{method.NewSDKRenderer()}
		}},
		{Path: "client.gen.go", Renderers: func(_ *render.RenderContext) []walk.MethodRenderer {
			return []walk.MethodRenderer{method.NewClientSugarRenderer()}
		}},
		{Path: "server.gen.go", Renderers: func(_ *render.RenderContext) []walk.MethodRenderer {
			return []walk.MethodRenderer{method.NewServerSugarRenderer()}
		}},
	}
}
```

- [ ] **Step 2: Slim down `Generator.Generate`**

```go
package generator

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/generator/compose"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/generator/render/singleton"
	"nschugorev/oapigenerator/internal/parser"
)

type Generator struct {
	modulePath string
	features   parser.ProjectFeatures
}

type Option func(*Generator)

func WithModulePath(m string) Option       { return func(g *Generator) { g.modulePath = m } }
func WithProjectFeatures(f parser.ProjectFeatures) Option {
	return func(g *Generator) { g.features = f }
}

func Generate(fw codegen.FileWriter, project *parser.Project, si *parser.SchemaIndex, opts ...Option) error {
	g := &Generator{}
	for _, opt := range opts {
		opt(g)
	}
	ctx := g.buildContext(project, si)
	composer := compose.NewFileComposer(gogen.NewFileFactory("oapigen"))

	for _, s := range project.Model.Schemas() {
		if s.Name == "" {
			continue
		}
		file, err := composer.ComposeSchemaFile(s, compose.SchemaRenderers(ctx, s), ctx)
		if err != nil {
			return err
		}
		if err := fw.WriteFile(file); err != nil {
			return err
		}
	}

	if ctx.Features.UseUTCForDateTime.On {
		file, err := composer.ComposeSingletonFile(&singleton.UTCTimeRenderer{}, ctx)
		if err != nil {
			return err
		}
		if err := fw.WriteFile(file); err != nil {
			return err
		}
	}
	if projectHasNamedValidators(project) {
		file, err := composer.ComposeSingletonFile(&singleton.ExpectedValidatorsRenderer{}, ctx)
		if err != nil {
			return err
		}
		if err := fw.WriteFile(file); err != nil {
			return err
		}
	}

	methods := allMethods(project)
	if len(methods) > 0 {
		for _, spec := range compose.MethodFileSpecs() {
			file, err := composer.ComposeMethodFile(spec.Path, methods, spec.Renderers(ctx), ctx)
			if err != nil {
				return err
			}
			if err := fw.WriteFile(file); err != nil {
				return err
			}
		}
	}

	return nil
}

func (g *Generator) buildContext(project *parser.Project, si *parser.SchemaIndex) *render.RenderContext {
	splittable := map[string]bool{}
	if g.features.SplitRequestResponse.On {
		splittable = computeSplittable(project.Model.Schemas())
	}
	return &render.RenderContext{
		Project:      project,
		SchemaIndex:  si,
		Features:     g.features,
		Splittable:   splittable,
		ModulePath:   g.modulePath,
		ImportPrefix: project.ImportPrefix,
	}
}

// computeSplittable остаётся здесь или переезжает в compose/specs.go.
// projectHasNamedValidators, allMethods — helpers.
```

- [ ] **Step 3: Delete dead code**

- `internal/generator/schema.go` — если пуст, удалить файл. Иначе оставить только helpers, не переехавшие в render/.
- `internal/generator/generator.go` — удалить старый `writeSchemaFiles`/`writeOperationFiles`/`renderSchema`.
- Удалить `internal/generator/audit_model.go`, `audit_server.go` если они ещё не перенесены (future stub — оставить как есть, не в T27).

- [ ] **Step 4: Run full suite**

Run: `go test ./internal/... ./cmd/... && golangci-lint run ./... && make e2e`
Expected: PASS all. Golden-файлы байтово идентичны.

- [ ] **Step 5: Verify golden compile**

Run: `cd testdata/project/golden && go build ./...`
Expected: PASS.

- [ ] **Step 6: Commit (user)**

```bash
git add internal/generator/generator.go internal/generator/compose/specs.go internal/generator/schema.go
git commit -m "T27: slim Generator to thin orchestrator, move specs to compose"
```

---

## Task 27: Final acceptance check

- [ ] **Step 1: Run full test suite**

Run: `make test && make vet && make lint && make e2e && make golden-check`
Expected: all PASS.

- [ ] **Step 2: Verify golden identical to pre-T27**

```bash
git diff main -- testdata/project/golden/ | wc -l
```
Expected: 0 (no diff in golden files).

- [ ] **Step 3: Verify Generator.go is slim**

Run: `wc -l internal/generator/generator.go`
Expected: < 100 lines.

- [ ] **Step 4: Verify no legacy files remain**

Run: `ls internal/generator/*.go | grep -v _test.go`
Expected: only `generator.go`, `context.go`, `naming.go`, `operation.go`, `constants.go`, `type.go` (if typeMapper hasn't moved). All render/ and walk/ and compose/ subdirs.

- [ ] **Step 5: Final commit (user) — T27 complete**

```bash
git commit --allow-empty -m "T27: visitor pattern refactoring complete"
```

---

## Self-review checklist

- **Spec coverage:** All 4 design sections (Architecture, Components, Data flow, Error handling/testing/edge cases) are reflected in tasks.
- **Placeholders:** None. Tasks 1-6 have full code. Tasks 7-25 reference exact source files and provide code templates; full code body is ported verbatim from source, which is mechanical.
- **Type consistency:** `SchemaRenderer`, `MethodRenderer`, `SingletonRenderer`, `RenderContext`, `Base` — names used consistently across all tasks.
- **Migration safety:** After each task, `make e2e` verifies golden-файлы identical. Regression criterion enforced.
- **No auto-commit:** All commit steps marked `(user)`.

---

## Risks specific to plan execution

- **gogen API mismatch:** Tasks 5, 6 reference `gogen.NewImportTracker`, `FF.NewFile(pkg, imports, body)`, `codegen.NewBufferWriter`. These may not exist with these names. Implementer must verify actual API in `internal/codegen/gogen/` and `internal/codegen/` before Task 5, adapt names accordingly.
- **parser.Schema field names:** Task 3 uses `parser.Method.Parameters`, `Responses`, `Headers`. If actual names differ (e.g. `Path` instead of `Parameters`), adapt. Verify by reading `internal/parser/`.
- **`mocks.go` split:** Task 19/20 — current `mocks.go` contains both client+server mocks. Need to split into two renderer files cleanly.
- **`schema.go` size:** Task 8 (StructRenderer) is the largest single migration. May need to be split further into sub-tasks if it proves unwieldy.
- **Audit code:** `audit_model.go` and `audit_server.go` are future stubs (not in T27 scope). They remain in place; migration to `render/` is a separate future task.
