package parser

// Mode constants for LookupForMode. Match the conventions used by the
// generator package (internal/generator/constants.go).
const (
	ModeRequest  = "Request"
	ModeResponse = "Response"
)

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
// текущего использования ("", ModeRequest, ModeResponse). Если во владельце
// не включён split-mode, GoType возвращается как есть.
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
	case ModeRequest:
		out.GoType = r.GoType + "Request"
	case ModeResponse:
		out.GoType = r.GoType + "Response"
	}
	return &out, true
}
