# Multi-service project layout — Design (rev 2)

- Дата: 2026-07-15
- Спека: T26 (см. TASKS.md)
- Статус: draft, supersedes rev1 (`2026-07-14-multi-service-layout-design.md`)
- Автор: Никита Щугорев

## 1. Типы T26.1 (минимальные, без Service/Method)

### 1.1 `gogen.Import` — расширение

`internal/codegen/gogen/gogen.go`:

```go
type ImportType int

const (
    LocalImport ImportType = iota
)

type Import struct {
    Path    string
    Alias   string
    Package string // имя пакета для использования в коде ("client", "httpclient")
    Type    ImportType
}
```

Старый код (`m.addImport("encoding/json", "")`) совместим: `Package`/`Type`
нулевые там, где не нужны. `renderImports` использует только `Path`/`Alias` —
без изменений.

### 1.2 `PathImports`

`internal/parser/project_paths.go` (новый):

```go
package parser

import "nschugorev/oapigenerator/internal/codegen/gogen"

// PathImports — типизированные Go-импорты артефактов одного сервиса.
// Создаётся один раз при Project.CreatePaths(basePath) и переиспользуется
// генератором вместо строковой конкатенации modulePath+"/<artifact>".
type PathImports struct {
    ClientInterfaces gogen.Import // <base>/interfaces/client
    ServerInterfaces gogen.Import // <base>/interfaces/server
    ClientHTTP       gogen.Import // <base>/impl/httpclient
    ServerHTTP       gogen.Import // <base>/impl/echoserver
    ClientMocks      gogen.Import // <base>/impl/mocks/client
    ServerMocks      gogen.Import // <base>/impl/mocks/server
    Model            gogen.Import // <base>/model
    SDK              gogen.Import // <base>/sdk
}
```

Имена подпапок — текущие из генератора (`impl/httpclient`, `impl/echoserver`,
`impl/mocks/{client,server}`). 
### 1.3 `Project` — новая структура

`internal/parser/project_set.go` (переписать):

```go
package parser

import "nschugorev/oapigenerator/internal/codegen/gogen"

type Project struct {
    Folder       string          // относительный путь от input ("userBackend", "common", "category/userService")
    SpecPath     string          // абсолютный путь к src/openapi/openapi.yaml
    FlagsPath    string          // абсолютный путь к generation_flags.yaml
    Features     ProjectFeatures
    Document     *Document       // T26.1: парсинг-промежуточный. В T26.1a убирается из Project.
    Model        *Model          // T26.1: минимальная обёртка
    Paths        *Paths          // T26.1: минимальная обёртка
    OutputDir    string          // <output>/<folder> — для ProjectLoader и compileCheck
    ImportPrefix string          // <import-prefix>/<folder> — основа для PathImports
}

type ProjectSet struct {
    Common   *Project
    Projects []*Project
    ByName   map[string]*Project
}

func (ps *ProjectSet) ByNameLookup(name string) (*Project, bool) {
    p, ok := ps.ByName[name]
    return p, ok
}

func (p *Project) CreateModel(imp gogen.Import) *Model {
    imp.Type = gogen.LocalImport
    m := &Model{project: p, Import: imp}
    p.Model = m
    return m
}

func (p *Project) CreatePaths(basePath string) *Paths {
    imp := func(sub, pkg, alias string) gogen.Import {
        return gogen.Import{
            Path:    basePath + "/" + sub,
            Alias:   alias,
            Package: pkg,
            Type:    gogen.LocalImport,
        }
    }
    pi := &Paths{
        Imports: PathImports{
            ClientInterfaces: imp("interfaces/client", "client", ""),
            ServerInterfaces: imp("interfaces/server", "server", ""),
            ClientHTTP:       imp("impl/httpclient", "client", "http"),
            ServerHTTP:       imp("impl/echoserver", "server", "http"),
            ClientMocks:      imp("impl/mocks/client", "client", "mock"),
            ServerMocks:      imp("impl/mocks/server", "server", "mock"),
            Model:            imp("model", "", "model"),
            SDK:              imp("sdk", "", ""),
        },
        project: p,
    }
    p.Paths = pi
    return pi
}
```

