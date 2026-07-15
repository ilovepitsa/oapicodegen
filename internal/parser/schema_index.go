package parser

// Mode-константы для LookupForMode. Единственный источник истины для
// суффиксов split-mode ("Request"/"Response"); пакет generator алиасит
// их через modeRequest/modeResponse.
const (
	ModeRequest  = "Request"
	ModeResponse = "Response"
)

// SchemaIndex — глобальный индекс схем всех сервисов. Ключ — абсолютный
// путь к yaml-файлу схемы. Используется генератором для разрешения
// cross-service $ref: вместо генерации дубликата типа в текущем сервисе,
// эмитится Go-импорт в пакет сервиса-владельца.
type SchemaIndex struct {
	Schemas map[string]*SchemaEntry
}

// SchemaEntry — запись в индексе: где лежит схема, какой Go-импорт и
// имя типа использовать при cross-service ссылках.
type SchemaEntry struct {
	Project    *Project
	SchemaName string // имя схемы во владельце (для диагностики)
	GoImport   string // например "nschugorev/oapigenerator/go/common"
	GoType     string // например "User"; с учётом split-mode: "UserRequest"/"UserResponse"
}

// LookupByFile возвращает SchemaEntry по абсолютному пути к yaml-файлу.
// Второе возвращаемое — false если путь не зарегистрирован.
func (si *SchemaIndex) LookupByFile(absPath string) (*SchemaEntry, bool) {
	e, ok := si.Schemas[absPath]

	return e, ok
}

// LookupForMode возвращает SchemaEntry с GoType, адаптированным под mode
// текущего использования ("", ModeRequest, ModeResponse). Если во владельце
// не включён split-mode, GoType возвращается как есть.
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
