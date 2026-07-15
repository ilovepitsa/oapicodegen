# Multi-service project layout — Design

- Дата: 2026-07-14
- Спека: T26 (см. TASKS.md)
- Статус: draft
- Автор: Никита Щугорев

## 1. Goals & Scope

### Цель

Генератор принимает папку проекта (а не один spec-файл) с произвольной
вложенностью сервисов. Для каждого сервиса эмитит Go-пакеты в
`<output>/<service-path>/...` с import-prefix `<import-prefix>/<service-path>/...`.
Поддерживается cross-service `$ref` через файловые пути между любыми сервисами
(ответственность за отсутствие циклов — на пользователе).

### Входит в scope

- Multi-service layout: рекурсивное обнаружение сервисов по маркерам
  `generation_flags.yaml` + `src/openapi/openapi.yaml`.
- Common-проект как обычный сервис с фиксированным именем `common` (никакого
  special-case в discovery).
- Cross-service `$ref` через файловые пути (`../<other-service>/src/openapi/...`)
  → Go-импорт в чужой пакет. Любой сервис может ссылаться на любой другой.
- Удаление старого single-spec режима.
- Post-generation `go build ./...` compile check.
- Требование: каталог `-output` — корень Go-модуля (один `go.mod` на весь вывод).
- Новые testdata: `testdata/project/` с `common` + 2 сервисами, cross-service ref'ами.
- Архитектурные абстракции `ProjectSet` / `Project` / `ResourcesSet` / `ProjectLoader`
  по образцу `../api/mwsapigen` (в урезанном виде: без cmdtree, без tf-provider,
  без opensource-yaml, без мультиязычности).

### Не входит (отдельные будущие спеки)

- Subpackage splitting (дробление `model/` по подпапкам на основе структуры FS).
  Stub-задача T28.
- Visitor pattern refactoring. Stub-задача T27. Полный дизайн — отдельный
  brainstorming после реализации T26 и появления хотя бы одного нового
  артефакта, на котором станет ясно, какие абстракции visitor'а нужны.
- Кастомные `x-*` расширения, audit-data, cmdtree (T21), terraform, opensource-yaml
  (T22) — остаются в глубоком бэклоге TASKS.md без изменений.

### Принципы (сохраняются из существующей ARCHITECTURE.md)

- Один пакет = одна ответственность. `parser` не знает про Go, `generator` не
  читает файлы, `codegen` не знает про OpenAPI.
- Стандартный OpenAPI сначала.
- Минимум внешних зависимостей.
- Явное лучше неявного.

## 2. Layout & Discovery

### Структура проекта на входе

```
security-gate/                       # -input
├── common/
│   ├── generation_flags.yaml
│   └── src/openapi/
│       ├── openapi.yaml
│       └── schemas/
│           └── User.yaml
├── userBackend/
│   ├── generation_flags.yaml
│   └── src/openapi/
│       ├── openapi.yaml
│       └── schemas/
│           └── Profile.yaml          # $ref: ../../../common/src/openapi/schemas/User.yaml
└── authBackend/
    ├── generation_flags.yaml
    └── src/openapi/
        ├── openapi.yaml
        └── schemas/
            └── Credentials.yaml      # $ref: ../../../common/src/openapi/schemas/User.yaml
```

### Структура выхода (`-output go`)

```
go/                                  # -output (корень Go-модуля)
├── go.mod                           # module <import-prefix>, пользователь инициализирует сам
├── common/
│   ├── model/...
│   ├── interfaces/...
│   ├── impl/...
│   └── sdk/...
├── userBackend/...
└── authBackend/...
```

### Discovery

Сервис = папка, содержащая **оба** маркера:

- `generation_flags.yaml` — per-service generation flags (override поверх
  глобального `-generation-flags-config-path`).
- `src/openapi/openapi.yaml` — главная спека сервиса.

Поиск:

- `filepath.WalkDir(input)` обходит дерево.
- Для каждой папки проверяем наличие обоих маркеров.
- Если оба есть — это сервис. **Подпапки этого сервиса дальше не сканируются**
  на предмет других сервисов (запрещаем вложенные сервисы).
