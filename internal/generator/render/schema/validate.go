// Package schema: ValidateOwnRenderer рендерит
// `func (x <Name>) ValidateOwn(reg *validator.Registry) error` для object-схем,
// у которых есть хотя бы одно правило валидации (x-validations) на schema- или
// property-level. Поддерживает моно-режим (<Name>) и split-режим
// (<Name>Request + <Name>Response). Update-вариант (Update<Name>)
// рендерится UpdateStructRenderer'ом через общий renderValidateOwnInto.
//
// Портирован из Generator.renderValidateOwn + renderPropertyValidations +
// renderSimpleRule + fieldAccessor + inverseOperator + opSymbol +
// formatValueLiteral + renderNamedValidatorCall(Indented)
// (internal/generator/validate.go). Старый путь остаётся активным до Task 8
// (удаление Callbacks-моста).
//
// Renderer embed'ит render.Base (Buf/Imports/Ctx) и walk.NoopSchemaRenderer
// (остальные хуки не нужны). Skip не реализуется — StructRenderer первым в
// pack'е реализует SkipDescendants, поэтому walker не доходит до детей.
package schema

import (
	"fmt"
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/generator/walk"
	"nschugorev/oapigenerator/internal/parser"
	"strconv"
	"strings"
)

// validatorPkg — import-path runtime-пакета validator.Registry.
const validatorPkg = "nschugorev/oapigenerator/pkg/validator"

// ValidateOwnRenderer рендерит ValidateOwn-метод для object-схем с
// x-validations. Срабатывает на OnStruct (моно-режим, <Name>) и OnSplitStruct
// (<Name>Request + <Name>Response). Update-вариант (Update<Name>) рендерится
// UpdateStructRenderer'ом через общий renderValidateOwnInto.
type ValidateOwnRenderer struct {
	render.Base
	walk.NoopSchemaRenderer
}

// NewValidateOwnRenderer возвращает ValidateOwnRenderer с нулевым состоянием.
// Buf и Imports вливаются через Base.Init в compose.FileComposer перед обходом.
func NewValidateOwnRenderer() *ValidateOwnRenderer { return &ValidateOwnRenderer{} }

// OnStruct рендерит ValidateOwn для основной <Name>-структуры в моно-режиме.
// Если валидаций нет — noop. Update-вариант здесь НЕ рендерится — он рендерится
// UpdateStructRenderer'ом (Task 5) через общий renderValidateOwnInto.
func (r *ValidateOwnRenderer) OnStruct(s *parser.Schema) error {
	// Гарантируем, что после возврата renderer'а mode сброшен в "", чтобы
	// последующие renderer'ы не видели stale modeRequest/modeResponse от
	// StructRenderer. defer срабатывает даже при раннем return.
	defer r.Ctx.TypeMapper.SetMode("")

	// Явно сбрасываем режим на входе: walker вызывает OnStruct на каждом
	// renderer'е последовательно, и StructRenderer может оставить mode в
	// modeRequest/modeResponse. ValidateOwnRenderer отвечает за свой режим
	// сам — main variant рендерится в моно-режиме.
	r.Ctx.TypeMapper.SetMode("")

	renderValidateOwnInto(&r.Base, s, goName(s.Name), false, nil)

	return nil
}

// OnSplitStruct рендерит ValidateOwn отдельно для <Name>Request и
// <Name>Response, когда включён GOLANG_SPLIT_REQUEST_RESPONSE.
// TypeMapper переключается между modeRequest/modeResponse, чтобы $ref на
// splittable-схемы разрешался корректно (Required-логика зависит от режима).
//
// Update-вариант в split-режиме рендерится UpdateStructRenderer'ом (Task 5)
// через общий renderValidateOwnInto.
func (r *ValidateOwnRenderer) OnSplitStruct(s *parser.Schema) error {
	defer r.Ctx.TypeMapper.SetMode("")

	if !hasValidationRules(s) {
		return nil
	}

	name := goName(s.Name)

	reqKeep := func(p *parser.Property) bool { return p.Schema == nil || !p.Schema.ReadOnly }
	respKeep := func(p *parser.Property) bool { return p.Schema == nil || !p.Schema.WriteOnly }

	r.Ctx.TypeMapper.SetMode(modeRequest)
	renderValidateOwnInto(&r.Base, s, name+"Request", false, reqKeep)

	r.Ctx.TypeMapper.SetMode(modeResponse)
	renderValidateOwnInto(&r.Base, s, name+"Response", false, respKeep)

	return nil
}

