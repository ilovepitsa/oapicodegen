# oapigenerator

Go-генератор серверного и клиентского кода из OpenAPI 3.x спецификаций.
Проект — облегчённая альтернатива `mws/api/cmd/mwsapigen`: только Go-генерация,
без Kotlin/TypeScript/Terraform, со собственными библиотеками-заменами `platform-go`
в `internal/`.

## Статус

Проект на ранней стадии. Поддерживается стандартный OpenAPI 3.x:
`paths`, `parameters`, `requestBody`, `responses`, `schemas` (`object`, `array`,
примитивы, `$ref`), `oneOf`/`anyOf`/`allOf`, `required`, `enum`, `format`,
`default`, `nullable`, `deprecated`, `additionalProperties: false`,
cookie-параметры.

Дополнительно реализовано через **generation flags** (см. раздел ниже):
- `GOLANG_SPLIT_REQUEST_RESPONSE` — раздельные `<Name>Request`/`<Name>Response` модели.
- `USE_UTC_FOR_DATE_TIME` — принудительная сериализация `time.Time` в UTC через кастомный тип `UTCTime`.
- `GOLANG_SERVER_BODY_REQUEST_NO_AUTO_DEFAULTS` — флаг заведён в конфиг, но генераторно пока
  не влияет (требует `SetDefaults()` из T25a — см. `TASKS.md`).
- `USE_REQUIRED_V2` — флаг заведён в конфиг и `ProjectFeatures`, генераторной поддержки пока нет.

Типизированные response headers: для ответов с `headers` генерируется
`<Name><Code>PayloadWithHeaders`-обёртка с body и типизированными header-полями
(string/int/int32/int64/float32/float64/bool).

Кастомные `x-*` валидации, audit-data, URL-form encoding и update-схемы
остаются в бэклоге — см. `TASKS.md`.

## Установка

```sh
git clone <repo-url> oapigenerator
cd oapigenerator
make build
```

Требуется Go 1.26+.

## Использование

```sh
# Сгенерировать Go-код из OpenAPI-спек в ./mws в ./go/mws
go run ./cmd/oapigen \
  -input ./mws \
  -output ./go/mws \
  -import-prefix nschugorev/oapigenerator/go/mws
```

Флаги:

| Флаг | По умолчанию | Назначение |
|------|--------------|------------|
| `-input` | — | Путь к OpenAPI 3.x spec-файлу (обязательный) |
| `-output` | — | Каталог для сгенерированного кода (обязательный, если не `-dry-run`) |
| `-import-prefix` | — | Go import-path префикс для пакетов (обязательный) |
| `-dry-run` | `false` | Парсить и генерировать без записи на FS |
| `-generation-flags-config-path` | — | Путь к глобальному `generation_flags.yaml` |
| `-project-flags-path` | — | Путь к per-project override (требует `-generation-flags-config-path`) |
| `-log-level` | `info` | debug\|info\|warn\|error\|fatal |
| `-log-format` | `console` | console\|json |
| `-log-development` | `false` | zap development mode (stacktraces, no sampling) |

## Generation flags

Generation flags настраивают поведение генератора через YAML-конфиг и опциональный
per-project override. Загружаются через `-generation-flags-config-path` (глобальный
`generation_flags.yaml`) и `-project-flags-path` (перекрытие для конкретного проекта).

Поддерживаемые флаги (имена совпадают с ключами в `generation_flags.yaml`):

| Флаг | Эффект когда `on` |
|------|-------------------|
| `GOLANG_SPLIT_REQUEST_RESPONSE` | Для object-схем генерируются раздельные `<Name>Request` (без `readOnly`, с `writeOnly`) и `<Name>Response` (без `writeOnly`, с `readOnly`, без defaults). Сплит сужен для composites (`oneOf`/`anyOf`/`allOf`/`items`/`additionalProperties`-ссылки исключаются). |
| `USE_UTC_FOR_DATE_TIME` | Для `date-time` полей генерируется тип `model.UTCTime` (обёртка над `time.Time`, принудительный `.UTC()` в marshal/unmarshal). |
| `GOLANG_SERVER_BODY_REQUEST_NO_AUTO_DEFAULTS` | Заведён в `ProjectFeatures`, но генераторно пока не влияет: требует `SetDefaults()` (T25a, бэклог). |
| `USE_REQUIRED_V2` | Заведён в `ProjectFeatures`; генераторной поддержки `x-request-required`/`x-response-required` пока нет. |

Формат `generation_flags.yaml` (одна запись на флаг):

```yaml
- name: GOLANG_SPLIT_REQUEST_RESPONSE
  description: "Split Request/Response models"
  enabled: true
  defaultValue: false
  targetValue: true
  affects: [golang]
  dependsOn: {}
```

Per-project override — простой YAML `flag-name: bool`:

```yaml
GOLANG_SPLIT_REQUEST_RESPONSE: true
USE_UTC_FOR_DATE_TIME: true
```

## Make-таргеты

```sh
make build         # go build ./...
make test          # go test ./...
make vet           # go vet ./...
make fmt           # gofmt -s -w .
make lint          # golangci-lint run (требует установленного golangci-lint)
make generate      # перегенерировать golden-файлы (petstore + minimal e2e) с -update
make e2e           # запустить e2e-тест генерации
make golden-check  # верифицировать актуальность golden-файлов (для CI)
make tidy          # go mod tidy
make cover         # текстовый отчёт покрытия
make cover-html    # HTML-отчёт покрытия
make clean         # удалить артефакты сборки
```

## Структура репозитория

```
oapigenerator/
├── cmd/oapigen/        # точка входа CLI (main.go, e2e-тесты)
├── internal/
│   ├── parser/         # чтение OpenAPI 3.x + generation_flags loader
│   ├── generator/      # ядро генератора (schema, client, server, http, mocks, sdk, response_headers, utc_time)
│   ├── codegen/        # абстракции вывода + gogen-рендер + configurator
│   ├── cli/logging/    # zap-логирование из флагов
│   ├── ptr/ must/ fs/  # мини-замены platform-go
│   └── golden/         # golden-тесты
├── testdata/           # эталонные OpenAPI-спеки и golden-файлы
├── TASKS.md            # план задач и карта замен platform-go
└── ARCHITECTURE.md     # архитектурный обзор
```

## Лицензия

(определить позже)
