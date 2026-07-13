package parser

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	highbase "github.com/pb33f/libopenapi/datamodel/high/base"
	"gopkg.in/yaml.v3"
)

// simpleRuleRe парсит простые правила вида:
//
//	>0
//	<=100
//	Size >0
//	Length >=2
//
// Группы: 1 = target (Size|Length, опционально), 2 = operator, 3 = number.
var simpleRuleRe = regexp.MustCompile(
	`^(?:(Size|Length)\s+)?(>=|<=|==|!=|>|<)\s*(-?\d+(?:\.\d+)?)$`,
)

// parsePropertyValidations читает x-validations из property-схемы и
// возвращает (immutable, rules). Immutable — маркер для update-marker'а
// (skip), остальные строки парсятся в ValidationRule.
//
// Невалидные строки silently пропускаются: простые правила с опечаткой
// (>N с нечисловым N) не попадут в rules, именованные валидаторы с
// опечаткой поймает AssertExact при старте.
func parsePropertyValidations(sh *highbase.Schema) (bool, []ValidationRule) {
	entries := readValidationsSequence(sh)
	if entries == nil {
		return false, nil
	}

	immutable := false
	rules := make([]ValidationRule, 0, len(entries))

	for _, e := range entries {
		if e == extValidationImmutable {
			immutable = true

			continue
		}

		if rule, ok := parseValidationRule(e); ok {
			rules = append(rules, rule)
		}
	}

	return immutable, rules
}

// parseSchemaValidations читает x-validations из самой схемы (не property).
// Schema-level поддерживает только named-валидаторы (cross-field). Простые
// правила на уровне схемы silently пропускаются.
func parseSchemaValidations(sh *highbase.Schema) []ValidationRule {
	entries := readValidationsSequence(sh)
	if entries == nil {
		return nil
	}

	rules := make([]ValidationRule, 0, len(entries))

	for _, e := range entries {
		if e == extValidationImmutable {
			continue
		}

		rule, ok := parseValidationRule(e)
		if !ok {
			continue
		}

		// Schema-level: только named, простые правила пропускаются.
		if _, isNamed := rule.(NamedRule); !isNamed {
			continue
		}

		rules = append(rules, rule)
	}

	return rules
}

// readValidationsSequence читает x-validations как []string. Возвращает
// nil, если расширение отсутствует или не sequence.
func readValidationsSequence(sh *highbase.Schema) []string {
	if sh == nil || sh.Extensions == nil {
		return nil
	}

	node := sh.Extensions.GetOrZero(extValidations)
	if node == nil || node.Kind != yaml.SequenceNode {
		return nil
	}

	var entries []string
	if err := node.Decode(&entries); err != nil {
		return nil
	}

	return entries
}

// parseValidationRule разбирает одну строку из x-validations в
// SimpleRule или NamedRule. Возвращает (rule, ok) — ok=false, если
// строка не является ни простым правилом, ни именованным валидатором.
//
// Простые правила: `>0`, `<=100`, `Size >0`, `Length >=2`.
// Именованные валидаторы: `cdn.EmailFormat`, `pkg.subpkg.Validator` —
// должны содержать точку и быть валидными Go-идентификаторами в каждой
// части.
func parseValidationRule(s string) (ValidationRule, bool) {
	if m := simpleRuleRe.FindStringSubmatch(s); m != nil {
		return parseSimpleRule(m)
	}

	if isValidNamedValidator(s) {
		return NamedRule{Name: s}, true
	}

	return nil, false
}

func parseSimpleRule(m []string) (ValidationRule, bool) {
	target := TargetValue
	if m[1] == "Size" || m[1] == "Length" {
		target = TargetSize
	}

	op, ok := parseOperator(m[2])
	if !ok {
		return nil, false
	}

	val, err := strconv.ParseFloat(m[3], 64)
	if err != nil {
		return nil, false
	}

	return SimpleRule{Target: target, Op: op, Value: val}, true
}

func parseOperator(s string) (Operator, bool) {
	switch s {
	case ">":
		return OpGT, true
	case ">=":
		return OpGE, true
	case "<":
		return OpLT, true
	case "<=":
		return OpLE, true
	case "==":
		return OpEQ, true
	case "!=":
		return OpNE, true
	default:
		return 0, false
	}
}

// isValidNamedValidator проверяет, что s — валидное имя именованного
// валидатора: содержит минимум одну точку, каждая часть — непустой
// Go-идентификатор.
func isValidNamedValidator(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) < 2 {
		return false
	}

	for _, p := range parts {
		if !isIdentifier(p) {
			return false
		}
	}

	return true
}

// isIdentifier проверяет, что s — валидный Go-идентификатор: непустой,
// начинается с буквы или _, состоит из букв/цифр/_.
func isIdentifier(s string) bool {
	if s == "" {
		return false
	}

	for i, r := range s {
		if i == 0 {
			if !unicode.IsLetter(r) && r != '_' {
				return false
			}

			continue
		}

		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}

	return true
}
