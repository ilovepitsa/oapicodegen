// Package schema: helpers для рендера default-value литералов. Портированы
// из internal/generator/set_defaults.go (defaultValueLiteral, zeroValueLiteral,
// isNonPrimitiveStringFormat, intLiteral, floatLiteral, boolLiteral, toInt64,
// toFloat64). Оригиналы в internal/generator/set_defaults.go удалены в Task 8
// (мост Callbacks больше не используется). Дублирование разрешено для развязки
// пакетов (см. defaults.go).
package schema

import (
	"fmt"
	"nschugorev/oapigenerator/internal/parser"
	"strconv"
)

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