`OutputDir`/`ImportPrefix` оставлены плоскими полями — они нужны `ProjectLoader`
и `compileCheck` для файловой системы, не связаны с Go-импортами напрямую.
`Folder` заменяет старое `Name`.

### 1.4 `Model` / `Paths` — минимальные обёртки (T26.1)

`internal/parser/model.go` (новый):

```go
package parser

import "nschugorev/oapigenerator/internal/codegen/gogen"

// Model — доменная модель схем сервиса. В T26.1 минимальна:
// обёртка над schemas + Import. В T26.1a расширится (schemasIndex, Prefix,
// Lookup, Index).
type Model struct {
    project *Project
    Import  gogen.Import
    schemas []*Schema // в T26.1 не заполняется; в T26.2 ProjectLoader переносит doc.Schemas
}

func (m *Model) Schemas() []*Schema { return m.schemas }
```

`internal/parser/paths_wrapper.go` (новый; имя чтобы не конфликтовать с
существующим `paths.go`, где `extractPaths`):

```go
package parser

import "nschugorev/oapigenerator/internal/codegen/gogen"

// Paths — доменная модель операций сервиса. В T26.1: PathImports + ссылка на
// Document.Operations (плоский список). В T26.1a — Services []*Service.
type Paths struct {
    Imports  PathImports
    project  *Project
    document *Document // доступ к Operations/PathItems в T26.1
}

func (p *Paths) Operations() []*Operation { return p.document.Operations }
func (p *Paths) PathItems() []*PathItem    { return p.document.Paths }
```

### 1.5 `SchemaIndex` (переименование `ResourcesSet`)

`internal/parser/resources_set.go` → переименовать в `schema_index.go`:

```go
package parser

const (
    ModeRequest  = "Request"
    ModeResponse = "Response"
)

// SchemaIndex — глобальный индекс схем всех сервисов по абсолютному пути
// yaml-файла. Используется генератором для cross-service $ref: вместо
// дубликата типа в текущем сервисе эмитится Go-импорт в пакет владельца.
type SchemaIndex struct {
    Schemas map[string]*SchemaEntry
}

// SchemaEntry — запись в индексе: где лежит схема, какой Go-импорт и имя типа
// использовать при cross-service ссылках.
type SchemaEntry struct {
    Project    *Project
    SchemaName string // имя схемы во владельце (для диагностики)
    GoImport   string // например "nschugorev/oapigenerator/go/common"
    GoType     string // "User"; с учётом split-mode: "UserRequest"/"UserResponse"
}

func (si *SchemaIndex) LookupByFile(absPath string) (*SchemaEntry, bool) {
    e, ok := si.Schemas[absPath]
    return e, ok
}

func (si *SchemaIndex) LookupForMode(absPath, mode string) (*SchemaEntry, bool) {
    e, ok := si.LookupByFile(absPath)
    if !ok {
        return nil, false
    }
    if e.Project == nil || !e.Project.Features.SplitRequestResponse.Value {
        return e, true
    }
    out := *e
    switch mode {
    case ModeRequest:
        out.GoType = e.GoType + "Request"
    case ModeResponse:
        out.GoType = e.GoType + "Response"
    }
    return &out, true
}
```

Тесты `resources_set_test.go` → `schema_index_test.go`, обновить ссылки на
`SchemaIndex`/`SchemaEntry`.

### 1.6 Что меняется в незакоммиченном коде T26.1

- `gogen/gogen.go`: добавить `ImportType`, `Package`, `Type` на `Import`.
- `project_set.go`: переписать `Project` (новые поля + фабрики), `ProjectSet`
  остаётся, `ByNameLookup` без изменений.
- `project_paths.go`: новый, `PathImports`.
- `model.go`: новый, минимальный `Model`.
- `paths_wrapper.go`: новый, минимальный `Paths`.
- `resources_set.go` → `schema_index.go`: переименование типов + файла.
- Тесты: `project_set_test.go` обновить, `resources_set_test.go` →
  `schema_index_test.go`.

