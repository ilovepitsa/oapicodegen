package generator

import "nschugorev/oapigenerator/internal/parser"

// schemaTreeHasDefaults рекурсивно (через $ref на object-схемы) проверяет,
// есть ли в дереве схемы хотя бы одно property с Default != nil, проходящее
// фильтр keep. visited защищает от циклических $ref.
//
// Используется filteredSchemaHasDefaults (schema.go) для audit-server-response-
// фильтрации в impl_server.go. SetDefaults-рендеринг в render/schema/ имеет
// собственную копию (resolveRefSchema + SchemaTreeHasDefaults) — дублирование
// для развязки пакетов.
//
//nolint:gocyclo,cyclop // recursive tree walk with early returns
func (g *Generator) schemaTreeHasDefaults(
	sh *parser.Schema,
	keep func(*parser.Property) bool,
	visited map[string]bool,
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

		target := g.resolveRefSchema(p.Schema)
		if target != nil && len(target.Properties) > 0 && !visited[target.Name] {
			visited[target.Name] = true
			if g.schemaTreeHasDefaults(target, keep, visited) {
				return true
			}
		}
	}

	return false
}
