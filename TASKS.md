# План задач: oapigenerator

Проект: Go-генератор из OpenAPI-спек, аналог `../api/cmd/mwsapigen`, **без Kotlin, без TypeScript, без Terraform-provider, без `mwsp` CLI**. Все замены `platform-go` — собственные библиотеки в `internal/`. Валидатор (TUI) — в бэклоге.

- **Go-модуль**: `nschugorev/oapigenerator` (временное имя, потом сменим)
- **Рабочий процесс**: одна задача → одна ветка `feat/...` → merge в `main`
- **Референс**: `/Users/n.shchugorev/projects/api/` (исходники `cmd/mwsapigen`, `cmd/validator`, `go/`)

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
- `GOLANG_SERVER_BODY_REQUEST_NO_AUTO_DEFAULTS` — флаг заведён в `ProjectFeatures` (T24e — partial, см. ниже)
- `USE_REQUIRED_V2` — флаг заведён в `ProjectFeatures`, генераторной поддержки пока нет (T24g — pending)

**Не поддерживается (бэклог):**
- Кастомные расширения `x-*` (`x-request-required`, `x-response-required`, `x-optional-response`, `x-validations`, `x-audit-data`, `x-mws-*` и пр.)
- Кастомные валидации (x-validations)
- `audit-data` схемы и связанный код
- URL-form encoding (`application/x-www-form-urlencoded`)
- Конвертёры между Request/Response-моделями (требуют split + T25)
- Update-схемы (`*_update`) — T25b
- `SetDefaults()` методы — T25a (блокирует полную реализацию T24e)

Это значит: задачи T13, T14, T16 (audit), T18 (HTTP server для audit) упрощаются или откладываются. См. раздел «Корректировки задач» ниже.

## Корректировки задач под первую итерацию

- **T13 Schema + JSON-методы** — DONE (без `x-*`-специфики и без `GenerateConverters`)
- **T14 URLForm, WithDefaults, Update-схемы** → **в бэклог** (требует URL-form encoding и update-схемы); см. T25a–T25c
- **T15 Client interfaces + sugar** — DONE (без x-validations)
- **T16 Server interfaces + audit** — DONE (без `ServerAuditData`/`audit_data`-схемы — в бэклоге)
- **T17 HTTP client** — DONE (без URL-form)
- **T18 HTTP server** — DONE (базовый роутинг, без audit-data; типизированные response headers — DONE)
- **T19 Mocks** — DONE
- **T20 SDK** — DONE
- **T21 cmdtreegenerator** → **глубокий бэклог** (не нужен в первой итерации; весь функционал завязан на `x-cli` расширения, CRUD-эвристики и multi-service parser — пересмотреть при появлении реального требования к CLI)
- **T22 opensourceyaml** → **глубокий бэклог** (не нужен в первой итерации; публикацией спеки управляет внешняя инфраструктура, не генератор)
- **T12 GenerationFlagsLoader** → **реализован как T24a** (загрузчик `internal/parser/generation_flags.go` + `generation_flags_loader.go`); глобальный `generation_flags.yaml` + per-project override
- **T23 `cmd/oapigen`** — DONE
- **T24 e2e-тесты** — DONE
- **T25 CI, линтеры** — частично: Makefile-таргеты есть; `.gitlab-ci.yml`/`.golangci.yml` см. репозиторий
- **Типизированные response headers** (новая задача, вне исходной нумерации) — DONE: `internal/generator/response_headers.go`, `PayloadWithHeaders`-паттерн, типизированные header-поля (string/int/int32/int64/float32/float64/bool), client-decoder с `strconv` + error propagation, server-side `Headers()` метод

## Карта замен `platform-go`

