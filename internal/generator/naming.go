package generator

import (
	"strings"
	"unicode"
)

// goName конвертирует имя схемы/поля в PascalCase Go-идентификатор.
// "pet_id" → "PetID", "Pet" → "Pet", "my-schema" → "MySchema".
func goName(s string) string {
	if s == "" {
		return ""
	}

	var b strings.Builder

	capitalizeNext := true

	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) { //nolint:nestif // 2 уровня — рефакторинг ухудшит читаемость
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
	for _, abbr := range []string{"Id", "Url", "Uri", "Http", "Https", "Json", "Xml", "Api", "Uuid", "Ip"} {
		name = replaceWord(name, abbr, strings.ToUpper(abbr))
	}
	return name
}

func replaceWord(s, old, replacement string) string {
	// заменяет только целые слова (за заглавной буквой следует строчная в old)
	return strings.ReplaceAll(s, old, replacement)
}

// fileName конвертирует PascalCase в snake_case для имени файла.
// "PetCollection" → "pet_collection", "Pet" → "pet", "AB" → "ab".
func fileName(name string) string {
	if name == "" {
		return ""
	}

	runes := []rune(name)

	var b strings.Builder

	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) {
			prev := runes[i-1]
			nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])

			if unicode.IsLower(prev) || nextLower {
				b.WriteByte('_')
			}
		}

		b.WriteRune(unicode.ToLower(r))
	}

	return b.String()
}

// enumValueName возвращает Go-имя константы для enum-значения.
// "active" → "Active", "in-progress" → "InProgress".
func enumValueName(prefix, value string, _ int) string {
	if value == "" {
		return prefix + "Empty"
	}

	return prefix + goName(value)
}
