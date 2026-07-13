# План задач: oapigenerator

Проект: Go-генератор из OpenAPI-спек, **без Kotlin, без TypeScript, без Terraform-provider**. Все вспомогательные библиотеки — собственные, в `internal/` и `pkg/`. Валидатор (TUI) — в бэклоге.

- **Go-модуль**: `nschugorev/oapigenerator` (временное имя, потом сменим)
- **Рабочий процесс**: одна задача → одна ветка `feat/...` → merge в `main`

## Скоуп первой итерации (важно!)

**Поддерживаются только стандартные конструкции OpenAPI 3.x:**
- `paths`, `parameters` (path/query/header/cookie), `requestBody`, `responses`
- `schemas`: `object`, `array`, `string`/`integer`/`number`/`boolean`/`null`, `$ref`
- `oneOf`, `anyOf`, `allOf` (включая `allOf` из одного non-object элемента → alias), `discriminator`
- `required`, `enum` (с дедупликацией), `format`, `default`, `nullable`, `deprecated`
- `additionalProperties: false` → генерируется `struct{}` (закрытая структура)
- стандартные `securitySchemes` (только как метаданные, без кодогенерации мидлвэйров)
- типизированные response headers (через `<Name><Code>PayloadWithHeaders`)

**Поддерживается через generation flags (включаются в `generation_flags.yaml`):**
- `GOLANG_SPLIT_REQUEST_RESPONSE` — раздельные `<Name>Request`/`<Name>Response` модели (T24f — DONE)
- `USE_UTC_FOR_DATE_TIME` — принудительная UTC-сериализация `time.Time` через тип `UTCTime` (T24d — DONE)
- `GOLANG_SERVER_BODY_REQUEST_NO_AUTO_DEFAULTS` — когда off, server request-decoder вызывает `req.Body.SetDefaults()` (T24e — DONE, T25a — DONE)
- `USE_REQUIRED_V2` — флаг заведён в `ProjectFeatures`, генераторной поддержки пока нет (T24g — pending)

**Дополнительно реализовано (без generation flags):**
- `x-validations` — декларативные правила валидации (`>N`/`Size >=N`/`pkg.Name`/`Immutable`) → `ValidateOwn(reg)` + `ExpectedValidatorNames()` + reflection-walker `pkg/validator.Validate`
- Update-схемы `Update<Name>` для PUT/PATCH body (T25b — DONE)
- URL-form encoding `application/x-www-form-urlencoded` (T25c — DONE)
- `SetDefaults()` для object-схем с default-полями (T25a — DONE)
- Типизированные response headers (`<Name><Code>PayloadWithHeaders`)

**Не поддерживается (бэклог):**
- `x-audit-data` + `ServerAuditData` — комплаенс-логирование (audit-схема на каждую операцию)
- `GenerateConverters` — автоконвертеры `Request→Response` (только при split-mode)
- `x-request-required`/`x-response-required` — генераторная поддержка `USE_REQUIRED_V2` (T24g)
- Кастомные `x-*` расширения

Это значит: задачи T13, T14, T16 (audit), T18 (HTTP server для audit) упрощаются или откладываются. См. раздел «Корректировки задач» ниже.

## Корректировки задач под первую итерацию

- **T13 Schema + JSON-методы** — DONE (без `x-*`-специфики и без `GenerateConverters`)
- **T14 URLForm, WithDefaults, Update-схемы** — DONE (T25a SetDefaults, T25b Update-схемы, T25c URLForm)
- **T15 Client interfaces + sugar** — DONE (включая x-validations)
- **T16 Server interfaces + audit** — DONE (без `ServerAuditData`/`audit_data`-схемы — в бэклоге)
- **T17 HTTP client** — DONE (включая URL-form encoding)
- **T18 HTTP server** — DONE (базовый роутинг, URL-form decoding, типизированные response headers — DONE; без audit-data)
- **T19 Mocks** — DONE
- **T20 SDK** — DONE
- **T21 cmdtreegenerator** → **глубокий бэклог** (не нужен в первой итерации; весь функционал завязан на `x-cli` расширения, CRUD-эвристики и multi-service parser — пересмотреть при появлении реального требования к CLI)
- **T22 opensourceyaml** → **глубокий бэклог** (не нужен в первой итерации; публикацией спеки управляет внешняя инфраструктура, не генератор)
- **T12 GenerationFlagsLoader** → **реализован как T24a** (загрузчик `internal/parser/generation_flags.go` + `generation_flags_loader.go`); глобальный `generation_flags.yaml` + per-project override
- **T23 `cmd/oapigen`** — DONE
- **T24 e2e-тесты** — DONE
- **T25 CI, линтеры** — частично: Makefile-таргеты есть; `.gitlab-ci.yml`/`.golangci.yml` см. репозиторий
- **Типизированные response headers** (новая задача, вне исходной нумерации) — DONE: `internal/generator/response_headers.go`, `PayloadWithHeaders`-паттерн, типизированные header-поля (string/int/int32/int64/float32/float64/bool), client-decoder с `strconv` + error propagation, server-side `Headers()` метод

