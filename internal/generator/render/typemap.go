package render

import "nschugorev/oapigenerator/internal/parser"

// TypeMapper абстрагирует преобразование parser.Schema в Go-тип. Реализация
// живёт в пакете generator (typeMapper) и подключается через adapter, чтобы
// render не зависел от generator и не дублировал логику разрешения $ref,
// cross-service imports, nullable-обёрток и т.п.
type TypeMapper interface {
	GoType(s *parser.Schema) string
}
