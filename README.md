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

`x-validations`: декларативные правила валидации (простые `>N`/`Size >=N`/
именованные `pkg.Name`/маркер `Immutable`) → сгенерированный `ValidateOwn(reg)`
+ `model.ExpectedValidatorNames()` для startup-check'а. Runtime-walker —
`pkg/validator.Validate`. См. раздел [x-validations](#x-validations) ниже.

Update-схемы (`Update<Name>` для PUT/PATCH request body): все поля
оборачиваются в `optional.Optional[T]` (трёх-state PATCH-семантика),
`Immutable`-поля пропускаются (кроме `name`), генерируются getter'ы
`Get<Field>() (*T, bool)` и `ValidateOwn` на property-level правилах.

audit-data, URL-form encoding остаются в бэклоге — см. `TASKS.md`.

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

## x-validations

`x-validations` — расширение OpenAPI-схемы для декларативного описания
валидационных правил. Генератор переводит их в метод `ValidateOwn(reg)` на
каждой структуре с правилами; runtime-валидация выполняет
`pkg/validator.Validate` — reflection-walker, обходит struct-дерево и
вызывает `ValidateOwn` на каждой структуре, реализующей `Validatable`.

### Синтаксис правил

`x-validations` — список строк. Каждая строка — либо простое правило,
либо ссылка на именованный валидатор, либо маркер `Immutable`.

```yaml
properties:
  age:
    type: integer
    x-validations: [">0", "app.PositiveInt"]
  name:
    type: string
    x-validations: ["Size >=1", "Size <=100"]
  email:
    type: string
    x-validations: ["app.EmailFormat"]
  status:
    type: string
    x-validations: ["Immutable"]
```

| Правило | Семантика | Пример |
|---------|-----------|--------|
| `>N` `>=N` `<N` `<=N` `==N` `!=N` | Числовое сравнение значения поля | `>0`, `<=100` |
| `Size >N` `Size >=N` ... `Length ...` | `len()` поля (строки, slice, map) | `Size >=1`, `Length <=50` |
| `pkg.Name` | Именованный валидатор. `pkg.Name` — dotted-идентификатор (минимум одна точка) | `app.EmailFormat` |
| `Immutable` | Маркер update-marker'а: поле пропускается в `Update<Name>` (кроме `name`). Не валидация. | `Immutable` |

Правила на уровне свойства (`properties.<name>.x-validations`) применяются
к значению поля. Правила на уровне схемы (`x-validations` рядом с
`type: object`) — только именованные валидаторы, применяемые ко всей
структуре (cross-field).

### Сгенерированный код

Для каждой схемы с хотя бы одним правилом генерируется:

```go
func (x Pet) ValidateOwn(reg *validator.Registry) error {
    // Простые правила — inline if-проверки с инвертированным оператором.
    if x.Age <= 0 {
        return fmt.Errorf("field Age: must be > 0")
    }
    // Именованные property-валидаторы — reg.Get + Validate + wrap пути.
    v, ok := reg.Get("app.NonEmptyName")
    if !ok {
        return fmt.Errorf("validator %q not registered", "app.NonEmptyName")
    }
    if err := v.Validate(x.Name); err != nil {
        return fmt.Errorf("field Name: %w", err)
    }
    // Schema-level валидатор — вызывается на receiver'е, без обёртки пути.
    // ...
    return nil
}
```

Для `Update<Name>` (PUT/PATCH body) генерируется отдельный `ValidateOwn`
с `.IsSet() && !.IsNil()` guard'ами и `.Value()` accessor'ами —
schema-level валидаторы пропускаются (они зарегистрированы под основной
тип, а не под Update).

### ExpectedValidatorNames

Если в спеке есть хотя бы один named-валидатор, генератор создаёт
`model/expected_validators.gen.go`:

```go
func ExpectedValidatorNames() []string {
    return []string{
        "app.ItemConsistency",
        "app.NonEmptyName",
    }
}
```

Используется при старте сервера для fail-fast проверки registry.

### Pattern регистрации валидаторов (server-side)

```go
package main

import (
    "log"
    "nschugorev/oapigenerator/pkg/validator"

    // Сгенерированная модель с ExpectedValidatorNames и ValidateOwn.
    // import "your/module/gen/model"
)

func main() {
    reg := validator.New()
    reg.Register(app.EmailFormat{})
    reg.Register(app.NonEmptyName{})
    reg.Register(app.PetConsistency{})

    // Fail-fast: если spec требует валидатор, которого нет в registry
    // (или registry содержит лишний) — приложение не стартует.
    if err := reg.AssertExact(model.ExpectedValidatorNames()); err != nil {
        log.Fatalf("validator registry mismatch: %v", err)
    }

    // Обработка входящего запроса.
    var req model.Pet // decode from JSON
    if err := validator.Validate(req, reg); err != nil {
        writeError(w, 400, err) // e.g. "field Name: must be >= 1"
        return
    }
    // ...
}
```

Walker обходит struct/slice/array/map/ptr/interface через reflection,
вызывает `ValidateOwn` на каждой структуре и при ошибке заворачивает её
с путём вида `Owner.Pets[2].Name`. Fail-fast: первая ошибка прерывает обход.

Рабочий пример: `examples/validation/main.go` (запуск — `go run ./examples/validation/`).

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
├── examples/           # runnable examples (validation pattern и т.д.)
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