Без логики загрузки — только типы + аксессоры + фабрики. `Model.schemas` в
T26.1 не заполняется (нет ProjectLoader) — поле есть, но nil до T26.2.

Зависимости: T11.

## 2. Типы T26.1a (Service/Method + разделение Document)

### 2.1 `Method` (переименование `Operation`)

`internal/parser/operation.go` → переименовать в `method.go`. `Operation` →
`Method`. Поля те же + back-reference на `*Service`:

```go
type Method struct {
    MethodHTTP   string
    Path         string
    OperationID  string
    Summary      string
    Description  string
    Tags         []string
    Deprecated   bool
    Parameters   []*Parameter
    RequestBody  *RequestBody
    Responses    []*Response

    service *Service // back-reference, выставляется при AddMethod
}

func (m *Method) ServiceName() string // helper: m.service.Name
func (m *Method) Name() string        // OperationID или производное
```

Везде в генераторе `*parser.Operation` → `*parser.Method`. Имена хелперов
генератора вроде `operationMethodName(op)` можно оставить.

### 2.2 `Service` — группировка по тегу

`internal/parser/service.go` (новый):

```go
package parser

import "strings"

type Service struct {
    Name    string     // из op.Tags[0], normalised
    Methods []*Method

    paths *Paths // back-reference
}

func (s *Service) LowerName() string { return strings.ToLower(s.Name) }
```

Валидация тегов:

- Тегов 0 → сервис `"Service"` (дефолт).
- Тегов >1 → fail с указанием `path:method`.
- Тег пустой → fail.

### 2.3 `Paths` — расширение (T26.1a)

`internal/parser/paths_wrapper.go`:

```go
type Paths struct {
    Imports  PathImports
    Services []*Service

    servicesMap map[string]*Service
    project     *Project
}

func (p *Paths) AddMethod(serviceName string, m *Method) {
    if p.servicesMap == nil {
        p.servicesMap = map[string]*Service{}
    }
    s := p.servicesMap[serviceName]
    if s == nil {
        s = &Service{Name: serviceName, paths: p}
        p.servicesMap[serviceName] = s
        p.Services = append(p.Services, s)
    }
    s.Methods = append(s.Methods, m)
    m.service = s
}

func (p *Paths) DeleteService(name string) {
    delete(p.servicesMap, name)
    p.Services = slices.DeleteFunc(p.Services, func(s *Service) bool {
        return s.Name == name
    })
}
```

`Operations()` / `PathItems()` из T26.1 убираются — генератор переходит на
итерацию `Paths.Services[].Methods[]`.

### 2.4 Разделение `Document`

`Document` становится промежуточным типом парсера (не покидает `parser`):

```go
type Document struct {
    Schemas    []*Schema
    Operations []*Method   // переименование Operation → Method
    Paths      []*PathItem // промежуточный, для построения Services
    // ... остальные поля
}
```

`parser.ParseFile` возвращает `*Document` (как раньше). `ProjectLoader` берёт
Document, создаёт Project, вызывает `CreateModel(imp)` + `CreatePaths(basePath)`,
затем переносит:

- `doc.Schemas` → `project.Model.schemas` (прямо, без копирования).
- `doc.Operations` → `project.Paths.Services` через `AddMethod(serviceName(op), op)`.

После переноса `Document` больше не нужен Project — поле `Project.Document`
убирается (в T26.1 было, в T26.1a выкидываем). `Document` остаётся
экспортируемым — используется в тестах парсера и в `examples/`.

### 2.5 `Model` — расширение (T26.1a)

```go
type Model struct {
    project      *Project
    Import       gogen.Import
    schemas      []*Schema
    schemasIndex map[string]*Schema // lookup по имени
    Prefix       string             // для common: "common" alias
}

func (m *Model) Schemas() []*Schema
func (m *Model) Lookup(name string) (*Schema, bool)
func (m *Model) Index() // строит schemasIndex
```

`Prefix` нужен для common-проекта (alias `common` в импортах). В T26.1a вводится
минимально — без `OriginalSchemas`/`SplitRequestResponseSchemas` 