## Карта вспомогательных пакетов

| Группа | Пакет | Реализация | Задача |
|---|---|---|---|
| Codegen-ядро | `codegen` | `internal/codegen` | T6 |
| Codegen Go-рендер | `codegen/gogen` | `internal/codegen/gogen` | T7 |
| Codegen-конфигуратор | `codegen/configurator` | `internal/codegen/configurator` | T8 |
| FS | `fs` | `internal/fs` | T5 |
| CLI-логирование | `cli/logging` | `internal/cli/logging` | T9 |
| Утилиты | `ptr` | `internal/ptr` | T3 |
| Утилиты | `must` | `internal/must` | T4 |
| Тесты | `golden` | `internal/golden` | T10 |
| HTTP-клиент | `httpclient` | `pkg/httpclient` | — |

## Этап 0 — скелет

### T1 — Инициализация Go-модуля и скелет проекта
- `go.mod` (module `nschugorev/oapigenerator`, go 1.25)
- Структура: `cmd/`, `internal/`, `testdata/`
- `.gitignore`, `Makefile` (минимальный)
- Ветка: `feat/skeleton`
- Зависимости: нет

### T2 — README и архитектурный документ
- `README.md` (назначение, использование, статус)
- `ARCHITECTURE.md` (что генерируем, карта `internal/`, поток parser → generator → writer)
- Ветка: `feat/docs`
- Зависимости: T1

## Этап 1 — вспомогательные пакеты

### T3 — `internal/ptr`
- API: `Ptr[T](v) *T`, `From[T](*T) T` с zero-value fallback, `Or`, `Equal`, и т.д.
- Тесты + бенч
- Ветка: `feat/internal-ptr`
- Зависимости: T1

### T4 — `internal/must`
- API: `Must(err)`, `MustGet[T](v T, err error) T`, `MustNoError(err)` и пр.
- Тесты
- Ветка: `feat/internal-must`
- Зависимости: T1

### T5 — `internal/fs`
- API: `RealFS`, `NewRecommendedReal(opts...)`, `WithBaseDir`, интерфейс FS (read/write, MkdirAll, Stat)
- Тесты на `testing/fstest`
- Ветка: `feat/internal-fs`
- Зависимости: T1

### T6 — `internal/codegen` — ядро
- API: `File`, `FileWriter` (`WriteFile(name, File) error`, `Close() error`), `BufferWriter`, `WithPath(fw, ...)`, `noopFileWriter`, `NewBufferWriter`
- Тесты
- Ветка: `feat/internal-codegen-core`
- Зависимости: T1

### T7 — `internal/codegen/gogen` — FileFactory и рендер Go-файлов
- API: `FileFactory`, `NewFileFactory(toolName)`, `Create(File) *bytes.Buffer` / `io.Reader`, gofmt-рендер
- Тесты на рендер
- Ветка: `feat/internal-codegen-gogen`
- Зависимости: T6

### T8 — `internal/codegen/configurator` — FileWriter из флагов
- API: `NewFileWriterConfiguratorFromFlags(*flag.FlagSet)`, `Create(log, output) FileWriter`
- Тесты
- Ветка: `feat/internal-codegen-configurator`
- Зависимости: T5, T6

### T9 — `internal/cli/logging` — zap-логирование из флагов
- API: `NewLoggerConfiguratorFromFlags(*flag.FlagSet)`, `Create() *zap.Logger`
- Тесты
- Ветка: `feat/internal-cli-logging`
- Зависимости: T1

