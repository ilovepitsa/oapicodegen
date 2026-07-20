# T27 — Visitor Pattern Refactoring of `internal/generator/*`

- **Дата:** 2026-07-20
- **Статус:** Approved (brainstorming complete)
- **Автор:** Никита Щугорев
- **Связанный TASKS.md:** T27 (stub → заменяется на DONE по завершении)
- **Предыдущий контекст:** T26 (Multi-service layout) — DONE

## 1. Цель

Refactor `internal/generator/*` на visitor-pattern: разделить «обход доменной модели» (`walk` package) и «рендер Go-кода» (`render` package). Все генераторы — schema-side и operations-side — становятся renderer'ами, регистрируемыми в `compose.FileComposer`. `Generator` превращается в тонкий диспетчер.

Достигаемые выгоды:
- Унификация обхода `parser.Schema` и `parser.Method` через walker'ы.
- Дедупликация логики обхода между schema/client/server/impl/mocks/sdk-генераторами.
- Изоляция renderer'ов — каждый файл рендерится независимым компонентом, тестируемым поштучно.
- Устранение моста `SchemaCallbacks`/`generatorCallbacks` между новым walker-миром и старым generator-кодом.

## 2. Не-цели

- Изменение генерируемого output'а (byte-for-byte идентичность — раздел 4).
- Добавление новых возможностей генератора (включая элементы из глубокого бэклога: `x-audit-data`, `GenerateConverters`, `USE_REQUIRED_V2` generator support).
- Рефакторинг пакетов за пределами `internal/generator/` (`parser`, `codegen`, `fs`, и т.д.).
- T28 (Subpackage splitting) — отдельный spec.

## 3. Существующая инфраструктура (переиспользуем)

Walker-pattern частично уже внедрён. После T26 и незакоммиченного schema-walker WIP:

- `internal/generator/walk/` — `SchemaWalker`, `MethodWalker`, интерфейсы `SchemaRenderer`/`MethodRenderer`, `NoopSchemaRenderer`/`NoopMethodRenderer`, `SkipDescendants` optional-интерфейс.
- `internal/generator/compose/` — `FileComposer` с `ComposeSchemaFile`, `ComposeMethodFile`, `ComposeSingletonFile`. Оркестрирует walker-прогоны, shared `Buf`/`ImportTracker` вливаются через `Base.Init`.
- `internal/generator/render/` — `Base` (встраиваемый тип с `Buf`/`Imports`/`Ctx`), `RenderContext`, `ImportTracker`, `SingletonRenderer`.
- `internal/generator/render/schema/` — `StructRenderer`, `AliasRenderer`, `EnumRenderer`, `JSONRenderer` (для union MarshalJSON/UnmarshalJSON).
- `internal/generator/render_callbacks_adapter.go` — мост `generatorCallbacks` к ещё не мигрированным методам `Generator` (SetDefaults, ValidateOwn, schemaTreeHasDefaults).

## 4. Принципы и ограничения

### 4.1. Стабильность выхода

