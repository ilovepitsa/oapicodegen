package generator

import (
	"fmt"
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/parser"
	"strconv"
	"strings"
)

// validatorPkg — import-path runtime-пакета validator.Registry.
const validatorPkg = "nschugorev/oapigenerator/pkg/validator"

// hasValidationRules проверяет, есть ли хотя бы одно правило валидации
// у схемы (schema-level) или её свойств (property-level).
func hasValidationRules(sh *parser.Schema) bool {
	if len(sh.Validations) > 0 {
		return true
	}

	for _, p := range sh.Properties {
		if len(p.Validations) > 0 {
			return true
		}
	}

	return false
}

// renderValidateOwn рендерит метод ValidateOwn(reg) для структуры.
// Генерируется только если есть хотя бы одно правило (см.
// hasValidationRules). Value receiver — чтобы walker работал с
// не-addressable значениями (map values).
//
// isUpdate=true — рендер для Update<Name> (поля обёрнуты в Optional[T]),
// isUpdate=false — рендер для основной <Name> структуры.
//
// keep фильтрует properties (nil = все). Для split-вариантов передаётся
// фильтр Request/Response; для Update — IsUsedInUpdate.
func (g *Generator) renderValidateOwn(
	w *codegen.BufferWriter,
	sh *parser.Schema,
	m *typeMapper,
	name string,
	isUpdate bool,
	keep func(*parser.Property) bool,
) {
	if !hasValidationRules(sh) {
		return
	}

	m.addImport(validatorPkg, "validator")
	m.addImport("fmt", "")

	receiver := "x"
	w.Print("func (", receiver, " ", name, ") ValidateOwn(reg *validator.Registry) error {\n")

	// Property-level rules.
	for _, p := range sh.Properties {
		if len(p.Validations) == 0 {
			continue
		}

		if keep != nil && !keep(p) {
			continue
		}

		g.renderPropertyValidations(w, p, m, receiver, isUpdate)
	}

	// Schema-level rules (cross-field named validators).
	// Skipped for Update structs: validator is registered for the main
	// struct type (ItemCreate), but Update receiver is UpdateItemCreate —
	// different type. Cross-field validation on PATCH-bodies needs separate
	// semantics (Optional[T] fields) and is out of scope for v1.
	if !isUpdate {
		for _, rule := range sh.Validations {
			nr, ok := rule.(parser.NamedRule)
			if !ok {
				continue
			}

			renderNamedValidatorCall(w, nr.Name, receiver, "")
		}
	}

	w.Print("\treturn nil\n")
	w.Print("}\n\n")
}

// renderPropertyValidations рендерит все правила одного свойства.
// Для каждого правила — guard (если нужен) + проверка.
func (g *Generator) renderPropertyValidations(
	w *codegen.BufferWriter,
	p *parser.Property,
	m *typeMapper,
	receiver string,
	isUpdate bool,
) {
	fieldName := goName(p.Name)

	for _, rule := range p.Validations {
		switch r := rule.(type) {
		case parser.SimpleRule:
			g.renderSimpleRule(w, r, p, m, receiver, fieldName, isUpdate)
		case parser.NamedRule:
			accessor, guard := g.fieldAccessor(p, m, receiver, isUpdate, parser.TargetValue)
			if guard != "" {
				w.Print("\tif ", guard, " {\n")
				renderNamedValidatorCallIndented(w, r.Name, accessor, fieldName, "\t")
				w.Print("\t}\n")
			} else {
				renderNamedValidatorCallIndented(w, r.Name, accessor, fieldName, "")
			}
		}
	}
}

// renderSimpleRule рендерит inline-проверку для SimpleRule.
// Инвертирует оператор: правило ">0" → ошибка если "<=0".
func (g *Generator) renderSimpleRule(
	w *codegen.BufferWriter,
	rule parser.SimpleRule,
	p *parser.Property,
	m *typeMapper,
	receiver string,
	fieldName string,
	isUpdate bool,
) {
	accessor, guard := g.fieldAccessor(p, m, receiver, isUpdate, rule.Target)
	literal := formatValueLiteral(rule.Value)

	inverseOp := inverseOperator(rule.Op)
	condition := accessor + " " + inverseOp + " " + literal

	msg := fmt.Sprintf("field %s: must be %s %s", fieldName, opSymbol(rule.Op), literal)

	if guard != "" {
		w.Print("\tif ", guard, " && ", condition, " {\n")
		w.Print("\t\treturn fmt.Errorf(", strconv.Quote(msg), ")\n")
		w.Print("\t}\n")
	} else {
		w.Print("\tif ", condition, " {\n")
		w.Print("\t\treturn fmt.Errorf(", strconv.Quote(msg), ")\n")
		w.Print("\t}\n")
	}
}