### T10 — `internal/golden` — golden-тесты
- API: `Equals(t, got, want string)`, `Update(path, content)`, флаг `-update`
- Тесты
- Ветка: `feat/internal-golden`
- Зависимости: T1

## Этап 2 — парсер OpenAPI

### T11 — `internal/parser` — портирование парсера OpenAPI (стандартный OpenAPI) — DONE
- `ResourcesSet`, `ProjectSet`, `Project`, `Schema`, `Paths`, `Service`, `Method`, `Imports`
- `ResourcesLoader`, `ProjectLoader`, `AugmentProjectSet`
- Чтение OpenAPI через `github.com/pb33f/libopenapi`
- **Только стандартные поля OpenAPI 3.x** — `x-*` расширения игнорируем на парсинге (с предупреждением в лог)
- Ветка: `feat/parser`
- Зависимости: T3, T4, T5

### T12 — GenerationFlagsLoader — DONE (реализован как T24a)
- Реализован в `internal/parser/generation_flags.go` + `generation_flags_loader.go`
- Грузит глобальный `generation_flags.yaml` + per-project override
- См. T24a для деталей

## Этап 3 — генераторы

### T13 — generator: Schema + JSON-методы (стандартный OpenAPI) — DONE
- `NewSchema` (object/array/primitive/oneOf/anyOf/allOf/$ref), `NewJSONMethods`, `NewSchemaOneOfResource`, `NewSchemaOneOfResourceJSON`
- Без `GenerateConverters` (требует split request/response — бэклог)
- Без обработки `x-*` расширений
- Ветка: `feat/gen-schema`
- Зависимости: T7, T11

### T14 — generator: URLForm, WithDefaults, Update-схемы → БЭКЛОГ
- Требует URL-form encoding и update-схемы (кастомная семантика)
- Откладываем до второй итерации

### T15 — generator: Client interfaces + sugar (стандартный OpenAPI) — DONE
- `ClientOptions`, `ClientInterface`, `ClientSugar`, `TestSugarMethods`
- Без x-validations
- Ветка: `feat/gen-client-iface`
- Зависимости: T13

### T16 — generator: Server interfaces (без audit) — DONE
- `ServerInterface`, `ServerAllServicesInterface`, `TestServer`
- ~~`ServerAuditData`~~ → бэклог (требует x-audit-data)
- Ветка: `feat/gen-server-iface`
- Зависимости: T13

### T17 — generator: HTTP client — DONE
- `HTTPClientInit`, `HTTPClientMethods`, `HTTPClientDecoder`
- Ветка: `feat/gen-http-client`
- Зависимости: T15

### T18 — generator: HTTP server — DONE
- `HTTPServer`, `HTTPServerAllServices`, `NewHTTPServerFeatures`
- Ветка: `feat/gen-http-server`
- Зависимости: T16

### T19 — generator: Mocks — DONE
- `Mock` (client + server), AllServices mock
- Ветка: `feat/gen-mocks`
- Зависимости: T15, T16

### T20 — generator: SDK — DONE
- `SDK`, `SDKService`
- Ветка: `feat/gen-sdk`
- Зависимости: T15

### T21 — cmdtreegenerator — дерево команд CLI → ГЛУБОКИЙ БЭКЛОГ
- ~~Порт генератора дерева команд CLI~~
- Причина: оригинал (4841 строка) завязан на `x-cli` расширения, CRUD-эвристики, multi-service parser (`parser.Project`/`Service`/`Method`), кастомный CLI-фреймворк `cli.Command[T]` с profile config — ничего из этого у нас нет и не планируется в первой итерации.
- Пересмотреть при появлении реального требования к auto-generated CLI.
- Ветка: ~~`feat/cmdtree`~~ (не создаётся)
- ~~Зависимости: T11, T13~~

### T22 — opensourceyaml — публичный OpenAPI-spec → ГЛУБОКИЙ БЭКЛОГ
- ~~Порт генератора публичного OpenAPI-spec~~
- Причина: публикацией публичного OpenAPI-spec управляет внешняя инфраструктура (repo/release pipeline), а не генератор. В первой итерации не нужно.
- Пересмотреть при появлении требования «генератор должен вырезать `x-*` из internal-spec и публиковать public-spec».
- Ветка: ~~`feat/opensource-yaml`~~ (не создаётся)
- ~~Зависимости: T11~~