### 2.6 Влияние на генератор (T26.4)

- `g.doc.Schemas` → `g.project.Model.Schemas()`.
- `g.doc.Operations` → `for _, s := range g.project.Paths.Services { for _, m := range s.Methods { ... } }`.
- `g.modulePath+"/interfaces/client"` → `g.project.Paths.Imports.ClientInterfaces.Path`.
- `g.modulePath+"/impl/httpclient"` → `g.project.Paths.Imports.ClientHTTP.Path`.
- и т.д. для всех 8 артефактов.

Файловые пути в `writeOperationFiles` (generator.go:234-242) остаются
относительными (`"interfaces/client/client.gen.go"`) — `FileWriter` уже
получает basePath через `codegen.WithPath(fw, project.OutputDir)`.

### Граф зависимостей

```
T26.1 ──> T26.1a ──┬──> T26.2 ──> T26.3 ──┐
                   │                        ├──> T26.4 ──> T26.5 ──┐
                   └────────────────────────┘                        ├──> T26.7 ──> T26.8 ──> T26.9
T26.1 ──> T26.6 ─────────────────────────────────────────────────────┘
```

T26.1a вставлена между T26.1 и T26.2. T26.4 явно зависит от T26.1a (генератор
переходит на `Paths.Services[].Methods[]`).

### T26.1 — `Project`/`ProjectSet`/`SchemaIndex`/`PathImports` типы (rev2)

- Файлы: `internal/codegen/gogen/gogen.go` (расширение `Import`),
  `internal/parser/project_set.go` (переписать), `internal/parser/project_paths.go`
  (новый), `internal/parser/model.go` (новый, минимальный),
  `internal/parser/paths_wrapper.go` (новый, минимальный),
  `internal/parser/schema_index.go` (переименование).
- Типы: `ImportType`, расширенный `gogen.Import`, `PathImports`, `Project`,
  `ProjectSet`, `Model` (минимальный), `Paths` (минимальный), `SchemaIndex`,
  `SchemaEntry`.
- Методы: `Project.CreateModel`, `Project.CreatePaths`, `ProjectSet.ByNameLookup`,
  `SchemaIndex.LookupByFile`, `SchemaIndex.LookupForMode`, `Model.Schemas`,
  `Paths.Operations`, `Paths.PathItems`.
- Юнит-тесты на аксессоры и фабрики.
- Зависимости: T11.
- Ветка: `feat/project-set-types` (текущая).

### T26.1a — `Service`/`Method` + разделение `Document`

- Файлы: `internal/parser/method.go` (переименование `operation.go`),
  `internal/parser/service.go` (новый), `internal/parser/paths_wrapper.go`
  (расширение), `internal/parser/model.go` (расширение),
  `internal/parser/document.go` (`Operations []*Method`), `internal/parser/paths.go`
  (`convertOperation` → `convertMethod`), `internal/parser/parser.go` (референсы).
- Типы: `Method` (переименование `Operation`), `Service`. Расширение `Model`
  (`schemasIndex`, `Prefix`, `Lookup`, `Index`). Расширение `Paths` (`Services`,
  `servicesMap`, `AddMethod`, `DeleteService`).
- Валидация тегов: ровно один непустой тег, иначе fail; дефолт `"Service"`.
- Удаление `Project.Document` поля (после переноса в Model/Paths).
- Юнит-тесты: теги → сервис, группировка, fail-кейсы.
- Зависимости: T26.1.
- Ветка: `feat/parser-service-method`.

### T26.2 — `ProjectLoader` (адаптация)

`ProjectLoader.Load()`:

1. `walkServices(input)` → `[]serviceDescriptor`.
2. Для каждого: `parser.ParseFile(rootFS, relSpecPath)` → `*Document`.
3. `flagsLoader.GetProjectFeatures(name)`.
4. `project := &Project{Folder: name, SpecPath: ..., FlagsPath: ..., Features: ..., OutputDir: ..., ImportPrefix: ...}`.
5. `project.CreateModel(gogen.Import{...})` + `project.CreatePaths(project.ImportPrefix)`.
6. Перенос: `project.Model.schemas = doc.Schemas`; для каждого `op := range doc.Operations` → `project.Paths.AddMethod(serviceName(op), op)`.
7. `markExternalRefs(doc, ...)` — разметка `Schema.SourceFile`/`ExternalRef`/`OwnerProject`.
8. Сборка `SchemaIndex` из всех `doc.Schemas` всех сервисов.
9. Возвращает `*ProjectSet`, `*SchemaIndex`.

