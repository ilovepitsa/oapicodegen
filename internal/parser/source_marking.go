package parser

import (
	"path/filepath"
	"strings"
)

// buildSchemaIndex наполняет SchemaIndex записями из всех top-level схем
// всех проектов ProjectSet. Ключ — absPath + "#/components/schemas/" +
// schemaName. GoImport — ImportPrefix проекта, GoType — имя схемы.
func buildSchemaIndex(si *SchemaIndex, ps *ProjectSet) {
	if si == nil || ps == nil {
		return
	}

	if si.Schemas == nil {
		si.Schemas = map[string]*SchemaEntry{}
	}

	for _, project := range ps.Projects {
		addProjectSchemas(si, project)
	}
}

func addProjectSchemas(si *SchemaIndex, project *Project) {
	if project == nil || project.Model == nil {
		return
	}

	for _, s := range project.Model.schemas {
		if s == nil || s.Name == "" || s.SourceFile == "" {
			continue
		}

		key := schemaIndexKey(s.SourceFile, s.Name)
		si.Schemas[key] = &SchemaEntry{
			Project:    s.OwnerProject,
			SchemaName: s.Name,
			GoImport:   project.ImportPrefix,
			GoType:     s.Name,
			SubPackage: s.SubPackage,
		}
	}
}

// markExternalRefs размечает source-marking поля в схемах проекта:
//
//   - SourceFile — абсолютный путь к spec-файлу проекта, выставляется на
//     каждой top-level схеме (components.schemas).
//   - OwnerProject — проект-владелец, выставляется на каждой top-level схеме.
//   - ExternalRef — выставляется на вложенных схемах (properties, items,
//     allOf, ...), чей $ref указывает на схему ДРУГОГО сервиса. Содержит
//     абсолютный путь к целевому openapi.yaml сервиса-владельца + фрагмент.
//
// specPath — абсолютный путь к openapi.yaml проекта. Используется как
// SourceFile и как база для разрешения относительных external $ref.
//
// Внутрисервисные cross-file $ref (схема из соседнего файла того же проекта,
// например ./UserStatus.yaml#/UserStatus внутри User.yaml) НЕ помечаются как
// external: rolodex libopenapi уже резолвит их в полную схему, а целевая схема
// присутствует в project.Model и разрешается генератором через qualifyModelType
// (с квалификацией subpackage). Ранее такие ref помечались ExternalRef'ом,
// резолвленным относительно корневого specPath (неверная база) с фрагментом
// #/<Name> вместо #/components/schemas/<Name> — qualifyExternalType не находил
// их в SchemaIndex и поле падало в any. Признак внутрисервисности — целевая
// схема найдена в project.Model по имени (Model.Lookup).
func markExternalRefs(project *Project, specPath string) {
	if project == nil || project.Model == nil {
		return
	}

	for _, s := range project.Model.schemas {
		markTopLevel(s, project, specPath)
		// База для resolution nested $ref — SourceFile top-level схемы (файл,
		// где физически лежат поля схемы), а НЕ specPath (openapi.yaml). Ref'ы
		// пишутся относительно файла схемы (User.yaml), и resolveExternalRef
		// относительно specPath уходит на неверную глубину (см. bug-report
		// cross-service). Для inline top-level схем SourceFile == specPath.
		markNestedRefs(s, project, s.SourceFile)
	}
}