## Этап 4 — точка входа

### T23 — `cmd/oapigen` — точка входа генератора — DONE
- `main.go`: флаги (`output`, `input`, `import-prefix`, `dry-run`, `generation-flags-config-path`, `project-flags-path`, `log-*`)
- ~~`new-validator`~~ → бэклог (секция валидатора)
- Связка: parser → generator (Schema/Client/Server/HTTP/Mock/SDK)
- Ветка: `feat/oapigen-main`
- Зависимости: T8, T9, T11, T13, T15, T16, T17, T18, T19, T20

## Этап 5 — тесты и инфраструктура

### T24 — e2e-тесты генерации (стандартный OpenAPI) — DONE
- `testdata/minimal/` — эталонная мини-спека **только со стандартными конструкциями** (object/array/oneOf/$ref, без `x-*`)
- Сравнение вывода с golden-файлами через `internal/golden`
- Ветка: `feat/e2e-tests`
- Зависимости: T23, T10

### T25 — CI, линтеры, Makefile-таргеты
- `.gitlab-ci.yml` (скелет: build, test, lint)
- `.golangci.yml`
- Makefile-таргеты: `build`, `test`, `generate`, `lint`, `e2e`
- Ветка: `feat/ci-lint`
- Зависимости: T24

## Вторая итерация — детальные подзадачи

### T24: GenerationFlagsLoader — разбит на T24a–T24g

#### T24a — GenerationFlagsLoader: infrastructure — DONE
- Реализовано: `internal/parser/generation_flags.go` (флаги, `ProjectFeatures`, `ProjectFeature`) + `internal/parser/generation_flags_loader.go` (загрузка, валидация, per-project override)
- `GenerationFlagConfig` (yaml: name, description, enabled, defaultValue, targetValue, affects, dependsOn, migrateUntil)
- `ProjectFeatures` struct с 4 флагами: `GOLANG_SERVER_BODY_REQUEST_NO_AUTO_DEFAULTS`, `GOLANG_SPLIT_REQUEST_RESPONSE`, `USE_REQUIRED_V2`, `USE_UTC_FOR_DATE_TIME`
- `Load(source)` — грузит глобальный `generation_flags.yaml`
- `GetProjectFeatures(projectPath)` — грузит per-project override, резолвит финальные значения
- Валидация: affects содержит `golang`, зависимости, migrateUntil
- Файлы: `internal/parser/generation_flags.go`, `internal/parser/generation_flags_loader.go` + тесты + testdata
- Зависимости: нет
- Ветка: `feat/genflags-loader`

#### T24b — CLI флаг `--generation-flags-config-path` — DONE
- Добавлены флаги `-generation-flags-config-path` и `-project-flags-path` в `cmd/oapigen/main.go`
- Если задан config-path — `GenerationFlagsLoader.Load()` + `GetProjectFeatures()`, передаёт в `Generate()` через `WithProjectFeatures`
- Если не задан — `ProjectFeatures` с всеми false
- `-project-flags-path` требует `-generation-flags-config-path` (валидация в CLI)
- Зависимости: T24a, T24c
- Ветка: `feat/genflags-cli`

#### T24c — Wire `ProjectFeatures` into Generator — DONE
- `Generator.features ProjectFeatures` + option `WithProjectFeatures(parser.ProjectFeatures)`
- Все флаги default false (zero value `ProjectFeatures`)
- Зависимости: T24a
- Ветка: `feat/genflags-wire`

#### T24d — `USE_UTC_FOR_DATE_TIME` flag — DONE
- Реализован вариант A: кастомный тип `UTCTime` (обёртка над `time.Time`)
- Когда флаг on, `typeMapper` мапит `date-time` строки в `model.UTCTime` (или `UTCTime` внутри model-пакета)
- `internal/generator/utc_time.go` генерирует `model/utc_time.gen.go` с `MarshalJSON`/`UnmarshalJSON`, принудительно вызывающими `.UTC()`
- Файл `utc_time.gen.go` генерируется только когда флаг включён
- Зависимости: T24c
- Ветка: `feat/genflag-utc`

