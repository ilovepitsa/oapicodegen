# T27: Visitor Pattern Refactoring — Design

**Goal:** Рефакторинг `internal/generator/*` на visitor-pattern: разделить recursive обход доменной модели (`Schema`, `Method`) и рендер артефактов. Снизить дублирование между 9+ файловыми генераторами (schema/client/server/impl/mocks/sdk), улучшить тестируемость и расширяемость.

**Architecture:** Трёхслойная visitor-модель — `Walker` (recursive обход дерева `Schema`/`Method`, никакой рендер-логики) → `Renderer` (реактивные компоненты, подписанные на подмножество хуков, пишут в собственный буфер) → `FileComposer` (оркеструет прогон renderer'ов через walker, склеивает буферы в `codegen.File`). Поверх трёх слоёв — `Generator` как тонкий оркестратор: для каждого проекта прогоняет Composer'ы всех выходных файлов.

**Tech Stack:** Go 1.26, существующие `internal/parser` (доменная модель), `internal/codegen/gogen` (рендер Go-файлов), `internal/codegen` (FileWriter). Без новых внешних зависимостей.

---

## Контекст и мотивация

`internal/generator/` содержит 25 .go файлов (~4500 LOC, исключая тесты). Главный файл `generator.go` — оркестратор `Generate()`, который:

1. Для каждой `parser.Schema` из `project.Model.Schemas` вызывает `writeSchemaFiles` — рендерит 6 под-файлов в один `model/<name>.gen.go` (struct + JSON + URLForm + converters + audit + validate + set_defaults).
2. Для каждой операции вызывает `writeOperationFiles` — рендерит 9 файлов (client, server, httpclient, echoserver, mocks×2, sdk, sugars×2).
3. Плюс singleton-файлы (`utc_time.gen.go`, `expected_validators.gen.go`).

Проблемы текущей архитектуры:

- **Дублирование обхода.** Каждый файловый метод (`schemaFile`, `clientFile`, `serverFile`, ...) самостоятельно итерирует schemas/methods. Циклы `for _, s := range g.project.Model.Schemas` повторяются.
- **Сильная связность.** `renderSchema` (switch по типу: splitStruct/union/alias/allOf/arraySchema/enum/mapAlias/struct) встроен в генератор. Связан с `typeMapper`, `splittable`, `features`. Добавление нового артефакта (например, `audit.gen.go` или `terraform.gen.go`) требует дублирования логики обхода.
- **Сложность тестирования.** Renderer-логику нельзя протестировать изолированно — нужно прогнать весь `Generate()` и сравнить golden-файлы целиком. Тест медленный, локализация бага сложная.
- **Толстые файлы.** `url_form_methods.go` (471), `schema.go` (460), `set_defaults.go` (351), `validate.go` (346), `impl_server.go` (338), `impl_client.go` (324). Каждый — смесь обхода и рендера.

**Motivation:** полный visitor-редизайн (вариант (d) из brainstorming) — снижение дублирования, расширяемость, тестируемость, все сразу.

---

## Дизайн

### Слой 1: Walkers (`internal/generator/walk/`)

Walker'ы делают recursive обход доменной модели и диспатчат хуки в зарегистрированные renderer'ы. Никакого рендера, никакого I/O, никакого состояния кроме позиции в дереве.

#### `SchemaWalker`

```go
package walk

type SchemaWalker struct {
    renderers []SchemaRenderer
}

func NewSchemaWalker(r ...SchemaRenderer) *SchemaWalker

// Walk рекурсивно обходит схему, диспатчит хуки в регистрированные renderer'ы.
func (w *SchemaWalker) Walk(s *parser.Schema) error
```

**Хук-интерфейс** — гибрид: type-dispatch + per-child (выбрано в brainstorming, вариант (c)).

```go
type SchemaRenderer interface {
    // Type-dispatch — вызывается ровно один для s.
    OnStruct(s *parser.Schema) error
    OnEnum(s *parser.Schema) error
    OnAlias(s *parser.Schema) error
    OnArray(s *parser.Schema) error
    OnMap(s *parser.Schema) error
    OnUnion(s *parser.Schema, kind UnionKind) error // oneOf/anyOf, kind = UnionOneOf|UnionAnyOf
    OnAllOf(s *parser.Schema) error
    OnSplitStruct(s *parser.Schema) error // Request/Response split

    // Per-child — walker сам итерирует, renderer'ы реагируют.
    OnStructProperty(s *parser.Schema, name string, prop *parser.Schema) error
    OnArrayItem(s *parser.Schema, idx int, item *parser.Schema) error
    OnMapValue(s *parser.Schema, value *parser.Schema) error
    OnUnionVariant(s *parser.Schema, idx int, variant *parser.Schema) error
    OnAllOfMember(s *parser.Schema, idx int, member *parser.Schema) error
}

// Опциональный интерфейс: renderer может попросить walker не спускаться
// в дочерние схемы (например, для external $ref).
type SkipDescendants interface {
    Skip(s *parser.Schema) bool
}

// noopSchemaRenderer даёт пустые реализации для неиспользуемых хуков.
// Renderer'ы embed'ят его и реализуют только нужные методы.
type noopSchemaRenderer struct{}

func (noopSchemaRenderer) OnStruct(*parser.Schema) error              { return nil }
func (noopSchemaRenderer) OnEnum(*parser.Schema) error                { return nil }
// ... и т.д. для всех 13 методов
```

**Порядок хуков внутри `Walk`:** type-dispatch → per-child → descend.

```go
func (w *SchemaWalker) Walk(s *parser.Schema) error {
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
```

Каждый renderer'овский хук вызывается последовательно; первый `err != nil` прерывает обход.

#### `MethodWalker`

```go
type MethodWalker struct {
    renderers []MethodRenderer
}

func (w *MethodWalker) Walk(m *parser.Method) error

type MethodRenderer interface {
    OnMethod(m *parser.Method) error
    OnPathParameter(m *parser.Method, p *parser.Parameter) error
    OnQueryParameter(m *parser.Method, p *parser.Parameter) error
    OnHeaderParameter(m *parser.Method, p *parser.Parameter) error
    OnCookieParameter(m *parser.Method, p *parser.Parameter) error
    OnRequestBody(m *parser.Method, body *parser.RequestBody) error
    OnResponse(m *parser.Method, code string, resp *parser.Response) error
    OnResponseHeader(m *parser.Method, code string, name string, h *parser.Parameter) error
}

type noopMethodRenderer struct{} // аналогично noopSchemaRenderer
```

**Обход method — плоский:** `OnMethod` → параметры по категориям (path/query/header/cookie) → `OnRequestBody` → `OnResponse`+`OnResponseHeader` для каждого кода. Recursive спуск в body/response schema **не делается** — method-walker не обязан знать про schema-дерево; для этого есть `SchemaWalker`. Renderer'ы, которым нужно обходить body как schema, вызывают `SchemaWalker` сами.

### Слой 2: Renderers (`internal/generator/render/`)

Renderer'ы — реактивные компоненты, реализующие подмножество хуков. Каждый пишет в собственный `*codegen.BufferWriter`.

```go
package render

type Base struct {
    Buf    *codegen.BufferWriter
    Import *gogen.ImportTracker
    // Общий контекст: features, typeMapper, splittable, project, schemaIndex.
    Ctx    *RenderContext
}

type RenderContext struct {
    Project      *parser.Project
    SchemaIndex  *parser.SchemaIndex
    Features     parser.ProjectFeatures
    Splittable   map[string]bool
    TypeMapper   *typeMapper
    // Import prefix, output dir, и т.д.
}
```

**Конкретные renderer'ы** (организованы по подмодулям):

`render/schema/`:
- `StructRenderer` — struct-определение + JSON-маршалинг (`MarshalJSON`, `UnmarshalJSON`). Реализует `OnStruct`, `OnSplitStruct`, `OnStructProperty` (для field tags), `OnArray`, `OnMap`, `OnUnion`, `OnAllOf`, `OnAlias`, `OnEnum`.
- `JSONRenderer` — выделенный JSON-маршалинг, если сложный (выделен из StructRenderer при необходимости).
- `ValidateRenderer` — `ValidateOwn(reg)` метод. Реализует `OnStruct`, `OnStructProperty` (property-level validators), `OnSplitStruct`.
- `SetDefaultsRenderer` — `SetDefaults()` метод. `OnStruct`, `OnStructProperty`.
- `URLFormRenderer` — `MarshalURLForm`/`UnmarshalURLForm`. `OnStruct`, `OnStructProperty`.
- `ConverterRenderer` — `RequestToResponse`/`ResponseToRequest` конвертеры (split mode). `OnSplitStruct`.
- `AuditModelRenderer` — audit-поля (future, заглушка).

`render/method/`:
- `ClientInterfaceRenderer` → `interfaces/client/client.gen.go`
- `ServerInterfaceRenderer` → `interfaces/server/server.gen.go`
- `HTTPClientRenderer` → `impl/httpclient/client.gen.go`
- `EchoServerRenderer` → `impl/echoserver/server.gen.go`
- `ClientMockRenderer` → `impl/mocks/client/mocks.gen.go`
- `ServerMockRenderer` → `impl/mocks/server/mocks.gen.go`
- `SDKRenderer` → `sdk/sdk.gen.go`
- `ClientSugarRenderer` → `client.gen.go`
- `ServerSugarRenderer` → `server.gen.go`

`render/singleton/`:
- `UTCTimeRenderer` — `model/utc_time.gen.go` (если `USE_UTC_FOR_DATE_TIME`).
- `ExpectedValidatorsRenderer` — `model/expected_validators.gen.go` (если есть named validators).

Singleton renderer'ы **не** используют walker — у них прямой метод `Render(ctx) codegen.File`.

### Слой 3: Composer (`internal/generator/compose/`)

```go
package compose

type FileComposer struct {
    ff *gogen.FileFactory
}

// ComposeSchemaFile: для одной schema генерирует model/<name>.gen.go.
// renderers — упорядоченный список; их буферы склеиваются в порядке списка.
func (c *FileComposer) ComposeSchemaFile(
    s *parser.Schema,
    renderers []walk.SchemaRenderer,
    ctx *render.RenderContext,
) codegen.File

// ComposeMethodFile: для набора методов генерирует один файл
// (e.g. interfaces/client/client.gen.go содержит все методы).
func (c *FileComposer) ComposeMethodFile(
    filePath string,
    methods []*parser.Method,
    renderers []walk.MethodRenderer,
    ctx *render.RenderContext,
) codegen.File

// ComposeSingletonFile: для project-level renderer (UTCTime, ExpectedValidators).
func (c *FileComposer) ComposeSingletonFile(
    filePath string,
    renderer render.SingletonRenderer,
    ctx *render.RenderContext,
) codegen.File
```

**Что делает `ComposeSchemaFile`:**

1. Создать общий `*gogen.ImportTracker` и `RenderContext`.
2. Создать `BufferWriter` для файла.
3. Для каждого renderer'а: инициализировать с общим `Buf`+`Import`+`Ctx`, прогнать `walker.Walk(s)`.
4. Склеить: `package` + `imports` + concat(renderer'ы в порядке списка).
5. Вернуть `gogen.File` (с gofmt).

**Важно:** `Buf` общий между renderer'ами в одном Composer-вызове — это допустимый shared state, иначе дубликаты импортов. Каждый renderer пишет последовательно; I/O изолировано через Composer.

### Generator (тонкий оркестратор поверх трёх слоёв)

```go
package generator

type Generator struct {
    modulePath string
    features   parser.ProjectFeatures
}

func (g *Generator) Generate(fw codegen.FileWriter, project *parser.Project, si *parser.SchemaIndex) error {
    ctx := g.buildContext(project, si)
    composer := &compose.FileComposer{FF: gogen.NewFileFactory()}

    // 1. Per-schema файлы.
    for _, s := range project.Model.Schemas {
        renderers := g.schemaRenderers(s, ctx)
        file := composer.ComposeSchemaFile(s, renderers, ctx)
        if err := fw.WriteFile(file); err != nil {
            return err
        }
    }

    // 2. Per-method файлы (один файл на артефакт, содержит все методы).
    methods := allMethods(project)
    for _, spec := range g.methodFileSpecs() {
        renderers := spec.renderers(ctx)
        file := composer.ComposeMethodFile(spec.path, methods, renderers, ctx)
        if err := fw.WriteFile(file); err != nil {
            return err
        }
    }

    // 3. Singleton файлы.
    if ctx.Features.UseUTCForDateTime.On {
        file := composer.ComposeSingletonFile("model/utc_time.gen.go", &singleton.UTCTimeRenderer{}, ctx)
        if err := fw.WriteFile(file); err != nil {
            return err
        }
    }
    if ctx.HasNamedValidators {
        file := composer.ComposeSingletonFile("model/expected_validators.gen.go", &singleton.ExpectedValidatorsRenderer{}, ctx)
        if err := fw.WriteFile(file); err != nil {
            return err
        }
    }

    return nil
}
```

`methodFileSpecs()` — статический список `{path, renderers(ctx)}` для 9 файлов (client/server/httpclient/echoserver/mocks×2/sdk/sugars×2).

---

## Data flow

```
Generator.Generate(fw, project, si)
    │
    ├─ buildContext(project, si)  →  RenderContext { features, splittable, typeMapper, ... }
    │
    ├─ for s in project.Model.Schemas:
    │     renderers := [Struct, Validate, SetDefaults, URLForm, Converter]  // filtered by features
    │     composer.ComposeSchemaFile(s, renderers, ctx)
    │         │
    │         ├─ create shared Buf + Import tracker
    │         ├─ for r in renderers:
    │         │     walker := NewSchemaWalker(r)
    │         │     walker.Walk(s)  → r writes to Buf via hooks
    │         └─ assemble: package + imports + concat(renderer outputs)
    │     fw.WriteFile(file)
    │
    ├─ for spec in methodFileSpecs:
    │     methods := allMethods(project)
    │     renderers := spec.renderers(ctx)
    │     composer.ComposeMethodFile(spec.path, methods, renderers, ctx)
    │         │
    │         ├─ create shared Buf + Import tracker
    │         ├─ for r in renderers:
    │         │     walker := NewMethodWalker(r)
    │         │     for m in methods:
    │         │         walker.Walk(m)  → r writes to Buf
    │         └─ assemble
    │     fw.WriteFile(file)
    │
    └─ singleton files (UTCTime, ExpectedValidators) — без walker, прямой вызов renderer'а
```

---

## Error handling

**Принцип:** fail-fast, propagate вверх. Walker прерывает обход на первой ошибке.

```go
func (w *SchemaWalker) Walk(s *parser.Schema) error {
    for _, r := range w.renderers {
        if err := r.OnStruct(s); err != nil {
            return fmt.Errorf("walk schema %q: %w", s.Name, err)
        }
    }
    // ... per-child хуки с тем же паттерном
    return nil
}
```

- Renderer возвращает ошибку с контекстом своего артефакта (`"validate: field Name: ..."`).
- Walker добавляет path-контекст (`"schema Pet: ..."`).
- `Generator.Generate` прерывается на первой ошибке (как сейчас).
- I/O ошибки (`ImportTracker`, `BufferWriter`) propagate без преобразования.

**Не делаем:** error channels, panic/recover, multi-error accumulation. T27 — synchronous, fail-fast.

---

## Testing strategy

**Три уровня тестов:**

1. **Walker unit-tests** (`internal/generator/walk/walk_test.go`):
   - Recording walker (mock renderer, записывает последовательность хуков) → проверка порядка обхода для struct/array/map/union/allOf/enum/alias.
   - Проверка `SkipDescendants` (опц. интерфейс).
   - Проверка прерывания на ошибке.
   - Покрытие: каждый type-dispatch кейс, каждый per-child хук.

2. **Renderer unit-tests** (`internal/generator/render/schema/*_test.go`, `render/method/*_test.go`):
   - Каждый renderer тестируется изолированно: строим `*parser.Schema`/`*parser.Method` руками (или через тестовый helper), прогоняем renderer, assert на содержимом `Buf`.
   - Snapshot-тесты на golden-фрагменты через `internal/golden`.
   - Главная выгода T27 — renderer'ы тестируются без полного `Generate()`.

3. **Integration tests** (существующие):
   - `cmd/oapigen/e2e_test.go` — golden-файлы должны остаться **байтово идентичными** до и после T27. Это регрессионный критерий.
   - `TestE2E_GoldenCompiles` остаётся.

**Регрессионный критерий T27:** golden-файлы в `testdata/project/golden/` не меняются ни на байт. Любое расхождение → баг миграции, а не "улучшение".

---

## Edge cases и явные решения

1. **`$ref` на external schema** (`SchemaIndex.LookupForMode`) — walker НЕ спускается в external `$ref`. External schema рендерится в своём проекте; в текущем проекте появляется только alias-import. Renderer'ы получают external тип через `typeMapper` (как сейчас).

2. **Split mode (`splitStruct`)** — отдельный type-dispatch хук `OnSplitStruct`. Renderer'ы, не различающие split/regular, реализуют только `OnStruct`. Split-aware renderer'ы (`StructRenderer`, `ConverterRenderer`) реализуют оба.

3. **`oneOf`/`anyOf`** — оба идут через `OnUnion`+`OnUnionVariant`. Walker передаёт `kind UnionKind` (`UnionOneOf`/`UnionAnyOf`) параметром в `OnUnion(s, kind)`, renderer'ы, которым нужно различать (для marshal-логики), читают этот параметр. `UnionKind` — новый тип в `walk/`, не затрагивает `parser.Schema`.

4. **`allOf` single-non-object → alias** — на уровне `parser.Schema` определяется как `alias`, не `allOf`. Walker видит `alias`, не делает descend.

5. **`additionalProperties: false`** → `struct{}` — обрабатывается в `OnStruct` (renderer видит флаг). Не отдельный хук.

6. **Cookie-параметры** — отдельный хук `OnCookieParameter`. Не сливаются с `OnHeaderParameter`.

7. **Response headers** — `OnResponseHeader` вызывается для каждого header внутри `OnResponse`. Renderer'ы (`EchoServerRenderer`, `HTTPClientRenderer`) читают их для `PayloadWithHeaders`-логики.

8. **Project-level singletons** (`utc_time.gen.go`, `expected_validators.gen.go`) — **не** используют walker. Это `SingletonRenderer` с прямым вызовом из `Generator`. Walker — для tree-shaped данных; singletons — для project-level сводной информации (список всех named validators, глобальный UTC flag).

9. **Идемпотентность renderer'ов** — renderer не должен иметь side effects за пределами своего `Buf`. `ImportTracker` — shared между renderer'ами в одном Composer-вызове (это допустимый shared state, иначе дубликаты импортов).

10. **Порядок хуков: type-dispatch → per-child → descend.** Это гарантирует, что renderer'ы видят "родителя перед детьми" — нужно для `ValidateRenderer` (накопить field names родителя перед спуском в nested struct).

---

## Migration approach (incremental)

T27 — большой рефакторинг. Migration идёт по одному renderer'у за раз, сохраняя старый `Generate()` рабочим до конца миграции.

**Принципы:**
- Каждый шаг — отдельная ветка/PR, ревьюится изолированно.
- После каждого шага — `make test` зелёный, golden-файлы байтово идентичны.
- Старый код удаляется только после того, как соответствующий renderer полностью мигрировал и протестирован.

**Шаги миграции (предлагаемый порядок, по возрастанию сложности):**

1. **Каркас.** Создать `walk/`, `render/`, `compose/` пакеты с интерфейсами, `noopSchemaRenderer`, `noopMethodRenderer`, `Base`, `RenderContext`. Без реализации renderer'ов. Юнит-тесты на walker'ы (recording mock).

2. **`AliasRenderer`, `EnumRenderer`** — самые простые, один type-dispatch хук. Перенос из `schema.go`. Старый код `renderSchema` case alias/enum — удаляется.

3. **`StructRenderer` + `JSONRenderer`** — основная масса кода. Самый большой шаг. Перенос `renderStruct`/`renderSplitStruct` + JSON-методов.

4. **`ValidateRenderer`** — перенос `validate.go` (`renderSchemaValidator`).

5. **`SetDefaultsRenderer`** — перенос `set_defaults.go`.

6. **`URLFormRenderer`** — перенос `url_form_methods.go`.

7. **`ConverterRenderer`** — перенос `converter_methods.go` (split mode).

8. **Method renderer'ы** — перенос `interfaces_client.go`, `interfaces_server.go`, `impl_client.go`, `impl_server.go`, `mocks_client.go`, `mocks_server.go`, `sdk.go`, `client_sugar.go`, `server_sugar.go`. По одному файлу за шаг.

9. **Singleton renderer'ы** — перенос `utc_time.go`, `expected_validators.go`.

10. **Финальная очистка.** Удаление старого `Generate()` (заменён на тонкий оркестратор), удаление `renderSchema` switch, удаление `writeSchemaFiles`/`writeOperationFiles`. Финальная проверка: golden-файлы идентичны, `go build ./...` чистый, `golangci-lint` чистый.

---

## Non-goals (явно за скобками T27)

- Изменение **формата** сгенерированного кода. Только внутренняя реорганизация.
- Поддержка новых фич (audit, x-* extensions) — это отдельные задачи.
- Изменение `parser.Schema`/`parser.Method` модели.
- Разделение на подмодули `model/` (это T28).
- Параллелизация renderer'ов (пока sequential).
- Изменение `codegen`/`gogen` пакетов.

---

## Структура пакетов

```
internal/generator/
├── generator.go              # Generator (тонкий оркестратор)
├── context.go                # RenderContext, buildContext
├── walk/                     # SchemaWalker, MethodWalker, interfaces, noop'ы
│   ├── schema_walker.go
│   ├── method_walker.go
│   ├── interfaces.go
│   └── walk_test.go
├── render/
│   ├── base.go               # Base, RenderContext
│   ├── schema/               # SchemaRenderer'ы
│   │   ├── struct.go
│   │   ├── json.go
│   │   ├── validate.go
│   │   ├── set_defaults.go
│   │   ├── url_form.go
│   │   ├── converter.go
│   │   └── audit.go          # future stub
│   ├── method/               # MethodRenderer'ы
│   │   ├── client_interface.go
│   │   ├── server_interface.go
│   │   ├── http_client.go
│   │   ├── echo_server.go
│   │   ├── client_mock.go
│   │   ├── server_mock.go
│   │   ├── sdk.go
│   │   ├── client_sugar.go
│   │   └── server_sugar.go
│   └── singleton/            # SingletonRenderer'ы (без walker)
│       ├── utc_time.go
│       └── expected_validators.go
└── compose/                  # FileComposer, маппинги renderer→файл
    ├── composer.go
    └── specs.go              # methodFileSpecs(), schemaRenderers()
```

---

## Acceptance criteria

1. `make test` зелёный.
2. `make e2e` зелёный; golden-файлы в `testdata/project/golden/` **байтово идентичны** до и после T27.
3. `TestE2E_GoldenCompiles` зелёный (golden-файлы компилируются).
4. `make vet` и `golangci-lint run` чистые (новые nolint'ы не добавляются без явной необходимости).
5. `internal/generator/*.go` (корневой пакет) — только `generator.go` + `context.go` (+ тесты). Весь рендер перенесён в `render/`, обход в `walk/`, оркестрация файлов в `compose/`.
6. Каждый renderer имеет unit-тесты с тестовыми `*parser.Schema`/`*parser.Method` (не только e2e).
7. Каждый walker имеет unit-тесты на порядок хуков.
8. `Generator.Generate` сводится к тонкому оркестратору из секции "Слой 4".

---

## Риски

- **Байтовая идентичность golden-файлов.** Любая мелочь (порядок импортов, пустая строка, форматирование) ломает golden. Миграция идёт по renderer'ам; после каждого шага — `make e2e` должен быть зелёным.
- **Сложность `StructRenderer`.** Перенос `renderStruct`+`renderSplitStruct`+`renderJSONMethods` в один renderer — большой объём кода. Возможен дальнейший split (Struct + JSON), но это решение принимается на шаге 3.
- **`typeMapper` coupling.** `typeMapper` сейчас встроен в `Generator`. Передача его в `RenderContext` может потребовать рефакторинга самого `typeMapper`. Решение: `typeMapper` принимает `mode` (Request/Response) и `ctx` — должен быть переиспользуем.
- **График миграции.** 10 шагов — это много. Возможен риск "застрять на полпути" с гибридным кодом. Митигация: каждый шаг самодостаточен и оставляет код в рабочем состоянии.
