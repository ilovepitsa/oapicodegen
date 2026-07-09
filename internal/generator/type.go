package generator

import (
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/parser"
	"strings"
)

// typeMapper мапит parser.Schema → Go-тип, собирая нужные импорты.
// currentPkg — пакет, в который сейчас рендерится код ("model" / "client" / "server").
// modulePath — Go import-path корня генерируемого кода (для ссылок на model).
// mode — "" / "Request" / "Response": суффикс для splittable schema-ссылок
// при включённом GOLANG_SPLIT_REQUEST_RESPONSE.
type typeMapper struct {
	currentPkg string
	modulePath string
	utcTime    bool
	mode       string
	splittable map[string]bool
	imports    []gogen.Import
}

// newTypeMapper создаёт typeMapper с флагами из Generator.
func (g *Generator) newTypeMapper(pkg string) *typeMapper {
	return &typeMapper{
		currentPkg: pkg,
		modulePath: g.modulePath,
		utcTime:    g.features.UseUTCForDateTime.Value,
		splittable: g.splittable,
	}
}

func (m *typeMapper) addImport(path, alias string) {
	for _, imp := range m.imports {
		if imp.Path == path && imp.Alias == alias {
			return
		}
	}

	m.imports = append(m.imports, gogen.Import{Path: path, Alias: alias})
}

// goType возвращает Go-тип для поля/элемента.
// nullable=true → pointer (для примитивов и структур; slices/maps/any — без pointer).
func (m *typeMapper) goType(s *parser.Schema) string {
	if s == nil {
		return goTypeAny
	}

	base := m.baseType(s)
	if s.Nullable && !isInherentlyNilable(base) {
		return "*" + base
	}

	return base
}

// isInherentlyNilable — типы, которые уже имеют нулевое значение nil,
// поэтому оборачивать в pointer не нужно.
func isInherentlyNilable(t string) bool {
	return strings.HasPrefix(t, "[]") || strings.HasPrefix(t, "map[") || t == goTypeAny
}

// baseType возвращает Go-тип без учёта nullable.
//
//nolint:gocyclo,cyclop // early-return chain by schema type
func (m *typeMapper) baseType(s *parser.Schema) string {
	if s.Ref != "" {
		return m.qualifyModelType(refToName(s.Ref))
	}

	if len(s.OneOf) > 0 || len(s.AnyOf) > 0 || len(s.AllOf) > 0 {
		if s.Name != "" {
			return m.qualifyModelType(s.Name)
		}

		return goTypeAny
	}

	if s.Type == "array" {
		if s.Items != nil {
			return "[]" + m.goType(s.Items)
		}

		return "[]any"
	}

	if s.Type == oapiTypeObject && len(s.Properties) == 0 {
		if s.AdditionalProperties != nil {
			return "map[string]" + m.goType(s.AdditionalProperties)
		}

		return "map[string]any"
	}

	if s.Type == oapiTypeObject && s.Name != "" {
		return m.qualifyModelType(s.Name)
	}

	if len(s.Enum) > 0 && s.Name != "" {
		return m.qualifyModelType(s.Name)
	}

	return m.primitiveGoType(s)
}

// primitiveGoType мапит примитивный OpenAPI-тип (string/integer/number/boolean)
// в Go-тип с учётом format. Возвращает goTypeAny, если тип неизвестен.
func (m *typeMapper) primitiveGoType(s *parser.Schema) string {
	switch s.Type {
	case oapiTypeString:
		return m.stringGoType(s)
	case oapiTypeInteger:
		switch s.Format {
		case oapiFormatInt32:
			return oapiFormatInt32
		case oapiFormatInt64:
			return oapiFormatInt64
		default:
			return "int"
		}
	case oapiTypeNumber:
		switch s.Format {
		case "float":
			return "float32"
		default:
			return "float64"
		}
	case oapiTypeBoolean:
		return "bool"
	}

	return goTypeAny
}

// stringGoType мапит строковый OpenAPI-тип в Go-тип с учётом format и флага UTC.
func (m *typeMapper) stringGoType(s *parser.Schema) string {
	switch s.Format {
	case "date-time":
		if m.utcTime {
			return m.qualifyUTCTime()
		}

		m.addImport("time", "")

		return "time.Time"
	case "date":
		m.addImport("time", "")

		return "time.Time"
	case "binary":
		return "[]byte"
	}

	return oapiTypeString
}

// qualifyModelType добавляет префикс "model." и импорт, если текущий пакет
// не "model". name — Go-имя схемы (до квалификации).
// При включённом split и заданном mode добавляет суффикс "Request"/"Response"
// для splittable схем.
func (m *typeMapper) qualifyModelType(name string) string {
	goName := goName(name)
	if m.mode != "" && m.isSplittable(name) {
		goName += m.mode
	}

	if m.currentPkg == "model" || m.modulePath == "" {
		return goName
	}

	m.addImport(m.modulePath+"/model", "model")

	return "model." + goName
}

// isSplittable проверяет, есть ли схема в splittable-наборе Generator.
// typeMapper не имеет прямой ссылки на Generator, поэтому поле splittable
// прокидывается через typeMapper.
func (m *typeMapper) isSplittable(name string) bool {
	return m.splittable != nil && m.splittable[name]
}

// qualifyUTCTime возвращает имя UTCTime-типа для текущего пакета.
// В model — просто UTCTime; в остальных — model.UTCTime + импорт.
func (m *typeMapper) qualifyUTCTime() string {
	if m.currentPkg == "model" || m.modulePath == "" {
		return "UTCTime"
	}

	m.addImport(m.modulePath+"/model", "model")

	return "model.UTCTime"
}

func refToName(ref string) string {
	if idx := strings.LastIndex(ref, "/"); idx >= 0 {
		return ref[idx+1:]
	}

	return ref
}