#### T24e — `GOLANG_SERVER_BODY_REQUEST_NO_AUTO_DEFAULTS` flag — DONE
- Когда флаг off (по умолчанию), server request-decoder вызывает `req.Body.SetDefaults()` после `c.Bind(req)`.
- Когда флаг on — вызов `SetDefaults()` не генерируется.
- Решение принимается на этапе codegen (нет runtime-проверки флага в сгенерированном коде).
- `shouldCallSetDefaults(op)` проверяет: body-схема существует (через `resolveBodySchema` — $ref lookup или inline), является object, имеет defaults в Request-фильтре (split-aware).
- Зависимости: T24c, T25a
- Ветка: `feat/genflag-no-defaults`

#### T24f — `GOLANG_SPLIT_REQUEST_RESPONSE` flag — DONE
- Когда on, генерируются раздельные `<Name>Request` и `<Name>Response` модели
- Request: без `readOnly`, с `writeOnly`; Response: без `writeOnly`, с `readOnly`, без defaults
- `typeMapper` с режимами `modeRequest`/`modeResponse` (см. `internal/generator/constants.go`, `type.go`)
- `computeSplittable` (`internal/generator/generator.go`) сужает split для composites:
  схемы, на которые ссылаются `oneOf`/`anyOf`/`allOf`/`items`/`additionalProperties`, исключаются
  (эти контексты рендерятся с `mode==""`, splittable-ссылка породила бы несуществующий идентификатор)
- Зависимости: T24c
- Ветка: `feat/genflag-split`

#### T24g — `USE_REQUIRED_V2` flag — DONE
- Парсер читает `x-request-required` / `x-response-required` расширения (list of strings на уровне schema object) через `readRequiredExtension`.
- `Schema.RequestRequired`/`ResponseRequired` ([]string) + `Property.RequestRequired`/`ResponseRequired` (bool) поля.
- `Generator.requiredForMode(p, mode)`:
  - v2 off → `p.Required` (стандартный OAS)
  - v2 on + modeRequest → `p.RequestRequired`
  - v2 on + modeResponse → `p.ResponseRequired`
  - v2 on + моно (mode=="") → если поле в любом x-* списке → `p.RequestRequired && p.ResponseRequired`, иначе fallback на `p.Required`
- `fieldIsOptional` сигнатура изменена на `(required bool, fieldType string)` — принимает уже вычисленный required.
- Зависимость `dependsOn: GOLANG_SPLIT_REQUEST_RESPONSE: true` — валидируется лоадером.
- Тесты: 3 parser + 4 generator (включая end-to-end compile-тест `TestGenerate_UseRequiredV2_Compiles`).
- Зависимости: T24c, T24f
- Ветка: `feat/genflag-required-v2`

### T25: URLForm, WithDefaults, Update-схемы — DONE (T25a–T25c)

#### T25a — WithDefaults: `SetDefaults()` методы — DONE
- Генерация `func (m *<Name>) SetDefaults()` для schema-struct'ов — заполняет поля из `default` из spec.
- Прямой codegen (НЕ visitor pattern) — ~210 строк в `internal/generator/set_defaults.go`.
- Покрытые типы: string, integer (int/int32/int64), number (float32/float64), boolean, enum (константное имя).
- Optional pointer-поля: `if m.Field == nil { v := <literal>; m.Field = &v }`.
- Required value-поля: `if m.Field == <zero> { m.Field = <literal> }`.
- `date-time`/`date`/`binary` форматы — skip (TODO: `time.Parse`).
- Nested object `$ref`: рекурсивный `m.Inner.SetDefaults()` (required) / `if m.Inner != nil { m.Inner.SetDefaults() }` (optional). Циклические $ref разрываются visited-set.
- Split-aware (T24f): `filteredSchemaHasDefaults` с keep-фильтром — Request-вариант исключает readOnly defaults.
- Compile-тест `TestGenerate_SetDefaults_Compiles` запускает `go build` на сгенерированном коде (проверка типов, не только синтаксис).
- Отложено: array items, oneOf/anyOf/allOf variants, allOf flattened fields, date-time/date/binary defaults.
- Зависимости: нет
- Ветка: `feat/gen-defaults`