// renderValidateOwnInto рендерит
// `func (x <Name>) ValidateOwn(reg *validator.Registry) error` в общий
// Buf/Imports через b. Используется ValidateOwnRenderer (для main-варианта)
// и UpdateStructRenderer (для Update-варианта).
//
// isUpdate=true — рендер для Update<Name> (поля обёрнуты в Optional[T]),
// isUpdate=false — рендер для основной <Name> структуры.
//
// keep фильтрует properties (nil = все). Для split-вариантов передаётся
// фильтр Request/Response.
//
// Тело перенесено из Generator.renderValidateOwn (validate.go:40-89) с
// заменами: m.addImport → b.Imports.Add, w.Print → b.Buf.Print,
// g.renderPropertyValidations → renderPropertyValidationsInto,
// renderNamedValidatorCall(w, ...) → renderNamedValidatorCall(b.Buf, ...).
func renderValidateOwnInto(
	b *render.Base,
	s *parser.Schema,
	name string,
	isUpdate bool,
	keep func(*parser.Property) bool,
) {
	if !hasValidationRules(s) {
		return
	}

	b.Imports.Add(gogen.Import{Path: validatorPkg, Alias: "validator"})
	b.Imports.Add(gogen.Import{Path: "fmt"})

	receiver := "x"
	b.Buf.Print("func (", receiver, " ", name, ") ValidateOwn(reg *validator.Registry) error {\n")

	// Property-level rules.
	for _, p := range s.Properties {
		if len(p.Validations) == 0 {
			continue
		}

		if keep != nil && !keep(p) {
			continue
		}

		renderPropertyValidationsInto(b, p, receiver, isUpdate)
	}

	// Schema-level rules (cross-field named validators).
	// Skipped for Update structs: validator is registered for the main
	// struct type (ItemCreate), but Update receiver is UpdateItemCreate —
	// different type. Cross-field validation on PATCH-bodies needs separate
	// semantics (Optional[T] fields) and is out of scope for v1.
	if !isUpdate {
		for _, rule := range s.Validations {
			nr, ok := rule.(parser.NamedRule)
			if !ok {
				continue
			}

			renderNamedValidatorCall(b.Buf, nr.Name, receiver, "")
		}
	}

	b.Buf.Print("\treturn nil\n")
	b.Buf.Print("}\n\n")
}

// renderPropertyValidationsInto рендерит все правила одного свойства в общий
// Buf/Imports через b. Для каждого правила — guard (если нужен) + проверка.
//
// Тело перенесено из Generator.renderPropertyValidations (validate.go:93-117)
// с заменами: w.Print → b.Buf.Print,
// g.renderSimpleRule → renderSimpleRuleInto,
// g.fieldAccessor → fieldAccessorInto,
// renderNamedValidatorCallIndented(w, ...) → renderNamedValidatorCallIndented(b.Buf, ...).
func renderPropertyValidationsInto(
	b *render.Base,
	p *parser.Property,
	receiver string,
	isUpdate bool,
) {
	fieldName := goName(p.Name)

	for _, rule := range p.Validations {
		switch r2 := rule.(type) {
		case parser.SimpleRule:
			renderSimpleRuleInto(b, r2, p, receiver, fieldName, isUpdate)
		case parser.NamedRule:
			accessor, guard := fieldAccessorInto(b, p, receiver, isUpdate, parser.TargetValue)
			if guard != "" {
				b.Buf.Print("\tif ", guard, " {\n")
				renderNamedValidatorCallIndented(b.Buf, r2.Name, accessor, fieldName, "\t")
				b.Buf.Print("\t}\n")
			} else {
				renderNamedValidatorCallIndented(b.Buf, r2.Name, accessor, fieldName, "")
			}
		}
	}
}

// renderSimpleRuleInto рендерит inline-проверку для SimpleRule в общий
// Buf/Imports через b. Инвертирует оператор: правило ">0" → ошибка если "<=0".
//
// Тело перенесено из Generator.renderSimpleRule (validate.go:121-147) с
// заменами: g.fieldAccessor → fieldAccessorInto, w.Print → b.Buf.Print.
func renderSimpleRuleInto(
	b *render.Base,
	rule parser.SimpleRule,
	p *parser.Property,
	receiver, fieldName string,
	isUpdate bool,
) {
	accessor, guard := fieldAccessorInto(b, p, receiver, isUpdate, rule.Target)
	literal := formatValueLiteral(rule.Value)

	inverseOp := inverseOperator(rule.Op)
	condition := accessor + " " + inverseOp + " " + literal

	msg := fmt.Sprintf("field %s: must be %s %s", fieldName, opSymbol(rule.Op), literal)

	if guard != "" {
		b.Buf.Print("\tif ", guard, " && ", condition, " {\n")
		b.Buf.Print("\t\treturn fmt.Errorf(", strconv.Quote(msg), ")\n")
		b.Buf.Print("\t}\n")
	} else {
		b.Buf.Print("\tif ", condition, " {\n")
		b.Buf.Print("\t\treturn fmt.Errorf(", strconv.Quote(msg), ")\n")
		b.Buf.Print("\t}\n")
	}
}