| Группа | Пакет platform-go | Замена в проекте | Задача |
|---|---|---|---|
| Codegen-ядро | `pkg/codegen` | `internal/codegen` | T6 |
| Codegen Go-рендер | `pkg/codegen/gogen` | `internal/codegen/gogen` | T7 |
| Codegen-конфигуратор | `pkg/codegen/configurator` | `internal/codegen/configurator` | T8 |
| FS | `pkg/fs` | `internal/fs` | T5 |
| CLI-логирование | `pkg/cli/logging` | `internal/cli/logging` | T9 |
| Утилиты | `pkg/ptr` | `internal/ptr` | T3 |
| Утилиты | `pkg/must` | `internal/must` | T4 |
| Тесты | `pkg/golden` | `internal/golden` | T10 |
| Exec | `pkg/exec` | — (только для validator, бэклог) | — |
| HTTP-инфра | `pkg/http/*` | — (нужна рантайму сгенерированного кода, не генератору) | — |
| Прочие | `cmdtool`, `rootcmd`, `app`, `zapctx`, `zaputil`, `env`, `os`, `consterr`, `cmdtest`, `ztest`, `encryption`, `vault`, `configloader`, `cli/browser` | по мере необходимости | — |

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

## Этап 1 — внутренние замены platform-go

### T3 — `internal/ptr`
- API: `Ptr[T](v) *T`, `From[T](*T) T` с zero-value fallback, `Or`, `Equal`, и т.д.
- Замена: `git.mws-team.ru/mws/devp/platform-go/pkg/ptr`
- Тесты + бенч
- Ветка: `feat/internal-ptr`
- Зависимости: T1

### T4 — `internal/must`
- API: `Must(err)`, `MustGet[T](v T, err error) T`, `MustNoError(err)` и пр.
- Замена: `platform-go/pkg/must`
- Тесты
- Ветка: `feat/internal-must`
- Зависимости: T1

### T5 — `internal/fs`
- API: `RealFS`, `NewRecommendedReal(opts...)`, `WithBaseDir`, интерфейс FS (read/write, MkdirAll, Stat)
- Замена: `platform-go/pkg/fs`
- Тесты на `testing/fstest`
- Ветка: `feat/internal-fs`
- Зависимости: T1

### T6 — `internal/codegen` — ядро
- API: `File`, `FileWriter` (`WriteFile(name, File) error`, `Close() error`), `BufferWriter`, `WithPath(fw, ...)`, `noopFileWriter`, `NewBufferWriter`
- Замена: `platform-go/pkg/codegen` (без gogen)
- Тесты
- Ветка: `feat/internal-codegen-core`
- Зависимости: T1

### T7 — `internal/codegen/gogen` — FileFactory и рендер Go-файлов
- API: `FileFactory`, `NewFileFactory(toolName)`, `Create(File) *bytes.Buffer` / `io.Reader`, gofmt-рендер
- Замена: `platform-go/pkg/codegen/gogen`
- Тесты на рендер
- Ветка: `feat/internal-codegen-gogen`
- Зависимости: T6

### T8 — `internal/codegen/configurator` — FileWriter из флагов
- API: `NewFileWriterConfiguratorFromFlags(*flag.FlagSet)`, `Create(log, output) FileWriter`
- Замена: `platform-go/pkg/codegen/configurator`
- Тесты
- Ветка: `feat/internal-codegen-configurator`
- Зависимости: T5, T6

### T9 — `internal/cli/logging` — zap-логирование из флагов
- API: `NewLoggerConfiguratorFromFlags(*flag.FlagSet)`, `Create() *zap.Logger`
- Замена: `platform-go/pkg/cli/logging`
- Тесты
- Ветка: `feat/internal-cli-logging`
- Зависимости: T1

### T10 — `internal/golden` — golden-тесты
- API: `Equals(t, got, want string)`, `Update(path, content)`, флаг `-update`
- Замена: `platform-go/pkg/golden`
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

> Все генераторы портируются из `cmd/mwsapigen/internal/generator` без kotlin/terraform-специфики.

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
- ~~Порт `cmd/mwsapigen/internal/cmdtreegenerator`~~
- Причина: оригинал (4841 строка) завязан на `x-cli` расширения, CRUD-эвристики, multi-service parser (`parser.Project`/`Service`/`Method`), кастомный CLI-фреймворк `cli.Command[T]` с profile config — ничего из этого у нас нет и не планируется в первой итерации.
- Пересмотреть при появлении реального требования к auto-generated CLI.
- Ветка: ~~`feat/cmdtree`~~ (не создаётся)
- ~~Зависимости: T11, T13~~

### T22 — opensourceyaml — публичный OpenAPI-spec → ГЛУБОКИЙ БЭКЛОГ
- ~~Порт `cmd/mwsapigen/internal/opensourceyaml`~~
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

