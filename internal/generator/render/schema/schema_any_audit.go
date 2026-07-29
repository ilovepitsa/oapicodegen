package schema

import (
	"github.com/ilovepitsa/oapicodegen/internal/generator/render"
	"github.com/ilovepitsa/oapicodegen/internal/parser"
)

// reportSchemaAny добавляет Diagnostic, если тип поля свёлся к any, НО схема
// НЕ использует явный additionalProperties (который — осознанный free-form map,
// не баг). nil-коллектор — no-op (renderer'ы могут работать без диагностики в
// silent-режиме или в тестах).
//
// Исключение для map[string]any: type mapper даёт map[string]any как для
// additionalProperties: true (осознанный free-form), так и для пустого schema {}
// (баг). Различаем по источнику — наличию AdditionalProperties в *parser.Schema.
func reportSchemaAny(col *render.Collector, location string, s *parser.Schema, goType string) {
	if col == nil {
		return
	}
	if !isAnyType(goType) {
		return
	}
	// Явный additionalProperties (free-form map) — exempt. Дополнительно
	// exempt схемы с additionalProperties: false (закрытая структура).
	if s != nil && (s.AdditionalProperties != nil || s.AdditionalPropertiesFalse) {
		return
	}
	col.Append(render.Diagnostic{Location: location, Reason: anyReason(s, goType)})
}

// isAnyType сообщает, сводится ли Go-тип к any (с учётом обёрток pointer /
// slice / map[string]). Конкретные типы (string, *string, []int,
// map[string]Pet) — false. Дублировано из operations-пакета во избежание
// import-цикла.
func isAnyType(typ string) bool {
	switch typ {
	case "any", "*any", "[]any", "map[string]any":
		return true
	}
	return false
}

// anyReason возвращает человекочитаемую причину, по которой схема свелась к
// Go-типу any. Различает nil-схему, неразрешённый внешний $ref, пустой объект
// (schema {}) и прочий fallback (union без имени / неизвестный примитив).
func anyReason(s *parser.Schema, goType string) string {
	if s == nil {
		return "schema resolves to Go `" + goType + "` (nil schema)"
	}
	if s.ExternalRef != "" {
		return "schema resolves to Go `" + goType + "` (unresolved external $ref: " + s.ExternalRef + ")"
	}
	if s.Type == "object" && len(s.Properties) == 0 && s.AdditionalProperties == nil && !s.AdditionalPropertiesFalse {
		return "schema resolves to Go `" + goType + "` (empty schema {})"
	}
	return "schema resolves to Go `" + goType + "` (unknown/union fallback)"
}

// schemaFieldLocation строит best-effort путь поля в спеке для диагностики.
// structName — имя текущей object-схемы (передаётся renderer'ом), p — свойство.
// Пустой structName → путь только по имени свойства.
func schemaFieldLocation(structName string, p *parser.Property) string {
	if p == nil {
		return "schema.field"
	}
	if structName == "" {
		return "schema.properties." + p.Name
	}
	return "components.schemas." + structName + ".properties." + p.Name
}