// markTopLevel выставляет SourceFile и OwnerProject на top-level схеме.
// Если схема загружена из внешнего файла через $ref, SourceFile берётся из Ref
// (резолвится относительно specPath). Для inline-схем используется specPath.
//
// После вычисления SourceFile $ref очищается: для top-level схемы это source-
// pointer (откуда загружена), а не type-reference. Без очистки baseType (через
// AliasRenderer.GoType) трактует $ref как type-ref → `type UUIDv7 UUIDv7`
// (рекурсия) или пустой стаб (isAliasLike excludes sh.Ref). Field-схемы
// сохраняют свой Ref — он нужен для type-resolution.
func markTopLevel(s *Schema, project *Project, specPath string) {
	if s == nil {
		return
	}

	s.OwnerProject = project

	// Схема загружена из внешнего файла через $ref — резолвим SourceFile.
	if s.Ref != "" && !strings.HasPrefix(s.Ref, "#/") {
		s.SourceFile = resolveExternalRef(s.Ref, specPath)
		// Убираем фрагмент (#/Address) — SourceFile только путь к файлу.
		if idx := strings.Index(s.SourceFile, "#"); idx >= 0 {
			s.SourceFile = s.SourceFile[:idx]
		}
		s.Ref = ""
		return
	}

	s.SourceFile = specPath
}

// markNestedRefs обходит вложенные схемы и выставляет ExternalRef на
// тех, чей $ref указывает на схему другого сервиса (cross-service).
// Внутрисервисные cross-file ref пропускаются — см. комментарий markExternalRefs.
func markNestedRefs(s *Schema, project *Project, specPath string) {
	if s == nil {
		return
	}

	walkNested(s, project, specPath)
}

// walkNested рекурсивно обходит вложенные схемы, пропуская top-level
// (вызывается после markTopLevel). Для каждой схемы с $ref выставляет
// ExternalRef, только если целевая схема НЕ найдена в project.Model
// (т.е. $ref указывает на другой сервис). Внутрисервисные ref оставляются
// без ExternalRef — генератор резолвит их через qualifyModelType.
func walkNested(s *Schema, project *Project, specPath string) {
	if s == nil {
		return
	}

	if s.Ref != "" {
		name := refToSchemaName(s.Ref)
		_, isLocal := project.Model.Lookup(name)

		// Внутрисервисный cross-file $ref: целевая схема есть в этом проекте.
		// rolodex уже резолвил её; генератор использует локальный тип. Не
		// выставляем ExternalRef (иначе qualifyExternalType даст any из-за
		// неверной базы/формата, см. markExternalRefs).
		if !isLocal {
			extRef := resolveExternalRef(s.Ref, specPath)
			if extRef != "" {
				// Path-only $ref (без #/-фрагмента, zvonilka one-schema-per-file)
				// → дополняем #/components/schemas/<Name>, иначе qualifyExternalType
				// не найдёт разделитель и поле падёт в any. Для ref с #/ оставляем
				// как есть (включая нестандартные фрагменты — тем занимается
				// upstream rolodex).
				if !strings.Contains(extRef, "#") {
					extRef = extRef + "#/components/schemas/" + name
				}
				s.ExternalRef = extRef
			}
		}
	}

	for _, prop := range s.Properties {
		walkNested(prop.Schema, project, specPath)
	}

	walkNested(s.Items, project, specPath)
	walkNested(s.AdditionalProperties, project, specPath)

	for _, sub := range s.AllOf {
		walkNested(sub, project, specPath)
	}

	for _, sub := range s.OneOf {
		walkNested(sub, project, specPath)
	}

	for _, sub := range s.AnyOf {
		walkNested(sub, project, specPath)
	}
}

// resolveExternalRef возвращает абсолютный путь + фрагмент для внешнего
// $ref, или пустую строку если ref локальный (начинается с "#/").
//
// Пример:
//
//	ref = "../common/src/openapi/openapi.yaml#/components/schemas/User"
//	specPath = "/input/userBackend/src/openapi/openapi.yaml"
//	→ "/input/common/src/openapi/openapi.yaml#/components/schemas/User"
func resolveExternalRef(ref, specPath string) string {
	if ref == "" || strings.HasPrefix(ref, "#/") {
		return ""
	}

	filePart := ref
	fragment := ""

	if idx := strings.Index(ref, "#"); idx >= 0 {
		filePart = ref[:idx]
		fragment = ref[idx:]
	}

	if filePart == "" {
		return ""
	}

	dir := filepath.Dir(specPath)
	resolved := filepath.Clean(filepath.Join(dir, filePart))

	return resolved + fragment
}