**Byte-for-byte идентичность.** Все существующие golden-тесты (`testdata/project/minimal/`, `internal/golden`, unit-тесты renderer'ов с golden-сравнением) остаются зелёными после каждой фазы. Любая разница = баг. Порядок вывода в Buf управляется порядком renderer'ов в pack'е и детерминированным порядком хуков walker'а.

### 4.2. Один pack = один выходной файл

Pack renderer'ов = список `[]walk.SchemaRenderer` или `[]walk.MethodRenderer`, передаваемый в `ComposeSchemaFile`/`ComposeMethodFile`. Все renderer'ы в pack'е пишут в общий Buf через `Base.Init`. Порядок хуков walker'а + порядок renderer'ов в pack'е → детерминированный порядок вывода.

### 4.3. Симметрия schema/operations

И schema-side, и operations-side используют одну и ту же модель: `RenderContext` + pack renderer'ов + `compose.FileComposer`. Никаких специальных путей для operations.

### 4.4. Устранение моста `SchemaCallbacks`

К концу Фазы 1 мост `SchemaCallbacks`/`generatorCallbacks` полностью устранён. Все методы, которые он прокидывал (`SchemaTreeHasDefaults`, `RenderSetDefaults`, `RenderValidateOwn`), становятся:

- Либо хуками на новых renderer'ах (`SetDefaultsRenderer.OnStruct`, `ValidateOwnRenderer.OnStruct`).
- Либо чистыми функциями в `render/schema/` (`treeHasDefaults(s, mode)`), используемыми renderer'ами напрямую.

Поле `RenderContext.Callbacks` удаляется.

### 4.5. TypeMapper — полный вынос в `render/typemap/`

`typeMapper` (ныне в `internal/generator/typemap.go`, `type.go`) и adapter `typemap_adapter.go` переезжают в `internal/generator/render/typemap/`. Adapter (`newRenderTypeMapper`) удаляется. Renderer'ы используют `render.TypeMapper` напрямую; реализация — в `render/typemap/`. Проводится в Фазе 6.

### 4.6. Фазирование

7 фаз (раздел 6). Каждая фаза оставляет `go build ./...` и `go test ./...` зелёными. Каждая фаза = отдельный PR, по правилам TASKS.md.

## 5. Архитектура

### 5.1. Карта пакетов после рефакторинга

```
internal/generator/
  walk/                        — walker'ы (готово)
    schema_walker.go
    method_walker.go
    interfaces.go              — SchemaRenderer, MethodRenderer
    noop.go                    — NoopSchemaRenderer, NoopMethodRenderer
  compose/                     — FileComposer (готово)
    composer.go
  render/
    base.go                    — Base, RenderContext, ImportTracker, SingletonRenderer (готово)
    schema/
      struct.go                — StructRenderer (готово)
      alias.go                 — AliasRenderer (готово)
      enum.go                  — EnumRenderer (готово)
      json.go                  — JSONRenderer для union (готово)
      set_defaults.go          — SetDefaultsRenderer (новый, Фаза 1)
      validate.go              — ValidateOwnRenderer (новый, Фаза 1)
      update_struct.go         — UpdateStructRenderer (новый, Фаза 1)
      url_form.go              — URLFormRenderer (новый, Фаза 1)
      converters.go            — ConvertersRenderer для split+shared (новый, Фаза 1)
      defaults.go              — treeHasDefaults(s, mode) утилита (новый, Фаза 1)
      utc_time.go              — UTCTimeRenderer, SingletonRenderer (новый, Фаза 2)
      expected_validators.go   — ExpectedValidatorsRenderer, SingletonRenderer (новый, Фаза 2)
      naming.go                — goName, modeRequest/Response, inlineVariantName и т.д.
      constants.go             — goTypeAny, schemaType* и т.д.
    operations/
      client_interface.go      — ClientInterfaceRenderer (новый, Фаза 3)
      client_sugar.go          — ClientSugarRenderer (новый, Фаза 3)
      audit_client.go          — AuditClientRenderer (новый, Фаза 3)
      server_interface.go      — ServerInterfaceRenderer (новый, Фаза 3)
      httpclient_impl.go       — HTTPClientImplRenderer (новый, Фаза 4)
      echoserver_impl.go       — EchoServerImplRenderer (новый, Фаза 4)
      client_mocks.go          — ClientMocksRenderer (новый, Фаза 5)
      server_mocks.go          — ServerMocksRenderer (новый, Фаза 5)
      sdk.go                   — SDKRenderer (новый, Фаза 5)
      shared.go                — общие хелперы operations-генераторов
    typemap/
      typemap.go               — typeMapper реализация (переезжает из internal/generator/, Фаза 6)
      type.go                  — Mode, типы для typeMapper (переезжает, Фаза 6)
  generator.go                 — тонкий диспетчер (orchestration-only, Фаза 6)
  generator_test.go            — integration-тест на Generate
  constants.go                 — общие константы (если остаются)
  naming.go                    — общие хелперы именования (если остаются)
```

### 5.2. RenderContext (после устранения Callbacks)

```go
type RenderContext struct {
    Project      *parser.Project
    SchemaIndex  *parser.SchemaIndex
    Features     parser.ProjectFeatures
    Splittable   map[string]bool
    ModulePath   string
    ImportPrefix string
    TypeMapper   TypeMapper  // реализация в render/typemap/ после Фазы 6
    Imports      *ImportTracker
}
```

Поле `Callbacks SchemaCallbacks` удалено.

### 5.3. Generator (после Фазы 6)

```go
type Generator struct {
    project     *parser.Project
    schemaIndex *parser.SchemaIndex
    factory     *gogen.FileFactory
    composer    *compose.FileComposer
    splittable  map[string]bool
}

func Generate(fw codegen.FileWriter, project *parser.Project, si *parser.SchemaIndex, opts ...Option) error {
    g := newGenerator(project, si, opts...)
    if err := g.writeSchemas(fw); err != nil { return err }
    if err := g.writeSingletons(fw); err != nil { return err }
    if hasOperations(project) {
        if err := g.writeOperations(fw); err != nil { return err }
    }
    return nil
}
```

Каждый `write*` метод — простой цикл: для каждой schema/method вызывает `g.composer.Compose*File(...)` с нужным pack'ом и записывает результат через `fw.WriteFile(path, file)`. Никакого `r.Buf.Print` в `generator.go`.

## 6. Фазы миграции

Каждая фаза = отдельный PR. До merge: `go build ./...`, `go test ./...`, `golangci-lint run` — зелёные, golden-тесты без изменений.

### 6.1. Фаза 1 — Schema-side: устранение Callbacks-моста

**Pack composition для schema-файлов:**

| Тип схемы | Выходной файл | Pack |
|---|---|---|
| object (не split) | `model/<name>.gen.go` | `[StructRenderer, SetDefaultsRenderer, ValidateOwnRenderer, UpdateStructRenderer]` |
| object (split) | `model/<name>.gen.go` | `[StructRenderer, SetDefaultsRenderer, ValidateOwnRenderer, UpdateStructRenderer]` (SetDefaults/ValidateOwn aware of mode) |
| alias / map-alias | `model/<name>.gen.go` | `[AliasRenderer]` |
| enum | `model/<name>.gen.go` | `[EnumRenderer]` |
| union (oneOf/anyOf) | `model/<name>.gen.go` | `[StructRenderer, JSONRenderer]` |
| union — aux | `model/<name>_json.gen.go` | `[JSONRenderer]` (если вынесено в aux-файл) |
| object/union — aux | `model/<name>_url_form.gen.go` | `[URLFormRenderer]` (если применимо) |
| object (split + shared) — aux | `model/<name>_converters.gen.go` | `[ConvertersRenderer]` (если включён split и есть shared-поля) |

**Новые renderer'ы:**

- `SetDefaultsRenderer` — реализует `OnStruct`/`OnSplitStruct`; рендерит `func (m *<Name>) SetDefaults()`. Заменяет `Callbacks.RenderSetDefaults` + `set_defaults.go`.
- `ValidateOwnRenderer` — реализует `OnStruct`/`OnSplitStruct`; рендерит `func (m *<Name>) ValidateOwn(reg *validator.Registry) error` + `ExpectedValidators() []string`. Заменяет `Callbacks.RenderValidateOwn` + `validate.go`.
- `UpdateStructRenderer` — реализует `OnStruct`/`OnSplitStruct`; рендерит `Update<Name>` struct + getter'ы `Get<Field>()`. Заменяет логику, живущую сейчас в `StructRenderer.renderUpdateStruct`.
- `URLFormRenderer` — реализует `OnStruct`/`OnSplitStruct`; рендерит `MarshalURLForm`/`UnmarshalURLForm`. Заменяет `url_form_methods.go`.
- `ConvertersRenderer` — реализует `OnStruct`/`OnSplitStruct`; рендерит `<Name>RequestToResponse`/`<Name>ResponseToRequest` (если split + shared-поля). Заменяет `converter_methods.go`.

**Утилиты:**

- `render/schema/defaults.go` — `func treeHasDefaults(s *parser.Schema, mode Mode) bool`. Чистая функция, используемая `SetDefaultsRenderer` напрямую. Заменяет `Callbacks.SchemaTreeHasDefaults`.

**Удаление:**

- `render_callbacks_adapter.go` — целиком.
- `RenderContext.Callbacks` поле.
- `SchemaCallbacks` interface (если определён в `render/`).
- `set_defaults.go`, `validate.go`, `url_form_methods.go`, `converter_methods.go` — после переезда логики в renderer'ы.

**Порядок вывода:**

Walker'ы вызывают хуки в детерминированном порядке (`dispatchType` → `dispatchChildren` → `descend`). Все renderer'ы в pack'е получают один и тот же хук последовательно (`callEach`). Порядок вывода в Buf:

```
OnStruct(s): StructRenderer → SetDefaultsRenderer → ValidateOwnRenderer → UpdateStructRenderer
```

→ struct declaration, SetDefaults, ValidateOwn, UpdateStruct. Если текущий порядок отличается — корректируем порядок renderer'ов в pack'е (не логику внутри).

**Тестирование Фазы 1:**

- Существующие golden-тесты — зелёные после каждой подзадачи.
- Unit-тесты на каждый новый renderer (пример — `render/schema/json_test.go`, `render/schema/struct_test.go`): построить `RenderContext`, вызвать `OnStruct`/etc, проверить Buf.
- Compile-тест: `go build ./...` на сгенерированном коде.

### 6.2. Фаза 2 — Singleton-рендеры

**Renderer'ы:**

- `UTCTimeRenderer` — `SingletonRenderer`; рендерит `model/utc_time.gen.go` (тип `UTCTime`). Заменяет `utc_time.go`.
- `ExpectedValidatorsRenderer` — `SingletonRenderer`; рендерит `model/expected_validators.gen.go`. Заменяет `expected_validators.go`.

**Диспетчеризация:** через `compose.FileComposer.ComposeSingletonFile` (уже есть).

**Удаление:** `utc_time.go`, `expected_validators.go` — после переезда.

### 6.3. Фаза 3 — Operations-side: interfaces

**Renderer'ы:**

- `ClientInterfaceRenderer` → `interfaces/client/client.gen.go`. Заменяет `client.go`.
- `ClientSugarRenderer` → `interfaces/client/client_sugar.gen.go`. Заменяет `client_sugar.go`.
- `AuditClientRenderer` → `interfaces/client/audit.gen.go`. Заменяет `audit_model.go`/`audit_server.go` (audit-специфика, если есть; иначе пустой renderer для симметрии).
- `ServerInterfaceRenderer` → `interfaces/server/server.gen.go`. Заменяет `server.go`.

**Диспетчеризация:** `ComposeMethodFile(pkg, methods, pack, ctx)` per file. Pack из 1 элемента.

### 6.4. Фаза 4 — Operations-side: impl

**Renderer'ы:**

- `HTTPClientImplRenderer` → `impl/httpclient/client.gen.go`. Заменяет `impl_client.go`.
- `EchoServerImplRenderer` → `impl/echoserver/server.gen.go`. Заменяет `impl_server.go`.

Могут требовать `SchemaIndex` для inline-body-rendering — renderer'ы достают через `r.Ctx.SchemaIndex`.

### 6.5. Фаза 5 — Operations-side: mocks + sdk

**Renderer'ы:**

- `ClientMocksRenderer` → `impl/mocks/client/mocks.gen.go`. Заменяет `mocks.go` (client-часть).
- `ServerMocksRenderer` → `impl/mocks/server/mocks.gen.go`. Заменяет `mocks.go` (server-часть).
- `SDKRenderer` → `sdk/sdk.gen.go`. Заменяет `sdk.go`.

### 6.6. Фаза 6 — Generator slim-down + TypeMapper migration

**Generator:**

- `generator.go` превращается в тонкий диспетчер (см. 5.3).
- Удаляются `writeStructFileViaComposer`/`writeSchemaFilesViaComposer` — заменяются единым `writeSchemaFiles` с pack-диспетчеризацией по типу схемы.

**TypeMapper migration (Вариант B):**

- `typemap.go`, `type.go` → переезжают в `render/typemap/`.
- `typemap_adapter.go` (`newRenderTypeMapper`) — удаляется.
- `RenderContext.TypeMapper` ссылается на реализацию в `render/typemap/`.
- Renderer'ы используют `render.TypeMapper` interface напрямую, без adapter'а.

**Удаление ставших мёртвыми файлов:**

После Фаз 1–5 + Фазы 6 из `internal/generator/` удаляются (логика переехала в `render/`):
- `audit_model.go`, `audit_server.go`
- `client.go`, `client_sugar.go`
- `converter_methods.go`
- `expected_validators.go`
- `impl_client.go`, `impl_server.go`
- `mocks.go`
- `operation.go` (общие хелперы разбираются по `render/operations/shared.go`)
- `response_headers.go` (в `shared.go` или конкретный renderer)
- `schema.go` (если остаётся минимальный — иначе удаляется)
- `sdk.go`
- `server.go`
- `set_defaults.go`, `url_form_methods.go`, `validate.go`
- `render_callbacks_adapter.go`
- `utc_time.go`
- `typemap.go`, `type.go`, `typemap_adapter.go`

**Остаётся в `internal/generator/`:**

- `generator.go` — диспетчер
- `constants.go` — если используется несколькими renderer'ами (иначе переезжает)
- `naming.go` — если используется несколькими renderer'ами (иначе переезжает)
- `generator_test.go` — integration-тест

### 6.7. Фаза 7 — Документирование

**`ARCHITECTURE.md`:**

- Обновить секцию «Generator»:
  - Карта пакетов: `walk/`, `compose/`, `render/`, `render/schema/`, `render/operations/`, `render/typemap/`.
  - Диаграмма: Generator → composer → walker → renderers → Buf/Imports → File.
  - Принципы: один pack = один файл, симметрия schema/operations, устранение Callbacks.

**`TASKS.md`:**

- Заменить stub T27 на:
  ```
  ### T27 — Visitor pattern refactoring — DONE (Phases 1–7)
  См. дизайн-док: docs/superpowers/specs/2026-07-20-t27-visitor-refactoring-design.md
  ```

**`README.md`:**

- Если упоминается внутренняя структура generator'а — обновить.

## 7. Диспетчеризация operations-файлов

`writeOperationFiles` после Фаз 3–5:

```go
func (g *Generator) writeOperationFiles(fw codegen.FileWriter) error {
    methods := allMethods(g.project)
    ctx := g.opCtx()

    files := []struct {
        path string
        pkg  string
        pack []walk.MethodRenderer
    }{
        {"interfaces/client/client.gen.go",       "client",      []walk.MethodRenderer{oprender.NewClientInterfaceRenderer()}},
        {"interfaces/client/client_sugar.gen.go", "client",      []walk.MethodRenderer{oprender.NewClientSugarRenderer()}},
        {"interfaces/client/audit.gen.go",        "client",      []walk.MethodRenderer{oprender.NewAuditClientRenderer()}},
        {"interfaces/server/server.gen.go",       "server",      []walk.MethodRenderer{oprender.NewServerInterfaceRenderer()}},
        {"impl/httpclient/client.gen.go",         "httpclient",  []walk.MethodRenderer{oprender.NewHTTPClientImplRenderer()}},
        {"impl/echoserver/server.gen.go",         "echoserver",  []walk.MethodRenderer{oprender.NewEchoServerImplRenderer()}},
        {"impl/mocks/client/mocks.gen.go",        "mocksclient", []walk.MethodRenderer{oprender.NewClientMocksRenderer()}},
        {"impl/mocks/server/mocks.gen.go",        "mocksserver", []walk.MethodRenderer{oprender.NewServerMocksRenderer()}},
        {"sdk/sdk.gen.go",                        "sdk",         []walk.MethodRenderer{oprender.NewSDKRenderer()}},
    }

    for _, f := range files {
        cf, err := g.composer.ComposeMethodFile(f.pkg, methods, f.pack, ctx)
        if err != nil {
            return fmt.Errorf("compose %s: %w", f.path, err)
        }
        if err := fw.WriteFile(f.path, cf); err != nil {
            return fmt.Errorf("write %s: %w", f.path, err)
        }
    }
    return nil
}
```

Что НЕ меняется:
- Имена файлов, пакеты вывода, пути.
- Порядок методов в файле (порядок в `methods` slice, формируемом generator'ом).
- Порядок хуков walker'а (заголовки сортированы лексикографически через `sortedHeaderNames`).

## 8. Особенности renderer'ов (детали на этапе writing-plans)

- **ClientSugarRenderer** — генерирует сахар-методы (`<Method>With<X>`), своя state-машина поверх хуков.
- **HTTPClientImplRenderer / EchoServerImplRenderer** — генерируют HTTP-вызовы/роутинг; могут требовать `SchemaIndex` для inline-body-rendering.
- **MocksRenderer'ы** — генерируют mock-структуры и expect-call-методы; итерируют responses несколько раз.
- **SDKRenderer** — top-level SDK-фасад, агрегирующий несколько сервисов.

Эти детали прорабатываются в implementation plan для каждой фазы отдельно. Spec фиксирует только контракт: один renderer на файл, pack из 1 элемента, диспетчеризация через `ComposeMethodFile`.

## 9. Acceptance criteria

1. `go build ./...` — зелёный.
2. `go test ./...` — зелёный, включая все golden-тесты.
3. `golangci-lint run` — зелёный (с существующим `.golangci.yml`).
4. Сгенерированный output на `testdata/project/minimal/` byte-for-byte идентичен output'у до начала рефакторинга.
5. `internal/generator/generator.go` ≤ ~150 строк, только orchestration (без `r.Buf.Print`).
6. `render_callbacks_adapter.go` удалён.
7. `typemap_adapter.go` удалён; `typeMapper` живёт в `render/typemap/`.
8. Каждый renderer имеет unit-тест.
9. `ARCHITECTURE.md` и `TASKS.md` обновлены.

## 10. Риски и митигации

| Риск | Митигация |
|---|---|
| Byte-for-byte дрейф при переезде renderer'ов | Каждая подзадача Фазы 1–5 заканчивается прогоном golden-тестов; дрейф = баг, чиним до перехода к следующей. |
| Скрытые зависимости между генераторами (общее состояние, неявный порядок) | Фаза 6 (slim-down) только после Фаз 1–5 — к этому моменту все зависимости выявлены через renderer'ы. |
| TypeMapper migration (Фаза 6) — большой объём | Если риск > польза на этапе Фазы 6 — допускается откат к Варианту A (adapter остаётся). Решение принимается по факту оценки объёма. Но цель — Вариант B. |
| Mocks/SDK renderer'ы сложные, риск регрессий | Фаза 5 — последняя перед slim-down; unit-тесты на renderer + golden-тесты на полный output. |

## 11. Out of scope

- T28 (Subpackage splitting) — отдельный spec.
- Глубокий бэклог: `x-audit-data`, `GenerateConverters` (генераторная поддержка), `USE_REQUIRED_V2`, кастомные `x-*` расширения.
- Validator TUI, Terraform-provider, TypeScript-генератор, `graphgen` — отдельные компоненты.
- nolint review backlog (отдельная задача, не относится к T27).