#### T25b — Update-схемы: `Update<Name>` struct для PATCH — DONE (разбит на T25b.1–T25b.4)

Общая спецификация: `type Update<Name> struct { Field *T ... }` — все поля `*T` (pointer, даже required), без defaults, без validation, JSON-тег `omitempty` для всех. Генерируется только для схем, участвующих в PATCH/PUT request body.

##### T25b.1 — Parser: `IsUsedInUpdate` flag + trigger
- Добавить `IsUsedInUpdate bool` на `Schema` и `Property` в `internal/parser/document.go`.
- Marker-проход (аналог `update_marker.go`): обойти body всех operations, пометить schema `IsUsedInUpdate=true` если operation — PUT или PATCH (эвристика, т.к. `x-*` расширений пока нет).
- Поля помечать поштучно: readOnly пропускаются, immutable пропускаются (кроме `name`).
- Тесты: parser-тест с PUT-операцией → schema помечена.
- Зависимости: нет
- Ветка: `feat/gen-update-marker`

##### T25b.2 — `renderUpdateStruct` + typeMapper mode
- Новый файл `internal/generator/update_schema.go`: `renderUpdateStruct(w, sh, m, name)` рендерит `type Update<Name> struct { ... }`.
- Все поля принудительно `*T` (даже required, даже примитивы). Без defaults, без validation.
- JSON-теги: `json:"<name>,omitempty" yaml:"<name>,omitempty"`.
- Переиспользовать `renderField`-паттерн, но с принудительным pointer.
- Решение: `*T` (по спеке T25b), НЕ `Optional[T]` — проще, без зависимости от common-пакета.
- `computeUpdatable` (аналог `computeSplittable`) — precompute-набор схем с `IsUsedInUpdate=true`.
- Тесты: генерация `Update<Name>` для object-схемы, проверка что все поля `*T`.
- Зависимости: T25b.1
- Ветка: `feat/gen-update-struct`

##### T25b.3 — Getter-методы `Get<Field>() (*T, bool)`
- Новый файл `internal/generator/update_getters.go`: для каждого поля рендерит:
  ```go
  func (m *Update<Name>) Get<Field>() (*T, bool) {
      if m.Field != nil {
          return m.Field, true
      }
      return nil, false
  }
  ```
- Простой проход по properties.
- Тесты: проверка что getter-методы генерируются для всех полей.
- Зависимости: T25b.2
- Ветка: `feat/gen-update-getters`

##### T25b.4 — Интеграция в writeSchemaFiles + тесты + golden
- В `generator.go:writeSchemaFiles` добавить условный вызов `g.updateSchemaFile(sh)` если `sh.IsUsedInUpdate`.
- Compile-тест `TestGenerate_UpdateSchemas_Compiles` — `go build` на сгенерированном коде.
- Golden-тест: добавить testdata с PUT-операцией, update-schema в golden.
- Опционально: `AsUpdateModel()` метод на исходной модели (мост Create↔Update) — если понадобится, отдельная подзадача T25b.5.
- Зависимости: T25b.2, T25b.3
- Ветка: `feat/gen-update-integration`

#### T25c — URLForm: `MarshalURLForm`/`UnmarshalURLForm` — DONE (разбит на T25c.1–T25c.3)

Общая спецификация: для schema в `application/x-www-form-urlencoded` request body генерируются методы `MarshalURLForm() (url.Values, error)` и `UnmarshalURLForm(form url.Values) error`. Поддержка только примитивных полей (string/integer/number/boolean). Arrays/maps/$ref → `return error` в сгенерированном коде.

##### T25c.1 — `MarshalURLForm` для object-схем
- Новый файл `internal/generator/url_form_methods.go`: `renderMarshalURLForm(w, sh, m, name)`.
- `func (m <Name>) MarshalURLForm() (url.Values, error)` — создаёт `url.Values`, для каждого поля: `if m.Field != nil { values.Set("<name>", <converter>(m.Field)) }`.
- Converter: string → direct, integer/number → `fmt.Sprint`, bool → `strconv.FormatBool`, time → `.Format(...)` (с UTC если флаг on).
- Триггер: схема referenced из `RequestBody.Content["application/x-www-form-urlencoded"]` любой операции.
- `schemeHasURLFormat(sh, doc)` helper.
- Edge cases: arrays/maps/refs → `return nil, fmt.Errorf("not supported")` в сгенерированном коде.
- Тесты: генерация MarshalURLForm для object с string/int/bool полями.
- Зависимости: нет
- Ветка: `feat/gen-urlform-marshal`

