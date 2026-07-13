package parser

import "net/http"

// markUpdateSchemas обходит request body всех PUT/PATCH-операций и помечает
// схемы и их свойства как IsUsedInUpdate=true. Используется генератором T25b
// для построения Update<Name>-моделей.
//
// Эвристика триггера: HTTP-метод PUT или PATCH (x-upsert расширений пока нет).
//
// Skip-правила для свойств:
//   - ReadOnly свойства пропускаются (они не могут быть частью update-запроса).
//   - Immutable свойства (x-validations: [Immutable]) пропускаются, КРОМЕ
//     свойства с именем "name" — оно сохраняется на верхнем уровне объекта.
//
// Ограничение v1: маркируется только top-level request-body схема и её прямые
// свойства. Рекурсия в nested $ref / array items / allOf отсутствует —
// добавим, когда T25b.2 потребует сгенерированные Update-модели для вложенных
// схем. keepName-for-array-items (как в mwsapi) — тоже отложено.
func markUpdateSchemas(doc *Document) {
	for _, op := range doc.Operations {
		if op.Method != http.MethodPut && op.Method != http.MethodPatch {
			continue
		}

		if op.RequestBody == nil {
			continue
		}

		for _, mt := range op.RequestBody.Content {
			if mt.Schema == nil || mt.Schema.Ref == "" {
				continue
			}

			sh := findSchemaByName(doc, refToSchemaName(mt.Schema.Ref))
			if sh == nil {
				continue
			}

			markSchemaForUpdate(sh)
		}
	}
}

// markSchemaForUpdate помечает схему и её свойства. См. skip-правила в
// markUpdateSchemas.
func markSchemaForUpdate(sh *Schema) {
	sh.IsUsedInUpdate = true

	for _, p := range sh.Properties {
		if p.Schema != nil && p.Schema.ReadOnly {
			continue
		}

		if p.Immutable && p.Name != nameFieldName {
			continue
		}

		p.IsUsedInUpdate = true
	}
}

// findSchemaByName ищет schema в doc.Schemas по имени. Возвращает nil, если
// не найдена.
func findSchemaByName(doc *Document, name string) *Schema {
	for _, s := range doc.Schemas {
		if s.Name == name {
			return s
		}
	}

	return nil
}
