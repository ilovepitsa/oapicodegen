// Package schema содержит renderer'ы для schema-файлов: AliasRenderer
// (примитивные alias'ы и map-alias'ы) и EnumRenderer. Renderer'ы embed'ят
// walk.NoopSchemaRenderer и реализуют только нужные хуки (OnAlias/OnMap для
// AliasRenderer, OnEnum для EnumRenderer).
package schema

import (
	"strings"
	"unicode"
)

// goName конвертирует имя схемы/поля в PascalCase Go-идентификатор.
// "pet_id" → "PetID", "Pet" → "Pet", "my-schema" → "MySchema".
//
// Дублировано из пакета generator, чтобы render/schema не зависел от
// generator (generator зависит от render — был бы цикл). Future task может
// вынести naming в отдельный пакет и убрать дублирование.
func goName(s string) string {
	if s == "" {
		return ""
	}

	var b strings.Builder

	capitalizeNext := true

	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) { //nolint:nestif,lll // 2 levels, refactor worsens readability
			if capitalizeNext {
				b.WriteRune(unicode.ToUpper(r))

				capitalizeNext = false
			} else {
				b.WriteRune(r)
			}
		} else {
			capitalizeNext = true
		}
	}

	name := b.String()
	// common abbreviations → uppercase
	abbreviations := []string{"Id", "Url", "Uri", "Http", "Https", "Json", "Xml", "Api", "Uuid", "Ip"}
	for _, abbr := range abbreviations {
		name = strings.ReplaceAll(name, abbr, strings.ToUpper(abbr))
	}

	return name
}

// enumValueName возвращает Go-имя константы для enum-значения.
// "active" → "Active", "in-progress" → "InProgress".
//
// Дублировано из пакета generator (см. комментарий к goName).
func enumValueName(prefix, value string, _ int) string {
	if value == "" {
		return prefix + "Empty"
	}

	return prefix + goName(value)
}

// writeDocComment пишет description как серию Go-комментариев "// <line>".
// Используется для doc-комментариев перед type/const declarations.
func writeDocComment(w writer, desc string) {
	for line := range strings.SplitSeq(desc, "\n") {
		w.Print("// ", line, "\n")
	}
}