##### T25c.2 — `UnmarshalURLForm` + string-decoder
- Тот же файл: `renderUnmarshalURLForm(w, sh, m, name)`.
- `func (m *<Name>) UnmarshalURLForm(form url.Values) error` — для каждого поля: `if form.Has("<name>") { tmp := <decoder>(form.Get("<name>")); m.Field = &tmp }` (optional) или `m.Field = <decoded>` (required).
- Decoder: string → direct, integer → `strconv.Atoi`, bool → `strconv.ParseBool`, etc. С error propagation.
- Тесты: генерация UnmarshalURLForm, проверка decoder-выражений.
- Зависимости: T25c.1
- Ветка: `feat/gen-urlform-unmarshal`

##### T25c.3 — Интеграция + httpclient/server wire-up + тесты
- В `writeSchemaFiles` вызвать `urlFormMethodsFile(sh)` если `schemeHasURLFormat(sh, doc)`.
- В `impl_client.go`: если body content-type `application/x-www-form-urlencoded`, рендерить `url.Values`-encode + `Content-Type: application/x-www-form-urlencoded` вместо `json.Marshal`.
- В `impl_server.go`: если body content-type form-urlencoded, декодировать через `UnmarshalURLForm` вместо `json.Unmarshal`.
- Compile-тест `TestGenerate_URLForm_Compiles`.
- Golden-тест: testdata с form-encoded операцией.
- Зависимости: T25c.1, T25c.2
- Ветка: `feat/gen-urlform-integration`

### Typyped response headers (вне исходной нумерации) — DONE
- `internal/generator/response_headers.go` — генерация `<Name><Code>PayloadWithHeaders` struct
- Поля: `Payload` (body, типизированный через `typeMapper` в `modeResponse`) + типизированные header-поля
- Header-типы (`headerGoBaseType`): string, int, int32, int64, float32, float64, bool
- `MarshalJSON()` маршалит только `Payload` (headers не входят в JSON-body)
- `Headers() map[string]string` — для server-side установки заголовков в HTTP-ответ
- Client decoder (`internal/generator/impl_client.go`) использует `strconv` для не-string headers с error propagation
- Server impl (`internal/generator/impl_server.go`) — 4-вариантная логика (headers × schema):
  NoContent для пустых body, JSON для body, `renderHeaderSet` для заголовков
- Ветка: см. git log

### Глубокий бэклог (без детализации)
- `ServerAuditData` + `x-audit-data` + audit-data схемы — комплаенс-логирование: для каждой операции описывается audit-схема (что логировать при вызове — кто, что, с какими параметрами, результат), серверный интерфейс получает методы `ServerAuditData`. Не часть стандартного OpenAPI.
- `GenerateConverters` — автоконвертеры `func <Name>RequestToResponse(req) (resp, error)` между split-моделями; имеет смысл только при включённом `GOLANG_SPLIT_REQUEST_RESPONSE`.
- `USE_REQUIRED_V2` (T24g) — генераторная поддержка `x-request-required`/`x-response-required` расширений.
- Кастомные `x-*` расширения и фильтрация в opensourceyaml.

**Отдельные компоненты (не относятся к Go-генератору):**
- Validator TUI (`cmd/validator`, `bubbletea`) — terminal-инструмент для работы со списками замечаний валидации (код-ревью/линтинг/security-сканы): загрузка, фильтр по пути, привязка Jira-тикета, перемещение в исключения, сохранение.
- Terraform-provider
- TypeScript-генератор
- `graphgen` (`cmd/tools/graphgen`)

## Правила работы

1. Каждая задача = отдельная ветка `feat/<slug>`.
2. Ветка стартует от актуального `main`.
3. До merge: сборка `go build ./...`, тесты `go test ./...`, линтер.
4. Одна задача — один PR/MR (пользователь делает merge сам).
5. Коммиты в произвольной форме, но осмысленные.
6. Если задача вскрыла новый вспомогательный пакет — добавить реализацию в `internal/` отдельной под-задачей.