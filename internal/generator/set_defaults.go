package generator

import (
	"fmt"
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/parser"
	"strconv"
	"strings"
)

// renderSetDefaultsMethod рендерит метод `func (m *<Name>) SetDefaults()`,
// проставляющий default-значения для полей с Default != nil, а также
// рекурсивно вызывающий SetDefaults для вложенных object-полей с $ref.
//
// Покрытые типы: string, integer (int/int32/int64), number (float32/float64),
// boolean, enum (через константное имя). Поля-объекты с $ref на схему с
// defaults рекурсивно вызывают <Field>.SetDefaults().
//
// При включённом GOLANG_SPLIT_REQUEST_RESPONSE метод рендерится отдельно
// для <Name>Request и <Name>Response — только для тех, у которых среди
// отфильтрованных property есть default.
func (g *Generator) renderSetDefaultsMethod(
	w *codegen.BufferWriter,
	sh *parser.Schema,
	m *typeMapper,
	name string,
	keep func(*parser.Property) bool,
) {
	w.Print("func (m *", name, ") SetDefaults() {\n")

	for _, p := range sh.Properties {
		if keep != nil && !keep(p) {
			continue
		}

		if p.Schema == nil {
			continue
		}

		// Поле с собственным Default — рендерим присваивание литерала.
		if p.Schema.Default != nil {
			g.renderSetDefaultForField(w, p, m)

			continue
		}

		// Поле без Default, но с $ref на object-схему, у которой есть
		// defaults — рекурсивно вызываем <Field>.SetDefaults().
		g.renderNestedSetDefaultsCall(w, p, m, keep)
	}

	w.Print("}\n\n")
}

// renderSetDefaultForField рендерит блок `if m.<Field> == <zero> { m.<Field> = <literal> }`
// для одного property. Опциональные поля (pointer) проверяются на nil,
// required — на zero-value примитива.
//
// Для optional (pointer) полей literal оборачивается в локальную переменную
// и берётся указатель: `if m.Field == nil { v := <literal>; m.Field = &v }`,
// так как прямое присваивание `m.Field = <literal>` не компилируется для
// pointer-типов (*int, *string, *Code и т.д.).
func (g *Generator) renderSetDefaultForField(
	w *codegen.BufferWriter,
	p *parser.Property,
	m *typeMapper,
) {
	// B2: форматы, маппящиеся на не-примитивные Go-типы (time.Time, []byte),
	// не поддерживаются — присваивание строкового литерала не компилируется.
	//
	// Future work: support date-time/date defaults via time.Parse.
	if isNonPrimitiveStringFormat(p.Schema) {
		return
	}

	fieldName := goName(p.Name)
	goType := m.goType(p.Schema)

	optional := fieldIsOptional(g.requiredForMode(p, m.mode), goType)
	if optional {
		goType = "*" + goType
	}

	literal, ok := defaultValueLiteral(p.Schema)
	if !ok {
		// Тип не поддерживается (object $ref без enum, array, union) — пропускаем.
		return
	}

	if optional || strings.HasPrefix(goType, "*") {
		// Optional-поля имеют pointer-тип (*int, *string, *Code и т.д.).
		// Прямое присваивание `m.Field = <literal>` не компилируется —
		// нужен указатель. Используем паттерн `v := <literal>; m.Field = &v`,
		// который работает для всех поддерживаемых типов (int, int32, int64,
		// float32, float64, bool, string, named enum-const), так как literal
		// уже несёт корректный Go-тип, и v выводится в тот же тип.
		w.Print("\tif m.", fieldName, " == nil {\n")
		w.Print("\t\tv := ", literal, "\n")
		w.Print("\t\tm.", fieldName, " = &v\n")
		w.Print("\t}\n")

		return
	}

	w.Print("\tif m.", fieldName, " == ", zeroValueLiteral(p.Schema), " {\n")
	w.Print("\t\tm.", fieldName, " = ", literal, "\n")
	w.Print("\t}\n")
}

// renderNestedSetDefaultsCall рендерит рекурсивный вызов SetDefaults
// для поля, ссылающегося ($ref) на object-схему с defaults.
//
// Для optional (pointer) поля: `if m.<Field> != nil { m.<Field>.SetDefaults() }`.
// Для required (value) поля: `m.<Field>.SetDefaults()`.
//
// keep нужен, чтобы для splittable-схем учитывать только properties,
// попадающие в текущий <Name>Request/<Name>Response вариант: если ни один
// default-field целевой схемы не проходит фильтр — вызов пропускается.
func (g *Generator) renderNestedSetDefaultsCall(
	w *codegen.BufferWriter,
	p *parser.Property,
	m *typeMapper,
	keep func(*parser.Property) bool,
) {
	target := g.resolveRefSchema(p.Schema)
	if target == nil || len(target.Properties) == 0 {
		return
	}

	if !g.schemaTreeHasDefaults(target, keep, map[string]bool{target.Name: true}) {
		return
	}

	fieldName := goName(p.Name)
	fieldType := m.goType(p.Schema)

	optional := fieldIsOptional(g.requiredForMode(p, m.mode), fieldType)
	if optional {
		w.Print("\tif m.", fieldName, " != nil {\n")
		w.Print("\t\tm.", fieldName, ".SetDefaults()\n")
		w.Print("\t}\n")

		return
	}

	w.Print("\tm.", fieldName, ".SetDefaults()\n")
}

