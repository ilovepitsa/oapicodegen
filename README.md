# oapigenerator

Go-генератор серверного и клиентского кода из OpenAPI 3.x спецификаций.
Проект — облегчённая альтернатива `mws/api/cmd/mwsapigen`: только Go-генерация,
без Kotlin/TypeScript/Terraform, со собственными библиотеками-заменами `platform-go`
в `internal/`.

## Статус

Проект на ранней стадии. Поддерживается только стандартный OpenAPI 3.x:
`paths`, `parameters`, `requestBody`, `responses`, `schemas` (`object`, `array`,
примитивы, `$ref`), `oneOf`/`anyOf`/`allOf`, `required`, `enum`, `format`,
`default`, `nullable`, `deprecated`.

Кастомные расширения `x-*`, валидации, split Request/Response, audit-data,
URL-form encoding и update-схемы отложены во вторую итерацию — см. `TASKS.md`.

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

Флаги (полный список см. в `cmd/oapigen` после T23):

| Флаг | По умолчанию | Назначение |
|------|--------------|------------|
| `-input` | — | Путь к OpenAPI 3.x spec-файлу (обязательный) |
| `-output` | — | Каталог для сгенерированного кода (обязательный, если не `-dry-run`) |
| `-import-prefix` | — | Go import-path префикс для пакетов (обязательный) |
| `-dry-run` | `false` | Парсить и генерировать без записи на FS |
| `-log-level` | `info` | debug\|info\|warn\|error\|fatal |
| `-log-format` | `console` | console\|json |
| `-log-development` | `false` | zap development mode (stacktraces, no sampling) |

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
├── cmd/            # точки входа CLI (cmd/oapigen — T23)
├── internal/       # библиотеки-замены platform-go + ядро генератора
├── testdata/       # эталонные OpenAPI-спеки и golden-файлы (T24)
├── TASKS.md        # план задач и карта замен platform-go
└── ARCHITECTURE.md # архитектурный обзор
```

## Лицензия

(определить позже)
