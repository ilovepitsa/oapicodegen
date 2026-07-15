package parser

// Project — один сервис в составе project layout'а. Содержит распарсенный
// Document, резолвнутые generation flags и пути вывода.
type Project struct {
	Name         string          // относительный путь от input (например "userBackend", "common")
	SpecPath     string          // абсолютный путь к src/openapi/openapi.yaml
	FlagsPath    string          // абсолютный путь к generation_flags.yaml
	Features     ProjectFeatures // резолвнутые флаги (global + per-service override)
	Document     *Document       // распарсенный spec
	OutputDir    string          // <output>/<name>
	ImportPrefix string          // <import-prefix>/<name>
}

// ProjectSet — коллекция всех сервисов проекта.
type ProjectSet struct {
	Common   *Project            // nil если common не найден
	Projects []*Project          // все проекты включая common, для итерации
	ByName   map[string]*Project // индекс по Name
}

// ByNameLookup возвращает проект по имени. Второе возвращаемое — false если не найден.
func (ps *ProjectSet) ByNameLookup(name string) (*Project, bool) {
	p, ok := ps.ByName[name]
	return p, ok
}
