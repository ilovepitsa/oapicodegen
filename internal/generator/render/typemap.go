package render

import "nschugorev/oapigenerator/internal/parser"

// TypeMapper абстрагирует преобразование parser.Schema в Go-тип. Реализация
// живёт в пакете generator (typeMapper) и подключается через adapter, чтобы
// render не зависел от generator и не дублировал логику разрешения $ref,
// cross-service imports, nullable-обёрток и т.п.
//
// SetMode переключает режим генерации ("", "Request", "Response") —
// StructRenderer использует его при split-рендере: одно и то же поле
// рендерится с mode="Request" для <Name>Request и mode="Response" для
// <Name>Response, чтобы $ref на splittable-схемы разрешались корректно.
type TypeMapper interface {
	GoType(s *parser.Schema) string
	SetMode(mode string)
}
