package operations

import (
	"github.com/ilovepitsa/oapicodegen/internal/generator/render"
	"github.com/ilovepitsa/oapicodegen/internal/parser"
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
//
// Простая (schema-несознательная) версия — используется там, где источник
// типа уже известен вызывающему (например, audit-структуры с предопределённым
// `Payload any`). Для response-payload с реальной схемой используйте
// reportPayloadIfAny — он применяет exemption для явного additionalProperties.
func reportIfAny(col *render.Collector, location, typ, reason string) {
	if col == nil {
		return
	}
	if !isAnyType(typ) {
		return
	}
	col.Append(render.Diagnostic{Location: location, Reason: reason})
}

// reportPayloadIfAny — schema-aware аудит для response-payload типа.
// Добавляет Diagnostic, если Go-тип свёлся к any из-за пустой/неразрешённой
// схемы. Явный additionalProperties (free-form map) — exempt: type mapper даёт
// map[string]any как для additionalProperties: true, так и для schema {}, и
// различить их можно только по *parser.Schema. nil-коллектор — no-op.
func reportPayloadIfAny(col *render.Collector, location string, s *parser.Schema, goType string) {
	if col == nil {
		return
	}
	if !isAnyType(goType) {
		return
	}
	// Явный additionalProperties (free-form map) или closed struct — exempt.
	if s != nil && (s.AdditionalProperties != nil || s.AdditionalPropertiesFalse) {
		return
	}
	col.Append(render.Diagnostic{Location: location, Reason: anyReason(s, goType)})
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

// opLocation строит best-effort путь операции+response в спеке для диагностики.
// method — HTTP-метод ("get", "post", ...), path — путь ("/pets/{id}"),
// statusCode — код ответа из spec ("200", "4XX", "default").
func opLocation(method, path, statusCode string) string {
	return "paths." + method + "." + path + ".responses." + statusCode
}
