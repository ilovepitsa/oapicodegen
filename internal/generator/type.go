package generator

import (
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/parser"
	"strings"
)

// typeMapper мапит parser.Schema → Go-тип, собирая нужные импорты.
// currentPkg — пакет, в который сейчас рендерится код ("model" / "client" / "server").
// modelImport — Go import-path model-пакета (для ссылок на model из других пакетов).
// Пустой Path означает, что импорт не нужен (внутри model-пакета).
// mode — "" / "Request" / "Response": суффикс для splittable schema-ссылок
// при включённом GOLANG_SPLIT_REQUEST_RESPONSE.
// schemaIndex — глобальный индекс схем для разрешения cross-service $ref.
type typeMapper struct {
	currentPkg  string
	modelImport gogen.Import
	utcTime     bool
	mode        string
	splittable  map[string]bool
	schemaIndex *parser.SchemaIndex
	imports     []gogen.Import
}

// newTypeMapper создаёт typeMapper с флагами из Generator.
func (g *Generator) newTypeMapper(pkg string) *typeMapper {
	var modelImp gogen.Import
	if g.project != nil {
		modelImp = g.project.Paths.Imports.Model
	}

	return &typeMapper{
		currentPkg:  pkg,
		modelImport: modelImp,
		utcTime:     g.project.Features.UseUTCForDateTime.Value,
		splittable:  g.splittable,
		schemaIndex: g.schemaIndex,
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
	if s.ExternalRef != "" {
		return m.qualifyExternalType(s.ExternalRef)
	}

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
		if s.AdditionalPropertiesFalse {
			return "struct{}"
		}

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
		case oapiFormatFloat:
			return goTypeFloat32
		default:
			return goTypeFloat64
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

	if m.currentPkg == "model" || m.modelImport.Path == "" {
		return goName
	}

	m.addImport(m.modelImport.Path, "model")

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
	if m.currentPkg == "model" || m.modelImport.Path == "" {
		return "UTCTime"
	}

	m.addImport(m.modelImport.Path, "model")

	return "model.UTCTime"
}

func refToName(ref string) string {
	if idx := strings.LastIndex(ref, "/"); idx >= 0 {
		return ref[idx+1:]
	}

	return ref
}

// externalRefSeparator — разделитель между abs-путём файла и именем схемы
// в ExternalRef. Должен совпадать с parser.schemaRefSeparator.
const externalRefSeparator = "#/components/schemas/"

// qualifyExternalType разрешает cross-service $ref через SchemaIndex и
// возвращает Go-тип вида "<alias>.<Type>". Добавляет импорт model-пакета
// сервиса-владельца. Если SchemaIndex не задан или схема не найдена —
// возвращает goTypeAny (fallback).
//
// externalRef имеет вид "<absPath>#/components/schemas/<Name>", где absPath —
// абсолютный путь к openapi.yaml сервиса-владельца.
func (m *typeMapper) qualifyExternalType(externalRef string) string {
	if m.schemaIndex == nil {
		return goTypeAny
	}

	idx := strings.Index(externalRef, externalRefSeparator)
	if idx < 0 {
		return goTypeAny
	}

	absPath := externalRef[:idx]
	schemaName := externalRef[idx+len(externalRefSeparator):]

	entry, ok := m.schemaIndex.LookupForMode(absPath, schemaName, m.mode)
	if !ok {
		return goTypeAny
	}

	alias := crossServiceAlias(entry.GoImport)
	m.addImport(entry.GoImport+"/model", alias)

	return alias + "." + entry.GoType
}

// crossServiceAlias генерирует Go-alias для импорта model-пакета другого
// сервиса. Использует последний компонент import path, приведённый к нижнему
// регистру (Go convention для имён пакететов).
func crossServiceAlias(goImport string) string {
	if idx := strings.LastIndex(goImport, "/"); idx >= 0 {
		return strings.ToLower(goImport[idx+1:])
	}

	return strings.ToLower(goImport)
}