// schemaTreeHasDefaults рекурсивно (через $ref на object-схемы) проверяет,
// есть ли в дереве схемы хотя бы одно property с Default != nil, проходящее
// фильтр keep. visited защищает от циклических $ref.
//
// Глубина ограничена visited-set: каждая схема посещается не более одного раза.
// Для циклических ссылок (Outer → Inner → Outer) второй проход по Outer
// пропускается, что предотвращает бесконечную рекурсию.
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

		// Рекурсивно в $ref-на-object-схему.
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

// defaultValueLiteral возвращает Go-литерал для schema.Default, отрендеренный
// под Go-тип поля. Второе возвращаемое значение false, если тип не поддерживается.
//
// SetDefaults рендерится только в model-пакете (schemaFile создаёт typeMapper
// с currentPkg="model"), поэтому enum-константы ссылаются без package-префикса.
func defaultValueLiteral(s *parser.Schema) (string, bool) {
	// Enum: рендерим константное имя <TypeName><ValueName>.
	if len(s.Enum) > 0 && s.Name != "" {
		valStr := enumStringValue(s.Default)
		constName := enumValueName(goName(s.Name), valStr, 0)

		return constName, true
	}

	switch s.Type {
	case oapiTypeString:
		return strconv.Quote(fmt.Sprint(s.Default)), true
	case oapiTypeInteger:
		return intLiteral(s.Default, s.Format), true
	case oapiTypeNumber:
		return floatLiteral(s.Default, s.Format), true
	case oapiTypeBoolean:
		return boolLiteral(s.Default), true
	}

	return "", false
}

// isNonPrimitiveStringFormat сообщает, маппится ли string-format на
// не-примитивный Go-тип (time.Time, []byte), для которого нельзя
// присвоить строковый литерал. Такие поля пропускаются в SetDefaults.
func isNonPrimitiveStringFormat(s *parser.Schema) bool {
	if s == nil || s.Type != oapiTypeString {
		return false
	}

	switch s.Format {
	case oapiFormatDateTime, oapiFormatDate, oapiFormatBinary:
		return true
	}

	return false
}

// intLiteral рендерит числовой литерал для integer-default. Default в spec
// может прийти как float64 (yaml декодирует числа в float64) или int —
// конвертируем корректно. Для int32/int64 добавляем каст.
func intLiteral(v any, format string) string {
	n := toInt64(v)

	switch format {
	case oapiFormatInt32:
		return fmt.Sprintf("int32(%d)", n)
	case oapiFormatInt64:
		return fmt.Sprintf("int64(%d)", n)
	default:
		return strconv.FormatInt(n, 10)
	}
}

// floatLiteral рендерит числовой литерал для number-default. Go требует
// точку в float-литерале (3.0, не 3), поэтому используем %v от float64.
// Для float32 добавляем каст.
func floatLiteral(v any, format string) string {
	f := toFloat64(v)

	switch format {
	case oapiFormatFloat:
		return fmt.Sprintf("float32(%v)", f)
	default:
		return strconv.FormatFloat(f, 'g', -1, 64)
	}
}

func boolLiteral(v any) string {
	if b, ok := v.(bool); ok {
		return strconv.FormatBool(b)
	}

	return "false"
}

// toInt64 конвертирует yaml-декодированное значение в int64.
// yaml.v3 декодирует целые как int или float64.
func toInt64(v any) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int64:
		return n
	case float64:
		return int64(n)
	case string:
		if i, err := strconv.ParseInt(n, 10, 64); err == nil {
			return i
		}
	}

	return 0
}

// toFloat64 конвертирует yaml-декодированное значение в float64.
func toFloat64(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case string:
		if f, err := strconv.ParseFloat(n, 64); err == nil {
			return f
		}
	}

	return 0
}

// zeroValueLiteral возвращает Go zero-value литерал по schema type.
//
// Определение идёт по schema-типу (а не по Go-типу), чтобы корректно
// работать для named enum-типов: `type Code int32` имеет Go-имя "Code",
// но zero-value должен быть `0` (untyped int constant совместим с любым
// integer-based named type), а не `""`.
//
// Для enum zero-value определяется по base type: string → `""`, int → `0`.
func zeroValueLiteral(s *parser.Schema) string {
	if s == nil {
		return `""`
	}

	// Enum: zero-value по base type.
	if len(s.Enum) > 0 {
		switch enumBaseType(s) {
		case oapiTypeString:
			return `""`
		default:
			return "0"
		}
	}

	switch s.Type {
	case oapiTypeString:
		// Non-primitive string-formats (date-time, date, binary) не должны
		// доходить до сюда — их отсекает isNonPrimitiveStringFormat в
		// renderSetDefaultForField. Но на всякий случай возвращаем ""
		// (для обычного string).
		return `""`
	case oapiTypeInteger, oapiTypeNumber:
		return "0"
	case oapiTypeBoolean:
		return "false"
	}

	return `""`
}