// fieldAccessorInto возвращает (accessor, guard) для поля в зависимости от
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
// Тело перенесено из Generator.fieldAccessor (validate.go:165-217) с
// заменами: g.project.Features.UseOptional.Value → b.Ctx.Features.UseOptional.Value,
// m.goType → b.Ctx.TypeMapper.GoType,
// g.requiredForMode(p, m.mode) → requiredForMode(b.Ctx, p),
// fieldIsOptional → package-level.
//
//nolint:gocritic // unnamedResult conflicts with nonamedreturns
func fieldAccessorInto(
	b *render.Base,
	p *parser.Property,
	receiver string,
	isUpdate bool,
	target parser.Target,
) (string, string) {
	fieldName := goName(p.Name)
	fieldRef := receiver + "." + fieldName

	// optional-wrapped field: Update-struct always, main-struct when
	// UseOptional flag is on AND x-optional marker is set.
	wrapped := isUpdate || (b.Ctx.Features.UseOptional.Value && p.Optional)

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
	fieldType := b.Ctx.TypeMapper.GoType(p.Schema)
	required := requiredForMode(b.Ctx, p)

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

// hasValidationRules проверяет, есть ли хотя бы одно правило валидации
// у схемы (schema-level) или её свойств (property-level).
//
// Перенесена из validate.go:16-28 — чистая функция без состояния.
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

// inverseOperator возвращает оператор для условия ошибки.
// Правило ">N" → ошибка если "<=N".
//
// Перенесена из validate.go:221-238 — чистая функция.
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
//
// Перенесена из validate.go:241-258 — чистая функция.
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
//
// Перенесена из validate.go:262-268 — чистая функция.
func formatValueLiteral(v float64) string {
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}

	return strconv.FormatFloat(v, 'f', -1, 64)
}

// renderNamedValidatorCall рендерит lookup + вызов именованного валидатора.
// fieldPath — путь поля для сообщения об ошибке ("Owner.Tags"); пустой
// для schema-level валидатора.
//
// Перенесена из validate.go:273-275 — делегирует в indented-версию.
func renderNamedValidatorCall(w *codegen.BufferWriter, name, accessor, fieldPath string) {
	renderNamedValidatorCallIndented(w, name, accessor, fieldPath, "")
}

// renderNamedValidatorCallIndented — то же с отступом indent для вложенных
// в guard-блоки.
//
// Каждый lookup+вызов обёрнут в свой block-scope `{ ... }`, чтобы `v, ok :=`
// не конфликтовало при повторных вызовах в том же ValidateOwn (property-level
// + schema-level валидаторы).
//
// Перенесена из validate.go:283-307 — чистая функция, пишет в w.
func renderNamedValidatorCallIndented(
	w *codegen.BufferWriter,
	name, accessor, fieldPath, indent string,
) {
	notRegisteredMsg := strconv.Quote("validator %q not registered")

	w.Print(indent, "\t{\n")
	w.Print(indent, "\t\tv, ok := reg.Get(", strconv.Quote(name), ")\n")
	w.Print(indent, "\t\tif !ok {\n")
	w.Print(indent, "\t\t\treturn fmt.Errorf(", notRegisteredMsg, ", ", strconv.Quote(name), ")\n")
	w.Print(indent, "\t\t}\n")

	if fieldPath == "" {
		// Schema-level — ошибка без обёртывания путём поля.
		w.Print(indent, "\t\tif err := v.Validate(", accessor, "); err != nil {\n")
		w.Print(indent, "\t\t\treturn err\n")
		w.Print(indent, "\t\t}\n")
	} else {
		w.Print(indent, "\t\tif err := v.Validate(", accessor, "); err != nil {\n")
		w.Print(indent, "\t\t\treturn fmt.Errorf(", strconv.Quote("field "+fieldPath+": %w"), ", err)\n")
		w.Print(indent, "\t\t}\n")
	}

	w.Print(indent, "\t}\n")
}
