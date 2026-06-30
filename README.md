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
| `-input` | `./mws` | Корень каталога со спецификациями |
| `-output` | `./go/mws` | Каталог для сгенерированного кода |
| `-import-prefix` | `nschugorev/oapigenerator/go/mws` | Префикс Go-import |
| `-common-params-path` | `common/src/openapi/parameters/list.yaml` | Список общих параметров |
| `-dry-run` | `false` | Не записывать изменения на FS |
| `-debug-json` | `false` | Писать отладочные JSON-файлы |
| `-public` | `false` | Генерация публичного SDK |

## Make-таргеты

```sh
make build      # go build ./...
make test       # go test ./...
make vet        # go vet ./...
make fmt        # gofmt -s -w .
make lint       # golangci-lint run (если установлен)
make generate   # запуск генератора на testdata
make e2e        # go test -tags=e2e ./...
make tidy       # go mod tidy
make clean      # удалить артефакты сборки
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
