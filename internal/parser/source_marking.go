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
		}
	}
}

// markExternalRefs размечает source-marking поля в схемах проекта:
//
//   - SourceFile — абсолютный путь к spec-файлу проекта, выставляется на
//     каждой top-level схеме (components.schemas).
//   - OwnerProject — проект-владелец, выставляется на каждой top-level схеме.
//   - ExternalRef — выставляется на вложенных схемах (properties, items,
//     allOf, ...), чей $ref указывает на файл другого сервиса. Содержит
//     абсолютный путь к целевому файлу + фрагмент.
//
// specPath — абсолютный путь к openapi.yaml проекта. Используется как
// SourceFile и как база для разрешения относительных external $ref.
func markExternalRefs(project *Project, specPath string) {
	if project == nil || project.Model == nil {
		return
	}

	for _, s := range project.Model.schemas {
		markTopLevel(s, project, specPath)
		markNestedRefs(s, specPath)
	}
}

// markTopLevel выставляет SourceFile и OwnerProject на top-level схеме.
func markTopLevel(s *Schema, project *Project, specPath string) {
	if s == nil {
		return
	}

	s.SourceFile = specPath
	s.OwnerProject = project
}

// markNestedRefs обходит вложенные схемы и выставляет ExternalRef на
// тех, чей $ref указывает на внешний файл.
func markNestedRefs(s *Schema, specPath string) {
	if s == nil {
		return
	}

	walkNested(s, specPath)
}

// walkNested рекурсивно обходит вложенные схемы, пропуская top-level
// (вызывается после markTopLevel). Для каждой схемы с $ref на внешний
// файл выставляет ExternalRef.
func walkNested(s *Schema, specPath string) {
	if s == nil {
		return
	}

	if s.Ref != "" {
		if extRef := resolveExternalRef(s.Ref, specPath); extRef != "" {
			s.ExternalRef = extRef
		}
	}

	for _, prop := range s.Properties {
		walkNested(prop.Schema, specPath)
	}

	walkNested(s.Items, specPath)
	walkNested(s.AdditionalProperties, specPath)

	for _, sub := range s.AllOf {
		walkNested(sub, specPath)
	}

	for _, sub := range s.OneOf {
		walkNested(sub, specPath)
	}

	for _, sub := range s.AnyOf {
		walkNested(sub, specPath)
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
