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

В первой итерации парсер игнорирует все `x-*` расширения с warning в лог.
Никаких знаний о Go-генерации здесь нет — только доменная модель OpenAPI.

### `internal/generator` (T13, T15–T20)

Превращает модель из `parser` в `codegen.File` — абстракцию рендеримого файла.
Подпакеты по типу артефакта:

| Артефакт | Задача | Что генерирует |
|----------|--------|----------------|
| Schema | T13 | `model/*.gen.go` — типы + JSON-маршалинг |
| Client interfaces | T15 | `interfaces/client/*.gen.go` |
| Server interfaces | T16 | `interfaces/server/*.gen.go` |
| HTTP client | T17 | `impl/httpclient/*.gen.go` |
| HTTP server | T18 | `impl/echoserver/*.gen.go` |
| Mocks | T19 | `impl/mocks/{client,server}/*.gen.go` |
| SDK | T20 | `sdk/*.gen.go` |

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

### `cmd/oapigen` (T23)

Точка входа. Парсит флаги, инициализирует logger и FileWriter, связывает
`parser → generator`.

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

- Стандартные конструкции OpenAPI 3.x (`oneOf`/`anyOf`/`allOf`, `$ref`, `enum`,
  `required`, `format`, `default`, `nullable`, `deprecated`).
- Мономодель (без split Request/Response).
- JSON-маршалинг.
- HTTP client/server (vanilla) для стандартных методов.
- SDK, моки.

Что **не входит** (см. бэклог в `TASKS.md`):

- Кастомные `x-*` расширения и валидации.
- Split Request/Response моделей и конвертёры.
- Audit-data схемы и код.
- URL-form encoding и update-схемы.
- Generation flags (заведены в off/default).

## Принципы

1. **Один пакет = одна ответственность.** `parser` не знает про Go, `generator`
   не читает файлы, `codegen` не знает про OpenAPI.
2. **Стандартный OpenAPI сначала.** Любое `x-*` расширение — отдельная задача
   во второй итерации.
3. **Собственные замены вместо `platform-go`.** Внешние зависимости минимальны:
   `libopenapi`, `zap`, стандартная библиотека Go.
4. **Ветвление по задачам.** Одна задача → одна ветка `feat/...` → merge в
   `main` (см. `TASKS.md`).