Зависимости: T26.1a, T24a. Ветка: `feat/project-loader`.

### T26.3 — parser source-marking (без изменений)

Разметка `Schema.SourceFile`/`ExternalRef`/`OwnerProject`. Зависит от T26.2.
Ветка: `feat/parser-source-marking`.

### T26.4 — generator под `ProjectSet` (адаптация)

- `Generate(fw, ps, si, opts...)` (третий аргумент `*SchemaIndex`).
- `Generator` хранит `*parser.Project` + `*parser.SchemaIndex`.
- `g.modulePath` → `g.project.Paths.Imports.<Artifact>.Path`.
- `g.features` → `g.project.Features`.
- `g.doc.Schemas` → `g.project.Model.Schemas()`.
- `g.doc.Operations` → `for _, s := range g.project.Paths.Services { for _, m := range s.Methods { ... } }`.
- Существующие юнит-тесты генератора — fixtures с `Project` вручную.
- Зависимости: T26.1a, T26.3.
- Ветка: `feat/generator-projectset`.

### T26.5 — Cross-package Go imports (адаптация)

- `g.resolveRef(schema)` использует `SchemaIndex.LookupByFile`/`LookupForMode`.
- Имена типов: `SchemaEntry` вместо `ResourceSchema`.
- Зависимости: T26.4.
- Ветка: `feat/generator-cross-refs`.

### T26.6 — Post-generation compile check (без изменений)

Зависимости: T5 (или T26.1 — для `gogen`). Параллелен T26.1a/T26.4.
Ветка: `feat/compile-check`.

### T26.7 — CLI migration (без изменений по сути)

Обновить вызовы под `SchemaIndex` вместо `ResourcesSet`. Зависимости: T26.2,
T26.4, T26.5, T26.6. Ветка: `feat/oapigen-multiservice`.

### T26.8 — Testdata migration (без изменений)

Зависимости: T26.7. Ветка: `feat/testdata-project`.

### T26.9 — Docs update (без изменений)

Зависимости: T26.8. Ветка: `feat/docs-multiservice`.

### T27 (stub) / T28 (stub) — без изменений.

## 4. Принятие решений (дополнение к rev1 §9)

| Решение | Выбор | Альтернативы | Обоснование |
|---|---|---|---|
| Имя для индекса схем | `SchemaIndex` + `SchemaEntry` | `ResourcesSet`, `SchemaRegistry`, `ExternalSchemaMap`, `CrossServiceSchemaIndex` | Точно отражает структуру (map по abs-пути), не конфликтует с `pkg/validator.Registry` или REST-resource `ResourcesSet` |
| Структура `Project` | `Folder` + `Model *Model` + `Paths *Paths` + `Features` | Плоские поля (rev1) | Типизированные `PathImports` убирают строковую конкатенацию; готовый фундамент для T27 visitor |
| `gogen.Import` | Расширить `Package` + `Type` | Оставить `Path`+`Alias`, path strings в `PathImports` | Типизация импортов артефактов |
| `Service`/`Method` абстракция | Вводится (T26.1a) | Отложить до T27 | Генератору нужна группировка по тегу для эмитции server interfaces; T26.4/T26.5 уже потребуют |
| Разделение `Document` | `Document` → промежуточный парсера; `Model.schemas` + `Paths.Services` | Сохранить `Document` на Project |  `Document` не утекает в generator |
| Подход миграции | Инкрементальный (T26.1 + T26.1a) | Big-bang rewrite T26.1 | T26.1 = типы + аксессоры (узкий скоуп); T26.1a = доменная переработка парсера (отдельная ответственность) |