- Имя сервиса = относительный путь от `-input` до папки-сервиса, нормализованный:
  разделитель `/` (под Windows `\` → `/`), регистр сохраняется. Примеры:
  `userBackend`, `common`, `category/userService` (если есть промежуточная категория).
  Пользователь сам отвечает за то, чтобы имена папок соответствовали Go-конвенции
  для import paths (рекомендуется lowercase, но не обязательно).
- Порядок обработки: сначала `common` (если есть — для удобства, не критично),
  затем остальные в алфавитном порядке. Детерминированность важна для golden-тестов.

### Имя сервиса → пути

- `outputDir = filepath.Join(output, serviceName)`.
- `importPrefix = filepath.Join(importPrefix, serviceName)` (с нормализацией
  `\` → `/` под Go import path).
- Для категории `category/userservice`: `outputDir = <output>/category/userservice`,
  `importPrefix = <import-prefix>/category/userservice`.

### Ошибки discovery

- Нет ни одного сервиса → fail: "no services found under <input>; expected folders with both generation_flags.yaml and src/openapi/openapi.yaml".
- Вложенный сервис (сервис внутри сервиса) → fail: "service X is nested inside service Y; nested services are not supported".
- Duplicate import path (два сервиса дают одинаковый Go import после нормализации) → fail.
- Duplicate schema source path (один yaml-файл принадлежит двум сервисам) → fail
  на этапе построения `ResourcesSet`.
- `$ref` ведёт вне `-input` → fail.
- Cross-service `$ref` на несуществующий файл → fail с указанием ref'а.

## 3. CLI & внутренняя модель

### Изменения в `cmd/oapigen/main.go`

| Флаг | Было | Стало |
|------|------|------|
| `-input` | путь к одному spec-файлу | путь к папке проекта (обязательный) |
| `-output` | каталог для плоского вывода | каталог-корень для `<output>/<service>/...`; должен содержать `go.mod` (обязательный, если не `-dry-run`) |
| `-import-prefix` | Go import path | Go import path — корень для `<import-prefix>/<service>/...` (обязательный) |
| `-project-flags-path` | per-project override (один) | **удаляется** — per-service flags берутся из `<service>/generation_flags.yaml` автоматически |
| `-generation-flags-config-path` | глобальный конфиг | без изменений |
| `-dry-run`, `-log-*` | без изменений | без изменений |
| `-skip-compile-check` (новый) | — | пропускает post-gen `go build ./...` |

### Новая связка в `run()`

```
1. Парсим флаги.
2. Грузим глобальный generation_flags.yaml (если задан -generation-flags-config-path).
3. ProjectLoader.Load():
   a. walkServices(input) → находим сервисы.
   b. Для каждого сервиса: ParseFile + GetProjectFeatures (global + per-service override).
   c. Строим ProjectSet + ResourcesSet.
4. generator.Generate(fw, ps, rs):
   a. Для каждого Project в ps.Projects — генерация артефактов в <output>/<service>/...
   b. Cross-service $ref → cross-package Go imports (через ResourcesSet).
5. compileCheck(output) (если не dry-run и не -skip-compile-check):
   a. Проверка go.mod.
   b. go build ./...
```

### Новые типы в `internal/parser` (новые файлы `project_set.go`, `resources_set.go`)

```go
type Project struct {
    Name         string          // относительный путь от input, напр. "userbackend", "common"
    SpecPath     string          // абсолютный путь к src/openapi/openapi.yaml
    FlagsPath    string          // абсолютный путь к generation_flags.yaml
    Features     ProjectFeatures // резолвнутые флаги (global + per-service override)
    Document     *Document       // распарсенный spec
    OutputDir    string          // <output>/<name>
    ImportPrefix string          // <import-prefix>/<name>
}

type ProjectSet struct {
    Common   *Project             // nil если common не найден
    Projects []*Project           // все проекты включая common (для итерации)
    ByName   map[string]*Project  // индекс по Name
}

func (ps *ProjectSet) ByName(name string) (*Project, bool)

type ResourcesSet struct {
    Schemas map[string]*ResourceSchema // ключ — абсолютный путь к yaml-файлу схемы
}

type ResourceSchema struct {
    Project    *Project
    SchemaName string // имя схемы во владельце (для диагностики)
    GoImport   string // напр. "nschugorev/oapigenerator/go/common"
    GoType     string // напр. "User"; с учётом split-mode: "UserRequest"/"UserResponse"
}

func (rs *ResourcesSet) LookupByFile(absPath string) (*ResourceSchema, bool)
func (rs *ResourcesSet) LookupForMode(absPath string, mode string) (*ResourceSchema, bool)
```

### `ProjectLoader` (новый файл `project_loader.go`)

```go
type ProjectLoader struct {
    input        string
    output       string
    importPrefix string
    flagsLoader  *GenerationFlagsLoader
    fs           fs.FS
}

func NewProjectLoader(input, output, importPrefix string, fl *GenerationFlagsLoader, fs fs.FS) *ProjectLoader
func (l *ProjectLoader) Load() (*ProjectSet, *ResourcesSet, error)
```

Loader делает:

1. `walkServices(l.input)` → `[]serviceDescriptor{name, specPath, flagsPath}`.
2. Для каждого дескриптора: `parser.ParseFile(rootFS, relSpecPath)` + `flagsLoader.GetProjectFeatures(name)`.
3. `markExternalRefs(doc, serviceName, serviceSpecDirs)` — разметка `SourceFile`/`ExternalRef`/`OwnerProject` на схемах.
4. Строит `Project` с заполненными `OutputDir`/`ImportPrefix`.
5. Строит `ResourcesSet` индекс всех схем всех сервисов.
6. Возвращает `ProjectSet` + `ResourcesSet`.

### Изменения в `internal/generator`

Сигнатура:

```go
// Было:
func Generate(fw codegen.FileWriter, doc *parser.Document, opts ...Option) error

// Стало:
func Generate(fw codegen.FileWriter, ps *parser.ProjectSet, rs *parser.ResourcesSet, opts ...Option) error
```

Опции:

- `WithModulePath` — **удаляется** (`ImportPrefix` теперь на `Project`).
- `WithProjectFeatures` — **удаляется** (`Features` теперь на `Project`).
- `WithLogger` — добавляется, опционально (для отладочного вывода).

`Generator`:

```go
type Generator struct {
    project  *parser.Project
    rs       *parser.ResourcesSet
    logger   *zap.SugaredLogger
    // ...существующие поля, вычисляемые от project.Features / project.ImportPrefix...
}
```

Цикл генерации:

```go
for _, p := range ps.Projects {
    g := New(p, rs, opts...)
    if err := g.generate(fw); err != nil {
        return fmt.Errorf("generate project %q: %w", p.Name, err)
    }
}
```

## 4. Cross-service `$ref` resolution

### Контекст

Сейчас парсер через `libopenapi` + `os.DirFS(filepath.Dir(input))` резолвит
cross-file `$ref` внутри каталога спеки. Для сервиса `userBackend` `DirFS` =
`<input>/userBackend/src/openapi/`, поэтому `$ref: schemas/Profile.yaml` работает,
а `$ref: ../../../common/src/openapi/schemas/User.yaml` выходит за пределы `DirFS`
и не резолвится.

### Механика

#### 1. Расширение FS при парсинге

`ProjectLoader` создаёт `DirFS` от **корня `-input`** (а не от папки спеки
сервиса), передаёт в `ParseFile` путь к спеке относительно этого корня:

```go
rootFS := os.DirFS(l.input)
doc, err := parser.ParseFile(rootFS, relSpecPath)
// relSpecPath = "userBackend/src/openapi/openapi.yaml"
```

Тогда `$ref: ../../../common/src/openapi/schemas/User.yaml` резолвится
libopenapi'ем штатно — корень FS покрывает весь проект.

После резолвинга `parser.Document` сервиса `userBackend` содержит inline-копию
схем из common. Нас это не устраивает — нужен Go-импорт, не inline-дубликат.

#### 2. Разметка ref'ов на этапе парсинга

Добавляем поля на `parser.Schema` (в `internal/parser/document.go`):

```go
type Schema struct {
    // ...существующие поля...
    SourceFile   string // абсолютный путь к yaml-файлу схемы
    ExternalRef  bool   // true если схема пришла из другого сервиса
    OwnerProject string // имя сервиса-владельца (где физически лежит схема)
}
```

При парсинге сервиса `X`:

- Схемы с `SourceFile` внутри `<input>/X/src/openapi/` → `ExternalRef=false`, `OwnerProject=X`.
- Схемы с `SourceFile` внутри любого другого `<input>/Y/src/openapi/` → `ExternalRef=true`, `OwnerProject=Y`.

`markExternalRefs(doc, ownerProjectName, serviceSpecDirs)` — проход по `doc.Schemas`
+ всем inline-схемам в операциях/параметрах, заполняет поля.

#### 3. Использование `ResourcesSet` в генераторе

При генерации сервиса `X`:

- Схемы с `OwnerProject=="X"` → генерируются как обычно (тип в текущем пакете `model`).
- Схемы с `OwnerProject=="Y"` (любой Y ≠ X) → **не генерируются** в `X/model/`.
  `typeMapper` эмитит qualified identifier `Ypackage.<Type>` с Go-импортом
  `"<import-prefix>/Ypackage"` (где `Ypackage` = нормализованное имя сервиса Y).

Все места, использующие `$ref` (параметры, requestBody, responses, oneOf/anyOf/allOf,
response headers, mocks, sdk), проверяют `ExternalRef` на резолвнутой схеме и
эмитят cross-package identifier вместо локального.

#### 4. Split-mode и qualified type naming

Если в сервисе-владельце Y включён `GOLANG_SPLIT_REQUEST_RESPONSE` и схема
splittable, `ResourcesSet` хранит варианты:

- `User` (моно режим)
- `UserRequest` (request mode)
- `UserResponse` (response mode)

`ResourcesSet.LookupForMode(absPath, mode)` отдаёт правильный тип в зависимости от
`mode` (`""` / `modeRequest` / `modeResponse`) и фич владельца.

Update-схемы, UTC datetime, URL-form — cross-service импортируются по тем же
правилам: тип лежит во владельце, потребитель только импортирует.

#### 5. Циклы и валидация

- Циклы на уровне Go-импортов **не выявляются** генератором намеренно — это зона
  ответственности пользователя.
- Post-gen `go build ./...` поймает цикл (`import cycle not allowed`). Это и есть
  механизм детекции.
- Явные проверки в `ProjectLoader.Load()`:
  - Duplicate schema source path (один yaml-файл принадлежит двум сервисам) → fail.
  - Duplicate import path → fail.
  - `$ref` ведёт вне `-input` → fail.
  - Cross-service `$ref` на несуществующий файл → fail с указанием ref'а.

#### 6. Ограничения cross-service ref

- Поддерживается только `$ref` на top-level schema-файл или
  `#/components/schemas/...` в спеке другого сервиса. Inline-схемы внутри
  property другого сервиса — не поддерживаются.

## 5. Post-generation compile check

После `generator.Generate(...)` в `cmd/oapigen/main.go`:

```go
if !dryRun && !skipCompileCheck {
    if err := compileCheck(output, logger); err != nil {
        return fmt.Errorf("post-generation compile check failed: %w", err)
    }
}
```

`compileCheck(outputDir string, logger) error`:

- Проверяет наличие `go.mod` в `outputDir`. Если отсутствует — fail: "-output must be a Go module root (go.mod not found at <outputDir>); run `go mod init <import-prefix>`".
- Запускает `exec.Command("go", "build", "./...")` с `cmd.Dir = outputDir`.
- При ошибке — оборачивает с `stderr` компилятора, печатает его в лог.
- Флаг `-skip-compile-check` — escape hatch для случаев, когда пользователь
  намеренно хочет пропустить (например, при `go.mod` ещё не настроен).

### Требование к `go.mod`

- Каталог `-output` **должен** содержать `go.mod` с module path, совпадающим с
  `-import-prefix`.
- Генератор **не создаёт** `go.mod` и **не модифицирует** его — пользователь
  инициализирует модуль один раз (`go mod init <import-prefix>`).
- Все external-зависимости (`libopenapi`-produced runtime deps типа
  `github.com/labstack/echo/v4`, `pkg/optional`, `pkg/validator` и т.д.) —
  пользователь прописывает в `go.mod` сам через `go get`.
- Если в `-output` нет `go.mod` — генератор fail'ит с подсказкой.

## 6. Удаление старого режима + миграция testdata

### Удаление single-spec режима

Полностью убираем из `cmd/oapigen/main.go`:

- Семантику "`-input` указывает на файл". Теперь `-input` всегда папка.
- Флаг `-project-flags-path`.
- Опции `generator.WithModulePath` и `generator.WithProjectFeatures` — `ModulePath`
  и `Features` переезжают на `parser.Project`.
- Helper `loadProjectFeatures`.

### Миграция testdata

**Удаляем**:

- `testdata/minimal/spec.yaml` + `testdata/minimal/golden/` — старый формат.
- `testdata/split/openapi.yaml` + `testdata/split/paths/` + `testdata/split/schemas/` + `testdata/split/gen/` — старый формат.

**Создаём новый testdata-проект** `testdata/project/`:

```
testdata/project/
├── common/
│   ├── generation_flags.yaml
│   └── src/openapi/
│       ├── openapi.yaml
│       └── schemas/
│           ├── User.yaml
│           └── AuditMeta.yaml
├── userBackend/
│   ├── generation_flags.yaml
│   └── src/openapi/
│       ├── openapi.yaml
│       └── schemas/
│           └── Profile.yaml      # $ref: ../../../common/src/openapi/schemas/User.yaml
└── authBackend/
    ├── generation_flags.yaml
    └── src/openapi/
        ├── openapi.yaml
        └── schemas/
            └── Credentials.yaml  # $ref: ../../../common/src/openapi/schemas/User.yaml
```

**Golden** — для multi-service вывода goldens хранятся в `testdata/project/golden/`:

```
testdata/project/golden/
├── go.mod                        # module nschugorev/oapigenerator/testdata/project/golden
├── common/
│   ├── model/...
│   ├── interfaces/...
│   └── impl/...
├── userBackend/...
└── authBackend/...
```

Golden-директория — компилируемый Go-модуль. e2e-тест запускает `go build ./...`
на нём.

### Тесты

В `internal/generator/generator_test.go` или новом `internal/parser/project_loader_test.go`:

- `TestProjectLoader_DiscoversAllServices` — walk'ит `testdata/project/`, проверяет что найдены `common`, `userBackend`, `authBackend`.
- `TestProjectLoader_RejectsNestedServices` — синтетический testdata с вложенным сервисом → ожидаем ошибку.
- `TestProjectLoader_EmptyProjectFails`.
- `TestProjectLoader_DuplicateImportPathFails`.
- `TestGenerate_MultiService_Golden` — end-to-end генерация, сверка с golden.
- `TestGenerate_CrossServiceRef_EmitsGoImport` — в `userBackend/model/profile.gen.go` есть `import ".../common"` и тип `common.User`.
- `TestCompileCheck_NoGoModFails` — temp dir без `go.mod` → ошибка.
- `TestCompileCheck_BuildErrorPropagated`.
- `TestCompileCheck_ImportCycleDetected` — синтетический цикл (service A → B → A) → `go build` fail.
- `TestGenerate_CrossServiceRef_NonExistentTargetFails`.

### Миграция examples

`examples/validation/main.go` — обновить под новую сигнатуру `generator.Generate(fw, ps, rs)`. Пример упрощается: показываем запуск `oapigen -input ./project -output ./go -import-prefix ...`.

### Обновление документации

- `README.md`: раздел "Использование" переписать под multi-service. Убрать пример с `-input spec.yaml`, добавить пример с multi-service project. Обновить таблицу флагов.
- `ARCHITECTURE.md`: новая секция "Multi-service layout" с диаграммой потока `Walk → Parse → ProjectSet → Generate → CompileCheck`. Описание `ProjectSet`/`ResourcesSet`/`ProjectLoader`. Убрать опережающие упоминания (теперь они будут настоящими).
- `TASKS.md`: вставить задачи T26.1–T26.9 + stub'ы T27 (visitor) / T28 (subpackage).

## 7. Visitor pattern stub

В этой спеке **не реализуется**. Фиксируется как будущий рефакторинг (T27).

Текущий генератор использует серию ветвлений по типам артефактов
(`writeSchemaFiles`, `writeClientInterfaces`, `writeServerInterfaces`,
`writeHTTPClient`, `writeHTTPServer`, `writeMocks`, `writeSDK`) — каждый метод
имеет свой кодогенерирующий код и доступается из `Generate()`.

Минусы текущего подхода:

- Добавление нового артефакта (например, audit-data) требует правки `Generate()`
  и ещё одного `write*`-метода.
- Сложно параметризовать генерацию (включить/выключить артефакт, изменить порядок).
- Трудно переиспользовать логику обхода доменной модели между артефактами.

Visitor pattern предлагается как future-task: заменить прямые вызовы `write*` на
`ProjectSetVisitor` интерфейс, где каждый visitor отвечает за один артефакт, а
`Generate()` orchestr'ит обход.

Заготовка интерфейса (для будущего spec-файла):

```go
type ProjectSetVisitor interface {
    VisitProject(p *parser.Project) error
    VisitSchema(p *parser.Project, s *parser.Schema) error
    VisitOperation(p *parser.Project, op *parser.Operation) error
    // ...
}

// Конкретные visitors:
type SchemaVisitor struct{ ... }
type ClientInterfaceVisitor struct{ ... }
type ServerInterfaceVisitor struct{ ... }
// и т.д.
```

Полный дизайн — отдельный brainstorming после:

1. Реализации T26 (multi-service layout).
2. Появления хотя бы одного нового артефакта (audit-data, subpackage splitting,
   etc.), на котором станет ясно, какие именно абстракции visitor'а нужны.

## 8. План задач T26 (подзадачи и зависимости)

Каждая подзадача = отдельная ветка `feat/...`. Порядок указан dependency-порядком;
внутри одного уровня — можно параллелить.

### T26.1 — `ProjectSet`/`Project`/`ResourcesSet` типы

- Файлы: `internal/parser/project_set.go`, `internal/parser/resources_set.go`.
- Типы: `Project`, `ProjectSet`, `ResourcesSet`, `ResourceSchema`.
- Методы: `ProjectSet.ByName(name)`, `ResourcesSet.LookupByFile(absPath)`,
  `ResourcesSet.LookupForMode(absPath, mode)`.
- Без логики загрузки — только структуры + методы-аксессоры.
- Юнит-тесты на аксессоры.
- Зависимости: T11.
- Ветка: `feat/project-set-types`.

### T26.2 — `ProjectLoader`

- Файл: `internal/parser/project_loader.go`.
- `walkServices(input)` — `filepath.WalkDir` ищет пары маркеров, возвращает `[]serviceDescriptor`.
- Валидация: nested services, duplicate names, отсутствие сервисов.
- `Load()` orchestr'ит: walk → для каждого сервиса `ParseFile` + `GetProjectFeatures` → сборка `ProjectSet` + `ResourcesSet`.
- Имя сервиса = относительный путь от input, нормализованный (разделитель `/`, регистр сохраняется).
- Юнит-тесты: `TestWalkServices_DiscoversAll`, `TestWalkServices_RejectsNested`, `TestWalkServices_EmptyProjectFails`.
- Зависимости: T26.1, T24a.
- Ветка: `feat/project-loader`.

### T26.3 — `ParseFile` + разметка source-file

- Изменения в `internal/parser/document.go` (где живёт `ParseFile` и `Schema`).
- `ParseFile(fs, path)` — без изменения сигнатуры, но внутри прокидывает source-path каждой схемы в `Schema.SourceFile`.
- `markExternalRefs(doc, ownerProjectName, serviceSpecDirs)` (новый helper, можно вынести в `internal/parser/project_loader.go`) — проход: для каждой схемы вычисляет `ExternalRef`/`OwnerProject` по `SourceFile` против карты `<service-spec-dir>`.
- Юнит-тесты: schema локальная → `ExternalRef=false`; schema из соседнего сервиса → `ExternalRef=true`, `OwnerProject=...`.
- Зависимости: T26.1, T26.2.
- Ветка: `feat/parser-source-marking`.

### T26.4 — `generator.Generate` под `ProjectSet`

- Сигнатура: `Generate(fw, ps, rs, opts ...) error`.
- `Option`-ы: убрать `WithModulePath`, `WithProjectFeatures`. Добавить `WithLogger`.
- Цикл: `for p := range ps.Projects { New(p, rs).generate(fw) }`.
- `Generator` хранит `*parser.Project` (вместо `modulePath`/`features`) + `*parser.ResourcesSet`.
- Везде, где было `g.modulePath` → `g.project.ImportPrefix`.
- Везде, где было `g.features` → `g.project.Features`.
- Существующие юнит-тесты генератора переводятся на конструирование `Project` вручную (fixtures).
- Зависимости: T26.1, T26.3.
- Ветка: `feat/generator-projectset`.

### T26.5 — Cross-package Go imports

- Изменения в `internal/generator/type.go` (`typeMapper`) и во всех местах, использующих `$ref` (`schema.go`, `client.go`, `server.go`, `impl_client.go`, `impl_server.go`, `mocks.go`, `sdk.go`, `response_headers.go`).
- Новая функция `g.resolveRef(schema) (goImport string, goType string, isExternal bool)`:
  - Если `schema.ExternalRef=false` → локальный референс.
  - Если `true` → `ResourcesSet.LookupByFile(schema.SourceFile)` → `{GoImport, GoType}`.
- При эмите cross-package identifier'а — добавить import в генерируемый файл.
- Не генерировать файлы для external-схем (skip в `writeSchemaFiles`).
- Split-mode support: `ResourcesSet.LookupForMode(schema, mode)` отдаёт `UserRequest`/`UserResponse`/`User` в зависимости от фич владельца.
- Юнит-тесты: `TestResolveRef_LocalSchema`, `TestResolveRef_ExternalSchema_EmitsImport`, `TestWriteSchemaFiles_SkipsExternalSchemas`.
- Зависимости: T26.4.
- Ветка: `feat/generator-cross-refs`.

### T26.6 — Post-generation compile check

- Файл: `internal/codegen/compilecheck.go` (или `internal/cli/compilecheck.go`).
- `CompileCheck(outputDir string, logger) error`:
  - Проверка `go.mod` exists в `outputDir`.
  - `exec.Command("go", "build", "./...")` с `cmd.Dir = outputDir`.
  - При ошибке — обернуть с `stderr` компилятора.
- Юнит-тесты (через `t.TempDir`): `TestCompileCheck_NoGoModFails`, `TestCompileCheck_BuildErrorPropagated`, `TestCompileCheck_SuccessOnValidModule`.
- Зависимости: T5.
- Ветка: `feat/compile-check`.
- Можно делать параллельно с T26.4/T26.5.

### T26.7 — CLI: `cmd/oapigen/main.go` migration

- Удалить: семантику "input=file", `-project-flags-path`, `loadProjectFeatures` helper.
- Добавить: `-skip-compile-check` флаг.
- Новая последовательность в `run()`:
  1. Парсинг флагов.
  2. `flagsLoader.Load(-generation-flags-config-path)` (если задан).
  3. `projectLoader := parser.NewProjectLoader(input, output, importPrefix, flagsLoader, fs)`.
  4. `ps, rs, err := projectLoader.Load()`.
  5. `generator.Generate(fw, ps, rs)`.
  6. `compileCheck(output)` (если не dry-run и не skip).
- E2E тест `cmd/oapigen/e2e_test.go` обновить под новый testdata project.
- Зависимости: T26.2, T26.4, T26.5, T26.6.
- Ветка: `feat/oapigen-multiservice`.

### T26.8 — Testdata migration

- Удалить `testdata/minimal/`, `testdata/split/`.
- Создать `testdata/project/` (см. раздел 6).
- Создать `testdata/project/golden/` + `testdata/project/golden/go.mod` (module path `nschugorev/oapigenerator/testdata/project/golden`).
- Прогнать генератор один раз для записи goldens, проверить `go build ./...` в golden-директории.
- Зависимости: T26.7.
- Ветка: `feat/testdata-project`.

### T26.9 — Docs update

- `README.md`: раздел "Использование" переписать. Убрать пример с `-input spec.yaml`, добавить пример с multi-service project. Обновить таблицу флагов.
- `ARCHITECTURE.md`: новая секция "Multi-service layout" с диаграммой потока `Walk → Parse → ProjectSet → Generate → CompileCheck`. Описание `ProjectSet`/`ResourcesSet`/`ProjectLoader`. Убрать опережающие упоминания.
- `TASKS.md`: вставить задачи T26.1–T26.9 + stub'ы T27 (visitor) / T28 (subpackage) в раздел "Вторая итерация".
- Зависимости: T26.8.
- Ветка: `feat/docs-multiservice`.

### T27 (stub) — Visitor pattern refactoring

Stub. Полный дизайн — отдельный brainstorming после T26. См. раздел 7.

### T28 (stub) — Subpackage splitting

Stub. Дробление `model/` по подпапкам на основе структуры FS (`schemas/<subfolder>/...`
→ `model/<subfolder>/...`). Отдельный brainstorming позже.

### Граф зависимостей

```
T26.1 ──┬──> T26.2 ──> T26.3 ──> T26.4 ──> T26.5 ──┐
        │                                            ├──> T26.7 ──> T26.8 ──> T26.9
        └──────────────────────────────────────────>│
                              T26.6 ─────────────────┘
```

T26.1, T26.6 — можно стартовать параллельно. T26.2 ждёт T26.1. T26.3 ждёт T26.2.
T26.4 — T26.1+T26.3. T26.5 — T26.4. T26.7 — собирает всё. T26.8 — за T26.7.
T26.9 — финал.

## 9. Принятие решений

| Решение | Выбор | Альтернативы | Обоснование |
|---|---|---|---|
| Расположение `-output` | Сиблинг `-input` | Внутри проекта / внутри сервиса | Удобно для единого `go.mod` и import-prefix |
| Обнаружение сервисов | Рекурсивный поиск `openapi.yaml` + `generation_flags.yaml` | Явный манифест / явный список в коде | Минимум церемоний, легко добавлять сервисы |
| Import-prefix per service | Автоматически: `<import-prefix>/<service>` | Явный per-service / глобальный без service | Reflects FS layout, тривиально в использовании |
| Структура спецификаций | `<project>/<service>/src/openapi/openapi.yaml` | Упрощённая `<project>/<service>/openapi.yaml` | Соответствует `../api`, место для `schemas/`/`resources/` |
| Common-проект | Обычный сервис с именем `common` | Особый сервис с конфигом | Единообразно, без special-case |
| Cross-service `$ref` | Файловые пути, любой-к-любому | Только в common / логические имена | Естественнее для OpenAPI, libopenapi умеет резолвить; ответственность за циклы — на пользователе |
| Обратная совместимость | Удаляется | Сохраняется через отдельный режим / флаг | Меньше кода, проект на ранней стадии |
| Архитектурный подход | Подход 2 (ProjectSet/ResourcesSet абстракции) | Подход 1 (лёгкий список + индекс) | Готовая дорожка для будущих расширений |
| Post-gen проверка | `go build ./...` + требование `go.mod` | Без проверки / отдельная команда | Раннее обнаружение проблем (включая import cycles) |
| Visitor pattern | Stub (T27) | Полный дизайн в этой спеке / не упоминать | Преждевременно до появления нового артефакта |
| Subpackage splitting | Stub (T28) | В этой спеке | Слишком большой scope для одной спеки |