#### T24e — `GOLANG_SERVER_BODY_REQUEST_NO_AUTO_DEFAULTS` flag — PARTIAL (blocked на T25a)
- Флаг заведён в `ProjectFeatures` и проходит через CLI/logging, но **не влияет на генератор**:
  генератор не вызывает `SetDefaults()` в server request-decoder, потому что сам `SetDefaults()`
  (T25a) ещё не реализован. Нечего отключать.
- Статус: partial — конфиг-инфраструктура готова, генераторная логика ожидает T25a.
- Зависимости: T24c, T25a (блокер)
- Ветка: `feat/genflag-no-defaults` (генераторная часть отложена)

#### T24f — `GOLANG_SPLIT_REQUEST_RESPONSE` flag — DONE
- Когда on, генерируются раздельные `<Name>Request` и `<Name>Response` модели
- Request: без `readOnly`, с `writeOnly`; Response: без `writeOnly`, с `readOnly`, без defaults
- `typeMapper` с режимами `modeRequest`/`modeResponse` (см. `internal/generator/constants.go`, `type.go`)
- `computeSplittable` (`internal/generator/generator.go`) сужает split для composites:
  схемы, на которые ссылаются `oneOf`/`anyOf`/`allOf`/`items`/`additionalProperties`, исключаются
  (эти контексты рендерятся с `mode==""`, splittable-ссылка породила бы несуществующий идентификатор)
- Зависимости: T24c
- Ветка: `feat/genflag-split`

#### T24g — `USE_REQUIRED_V2` flag — PENDING
- Флаг заведён в `ProjectFeatures` и резолвится лоадером, но **генераторной поддержки нет**:
  парсер пока не читает `x-request-required`/`x-response-required`, генератор не использует `UseRequiredV2`.
- Поддержка `x-request-required` / `x-response-required` list-атрибутов — будущая работа.
- Зависимости: T24c, T24f
- Ветка: `feat/genflag-required-v2`

### T25: URLForm, WithDefaults, Update-схемы — разбит на T25a–T25c

#### T25a — WithDefaults: `SetDefaults()` методы
- Генерация `<Name>SetDefaults()` для schema-struct'ов — заполняет поля из `default` из spec
- `default_value_visitor.go` + `set_defaults_visitor.go` + `type_default_value_visitor.go` + `schema_with_defaults.go` (~740 строк)
- Зависимости: нет
- Ветка: `feat/gen-defaults`

#### T25b — Update-схемы: `Update<Name>` struct для PATCH
- Все поля `*T` (pointer), без defaults, без validation
- `update_schema.go` + `update_get_name.go` + `update_set_name.go` + `update_model_getter.go` + `test_update_json_methods.go` (~1370 строк)
- Getter-методы `Get<Field>() (*T, bool)`
- Зависимости: нет
- Ветка: `feat/gen-update-schemas`

#### T25c — URLForm: `MarshalURLForm`/`UnmarshalURLForm`
- Для schema в `application/x-www-form-urlencoded` request body
- `url_form_methods.go` (~476 строк)
- Требует parser-поддержки form-urlencoded content-type
- Зависимости: нет
- Ветка: `feat/gen-urlform`

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
- `ServerAuditData` + `x-audit-data` + audit-data схемы
- `GenerateConverters` (split Request/Response моделей — пересекается с T24f)
- `x-validations`
- Кастомные `x-mws-*` расширения и фильтрация в opensourceyaml

**Отдельные компоненты:**
- Validator TUI (`cmd/validator`, `bubbletea`)
- Terraform-provider (`terraform-provider-mwsp`)
- `mwsp` CLI
- TypeScript-генератор (`mws-typescript-api-generator`)
- `graphgen` (`cmd/tools/graphgen`)

## Правила работы

1. Каждая задача = отдельная ветка `feat/<slug>`.
2. Ветка стартует от актуального `main`.
3. До merge: сборка `go build ./...`, тесты `go test ./...`, линтер.
4. Одна задача — один PR/MR (пользователь делает merge сам).
5. Коммиты в произвольной форме, но осмысленные.
6. Если задача вскрыла новый пакет `platform-go` — добавить替换у в `internal/` отдельной под-задачей.