# Архитектура oapigenerator

## Назначение

Генератор читает OpenAPI 3.x спецификации из каталога `input` и производит
Go-пакеты: модели, клиентские/серверные интерфейсы, HTTP-клиент и HTTP-сервер,
моки, SDK. Выходной каталог и префикс импорта задаются
флагами `cmd/oapigen`.

Поток данных однонаправленный:

```
OpenAPI spec (YAML)
      │
      ▼
┌─────────────┐     ┌──────────────┐     ┌──────────────────┐
│  parser     │ ──▶ │ generator    │ ──▶ │ codegen/gogen    │
│ (internal/) │     │ (internal/)  │     │ + FileWriter      │
└─────────────┘     └──────────────┘     └──────────────────┘
                                                │
                                                ▼
                                       Go-исходники на FS
```

## Слои

### `internal/parser` (T11)

Читает OpenAPI через [`github.com/pb33f/libopenapi`](https://github.com/pb33f/libopenapi),
строит промежуточную модель: `ResourcesSet`, `ProjectSet`, `Project`, `Schema`,
`Paths`, `Service`, `Method`, `Imports`.

Никаких знаний о Go-генерации здесь нет — только доменная модель OpenAPI.

Дополнительно пакет содержит **generation flags loader** (`generation_flags.go` +
`generation_flags_loader.go`): грузит глобальный `generation_flags.yaml` и
per-project override, резолвит `ProjectFeatures` для генератора. Имена флагов,
`GenerationFlagConfig`, `ProjectFeature`, `ProjectFeatures` — см. `internal/parser/generation_flags.go`.

### `internal/generator` (T13, T15–T20)

Превращает модель из `parser` в `codegen.File` — абстракцию рендеримого файла.
Подпакеты по типу артефакта:

| Артефакт | Задача | Что генерирует |
|----------|--------|----------------|
| Schema | T13 | `model/*.gen.go` — типы + JSON-маршалинг |
| UTCTime | T24d | `model/utc_time.gen.go` — кастомный тип `UTCTime` (только когда `USE_UTC_FOR_DATE_TIME` on) |
| Response headers | (вне нумерации) | `<Name><Code>PayloadWithHeaders` struct в model/interfaces — body + типизированные header-поля, `MarshalJSON`, `Headers()` |
| Client interfaces | T15 | `interfaces/client/*.gen.go` |
| Server interfaces | T16 | `interfaces/server/*.gen.go` |
| HTTP client | T17 | `impl/httpclient/*.gen.go` |
| HTTP server | T18 | `impl/echoserver/*.gen.go` |
| Mocks | T19 | `impl/mocks/{client,server}/*.gen.go` |
| SDK | T20 | `sdk/*.gen.go` |

`Generator` конфигурируется через `Option`-ы (`WithModulePath`, `WithProjectFeatures`).
Поле `features parser.ProjectFeatures` хранит резолвнутые generation flags;
`splittable map[string]bool` (заполняется при `SplitRequestResponse=true`) —
имена object-схем, рендерящихся как `<Name>Request` + `<Name>Response`.

### `internal/codegen` (T6–T8)

Абстракции вывода, не зависящие от языка:

- **`codegen`** (T6) — интерфейсы `File`, `FileWriter` (`WriteFile`, `Close`),
  `BufferWriter`, `WithPath`, `noopFileWriter`.
- **`codegen/gogen`** (T7) — `FileFactory` и рендер Go-файлов с gofmt.
- **`codegen/configurator`** (T8) — сборка `FileWriter` из CLI-флагов
  (dry-run, output dir).

### `internal/cmdtreegenerator` (T21) → глубокий бэклог

Генерация дерева команд CLI по пути `paths`. В первой итерации **не
реализуется**: оригинал в mwsapi завязан на `x-cli` расширения, CRUD-эвристики
и multi-service parser, которых у нас нет. Пересмотреть при появлении
реального требования к auto-generated CLI.

### `internal/opensourceyaml` (T22) → глубокий бэклог

Сборка публичного OpenAPI-спека из внутренних. В первой итерации **не
реализуется**: публикацией спеки управляет внешняя инфраструктура, а не
генератор. Пересмотреть при появлении явного требования.

### `cmd/oapigen`

Точка входа. Парсит флаги (`-input`, `-output`, `-import-prefix`, `-dry-run`,
`-generation-flags-config-path`, `-project-flags-path`, `-log-*`),
инициализирует logger и FileWriter, связывает `parser → generator`.
При заданном `-generation-flags-config-path` грузит флаги и прокидывает их в
`Generate()` через `generator.WithProjectFeatures`.

## Замены `platform-go`

Все утилиты и инфраструктурные пакеты, которые в оригинальном `mws/api`
тянулись из `git.mws-team.ru/mws/devp/platform-go`, здесь реализованы
собственными силами в `internal/`:

| `platform-go/pkg/*` | Замена | Задача |
|---|---|---|
| `ptr` | `internal/ptr` | T3 |
| `must` | `internal/must` | T4 |
| `fs` | `internal/fs` | T5 |
| `codegen` | `internal/codegen` | T6 |
| `codegen/gogen` | `internal/codegen/gogen` | T7 |
| `codegen/configurator` | `internal/codegen/configurator` | T8 |
| `cli/logging` | `internal/cli/logging` | T9 |
| `golden` | `internal/golden` | T10 |

Пакеты `exec`, `http/*`, `cmdtool`, `rootcmd`, `app`, `zapctx`, `zaputil`,
`env`, `os`, `consterr`, `cmdtest`, `ztest`, `encryption`, `vault`,
`configloader`, `cli/browser` не требуются для самого генератора в первой
итерации; добавляются по мере необходимости в соответствующих задачах.

## Границы первой итерации

Что **входит**:

- Стандартные конструкции OpenAPI 3.x (`oneOf`/`anyOf`/`allOf` (включая single-non-object `allOf` → alias), `$ref`, `enum` (с дедупликацией), `required`, `format`, `default`, `nullable`, `deprecated`).
- `additionalProperties: false` → `struct{}`.
- Cookie-параметры.
- Типизированные response headers (`PayloadWithHeaders`-паттерн — см. ниже).
- Generation flags (см. раздел ниже) — split Request/Response, UTC datetime
  поддерживаются; `ServerNoAutoDefaults` и `UseRequiredV2` заведены в конфиг,
  но генераторно не влияют (см. `TASKS.md` T24e/T24g).
- JSON-маршалинг.
- HTTP client/server (vanilla) для стандартных методов.
- SDK, моки.

Что **не входит** (см. бэклог в `TASKS.md`):

- Кастомные `x-*` расширения и валидации.
- Audit-data схемы и код.
- URL-form encoding и update-схемы (T25b/T25c).
- `SetDefaults()` методы (T25a) — блокирует полную реализацию `ServerNoAutoDefaults`.
- Конвертёры между Request/Response-моделями.

## Generation flags

Generation flags настраивают поведение генератора через YAML-конфиг
(`generation_flags.yaml`) и опциональный per-project override. Лоадер живёт в
`internal/parser` (`generation_flags.go`, `generation_flags_loader.go`).

`ProjectFeatures` struct (`internal/parser/generation_flags.go`) — резолвнутый
набор флагов для проекта:

```go
type ProjectFeatures struct {
    ServerNoAutoDefaults ProjectFeature
    SplitRequestResponse ProjectFeature
    UseRequiredV2        ProjectFeature
    UseUTCForDateTime    ProjectFeature
}
```

Прокидывается в `Generator` через `WithProjectFeatures(parser.ProjectFeatures)`.
Без option все флаги false.

| Флаг | Эффект когда `on` |
|------|-------------------|
| `GOLANG_SPLIT_REQUEST_RESPONSE` | `<Name>Request`/`<Name>Response` модели. `typeMapper` с `modeRequest`/`modeResponse`. `computeSplittable` сужает split для composite-ссылок. |
| `USE_UTC_FOR_DATE_TIME` | `date-time` → `model.UTCTime`; генерируется `model/utc_time.gen.go`. |
| `GOLANG_SERVER_BODY_REQUEST_NO_AUTO_DEFAULTS` | Заведён в `ProjectFeatures`; генераторно не влияет (требует T25a). |
| `USE_REQUIRED_V2` | Заведён в `ProjectFeatures`; генераторной поддержки `x-request-required`/`x-response-required` пока нет. |

## PayloadWithHeaders паттерн

Для ответов с `headers` генератор (`internal/generator/response_headers.go`)
создаёт тип `<OperationName>Response<Code>PayloadWithHeaders`:

```go
type ListPetsResponse200PayloadWithHeaders struct {
    Payload *PetList           // body, типизирован через typeMapper в modeResponse
    XRateLimit int             // типизированный header (string/int/int32/int64/float32/float64/bool)
}

func (m ListPetsResponse200PayloadWithHeaders) MarshalJSON() ([]byte, error) {
    return json.Marshal(m.Payload)            // маршалит только body
}

func (m ListPetsResponse200PayloadWithHeaders) Headers() map[string]string {
    return map[string]string{                 // для server-side установки заголовков
        "X-Rate-Limit": fmt.Sprintf("%v", m.XRateLimit),
    }
}
```

- Client decoder (`impl_client.go`) читает заголовки через `resp.Header.Get`,
  конвертирует не-string типы через `strconv` с error propagation.
- Server impl (`impl_server.go`) — 4-вариантная логика (headers × schema):
  NoContent для пустых body, JSON для body, `renderHeaderSet` для заголовков.
- Header-типы определяются в `headerGoBaseType` по `type`/`format` схемы заголовка.

## Принципы

1. **Один пакет = одна ответственность.** `parser` не знает про Go, `generator`
   не читает файлы, `codegen` не знает про OpenAPI.
2. **Стандартный OpenAPI сначала.** Любое `x-*` расширение — отдельная задача
   во второй итерации.
3. **Собственные замены вместо `platform-go`.** Внешние зависимости минимальны:
   `libopenapi`, `zap`, стандартная библиотека Go.
4. **Ветвление по задачам.** Одна задача → одна ветка `feat/...` → merge в
   `main` (см. `TASKS.md`).
