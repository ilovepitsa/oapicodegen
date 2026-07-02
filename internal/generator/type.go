package generator

import (
	"strings"

	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/parser"
)

// typeMapper мапит parser.Schema → Go-тип, собирая нужные импорты.
// currentPkg — пакет, в который сейчас рендерится код ("model" / "client" / "server").
// modulePath — Go import-path корня генерируемого кода (для ссылок на model).
type typeMapper struct {
	currentPkg string
	modulePath string
	imports    []gogen.Import
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
		return "any"
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
	return strings.HasPrefix(t, "[]") || strings.HasPrefix(t, "map[") || t == "any"
}

// baseType возвращает Go-тип без учёта nullable.
func (m *typeMapper) baseType(s *parser.Schema) string {
	if s.Ref != "" {
		return m.qualifyModelType(refToName(s.Ref))
	}

	if len(s.OneOf) > 0 || len(s.AnyOf) > 0 || len(s.AllOf) > 0 {
		if s.Name != "" {
			return m.qualifyModelType(s.Name)
		}
		return "any"
	}

	if s.Type == "array" {
		if s.Items != nil {
			return "[]" + m.goType(s.Items)
		}
		return "[]any"
	}

	if s.Type == "object" && len(s.Properties) == 0 {
		if s.AdditionalProperties != nil {
			return "map[string]" + m.goType(s.AdditionalProperties)
		}
		return "map[string]any"
	}

	if s.Type == "object" && s.Name != "" {
		return m.qualifyModelType(s.Name)
	}

	if len(s.Enum) > 0 && s.Name != "" {
		return m.qualifyModelType(s.Name)
	}

	switch s.Type {
	case "string":
		switch s.Format {
		case "date-time", "date":
			m.addImport("time", "")
			return "time.Time"
		case "binary":
			return "[]byte"
		}
		return "string"
	case "integer":
		switch s.Format {
		case "int32":
			return "int32"
		case "int64":
			return "int64"
		default:
			return "int"
		}
	case "number":
		switch s.Format {
		case "float":
			return "float32"
		default:
			return "float64"
		}
	case "boolean":
		return "bool"
	}

	return "any"
}

// qualifyModelType добавляет префикс "model." и импорт, если текущий пакет
// не "model". name — Go-имя схемы (до квалификации).
func (m *typeMapper) qualifyModelType(name string) string {
	goName := goName(name)
	if m.currentPkg == "model" || m.modulePath == "" {
		return goName
	}
	m.addImport(m.modulePath+"/model", "model")
	return "model." + goName
}

func refToName(ref string) string {
	if idx := strings.LastIndex(ref, "/"); idx >= 0 {
		return ref[idx+1:]
	}
	return ref
}