// fieldAccessor возвращает (accessor, guard) для поля в зависимости от
// контекста (main struct vs Update) и target (Value vs Size).
//
//   - Main struct, value field: accessor="x.Age", guard=""
//   - Main struct, pointer field: accessor="*x.Age", guard="x.Age != nil"
//   - Main struct, optional.Optional[T] field (UseOptional+x-optional):
//     accessor="x.Age.Value()", guard="x.Age.IsSet() && !x.Age.IsNil()"
//   - Update struct: accessor="x.Age.Value()", guard="x.Age.IsSet() && !x.Age.IsNil()"
//
// Для TargetSize accessor оборачивается в len().
//
// Pointer-detection повторяет логику renderField: поле оборачивается в *T,
// если оно не required и его Go-тип не nilable. Nullable-схема (nullable: true)
// тоже даёт *T через m.goType.
//
//nolint:gocritic // unnamedResult conflicts with nonamedreturns
func (g *Generator) fieldAccessor(
	p *parser.Property,
	m *typeMapper,
	receiver string,
	isUpdate bool,
	target parser.Target,
) (string, string) {
	fieldName := goName(p.Name)
	fieldRef := receiver + "." + fieldName

	// optional-wrapped field: Update-struct always, main-struct when
	// UseOptional flag is on AND x-optional marker is set.
	wrapped := isUpdate || (g.features.UseOptional.Value && p.Optional)

	var accessor, guard string

	if wrapped {
		guard = fieldRef + ".IsSet() && !" + fieldRef + ".IsNil()"
		accessor = fieldRef + ".Value()"

		if target == parser.TargetSize {
			accessor = "len(" + accessor + ")"
		}

		return accessor, guard
	}

	// Main struct, regular field.
	fieldType := m.goType(p.Schema)
	required := g.requiredForMode(p, m.mode)

	// Replicate renderField's pointer-wrapping for non-required fields.
	if !strings.HasPrefix(fieldType, "*") && fieldIsOptional(required, fieldType) {
		fieldType = "*" + fieldType
	}

	if target == parser.TargetSize {
		// len() works with nil slices/strings — no guard needed.
		// But pointer-to-string/slice needs guard + deref before len().
		if strings.HasPrefix(fieldType, "*") {
			return "len(*" + fieldRef + ")", fieldRef + " != nil"
		}

		return "len(" + fieldRef + ")", ""
	}

	// TargetValue.
	if strings.HasPrefix(fieldType, "*") {
		return "*" + fieldRef, fieldRef + " != nil"
	}

	return fieldRef, ""
}

// inverseOperator возвращает оператор для условия ошибки.
// Правило ">N" → ошибка если "<=N".
func inverseOperator(op parser.Operator) string {
	switch op {
	case parser.OpGT:
		return "<="
	case parser.OpGE:
		return "<"
	case parser.OpLT:
		return ">="
	case parser.OpLE:
		return ">"
	case parser.OpEQ:
		return "!="
	case parser.OpNE:
		return "=="
	default:
		return "<="
	}
}

// opSymbol возвращает символ оператора для сообщения об ошибке.
func opSymbol(op parser.Operator) string {
	switch op {
	case parser.OpGT:
		return ">"
	case parser.OpGE:
		return ">="
	case parser.OpLT:
		return "<"
	case parser.OpLE:
		return "<="
	case parser.OpEQ:
		return "=="
	case parser.OpNE:
		return "!="
	default:
		return ">"
	}
}

// formatValueLiteral форматирует float64 как Go-литерал. Целые числа —
// как int (0, 100, -10), дробные — как float (1.5, -0.5).
func formatValueLiteral(v float64) string {
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}

	return strconv.FormatFloat(v, 'f', -1, 64)
}

// renderNamedValidatorCall рендерит lookup + вызов именованного валидатора.
// fieldPath — путь поля для сообщения об ошибке ("Owner.Tags"); пустой
// для schema-level валидатора.
func renderNamedValidatorCall(w *codegen.BufferWriter, name, accessor, fieldPath string) {
	renderNamedValidatorCallIndented(w, name, accessor, fieldPath, "")
}

// renderNamedValidatorCallIndented — то же с отступом indent для вложенных
// в guard-блоки.
func renderNamedValidatorCallIndented(
	w *codegen.BufferWriter,
	name, accessor, fieldPath, indent string,
) {
	notRegisteredMsg := strconv.Quote("validator %q not registered")

	w.Print(indent, "\tv, ok := reg.Get(", strconv.Quote(name), ")\n")
	w.Print(indent, "\tif !ok {\n")
	w.Print(indent, "\t\treturn fmt.Errorf(", notRegisteredMsg, ", ", strconv.Quote(name), ")\n")
	w.Print(indent, "\t}\n")

	if fieldPath == "" {
		// Schema-level — ошибка без обёртывания путём поля.
		w.Print(indent, "\tif err := v.Validate(", accessor, "); err != nil {\n")
		w.Print(indent, "\t\treturn err\n")
		w.Print(indent, "\t}\n")
	} else {
		w.Print(indent, "\tif err := v.Validate(", accessor, "); err != nil {\n")
		w.Print(indent, "\t\treturn fmt.Errorf(", strconv.Quote("field "+fieldPath+": %w"), ", err)\n")
		w.Print(indent, "\t}\n")
	}
}

// collectExpectedValidatorNames собирает уникальные имена именованных
// валидаторов со всех схем документа (property-level + schema-level).
func collectExpectedValidatorNames(doc *parser.Document) []string {
	seen := make(map[string]bool)

	for _, sh := range doc.Schemas {
		for _, rule := range sh.Validations {
			if nr, ok := rule.(parser.NamedRule); ok {
				seen[nr.Name] = true
			}
		}

		for _, p := range sh.Properties {
			for _, rule := range p.Validations {
				if nr, ok := rule.(parser.NamedRule); ok {
					seen[nr.Name] = true
				}
			}
		}
	}

	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}

	return sortStrings(out)
}

func sortStrings(s []string) []string {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}

	return s
}
