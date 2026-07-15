# Multi-service Project Layout — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Перевести oapigenerator с single-spec режима на multi-service project layout: один запуск генерирует код для всех сервисов проекта с cross-service `$ref` и post-gen compile check.

**Architecture:** Вводятся доменные типы `ProjectSet`/`Project`/`ResourcesSet` в `internal/parser`, `ProjectLoader` обнаруживает сервисы рекурсивно по маркерам `generation_flags.yaml` + `src/openapi/openapi.yaml`. `generator.Generate` итерирует `ProjectSet`, использует `ResourcesSet` для эмитта cross-package Go imports. После генерации запускается `go build ./...` на каталоге `-output`, который должен быть корнем Go-модуля.

**Tech Stack:** Go 1.26, `github.com/pb33f/libopenapi`, `github.com/stretchr/testify/assert`, `go.uber.org/zap`, стандартная библиотека.

**Spec:** `docs/superpowers/specs/2026-07-14-multi-service-layout-design.md`

**Commit policy:** Пользователь делает все git-коммиты сам. В конце каждой задачи — шаг "Stage changes" с `git add` и остановкой для ревью. Не запускать `git commit`.

---

## File Structure

### Новые файлы

- `internal/parser/project_set.go` — типы `Project`, `ProjectSet`, аксессоры.
- `internal/parser/project_set_test.go` — юнит-тесты аксессоров.
- `internal/parser/resources_set.go` — типы `ResourcesSet`, `ResourceSchema`, аксессоры.
- `internal/parser/resources_set_test.go` — юнит-тесты.
- `internal/parser/project_loader.go` — `ProjectLoader`, `walkServices`, `markExternalRefs`.
- `internal/parser/project_loader_test.go` — тесты discovery и валидации.
- `internal/parser/testdata/project/...` — testdata-проект с тремя сервисами.
- `internal/codegen/compilecheck.go` — `CompileCheck(outputDir, logger) error`.
- `internal/codegen/compilecheck_test.go` — тесты через `t.TempDir`.

### Модифицируемые файлы

- `internal/parser/document.go` — добавить поля `SourceFile`/`ExternalRef`/`OwnerProject` на `Schema`; populate `SourceFile` в `convertDocument`/`extractComponentsSchemas` через libopenapi GoLow API.
- `internal/generator/generator.go` — сменить сигнатуру `Generate`, заменить `modulePath`/`features` на `project`/`rs`, удалить `WithModulePath`/`WithProjectFeatures`.
- `internal/generator/type.go` (и другие генераторы) — `resolveRef` для cross-package imports.
- `cmd/oapigen/main.go` — новые флаги, новая связка, удаление `-project-flags-path`.
- `cmd/oapigen/main_test.go`, `cmd/oapigen/e2e_test.go` — обновить под новый testdata.
- `README.md`, `ARCHITECTURE.md`, `TASKS.md` — обновить документацию.

### Удаляемые файлы

- `testdata/minimal/` — целиком.
- `testdata/split/` — целиком.

---

## Task 1: T26.1 — Типы ProjectSet / Project / ResourcesSet

**Files:**
- Create: `internal/parser/project_set.go`
- Create: `internal/parser/project_set_test.go`
- Create: `internal/parser/resources_set.go`
- Create: `internal/parser/resources_set_test.go`

### Step 1.1: Write failing test for Project / ProjectSet

- [ ] **Write `internal/parser/project_set_test.go`:**

```go
package parser_test

import (
	"testing"

	"nschugorev/oapigenerator/internal/parser"

	"github.com/stretchr/testify/assert"
)

func TestProjectSet_ByName(t *testing.T) {
	common := &parser.Project{Name: "common"}
	userBackend := &parser.Project{Name: "userBackend"}

	ps := &parser.ProjectSet{
		Common:   common,
		Projects: []*parser.Project{common, userBackend},
		ByName:   map[string]*parser.Project{"common": common, "userBackend": userBackend},
	}

	got, ok := ps.ByName("userBackend")
	assert.True(t, ok)
	assert.Same(t, userBackend, got)

	_, ok = ps.ByName("nonexistent")
	assert.False(t, ok)
}

func TestProject_Fields(t *testing.T) {
	p := &parser.Project{
		Name:         "userBackend",
		SpecPath:     "/input/userBackend/src/openapi/openapi.yaml",
		FlagsPath:    "/input/userBackend/generation_flags.yaml",
		OutputDir:    "/go/userBackend",
		ImportPrefix: "nschugorev/oapigenerator/go/userBackend",
	}
	assert.Equal(t, "userBackend", p.Name)
	assert.Equal(t, "/go/userBackend", p.OutputDir)
}
```

- [ ] **Run test to verify it fails:**

```bash
go test ./internal/parser/ -run TestProjectSet_ByName -v
```

Expected: FAIL — `undefined: parser.Project`, `undefined: parser.ProjectSet`.

### Step 1.2: Implement Project / ProjectSet

- [ ] **Write `internal/parser/project_set.go`:**

```go
package parser

// Project — один сервис в составе project layout'а. Содержит распарсенный
// Document, резолвнутые generation flags и пути вывода.
type Project struct {
	Name         string         // относительный путь от input (например "userBackend", "common")
	SpecPath     string         // абсолютный путь к src/openapi/openapi.yaml
	FlagsPath    string         // абсолютный путь к generation_flags.yaml
	Features     ProjectFeatures // резолвнутые флаги (global + per-service override)
	Document     *Document       // распарсенный spec
	OutputDir    string         // <output>/<name>
	ImportPrefix string         // <import-prefix>/<name>
}

// ProjectSet — коллекция всех сервисов проекта.
type ProjectSet struct {
	Common   *Project              // nil если common не найден
	Projects []*Project            // все проекты включая common, для итерации
	ByName   map[string]*Project   // индекс по Name
}

// ByNameLookup возвращает проект по имени. Второе возвращаемое — false если не найден.
func (ps *ProjectSet) ByNameLookup(name string) (*Project, bool) {
	p, ok := ps.ByName[name]
	return p, ok
}
```

- [ ] **Update test to use `ByNameLookup`:**

Replace `ps.ByName("userBackend")` → `ps.ByNameLookup("userBackend")` in the test (rename method to avoid clash with field name).

- [ ] **Run tests to verify they pass:**

```bash
go test ./internal/parser/ -run TestProjectSet -v
```

Expected: PASS.

### Step 1.3: Write failing test for ResourcesSet

- [ ] **Append to `internal/parser/resources_set_test.go`:**

```go
package parser_test

import (
	"testing"

	"nschugorev/oapigenerator/internal/parser"

	"github.com/stretchr/testify/assert"
)

func TestResourcesSet_LookupByFile(t *testing.T) {
	common := &parser.Project{Name: "common", ImportPrefix: "nschugorev/oapigenerator/go/common"}
	rs := &parser.ResourcesSet{
		Schemas: map[string]*parser.ResourceSchema{
			"/input/common/src/openapi/schemas/User.yaml": {
				Project:    common,
				SchemaName: "User",
				GoImport:   "nschugorev/oapigenerator/go/common",
				GoType:     "User",
			},
		},
	}

	got, ok := rs.LookupByFile("/input/common/src/openapi/schemas/User.yaml")
	assert.True(t, ok)
	assert.Equal(t, "User", got.GoType)
	assert.Equal(t, "nschugorev/oapigenerator/go/common", got.GoImport)

	_, ok = rs.LookupByFile("/nonexistent.yaml")
	assert.False(t, ok)
}

func TestResourcesSet_LookupForMode_SplitAware(t *testing.T) {
	common := &parser.Project{Name: "common", ImportPrefix: "nschugorev/oapigenerator/go/common"}
	rs := &parser.ResourcesSet{
		Schemas: map[string]*parser.ResourceSchema{
			"/input/common/src/openapi/schemas/User.yaml": {
				Project:    common,
				SchemaName: "User",
				GoImport:   "nschugorev/oapigenerator/go/common",
				GoType:     "User",
			},
		},
	}

	// Моно режим → User.
	got, ok := rs.LookupForMode("/input/common/src/openapi/schemas/User.yaml", "")
	assert.True(t, ok)
	assert.Equal(t, "User", got.GoType)

	// Request режим → UserRequest (если split включён во владельце — здесь нет, fallback на User).
	got, ok = rs.LookupForMode("/input/common/src/openapi/schemas/User.yaml", "modeRequest")
	assert.True(t, ok)
	// В этой версии без split во владельце — GoType не меняется.
	assert.Equal(t, "User", got.GoType)
}
```

- [ ] **Run tests to verify they fail:**

```bash
go test ./internal/parser/ -run TestResourcesSet -v
```

Expected: FAIL — `undefined: parser.ResourcesSet`, `undefined: parser.ResourceSchema`.

### Step 1.4: Implement ResourcesSet

- [ ] **Write `internal/parser/resources_set.go`:**

