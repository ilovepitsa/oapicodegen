// Package schema: treeHasDefaults и сопутствующие утилиты для
// SetDefaultsRenderer (Task 2). Портированы из Generator.schemaTreeHasDefaults
// и Generator.resolveRefSchema (см. internal/generator/set_defaults.go и
// internal/generator/impl_server.go) — старый путь остаётся активным
// (filteredSchemaHasDefaults используется impl_server.go).
//
// Отличие от оригинала: resolveRefSchema теперь принимает *parser.Model
// явно, так как из пакета render/schema нет доступа к Generator.project.
// При model == nil (или схеме без $ref) функция возвращает nil — этого
// достаточно для unit-тестов Task 1, где $ref не используется. SetDefaultsRenderer
// (Task 2) передаст r.Ctx.Project.Model для разрешения вложенных object-схем.
package schema

import (
	"nschugorev/oapigenerator/internal/parser"
)

// treeHasDefaults рекурсивно (через $ref на object-схемы) проверяет, есть ли в
// дереве схемы хотя бы одно property с Default != nil, проходящее фильтр keep.
// keep nil = все свойства проходят.
//
// Публичная entrypoint-функция: инициализирует visited-set именем текущей
// схемы и делегирует рекурсивной реализации. Model для разрешения $ref здесь
// не передаётся (nil) — этого достаточно для unit-тестов Task 1, где $ref не
// используется. SetDefaultsRenderer в Task 2 вызывает
// treeHasDefaultsWithVisited напрямую с r.Ctx.Project.Model.
func treeHasDefaults(s *parser.Schema, keep func(*parser.Property) bool) bool {
	if s == nil {
		return false
	}

	return treeHasDefaultsWithVisited(s, keep, map[string]bool{s.Name: true}, nil)
}

// treeHasDefaultsWithVisited — рекурсивная реализация treeHasDefaults.
// visited защищает от циклов по $ref (A → B → A): имя текущей схемы
// добавляется в visited перед рекурсивным обходом, чтобы повторный заход
// в ту же схему пропускался.
//
// model нужен для разрешения $ref (Lookup по имени): при model == nil
// вложенные object-схемы не разрешаются — достаточно для unit-тестов Task 1.
// SetDefaultsRenderer передаёт r.Ctx.Project.Model.
//
// Тело перенесено из Generator.schemaTreeHasDefaults (set_defaults.go:158-191)
// с заменой g.resolveRefSchema → resolveRefSchema и g.schemaTreeHasDefaults →
// treeHasDefaultsWithVisited. Поведение идентично оригиналу.
//
//nolint:gocyclo,cyclop // recursive tree walk with early returns — port verbatim
func treeHasDefaultsWithVisited(
	sh *parser.Schema,
	keep func(*parser.Property) bool,
	visited map[string]bool,
	model *parser.Model,
) bool {
	if sh == nil {
		return false
	}

	for _, p := range sh.Properties {
		if keep != nil && !keep(p) {
			continue
		}

		if p.Schema == nil {
			continue
		}

		if p.Schema.Default != nil {
			return true
		}

		// Рекурсивно в $ref-на-object-схему.
		target := resolveRefSchema(p.Schema, model)
		if target != nil && len(target.Properties) > 0 && !visited[target.Name] {
			visited[target.Name] = true
			if treeHasDefaultsWithVisited(target, keep, visited, model) {
				return true
			}
		}
	}

	return false
}

// resolveRefSchema возвращает схему из model по $ref. Если s — не $ref
// (inline-схема), cross-service $ref (ExternalRef), или имя не найдено —
// возвращает nil.
//
// Портировано из Generator.resolveRefSchema (impl_server.go:215-226).
// Отличие: принимает *parser.Model явно, так как пакет render/schema не имеет
// доступа к Generator.project. При model == nil всегда возвращает nil —
// достаточно для unit-тестов Task 1 (без $ref). SetDefaultsRenderer (Task 2)
// будет передавать r.Ctx.Project.Model.
func resolveRefSchema(s *parser.Schema, model *parser.Model) *parser.Schema {
	if s == nil || s.ExternalRef != "" || s.Ref == "" {
		return nil
	}

	if model == nil {
		return nil
	}

	name := refToName(s.Ref)
	if sh, ok := model.Lookup(name); ok {
		return sh
	}

	return nil
}
