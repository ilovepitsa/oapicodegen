package operations

import (
	"github.com/ilovepitsa/oapicodegen/internal/generator/render"
)

// isAnyType сообщает, сводится ли Go-тип к any (с учётом обёрток pointer /
// slice / map[string]). Конкретные типы (string, *string, []int,
// map[string]Pet) — false. Используется аудитом GOLANG_SCHEMA_ANY.
func isAnyType(typ string) bool {
	switch typ {
	case "any", "*any", "[]any", "map[string]any":
		return true
	}
	return false
}

// reportIfAny добавляет Diagnostic, если typ — any-тип. nil-коллектор —
// no-op (renderer'ы могут работать без диагностики в silent-режиме или в
// тестах). location — путь в спеке, reason — человекочитаемая причина.
func reportIfAny(col *render.Collector, location, typ, reason string) {
	if col == nil {
		return
	}
	if !isAnyType(typ) {
		return
	}
	col.Append(render.Diagnostic{Location: location, Reason: reason})
}