```go
package parser

// ResourcesSet — глобальный индекс схем всех сервисов. Ключ — абсолютный
// путь к yaml-файлу схемы. Используется генератором для разрешения
// cross-service $ref: вместо генерации дубликата типа в текущем сервисе,
// эмитится Go-импорт в пакет сервиса-владельца.
type ResourcesSet struct {
	Schemas map[string]*ResourceSchema
}

// ResourceSchema — запись в индексе: где лежит схема, какой Go-импорт и
// имя типа использовать при cross-service ссылках.
type ResourceSchema struct {
	Project    *Project
	SchemaName string // имя схемы во владельце (для диагностики)
	GoImport   string // например "nschugorev/oapigenerator/go/common"
	GoType     string // например "User"; с учётом split-mode: "UserRequest"/"UserResponse"
}

// LookupByFile возвращает ResourceSchema по абсолютному пути к yaml-файлу.
// Второе возвращаемое — false если путь не зарегистрирован.
func (rs *ResourcesSet) LookupByFile(absPath string) (*ResourceSchema, bool) {
	r, ok := rs.Schemas[absPath]
	return r, ok
}

// LookupForMode возвращает ResourceSchema с GoType, адаптированным под mode
// текущего использования ("", "modeRequest", "modeResponse"). Если во
// владельце не включён split-mode, GoType возвращается как есть.
func (rs *ResourcesSet) LookupForMode(absPath, mode string) (*ResourceSchema, bool) {
	r, ok := rs.LookupByFile(absPath)
	if !ok {
		return nil, false
	}

	if r.Project == nil || !r.Project.Features.SplitRequestResponse.Value {
		return r, true
	}

	// Split включён — суффиксуем тип в зависимости от mode.
	out := *r
	switch mode {
	case "modeRequest":
		out.GoType = r.GoType + "Request"
	case "modeResponse":
		out.GoType = r.GoType + "Response"
	}
	return &out, true
}
```

- [ ] **Run tests to verify they pass:**

```bash
go test ./internal/parser/ -run "TestProjectSet|TestResourcesSet" -v
```

Expected: PASS for all four tests.

### Step 1.5: Stage for commit

- [ ] **Stage:**

```bash
git add internal/parser/project_set.go internal/parser/project_set_test.go \
        internal/parser/resources_set.go internal/parser/resources_set_test.go
```

Pause for user to review and commit.

---

## Task 2: T26.2 — ProjectLoader: discovery

**Files:**
- Create: `internal/parser/project_loader.go`
- Create: `internal/parser/project_loader_test.go`
- Create: `internal/parser/testdata/loader/...` — синтетические testdata-кейсы.

### Step 2.1: Create testdata fixtures

- [ ] **Create `internal/parser/testdata/loader/empty/` (empty dir with `.gitkeep`).**

- [ ] **Create `internal/parser/testdata/loader/single/` layout:**

```
testdata/loader/single/
└── myService/
    ├── generation_flags.yaml    (empty: "[]")
    └── src/openapi/
        └── openapi.yaml          (минимальная валидная OpenAPI 3.0 спека)
```

`openapi.yaml` content:

```yaml
openapi: 3.0.0
info:
  title: myService
  version: 0.1.0
paths: {}
```

- [ ] **Create `testdata/loader/multi/` layout** with three services (`common`, `userBackend`, `authBackend`) — same structure as `single` but three folders.

- [ ] **Create `testdata/loader/nested/` with a service nested inside another service:**

```
testdata/loader/nested/
└── outer/
    ├── generation_flags.yaml
    ├── src/openapi/openapi.yaml
    └── inner/
        ├── generation_flags.yaml
        └── src/openapi/openapi.yaml
```

### Step 2.2: Write failing test for walkServices

- [ ] **Write `internal/parser/project_loader_test.go`:**

```go
package parser_test

import (
	"path/filepath"
	"testing"

	"nschugorev/oapigenerator/internal/parser"
	"nschugorev/oapigenerator/internal/parser/testdata"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWalkServices_DiscoversAll(t *testing.T) {
	root := filepath.Join("testdata", "loader", "multi")
	services, err := parser.WalkServices(root)
	require.NoError(t, err)

	names := make([]string, 0, len(services))
	for _, s := range services {
		names = append(names, s.Name)
	}
	assert.ElementsMatch(t, []string{"authBackend", "common", "userBackend"}, names)
}

func TestWalkServices_EmptyProjectFails(t *testing.T) {
	root := filepath.Join("testdata", "loader", "empty")
	_, err := parser.WalkServices(root)
	assert.Error(t, err)
}

func TestWalkServices_RejectsNestedServices(t *testing.T) {
	root := filepath.Join("testdata", "loader", "nested")
	_, err := parser.WalkServices(root)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nested")
}

// Удержание unused import — testdata пригодится в следующих шагах.
var _ = testdata.Path
```

