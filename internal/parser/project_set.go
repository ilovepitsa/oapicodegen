package parser

import "nschugorev/oapigenerator/internal/codegen/gogen"

// Project — один сервис в составе project layout'а. Содержит Folder,
// абсолютные пути к спеке и флагам, резолвнутые generation flags, и пути
// вывода. Model и Paths — доменные модели схем и операций, создаются
// фабриками CreateModel/CreatePaths.
type Project struct {
	Folder       string          // относительный путь от input ("userBackend", "common", "category/userService")
	SpecPath     string          // абсолютный путь к src/openapi/openapi.yaml
	FlagsPath    string          // абсолютный путь к generation_flags.yaml
	Features     ProjectFeatures // резолвнутые флаги (global + per-service override)
	Model        *Model          // доменная модель схем; nil до CreateModel
	Paths        *Paths          // доменная модель операций; nil до CreatePaths
	OutputDir    string          // <output>/<folder>
	ImportPrefix string          // <import-prefix>/<folder> — основа для PathImports
}

// ProjectSet — коллекция всех сервисов проекта.
type ProjectSet struct {
	Common   *Project            // nil если common не найден
	Projects []*Project          // все проекты включая common, для итерации
	ByName   map[string]*Project // индекс по Folder
}

// ByNameLookup возвращает проект по имени. Второе возвращаемое — false если не найден.
func (ps *ProjectSet) ByNameLookup(name string) (*Project, bool) {
	p, ok := ps.ByName[name]
	return p, ok
}

// CreateModel создаёт Model с заданным импортом и привязывает к Project.
// imp.Type принудительно устанавливается в LocalImport.
func (p *Project) CreateModel(imp gogen.Import) *Model {
	imp.Type = gogen.LocalImport
	m := &Model{project: p, Import: imp}
	p.Model = m
	return m
}

// CreatePaths создаёт Paths с типизированными PathImports для всех артефактов
// и привязывает к Project. basePath — Go import path корня сервиса
// (например "nschugorev/oapigenerator/go/userBackend").
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