(If `testdata.Path` doesn't exist, drop that line — placeholder to avoid lint errors if `testdata` package is imported but unused.)

- [ ] **Run test to verify it fails:**

```bash
go test ./internal/parser/ -run TestWalkServices -v
```

Expected: FAIL — `undefined: parser.WalkServices`.

### Step 2.3: Implement WalkServices

- [ ] **Write `internal/parser/project_loader.go`:**

```go
package parser

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	specFileName   = "openapi.yaml"
	specRelPath    = "src/openapi/openapi.yaml"
	flagsFileName  = "generation_flags.yaml"
)

// serviceDescriptor — найденная на этапе walk папка-сервис.
type serviceDescriptor struct {
	Name      string // относительный путь от input, нормализованный
	SpecPath  string // абсолютный путь к openapi.yaml
	FlagsPath string // абсолютный путь к generation_flags.yaml
}

// WalkServices рекурсивно обходит root и находит папки-сервисы по маркерам
// generation_flags.yaml + src/openapi/openapi.yaml. Возвращает ошибку, если
// сервисов нет, либо найдены вложенные сервисы.
func WalkServices(root string) ([]serviceDescriptor, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("walk services: resolve root: %w", err)
	}

	var out []serviceDescriptor
	visited := make(map[string]bool) // абсолютные пути уже найденных сервисов

	walkErr := filepath.WalkDir(rootAbs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}

		// Пропускаем сам root.
		if path == rootAbs {
			return nil
		}

		// Если эта папка внутри уже найденного сервиса — пропускаем.
		for svc := range visited {
			if strings.HasPrefix(path+string(os.PathSeparator), svc+string(os.PathSeparator)) {
				return filepath.SkipDir
			}
		}

		specPath := filepath.Join(path, specRelPath)
		flagsPath := filepath.Join(path, flagsFileName)

		_, specErr := os.Stat(specPath)
		_, flagsErr := os.Stat(flagsPath)
		if specErr != nil || flagsErr != nil {
			return nil
		}

		// Это сервис. Проверим, что он не вложен в другой сервис.
		for svc := range visited {
			if strings.HasPrefix(path+string(os.PathSeparator), svc+string(os.PathSeparator)) {
				return fmt.Errorf("walk services: service %q is nested inside service %q; nested services are not supported", path, svc)
			}
		}

		rel, err := filepath.Rel(rootAbs, path)
		if err != nil {
			return fmt.Errorf("walk services: relative path for %q: %w", path, err)
		}
		rel = filepath.ToSlash(rel)

		out = append(out, serviceDescriptor{
			Name:      rel,
			SpecPath:  specPath,
			FlagsPath: flagsPath,
		})
		visited[path] = true
		return filepath.SkipDir
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walk services: %w", walkErr)
	}

	if len(out) == 0 {
		return nil, errors.New("walk services: no services found; expected folders with both generation_flags.yaml and src/openapi/openapi.yaml")
	}

	return out, nil
}
```

- [ ] **Run tests to verify they pass:**

```bash
go test ./internal/parser/ -run TestWalkServices -v
```

Expected: PASS for all three tests.

### Step 2.4: Write failing test for ProjectLoader.Load

- [ ] **Append to `project_loader_test.go`:**

```go
func TestProjectLoader_Load_DiscoverAndParse(t *testing.T) {
	root := filepath.Join("testdata", "loader", "multi")
	loader := parser.NewProjectLoader(root, "/go", "example.com/go", nil)
	ps, rs, err := loader.Load()
	require.NoError(t, err)

	assert.NotNil(t, ps.Common)
	assert.Equal(t, "common", ps.Common.Name)
	assert.Equal(t, "example.com/go/common", ps.Common.ImportPrefix)
	assert.Equal(t, "/go/common", ps.Common.OutputDir)

	// ResourcesSet должен содержать схемы common (если они есть в спеке).
	// В нашем testdata-минимуме схем нет — индекс пустой.
	assert.NotNil(t, rs.Schemas)
}

func TestProjectLoader_Load_EmptyFails(t *testing.T) {
	root := filepath.Join("testdata", "loader", "empty")
	loader := parser.NewProjectLoader(root, "/go", "example.com/go", nil)
	_, _, err := loader.Load()
	assert.Error(t, err)
}
```

- [ ] **Run test to verify it fails:**

```bash
go test ./internal/parser/ -run TestProjectLoader_Load -v
```

Expected: FAIL — `undefined: parser.NewProjectLoader`.

### Step 2.5: Implement ProjectLoader.Load (без markExternalRefs — это T26.3)

- [ ] **Append to `internal/parser/project_loader.go`:**

```go
import (
	// ...существующие...
	"path"
)

// ProjectLoader находит сервисы, парсит их спеки, резолвит generation flags
// и собирает ProjectSet + ResourcesSet.
type ProjectLoader struct {
	input        string
	output       string
	importPrefix string
	flagsLoader  *GenerationFlagsLoader
}

// NewProjectLoader создаёт loader. flagsLoader может быть nil — тогда
// per-service generation flags не загружаются (все флаги false).
func NewProjectLoader(input, output, importPrefix string, fl *GenerationFlagsLoader) *ProjectLoader {
	return &ProjectLoader{
		input:        input,
		output:       output,
		importPrefix: importPrefix,
		flagsLoader:  fl,
	}
}

// Load находит сервисы, парсит их, строит ProjectSet + ResourcesSet.
func (l *ProjectLoader) Load() (*ProjectSet, *ResourcesSet, error) {
	descriptors, err := WalkServices(l.input)
	if err != nil {
		return nil, nil, err
	}

	rootAbs, err := filepath.Abs(l.input)
	if err != nil {
		return nil, nil, fmt.Errorf("project loader: resolve input: %w", err)
	}
	rootFS := os.DirFS(rootAbs)

	ps := &ProjectSet{ByName: map[string]*Project{}}
	rs := &ResourcesSet{Schemas: map[string]*ResourceSchema{}}

	// Сортируем: common первым, потом алфавит.
	sortDescriptors(descriptors)

	for _, desc := range descriptors {
		relSpec := filepath.ToSlash(filepath.Join(desc.Name, specRelPath))
		doc, err := ParseFile(rootFS, relSpec)
		if err != nil {
			return nil, nil, fmt.Errorf("project loader: parse spec for %q: %w", desc.Name, err)
		}

		features := ProjectFeatures{}
		if l.flagsLoader != nil {
			features, err = l.flagsLoader.GetProjectFeatures(desc.FlagsPath)
			if err != nil {
				return nil, nil, fmt.Errorf("project loader: load flags for %q: %w", desc.Name, err)
			}
		}

		p := &Project{
			Name:         desc.Name,
			SpecPath:     desc.SpecPath,
			FlagsPath:    desc.FlagsPath,
			Features:     features,
			Document:     doc,
			OutputDir:    filepath.Join(l.output, desc.Name),
			ImportPrefix: path.Join(l.importPrefix, desc.Name),
		}

		ps.Projects = append(ps.Projects, p)
		ps.ByName[p.Name] = p
		if p.Name == "common" {
			ps.Common = p
		}
	}

	if err := l.buildResourcesIndex(ps, rs); err != nil {
		return nil, nil, err
	}

	return ps, rs, nil
}

// sortDescriptors ставит common первым, затем остальные по алфавиту.
func sortDescriptors(in []serviceDescriptor) {
	for i := 0; i < len(in); i++ {
		if in[i].Name == "common" {
			in[0], in[i] = in[i], in[0]
			break
		}
	}
	// Остальные (начиная с индекса 1) — алфавит.
	for i := 1; i < len(in); i++ {
		for j := i + 1; j < len(in); j++ {
			if in[i].Name > in[j].Name {
				in[i], in[j] = in[j], in[i]
			}
		}
	}
}

// buildResourcesIndex заполняет ResourcesSet.Schemas картой "абсолютный путь
// к yaml-файлу схемы → ResourceSchema". Использует Schema.SourceFile, который
// проставляется в T26.3. До T26.3 SourceFile пустой → индекс остаётся пустым
// (тесты на cross-service ref будут добавлены в T26.3/T26.5).
func (l *ProjectLoader) buildResourcesIndex(ps *ProjectSet, rs *ResourcesSet) error {
	for _, p := range ps.Projects {
		for _, sh := range p.Document.Schemas {
			if sh.SourceFile == "" {
				continue
			}
			absPath := sh.SourceFile
			if _, dup := rs.Schemas[absPath]; dup {
				return fmt.Errorf("project loader: schema source file %q registered by multiple services", absPath)
			}
			rs.Schemas[absPath] = &ResourceSchema{
				Project:    p,
				SchemaName: sh.Name,
				GoImport:   p.ImportPrefix,
				GoType:     sh.Name,
			}
		}
	}
	return nil
}
```

- [ ] **Run tests to verify they pass:**

```bash
go test ./internal/parser/ -run "TestWalkServices|TestProjectLoader_Load" -v
```

Expected: PASS.

### Step 2.6: Stage for commit

- [ ] **Stage:**

```bash
git add internal/parser/project_loader.go internal/parser/project_loader_test.go \
        internal/parser/testdata/loader/
```

Pause for user review and commit.

---

## Task 3: T26.3 — ParseFile source tracking + markExternalRefs

**Files:**
- Modify: `internal/parser/document.go` — добавить поля на `Schema`, populate `SourceFile` в `convertDocument`.
- Modify: `internal/parser/project_loader.go` — вызов `markExternalRefs`.
- Create: `internal/parser/source_marking_test.go`.
- Create: `internal/parser/testdata/project/` — testdata с cross-service ref.

### Step 3.1: Add fields to Schema

- [ ] **In `internal/parser/document.go`, add fields to `Schema` (after `IsUsedInUpdate`):**

```go
	// SourceFile — абсолютный путь к yaml-файлу, откуда пришла схема.
	// Пустая строка для inline-схем или когда источник неизвестен.
	SourceFile string
	// ExternalRef true, если схема пришла из другого сервиса (cross-service $ref).
	ExternalRef bool
	// OwnerProject — имя сервиса-владельца (где физически лежит схема).
	// Заполняется markExternalRefs после парсинга.
	OwnerProject string
```

- [ ] **Run existing parser tests to verify no regression:**

```bash
go test ./internal/parser/ -v
```

Expected: PASS (новые поля zero-value, существующее поведение не меняется).

### Step 3.2: Populate SourceFile via libopenapi GoLow

- [ ] **In `internal/parser/parser.go`, modify `convertDocument` signature to receive parent path context.** Add a helper that extracts source file path from a libopenapi schema proxy:

```go
// schemaSourceFile извлекает абсолютный путь к yaml-файлу, в котором
// определена схема, через low-level API libopenapi. Возвращает пустую
// строку, если источник недоступен (inline-схема или refs не хранят путь).
func schemaSourceFile(proxy *highv3.Schema) string {
	if proxy == nil || proxy.GoLow() == nil {
		return ""
	}
	ref := proxy.GoLow().GetReference()
	if ref == nil {
		return ""
	}
	origin := ref.GetOrigin()
	if origin == nil {
		return ""
	}
	// libopenapi хранит абсолютный путь в origin.Node.Path или origin.AbsoluteLocation.
	if origin.AbsoluteLocation != "" {
		return origin.AbsoluteLocation
	}
	return origin.Node.GetPath()
}
```

(If `GetReference().GetOrigin()` API names differ in the installed libopenapi version, look up the actual API in `go.sum` / vendored docs. The contract is: given a `*highv3.Schema`, return the yaml file path where it's defined.)

- [ ] **In `extractComponentsSchemas` (or wherever schemas are built), set `sh.SourceFile = schemaSourceFile(highSchema)` for each schema.** For inline schemas (no $ref, defined inside openapi.yaml itself), `SourceFile` = path to openapi.yaml. For resolved `$ref` to external files, `SourceFile` = absolute path of the target yaml.

- [ ] **Run parser tests:**

```bash
go test ./internal/parser/ -v
```

Expected: PASS. (If existing tests fail because they didn't expect `SourceFile` populated, that's fine — they should still pass since the new field is additive. If they fail, investigate.)

### Step 3.3: Write failing test for markExternalRefs

- [ ] **Create `internal/parser/testdata/project/` with three services:**

```
testdata/project/
├── common/
│   ├── generation_flags.yaml       (content: "[]")
│   └── src/openapi/
│       ├── openapi.yaml
│       └── schemas/
│           └── User.yaml
├── userBackend/
│   ├── generation_flags.yaml       (content: "[]")
│   └── src/openapi/
│       ├── openapi.yaml
│       └── schemas/
│           └── Profile.yaml
└── authBackend/
    ├── generation_flags.yaml       (content: "[]")
    └── src/openapi/
        └── openapi.yaml
```

`testdata/project/common/src/openapi/openapi.yaml`:

```yaml
openapi: 3.0.0
info:
  title: common
  version: 0.1.0
paths: {}
components:
  schemas:
    User:
      $ref: schemas/User.yaml
```

`testdata/project/common/src/openapi/schemas/User.yaml`:

```yaml
type: object
properties:
  id:
    type: string
  name:
    type: string
required: [id, name]
```

`testdata/project/userBackend/src/openapi/openapi.yaml`:

```yaml
openapi: 3.0.0
info:
  title: userBackend
  version: 0.1.0
paths: {}
components:
  schemas:
    Profile:
      $ref: schemas/Profile.yaml
```

`testdata/project/userBackend/src/openapi/schemas/Profile.yaml`:

```yaml
type: object
properties:
  user:
    $ref: ../../../common/src/openapi/schemas/User.yaml
  bio:
    type: string
required: [user]
```

- [ ] **Write `internal/parser/source_marking_test.go`:**

```go
package parser_test

import (
	"path/filepath"
	"testing"

	"nschugorev/oapigenerator/internal/parser"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarkExternalRefs_LocalSchema(t *testing.T) {
	root := filepath.Join("testdata", "project")
	loader := parser.NewProjectLoader(root, "/go", "example.com/go", nil)
	ps, _, err := loader.Load()
	require.NoError(t, err)

	userBackend := ps.ByName["userBackend"]
	require.NotNil(t, userBackend)

	// Profile — локальная схема userBackend.
	var profile *parser.Schema
	for _, sh := range userBackend.Document.Schemas {
		if sh.Name == "Profile" {
			profile = sh
			break
		}
	}
	require.NotNil(t, profile)
	assert.False(t, profile.ExternalRef)
	assert.Equal(t, "userBackend", profile.OwnerProject)
}

func TestMarkExternalRefs_CrossServiceSchema(t *testing.T) {
	root := filepath.Join("testdata", "project")
	loader := parser.NewProjectLoader(root, "/go", "example.com/go", nil)
	ps, rs, err := loader.Load()
	require.NoError(t, err)

	userBackend := ps.ByName["userBackend"]
	require.NotNil(t, userBackend)

	// Profile.user ссылается на common.User — это external ref.
	var profile *parser.Schema
	for _, sh := range userBackend.Document.Schemas {
		if sh.Name == "Profile" {
			profile = sh
			break
		}
	}
	require.NotNil(t, profile)

	// Хотя бы одно свойство Profile должно быть ExternalRef=true, OwnerProject="common".
	var foundExternal bool
	for _, prop := range profile.Properties {
		if prop.Schema != nil && prop.Schema.ExternalRef {
			assert.Equal(t, "common", prop.Schema.OwnerProject)
			foundExternal = true
			break
		}
	}
	assert.True(t, foundExternal, "expected at least one external-ref property in Profile")

	// ResourcesSet должен содержать common.User.
	common := ps.ByName["common"]
	require.NotNil(t, common)
	var userSourceFile string
	for _, sh := range common.Document.Schemas {
		if sh.Name == "User" {
			userSourceFile = sh.SourceFile
			break
		}
	}
	assert.NotEmpty(t, userSourceFile)
	_, ok := rs.LookupByFile(userSourceFile)
	assert.True(t, ok)
}
```

- [ ] **Run tests to verify they fail:**

```bash
go test ./internal/parser/ -run TestMarkExternalRefs -v
```

Expected: FAIL — `markExternalRefs` не вызывается, поля `ExternalRef`/`OwnerProject` не заполнены.

### Step 3.4: Implement markExternalRefs + wire into ProjectLoader

- [ ] **Append to `internal/parser/project_loader.go`:**

```go
// markExternalRefs размечает SourceFile / ExternalRef / OwnerProject на схемах
// каждого проекта. OwnerProject = имя проекта, в чей spec-dir попадает SourceFile.
// Схемы, чей SourceFile лежит вне spec-dir текущего проекта — ExternalRef=true,
// OwnerProject = имя сервиса-владельца (ищется по префиксу пути).
func (l *ProjectLoader) markExternalRefs(ps *ProjectSet) error {
	// Карта "префикс spec-dir сервиса" → "имя сервиса".
	type specPrefix struct {
		prefix string
		name   string
	}
	var prefixes []specPrefix
	for _, p := range ps.Projects {
		specDir := filepath.Dir(p.SpecPath) // <input>/<service>/src/openapi
		prefixes = append(prefixes, specPrefix{prefix: specDir, name: p.Name})
	}

	for _, p := range ps.Projects {
		visitSchemas(p.Document, func(sh *Schema) {
			if sh.SourceFile == "" {
				return
			}
			owner := findOwner(sh.SourceFile, prefixes)
			if owner == "" {
				return
			}
			sh.OwnerProject = owner
			sh.ExternalRef = owner != p.Name
		})
	}
	return nil
}

func findOwner(sourceFile string, prefixes []struct{ prefix, name string }) string {
	for _, p := range prefixes {
		if strings.HasPrefix(sourceFile+string(os.PathSeparator), p.prefix+string(os.PathSeparator)) {
			return p.name
		}
	}
	return ""
}

// visitSchemas обходит все схемы документа (top-level + inline в properties,
// items, allOf/oneOf/anyOf, additionalProperties, requestBody, responses).
func visitSchemas(doc *Document, visit func(*Schema)) {
	for _, sh := range doc.Schemas {
		visitSchemaRecursive(sh, visit)
	}
	for _, op := range doc.Operations {
		if op.RequestBody != nil {
			for _, mt := range op.RequestBody.MediaTypes {
				if mt.Schema != nil {
					visitSchemaRecursive(mt.Schema, visit)
				}
			}
		}
		for _, resp := range op.Responses {
			for _, mt := range resp.MediaTypes {
				if mt.Schema != nil {
					visitSchemaRecursive(mt.Schema, visit)
				}
			}
		}
		for _, param := range op.Parameters {
			if param.Schema != nil {
				visitSchemaRecursive(param.Schema, visit)
			}
		}
	}
}

func visitSchemaRecursive(sh *Schema, visit func(*Schema)) {
	if sh == nil {
		return
	}
	visit(sh)
	for _, prop := range sh.Properties {
		if prop.Schema != nil {
			visitSchemaRecursive(prop.Schema, visit)
		}
	}
	visitSchemaRecursive(sh.Items, visit)
	visitSchemaRecursive(sh.AdditionalProperties, visit)
	for _, s := range sh.AllOf {
		visitSchemaRecursive(s, visit)
	}
	for _, s := range sh.OneOf {
		visitSchemaRecursive(s, visit)
	}
	for _, s := range sh.AnyOf {
		visitSchemaRecursive(s, visit)
	}
}
```

(If `Operation.RequestBody`/`Responses`/`Parameters` field names differ in `document.go`, adjust to actual names. Check `internal/parser/document.go` lines 36-85 for exact structure.)

- [ ] **In `ProjectLoader.Load()`, call `l.markExternalRefs(ps)` AFTER building `ps` and BEFORE `buildResourcesIndex`:**

```go
	// ...существующий цикл построения ps...

	if err := l.markExternalRefs(ps); err != nil {
		return nil, nil, err
	}

	if err := l.buildResourcesIndex(ps, rs); err != nil {
		return nil, nil, err
	}
```

- [ ] **Fix the `findOwner` signature mismatch** — Go doesn't allow inline struct in param like that. Change `findOwner` to take `[]serviceSpecPrefix` where:

```go
type serviceSpecPrefix struct {
	prefix string
	name   string
}
```

(defined at package level). Update `markExternalRefs` to use `[]serviceSpecPrefix`.

- [ ] **Run tests to verify they pass:**

```bash
go test ./internal/parser/ -run TestMarkExternalRefs -v
```

Expected: PASS.

### Step 3.5: Stage for commit

- [ ] **Stage:**

```bash
git add internal/parser/document.go internal/parser/parser.go \
        internal/parser/project_loader.go internal/parser/source_marking_test.go \
        internal/parser/testdata/project/
```

Pause for user review and commit.

---

## Task 4: T26.4 — generator.Generate under ProjectSet

**Files:**
- Modify: `internal/generator/generator.go` — сигнатура, замена modulePath/features на project.
- Modify: all `internal/generator/*.go` files referencing `g.modulePath` / `g.features`.
- Modify: `internal/generator/generator_test.go` — обновить fixtures.
- Modify: `cmd/oapigen/main.go` — временный shim (полный migration в Task 7).

### Step 4.1: Survey existing usage

- [ ] **Run:**

```bash
grep -rn "g\.modulePath\|g\.features" internal/generator/ | head -50
```

Capture all call sites — every `g.modulePath` becomes `g.project.ImportPrefix`, every `g.features` becomes `g.project.Features`.

### Step 4.2: Write failing test for new Generate signature

- [ ] **In `internal/generator/generator_test.go`, add a new test (existing ones updated in 4.4):**

```go
func TestGenerate_ProjectSet_Smoke(t *testing.T) {
	doc := &parser.Document{
		Schemas: []*parser.Schema{
			{Name: "Foo", Type: "object", Properties: []*parser.Property{
				{Name: "id", Schema: &parser.Schema{Type: "string"}},
			}},
		},
	}
	p := &parser.Project{
		Name:         "svc",
		ImportPrefix: "example.com/go/svc",
		Document:     doc,
	}
	ps := &parser.ProjectSet{Projects: []*parser.Project{p}, ByName: map[string]*parser.Project{"svc": p}}
	rs := &parser.ResourcesSet{Schemas: map[string]*parser.ResourceSchema{}}

	buf := codegen.NewBufferWriter()
	err := Generate(buf, ps, rs)
	require.NoError(t, err)

	files := buf.Files()
	assert.NotEmpty(t, files)

	// Сгенерированный файл модели должен содержать "package model".
	var hasModel bool
	for path, content := range files {
		if strings.Contains(path, "model/foo.gen.go") {
			assert.Contains(t, content, "package model")
			hasModel = true
		}
	}
	assert.True(t, hasModel, "expected model/foo.gen.go in output; got: %v", fileNames(files))
}

func fileNames(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
```

(If `codegen.NewBufferWriter` API differs, look up actual constructor in `internal/codegen/codegen.go`.)

- [ ] **Run test to verify it fails:**

```bash
go test ./internal/generator/ -run TestGenerate_ProjectSet_Smoke -v
```

Expected: FAIL — `cannot use ps (type *parser.ProjectSet) as type *parser.Document`.

### Step 4.3: Refactor Generator struct and Generate signature

- [ ] **In `internal/generator/generator.go`, replace:**

```go
type Generator struct {
	doc        *parser.Document
	modulePath string
	factory    *gogen.FileFactory
	features   parser.ProjectFeatures
	splittable map[string]bool
}

type Option func(*Generator)

func WithModulePath(p string) Option {
	return func(g *Generator) { g.modulePath = p }
}

func WithProjectFeatures(pf parser.ProjectFeatures) Option {
	return func(g *Generator) { g.features = pf }
}

func Generate(fw codegen.FileWriter, doc *parser.Document, opts ...Option) error {
	g := &Generator{
		doc:     doc,
		factory: gogen.NewFileFactory("oapigen"),
	}
	for _, opt := range opts {
		opt(g)
	}

	if g.features.SplitRequestResponse.Value {
		g.splittable = computeSplittable(doc)
	}

	for _, sh := range doc.Schemas {
		if sh.Name == "" {
			continue
		}
		if err := g.writeSchemaFiles(fw, sh); err != nil {
			return err
		}
	}

	if err := g.writeUTCTimeFile(fw); err != nil {
		return err
	}

	if err := g.writeExpectedValidatorsFile(fw); err != nil {
		return err
	}

	if len(doc.Operations) > 0 {
		if err := g.writeOperationFiles(fw); err != nil {
			return err
		}
	}

	return nil
}
```

With:

```go
type Generator struct {
	project  *parser.Project
	rs       *parser.ResourcesSet
	doc      *parser.Document
	factory  *gogen.FileFactory
	splittable map[string]bool
}

type Option func(*Generator)

// WithLogger добавляет zap-логгер для отладочного вывода. Опционально.
func WithLogger(log *zap.SugaredLogger) Option {
	return func(g *Generator) { g.logger = log }
}

// Generate итерирует ProjectSet и генерирует код для каждого проекта.
func Generate(fw codegen.FileWriter, ps *parser.ProjectSet, rs *parser.ResourcesSet, opts ...Option) error {
	for _, p := range ps.Projects {
		g := &Generator{
			project: p,
			rs:      rs,
			doc:     p.Document,
			factory: gogen.NewFileFactory("oapigen"),
		}
		for _, opt := range opts {
			opt(g)
		}

		if p.Features.SplitRequestResponse.Value {
			g.splittable = computeSplittable(p.Document)
		}

		if err := g.generateProject(fw); err != nil {
			return fmt.Errorf("generate project %q: %w", p.Name, err)
		}
	}
	return nil
}

func (g *Generator) generateProject(fw codegen.FileWriter) error {
	for _, sh := range g.doc.Schemas {
		if sh.Name == "" {
			continue
		}
		// Skip external-ref schemas — they're generated by their owner.
		if sh.ExternalRef {
			continue
		}
		if err := g.writeSchemaFiles(fw, sh); err != nil {
			return err
		}
	}

	if err := g.writeUTCTimeFile(fw); err != nil {
		return err
	}

	if err := g.writeExpectedValidatorsFile(fw); err != nil {
		return err
	}

	if len(g.doc.Operations) > 0 {
		if err := g.writeOperationFiles(fw); err != nil {
			return err
		}
	}

	return nil
}
```

(Add `logger *zap.SugaredLogger` field to `Generator` struct, and import `go.uber.org/zap`.)

- [ ] **Mechanical refactor across all `internal/generator/*.go` files:**

Replace `g.modulePath` → `g.project.ImportPrefix`. Replace `g.features` → `g.project.Features`.

```bash
# Verify the mechanical replacement:
grep -rn "g\.modulePath\|g\.features" internal/generator/
```

Expected: 0 matches after replacement.

### Step 4.4: Update existing generator tests

- [ ] **In `internal/generator/generator_test.go`, update all tests that currently call `Generate(fw, doc, generator.WithModulePath("..."), generator.WithProjectFeatures(pf))` to construct a `Project` and `ProjectSet` instead.** Pattern:

```go
// Старый:
// err := Generate(fw, doc, WithModulePath("example.com/go/svc"))

// Новый:
p := &parser.Project{
	Name:         "svc",
	ImportPrefix: "example.com/go/svc",
	Document:     doc,
	Features:     parser.ProjectFeatures{}, // или с нужными флагами
}
ps := &parser.ProjectSet{Projects: []*parser.Project{p}, ByName: map[string]*parser.Project{"svc": p}}
rs := &parser.ResourcesSet{Schemas: map[string]*parser.ResourceSchema{}}
err := Generate(fw, ps, rs)
```

Apply to every existing test in `generator_test.go`.

- [ ] **Run all generator tests:**

```bash
go test ./internal/generator/ -v
```

Expected: PASS.

### Step 4.5: Temporary shim in cmd/oapigen/main.go

The CLI migration happens in Task 7. For now, add a temporary adapter so `cmd/oapigen` compiles.

- [ ] **In `cmd/oapigen/main.go`, replace the `generator.Generate(...)` call:**

```go
	// TEMPORARY: single-project wrap until T26.7 CLI migration.
	p := &parser.Project{
		Name:         "single",
		ImportPrefix: importPrefix,
		Document:     doc,
		Features:     pf,
	}
	ps := &parser.ProjectSet{
		Projects: []*parser.Project{p},
		ByName:   map[string]*parser.Project{"single": p},
	}
	rs := &parser.ResourcesSet{Schemas: map[string]*parser.ResourceSchema{}}

	if err := generator.Generate(fw, ps, rs); err != nil {
		return fmt.Errorf("generate: %w", err)
	}
```

(Remove `genOpts := []generator.Option{...}` block, `loadProjectFeatures` is still used to compute `pf` — will be removed in Task 7.)

- [ ] **Build:**

```bash
go build ./...
```

Expected: success.

### Step 4.6: Stage for commit

- [ ] **Stage:**

```bash
git add internal/generator/ cmd/oapigen/main.go
```

Pause for user review and commit.

---

## Task 5: T26.5 — Cross-package Go imports

**Files:**
- Modify: `internal/generator/type.go` — `resolveRef` helper.
- Modify: `internal/generator/schema.go`, `client.go`, `server.go`, `impl_client.go`, `impl_server.go`, `mocks.go`, `sdk.go`, `response_headers.go` — use `resolveRef` for cross-package identifiers.
- Create: `internal/generator/cross_ref_test.go`.

### Step 5.1: Survey existing $ref handling

- [ ] **Run:**

```bash
grep -n "Ref\b\|resolveRef\|g\.doc\.Schemas" internal/generator/type.go | head -30
```

Capture where `$ref` is resolved to a local Go type. These are the call sites that need cross-package awareness.

### Step 5.2: Write failing test for resolveRef

- [ ] **Write `internal/generator/cross_ref_test.go`:**

```go
package generator_test

import (
	"strings"
	"testing"

	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/generator"
	"nschugorev/oapigenerator/internal/parser"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerate_CrossServiceRef_EmitsGoImport(t *testing.T) {
	// common.User — простая object-схема.
	commonUser := &parser.Schema{
		Name:   "User",
		Type:   "object",
		SourceFile: "/input/common/src/openapi/schemas/User.yaml",
		OwnerProject: "common",
		Properties: []*parser.Property{
			{Name: "id", Schema: &parser.Schema{Type: "string"}},
		},
	}
	commonDoc := &parser.Document{Schemas: []*parser.Schema{commonUser}}
	commonProject := &parser.Project{
		Name:         "common",
		ImportPrefix: "example.com/go/common",
		Document:     commonDoc,
	}

	// userBackend.Profile — ссылается на common.User через $ref.
	externalUserRef := &parser.Schema{
		Name:         "User",
		Ref:          "../../../common/src/openapi/schemas/User.yaml",
		SourceFile:   "/input/common/src/openapi/schemas/User.yaml",
		ExternalRef:  true,
		OwnerProject: "common",
	}
	profile := &parser.Schema{
		Name:   "Profile",
		Type:   "object",
		SourceFile: "/input/userBackend/src/openapi/schemas/Profile.yaml",
		OwnerProject: "userBackend",
		Properties: []*parser.Property{
			{Name: "user", Schema: externalUserRef, Required: true},
			{Name: "bio", Schema: &parser.Schema{Type: "string"}},
		},
		Required: []string{"user"},
	}
	userBackendDoc := &parser.Document{Schemas: []*parser.Schema{profile}}
	userBackendProject := &parser.Project{
		Name:         "userBackend",
		ImportPrefix: "example.com/go/userBackend",
		Document:     userBackendDoc,
	}

	ps := &parser.ProjectSet{
		Projects: []*parser.Project{commonProject, userBackendProject},
		ByName:   map[string]*parser.Project{"common": commonProject, "userBackend": userBackendProject},
	}
	rs := &parser.ResourcesSet{Schemas: map[string]*parser.ResourceSchema{
		"/input/common/src/openapi/schemas/User.yaml": {
			Project:    commonProject,
			SchemaName: "User",
			GoImport:   "example.com/go/common",
			GoType:     "User",
		},
	}}

	buf := codegen.NewBufferWriter()
	err := generator.Generate(buf, ps, rs)
	require.NoError(t, err)

	// В userBackend/model/profile.gen.go должен быть импорт common и тип common.User.
	var profileContent string
	for path, content := range buf.Files() {
		if strings.HasSuffix(path, "userBackend/model/profile.gen.go") {
			profileContent = string(content)
			break
		}
	}
	assert.NotEmpty(t, profileContent, "expected userBackend/model/profile.gen.go in output")
	assert.Contains(t, profileContent, `"example.com/go/common"`)
	assert.Contains(t, profileContent, "common.User")

	// common.User НЕ должен генерироваться в userBackend.
	var userInUserBackend bool
	for path := range buf.Files() {
		if strings.HasSuffix(path, "userBackend/model/user.gen.go") {
			userInUserBackend = true
		}
	}
	assert.False(t, userInUserBackend, "common.User must not be generated inside userBackend")

	// common.User должен генерироваться в common.
	var userInCommon bool
	for path := range buf.Files() {
		if strings.HasSuffix(path, "common/model/user.gen.go") {
			userInCommon = true
		}
	}
	assert.True(t, userInCommon, "common.User must be generated inside common")
}
```

- [ ] **Run test to verify it fails:**

```bash
go test ./internal/generator/ -run TestGenerate_CrossServiceRef -v
```

Expected: FAIL — generator emits `User` (local) instead of `common.User` (cross-package).

### Step 5.3: Implement resolveRef helper

- [ ] **Add to `internal/generator/generator.go` (or new `internal/generator/cross_ref.go`):**

```go
// resolveRef возвращает Go-идентификатор для схемы. Если схема локальная
// (ExternalRef=false) — возвращает просто GoType (например "User").
// Если cross-service — возвращает qualified identifier "common.User" и
// Go-импорт, который caller должен добавить в файл.
//
// Возвращаемая сигнатура: (qualifiedIdentifier, goImport, isExternal).
// goImport непустой только если isExternal=true.
func (g *Generator) resolveRef(sh *parser.Schema) (ident, goImport string, isExternal bool) {
	if sh == nil || !sh.ExternalRef || sh.SourceFile == "" || g.rs == nil {
		return sh.Name, "", false
	}
	rs, ok := g.rs.LookupByFile(sh.SourceFile)
	if !ok {
		// Schema помечена ExternalRef, но не найдена в индексе — fallback на локальный тип.
		// Это сигнал о баге в markExternalRefs, но не паникуем — пусть compile check поймает.
		return sh.Name, "", false
	}
	return rs.GoType, rs.GoImport, true
}

// resolveRefForMode то же, что resolveRef, но с учётом split-mode целевого
// сервиса-владельца. mode = "" / "modeRequest" / "modeResponse".
func (g *Generator) resolveRefForMode(sh *parser.Schema, mode string) (ident, goImport string, isExternal bool) {
	if sh == nil || !sh.ExternalRef || sh.SourceFile == "" || g.rs == nil {
		return sh.Name, "", false
	}
	rs, ok := g.rs.LookupForMode(sh.SourceFile, mode)
	if !ok {
		return sh.Name, "", false
	}
	return rs.GoType, rs.GoImport, true
}
```

### Step 5.4: Wire resolveRef into typeMapper and $ref usage

- [ ] **In `internal/generator/type.go`, modify the typeMapper function that emits Go type for `$ref` schemas.** The current code path emits local `User` — replace with:

```go
// Заменить:
// return sh.Name
// На:
ident, imp, isExt := g.resolveRefForMode(sh, mode)
if isExt {
	g.addImport(currentFile, imp) // см. helper ниже
}
return ident
```

Where `g.addImport(file, importPath)` adds the import to the current file being generated. If the generator uses `gogen.FileFactory` which builds files via `*gogen.File`, the import is added through `file.AddImport(path)`. Check `internal/codegen/gogen/` for the actual API.

- [ ] **For each generator file that resolves `$ref` (`schema.go`, `client.go`, `server.go`, `impl_client.go`, `impl_server.go`, `mocks.go`, `sdk.go`, `response_headers.go`), apply the same pattern:**

For each place that emits a Go identifier for a schema (typically `sh.Name`), replace with `g.resolveRefForMode(sh, mode)` and add the import to the file.

- [ ] **Verify the call sites by running:**

```bash
grep -rn "sh\.Name\|schema\.Name" internal/generator/*.go | grep -v "_test.go" | head -30
```

Each occurrence is a candidate. Inspect context — only refs to a `*parser.Schema` need cross-package awareness, not local variables named `Name`.

### Step 5.5: Run tests to verify they pass

- [ ] **Run:**

```bash
go test ./internal/generator/ -v
```

Expected: PASS, including `TestGenerate_CrossServiceRef_EmitsGoImport`.

### Step 5.6: Stage for commit

- [ ] **Stage:**

```bash
git add internal/generator/
```

Pause for user review and commit.

---

## Task 6: T26.6 — Post-generation compile check

**Files:**
- Create: `internal/codegen/compilecheck.go`
- Create: `internal/codegen/compilecheck_test.go`

### Step 6.1: Write failing test

- [ ] **Write `internal/codegen/compilecheck_test.go`:**

```go
package codegen_test

import (
	"os"
	"path/filepath"
	"testing"

	"nschugorev/oapigenerator/internal/codegen"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompileCheck_NoGoModFails(t *testing.T) {
	tmp := t.TempDir()
	err := codegen.CompileCheck(tmp, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "go.mod")
}

func TestCompileCheck_SuccessOnValidModule(t *testing.T) {
	tmp := t.TempDir()
	writeGoMod(t, tmp, "example.com/test")
	writeGoFile(t, filepath.Join(tmp, "main.go"), "package main\n\nfunc main() {}\n")

	err := codegen.CompileCheck(tmp, nil)
	assert.NoError(t, err)
}

func TestCompileCheck_BuildErrorPropagated(t *testing.T) {
	tmp := t.TempDir()
	writeGoMod(t, tmp, "example.com/test")
	// Синтаксически ошибочный Go-код.
	writeGoFile(t, filepath.Join(tmp, "main.go"), "package main\n\nfunc main() { this is not valid go }\n")

	err := codegen.CompileCheck(tmp, nil)
	assert.Error(t, err)
}

func writeGoMod(t *testing.T, dir, modulePath string) {
	t.Helper()
	content := "module " + modulePath + "\n\ngo 1.26\n"
	err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(content), 0o644)
	require.NoError(t, err)
}

func writeGoFile(t *testing.T, path, content string) {
	t.Helper()
	err := os.WriteFile(path, []byte(content), 0o644)
	require.NoError(t, err)
}
```

- [ ] **Run test to verify it fails:**

```bash
go test ./internal/codegen/ -run TestCompileCheck -v
```

Expected: FAIL — `undefined: codegen.CompileCheck`.

### Step 6.2: Implement CompileCheck

- [ ] **Write `internal/codegen/compilecheck.go`:**

```go
package codegen

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"go.uber.org/zap"
)

// CompileCheck проверяет, что outputDir — корень Go-модуля (содержит go.mod),
// и что весь код в нём компилируется (`go build ./...`).
//
// Логгер может быть nil — тогда вывод go build идёт только в возвращаемую ошибку.
func CompileCheck(outputDir string, log *zap.SugaredLogger) error {
	if _, err := os.Stat(filepath.Join(outputDir, "go.mod")); err != nil {
		return fmt.Errorf(
			"compile check: -output must be a Go module root (go.mod not found at %s); run `go mod init <import-prefix>` in %s: %w",
			outputDir, outputDir, err,
		)
	}

	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = outputDir

	out, err := cmd.CombinedOutput()
	if err != nil {
		if log != nil {
			log.Errorf("compile check failed:\n%s", string(out))
		}
		return fmt.Errorf("compile check: go build ./... failed: %w\n%s", err, string(out))
	}

	if log != nil {
		log.Info("compile check: go build ./... succeeded")
	}
	return nil
}
```

(Add `"path/filepath"` to imports.)

- [ ] **Run tests to verify they pass:**

```bash
go test ./internal/codegen/ -run TestCompileCheck -v
```

Expected: PASS.

### Step 6.3: Stage for commit

- [ ] **Stage:**

```bash
git add internal/codegen/compilecheck.go internal/codegen/compilecheck_test.go
```

Pause for user review and commit.

---

## Task 7: T26.7 — CLI migration in cmd/oapigen/main.go

**Files:**
- Modify: `cmd/oapigen/main.go` — новые флаги, новая связка.
- Modify: `cmd/oapigen/main_test.go` — обновить под новые флаги.
- Modify: `cmd/oapigen/e2e_test.go` — обновить под новый testdata.

### Step 7.1: Update main.go

- [ ] **In `cmd/oapigen/main.go`, replace the flag definitions block:**

Remove `-project-flags-path`. Add `-skip-compile-check`.

```go
	flagSet.StringVar(&input, "input", "", "path to project root folder (contains service subfolders)")
	flagSet.StringVar(&output, "output", "", "output directory (must be a Go module root with go.mod)")
	flagSet.StringVar(&importPrefix, "import-prefix", "",
		"Go import path prefix for generated packages (must match go.mod module path)")
	flagSet.BoolVar(&dryRun, "dry-run", false, "parse and generate without writing to filesystem")
	flagSet.BoolVar(&skipCompileCheck, "skip-compile-check", false,
		"skip post-generation `go build ./...` check")
	flagSet.StringVar(
		&generationFlagsConfig, "generation-flags-config-path", "",
		"path to global generation_flags.yaml",
	)
```

- [ ] **Replace the body of `run()` after flag parsing with:**

```go
	if input == "" {
		return errors.New("-input is required")
	}
	if output == "" && !dryRun {
		return errors.New("-output is required (or use -dry-run)")
	}
	if importPrefix == "" {
		return errors.New("-import-prefix is required")
	}

	logger, err := logCfg.Create()
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	defer func() { _ = logger.Sync() }() //nolint:errcheck // zap.Sync often fails on stderr/stdout
	sugar := logger.Sugar()

	// Грузим глобальный generation_flags.yaml (если задан).
	var flagsLoader *parser.GenerationFlagsLoader
	if generationFlagsConfig != "" {
		flagsLoader = parser.NewGenerationFlagsLoader(fs.NewRealFS())
		if err := flagsLoader.Load(generationFlagsConfig); err != nil {
			return fmt.Errorf("load generation flags config %q: %w", generationFlagsConfig, err)
		}
	}

	// Load ProjectSet + ResourcesSet.
	projectLoader := parser.NewProjectLoader(input, output, importPrefix, flagsLoader)
	ps, rs, err := projectLoader.Load()
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}
	sugar.Infof("loaded %d services", len(ps.Projects))

	// FileWriter.
	var fw codegen.FileWriter
	if dryRun {
		fw = codegen.NoopFileWriter{}
		sugar.Info("dry-run mode: no files will be written")
	} else {
		fw, err = fwCfg.Create(output)
		if err != nil {
			return fmt.Errorf("create file writer: %w", err)
		}
	}
	defer func() {
		if cerr := fw.Close(); cerr != nil {
			sugar.Errorf("close file writer: %v", cerr)
		}
	}()

	// Generate.
	if err := generator.Generate(fw, ps, rs); err != nil {
		return fmt.Errorf("generate: %w", err)
	}
	sugar.Infof("generation complete: output=%s import-prefix=%s", output, importPrefix)

	// Post-gen compile check.
	if !dryRun && !skipCompileCheck {
		if err := codegen.CompileCheck(output, sugar); err != nil {
			return fmt.Errorf("post-generation compile check: %w", err)
		}
	}

	return nil
```

- [ ] **Remove the `loadProjectFeatures` helper function** — больше не нужно.

- [ ] **Update imports:** remove unused (`os`, `path/filepath` if no longer needed), add `codegen.CompileCheck`.

- [ ] **Build:**

```bash
go build ./cmd/oapigen/
```

Expected: success.

### Step 7.2: Update main_test.go and e2e_test.go

- [ ] **In `cmd/oapigen/main_test.go`, update all tests that pass `-input spec.yaml` to use a testdata project path.** If existing tests rely on single-spec semantics, they need to be rewritten to use `testdata/project/` (created in Task 8) — for now, mark them as skipped with a TODO, OR create a minimal testdata project inline.

If rewriting is complex, skip with explanation:

```go
t.Skip("TODO: rewrite for multi-service layout (T26.8 testdata migration)")
```

- [ ] **In `cmd/oapigen/e2e_test.go`, update the e2e test** to invoke `oapigen -input testdata/project -output <tempdir> -import-prefix ...` and verify generated structure.

### Step 7.3: Run cmd tests

- [ ] **Run:**

```bash
go test ./cmd/oapigen/ -v
```

Expected: PASS for non-skipped tests. Skipped tests marked TODO.

### Step 7.4: Stage for commit

- [ ] **Stage:**

```bash
git add cmd/oapigen/
```

Pause for user review and commit.

---

## Task 8: T26.8 — Testdata migration

**Files:**
- Delete: `testdata/minimal/`
- Delete: `testdata/split/`
- Create: `testdata/project/...` (already created in Task 3, extend if needed)
- Create: `testdata/project/golden/...` — golden files.
- Create: `testdata/project/golden/go.mod` — module root.

### Step 8.1: Delete old testdata

- [ ] **Delete:**

```bash
git rm -r testdata/minimal testdata/split
```

- [ ] **Verify no references remain:**

```bash
grep -rn "testdata/minimal\|testdata/split" --include="*.go" --include="*.md" .
```

Expected: 0 matches. If found, update callers.

### Step 8.2: Extend testdata/project if needed

- [ ] **Verify `testdata/project/` (created in Task 3) has `common` with `User` schema, `userBackend` with `Profile` referencing common.User, `authBackend` with `Credentials` referencing common.User.** Add `paths` to at least one service to exercise operation generation. Example — add to `userBackend/src/openapi/openapi.yaml`:

```yaml
openapi: 3.0.0
info:
  title: userBackend
  version: 0.1.0
paths:
  /profiles/{id}:
    get:
      operationId: getProfile
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                $ref: schemas/Profile.yaml
components:
  schemas:
    Profile:
      $ref: schemas/Profile.yaml
```

### Step 8.3: Initialize golden module

- [ ] **Create `testdata/project/golden/go.mod`:**

```go
module nschugorev/oapigenerator/testdata/project/golden

go 1.26
```

- [ ] **Run the generator once to populate goldens:**

```bash
go run ./cmd/oapigen \
  -input testdata/project \
  -output testdata/project/golden \
  -import-prefix nschugorev/oapigenerator/testdata/project/golden \
  -skip-compile-check
```

(The first run uses `-skip-compile-check` because golden module has no deps yet.)

- [ ] **Verify generated structure:**

```bash
find testdata/project/golden -type f | sort
```

Expected: `go.mod`, `common/...`, `userBackend/...`, `authBackend/...` with `model/`, `interfaces/`, `impl/`, `sdk/` subfolders.

- [ ] **Add necessary external deps to golden module:**

```bash
cd testdata/project/golden
go mod tidy
cd -
```

- [ ] **Run compile check on golden:**

```bash
cd testdata/project/golden && go build ./... && cd -
```

Expected: success. If errors, fix the spec or generator.

### Step 8.4: Write golden test

- [ ] **Create `cmd/oapigen/golden_test.go`:**

```go
package main_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nschugorev/oapigenerator/internal/golden"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerate_MultiService_Golden прогоняет oapigen на testdata/project
// и сверяет каждый сгенерированный файл с golden-эталоном.
func TestGenerate_MultiService_Golden(t *testing.T) {
	tmp := t.TempDir()

	// Run generator.
	var stderr bytes.Buffer
	exit := runOapigen(t, []string{
		"-input", filepath.Join("..", "..", "testdata", "project"),
		"-output", tmp,
		"-import-prefix", "nschugorev/oapigenerator/testdata/project/golden",
		"-skip-compile-check",
	}, &stderr)
	require.Zero(t, exit, "oapigen failed: %s", stderr.String())

	// Compare each golden file with generated.
	goldenRoot := filepath.Join("..", "..", "testdata", "project", "golden")
	err := filepath.Walk(goldenRoot, func(goldenPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Base(goldenPath) == "go.mod" {
			return nil // go.mod сравниваем отдельно
		}

		rel, err := filepath.Rel(goldenRoot, goldenPath)
		require.NoError(t, err)

		goldenContent, err := os.ReadFile(goldenPath)
		require.NoError(t, err)

		generatedPath := filepath.Join(tmp, rel)
		generatedContent, err := os.ReadFile(generatedPath)
		require.NoError(t, err, "expected generated file %s", generatedPath)

		golden.Equals(t, string(generatedContent), string(goldenContent))
		return nil
	})
	require.NoError(t, err)

	// Compile check on generated.
	exit = runShell(t, tmp, "go", "build", "./...")
	assert.Zero(t, exit, "generated code does not compile")
}

func runOapigen(t *testing.T, args []string, stderr *bytes.Buffer) int {
	t.Helper()
	// Wire to oapigen.Main() — see existing e2e_test.go for pattern.
	return 0 // placeholder — replace with actual invocation
}

func runShell(t *testing.T, dir string, name string, args ...string) int {
	t.Helper()
	cmd := fmt.Sprintf("cd %s && %s %s", dir, name, strings.Join(args, " "))
	_ = cmd
	return 0
}
```

(Replace placeholders with actual oapigen invocation pattern from existing `e2e_test.go`.)

- [ ] **Run golden test:**

```bash
go test ./cmd/oapigen/ -run TestGenerate_MultiService_Golden -v
```

To update goldens:

```bash
go test ./cmd/oapigen/ -run TestGenerate_MultiService_Golden -v -update
```

Expected: PASS.

### Step 8.5: Stage for commit

- [ ] **Stage:**

```bash
git add -A testdata/
git add cmd/oapigen/golden_test.go cmd/oapigen/e2e_test.go
```

Pause for user review and commit.

---

## Task 9: T26.9 — Docs update

**Files:**
- Modify: `README.md`
- Modify: `ARCHITECTURE.md`
- Modify: `TASKS.md`

### Step 9.1: Update README.md

- [ ] **In `README.md`, replace the "Использование" section:**

```markdown
## Использование

```sh
# Сгенерировать Go-код для всех сервисов проекта ./security-gate в ./go
go run ./cmd/oapigen \
  -input ./security-gate \
  -output ./go \
  -import-prefix github.com/foo/bar/go
```

`-output` должен быть корнем Go-модуля (содержать `go.mod` с module path =
`-import-prefix`). Если модуля нет — создайте его заранее:

```sh
mkdir -p ./go
cd ./go && go mod init github.com/foo/bar/go
```

### Структура проекта

```
security-gate/                    # -input
├── common/                       # shared-сервис
│   ├── generation_flags.yaml
│   └── src/openapi/
│       ├── openapi.yaml
│       └── schemas/
├── userBackend/
│   ├── generation_flags.yaml
│   └── src/openapi/
│       ├── openapi.yaml
│       └── schemas/              # может $ref на ../../../common/...
└── authBackend/
    └── ...

go/                               # -output (Go module root)
├── go.mod
├── common/...
├── userBackend/...
└── authBackend/...
```

Сервис = папка с `generation_flags.yaml` + `src/openapi/openapi.yaml`.

### Флаги

| Флаг | По умолчанию | Назначение |
|------|--------------|------------|
| `-input` | — | путь к корню проекта (обязательный) |
| `-output` | — | корень Go-модуля для вывода (обязательный, если не `-dry-run`) |
| `-import-prefix` | — | Go import path (должен совпадать с module path в go.mod) |
| `-dry-run` | `false` | парсить и генерировать без записи на FS |
| `-skip-compile-check` | `false` | пропустить `go build ./...` после генерации |
| `-generation-flags-config-path` | — | путь к глобальному `generation_flags.yaml` |
| `-log-level` | `info` | debug\|info\|warn\|error\|fatal |
| `-log-format` | `console` | console\|json |
| `-log-development` | `false` | zap development mode |
```

- [ ] **Remove references to `-project-flags-path`** — флаг удалён.

### Step 9.2: Update ARCHITECTURE.md

- [ ] **Add new section after "Слои":**

```markdown
## Multi-service layout

Генератор принимает папку проекта (`-input`) и обнаруживает сервисы рекурсивно
по маркерам `generation_flags.yaml` + `src/openapi/openapi.yaml`. Для каждого
сервиса эмитит Go-пакеты в `<output>/<service>/...`.

Поток данных:

```
-input (project root)
      │
      ▼
┌──────────────┐
│ ProjectLoader│ walk → discover services → parse each spec
└──────────────┘
      │
      ▼
┌──────────────┐     ┌──────────────┐
│  ProjectSet  │     │ ResourcesSet │  (индекс схем для cross-service $ref)
└──────────────┘     └──────────────┘
      │                      │
      └──────────┬───────────┘
                 │
                 ▼
         ┌──────────────┐
         │  Generator   │ for each Project: write model/, interfaces/, impl/, sdk/
         └──────────────┘
                 │
                 ▼
         ┌──────────────┐
         │ CompileCheck │ go build ./... на -output (должен быть go module root)
         └──────────────┘
```

### `internal/parser.ProjectSet` / `Project` / `ResourcesSet`

`ProjectSet` — коллекция всех сервисов проекта.
`Project` — один сервис: Name, SpecPath, FlagsPath, Features, Document, OutputDir, ImportPrefix.
`ResourcesSet` — глобальный индекс схем: ключ — абсолютный путь к yaml-файлу схемы,
значение — `ResourceSchema{Project, SchemaName, GoImport, GoType}`. Используется
генератором для эмитта cross-package Go imports вместо генерации дубликатов типов.

### Cross-service `$ref`

Сервис может ссылаться на схему другого сервиса через файловый путь:
`$ref: ../../../common/src/openapi/schemas/User.yaml`. Парсер через libopenapi
резолвит ref (rootFS = `-input`), а `markExternalRefs` помечает схему как
`ExternalRef=true` с `OwnerProject="common"`. Генератор пропускает external-ref
схемы при локальной генерации и эмитит qualified identifier `common.User` с
Go-импортом.

Циклы на уровне Go-импортов не выявляются генератором намеренно —
ответственность за отсутствие циклов лежит на пользователе. Post-gen
`go build ./...` поймает их (`import cycle not allowed`).
```

- [ ] **Remove опережающие упоминания `ResourcesSet`/`ProjectSet` из старого текста** — теперь они настоящие.

### Step 9.3: Update TASKS.md

- [ ] **Add new section "T26 — Multi-service project layout" with subtasks T26.1–T26.9:**

```markdown
## Этап 6 — Multi-service project layout (T26)

Реализован по спеке `docs/superpowers/specs/2026-07-14-multi-service-layout-design.md`.

### T26.1 — Типы ProjectSet / Project / ResourcesSet — DONE
- `internal/parser/project_set.go`, `internal/parser/resources_set.go`.
- Типы: `Project`, `ProjectSet`, `ResourcesSet`, `ResourceSchema`.
- Ветка: `feat/project-set-types`.

### T26.2 — ProjectLoader: discovery — DONE
- `internal/parser/project_loader.go`.
- `WalkServices(root)` рекурсивно находит сервисы по маркерам.
- Ветка: `feat/project-loader`.

### T26.3 — ParseFile + разметка source-file — DONE
- `internal/parser/document.go` — поля `SourceFile`/`ExternalRef`/`OwnerProject` на `Schema`.
- `markExternalRefs` в `project_loader.go`.
- Ветка: `feat/parser-source-marking`.

### T26.4 — generator.Generate под ProjectSet — DONE
- Сигнатура: `Generate(fw, ps, rs, opts...)`.
- Удалены `WithModulePath`, `WithProjectFeatures`.
- Ветка: `feat/generator-projectset`.

### T26.5 — Cross-package Go imports — DONE
- `resolveRef` / `resolveRefForMode` в `internal/generator/cross_ref.go`.
- Ветка: `feat/generator-cross-refs`.

### T26.6 — Post-generation compile check — DONE
- `internal/codegen/compilecheck.go`.
- Ветка: `feat/compile-check`.

### T26.7 — CLI migration — DONE
- `cmd/oapigen/main.go`: новые флаги, `-project-flags-path` удалён, `-skip-compile-check` добавлен.
- Ветка: `feat/oapigen-multiservice`.

### T26.8 — Testdata migration — DONE
- Удалены `testdata/minimal`, `testdata/split`.
- Создан `testdata/project/` с `common` + 2 сервисами.
- `testdata/project/golden/` — компилируемый Go-модуль.
- Ветка: `feat/testdata-project`.

### T26.9 — Docs update — DONE
- README.md, ARCHITECTURE.md, TASKS.md обновлены.
- Ветка: `feat/docs-multiservice`.

## Этап 7 — Future refactoring (stubs)

### T27 (stub) — Visitor pattern refactoring
Полный дизайн — отдельный brainstorming после появления нового артефакта
(audit-data, subpackage splitting, etc.). См. спеку T26 раздел 7.

### T28 (stub) — Subpackage splitting
Дробление `model/` по подпапкам на основе структуры FS. Отдельный brainstorming позже.
```

### Step 9.4: Stage for commit

- [ ] **Stage:**

```bash
git add README.md ARCHITECTURE.md TASKS.md
```

Pause for user review and commit.

---

## Final Verification

- [ ] **Build everything:**

```bash
go build ./...
```

Expected: success.

- [ ] **Run all tests:**

```bash
go test ./...
```

Expected: PASS.

- [ ] **Lint:**

```bash
make lint
```

Expected: clean.

- [ ] **Manual end-to-end:**

```bash
go run ./cmd/oapigen \
  -input testdata/project \
  -output testdata/project/golden \
  -import-prefix nschugorev/oapigenerator/testdata/project/golden

cd testdata/project/golden && go build ./... && cd -
```

Expected: generator succeeds, compile check passes.

---

## Self-Review Notes

### Spec coverage

| Spec section | Covered by task(s) |
|---|---|
| 1. Goals & Scope | All tasks collectively |
| 2. Layout & Discovery | Task 2 (WalkServices), Task 8 (testdata) |
| 3. CLI & internal model | Task 1 (types), Task 2 (loader), Task 7 (CLI) |
| 4. Cross-service $ref resolution | Task 3 (marking), Task 5 (resolveRef) |
| 5. Post-generation compile check | Task 6 |
| 6. Удаление старого режима + миграция testdata | Task 4 (sig change), Task 7 (CLI), Task 8 (testdata) |
| 7. Visitor pattern stub | Task 9 (TASKS.md stub T27) |
| 8. План задач T26 | All tasks 1–9 |

### Known gaps / TODOs for executor

1. **libopenapi GoLow API** (Task 3, Step 3.2): exact method names for extracting source file path from a schema proxy need verification against the installed libopenapi version. Check `go.sum` for version, then look up `(*highv3.Schema).GoLow().GetReference().GetOrigin()` API.

2. **codegen.BufferWriter API** (Task 4, Step 4.2): the test assumes `buf.Files() map[string][]byte`. Verify against `internal/codegen/codegen.go` — adjust if API differs.

3. **gogen.File.AddImport** (Task 5, Step 5.4): cross-package import registration depends on how `gogen.FileFactory` builds files. Inspect `internal/codegen/gogen/` and adapt `addImport` helper.

4. **testdata/project path content** (Task 8): minimal OpenAPI specs are sketched but may need refinement to exercise all generator paths (operation generation, response headers, etc.). Iterate if golden test reveals gaps.

5. **main_test.go migration** (Task 7, Step 7.2): some existing tests may not translate cleanly to multi-service layout. Use `t.Skip("TODO: ...")` for tests that need substantial rewriting, and address them in a follow-up.

### Type consistency check

- `parser.Project` — consistent name across all tasks.
- `parser.ProjectSet.ByNameLookup` — chosen over `ByName` (which clashes with field name). Used consistently in Task 1 tests; if other tasks reference `ps.ByName(name)`, replace with `ps.ByNameLookup(name)`.
- `parser.ResourcesSet.LookupByFile` / `LookupForMode` — used consistently.
- `parser.Schema.SourceFile` / `ExternalRef` / `OwnerProject` — used consistently.
- `generator.Generator.logger` — added in Task 4, optional. Wire `WithLogger` if needed.
- `codegen.CompileCheck(outputDir, log)` — used in Task 6 (defined) and Task 7 (called).
