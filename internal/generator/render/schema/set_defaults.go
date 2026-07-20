// Package schema: SetDefaultsRenderer рендерит `func (m *<Name>) SetDefaults()`
// для object-схем, у которых в дереве (через $ref) есть хотя бы одно property
// с Default != nil. Метод проставляет default-значения для полей и рекурсивно
// вызывает SetDefaults для вложенных object-полей с $ref.
//
// Портирован из Generator.renderSetDefaultsMethod + renderSetDefaultForField +
// renderNestedSetDefaultsCall (internal/generator/set_defaults.go). Оригиналы
// удалены в Task 8 (мост Callbacks больше не используется).
//
// Renderer embed'ит render.Base (Buf/Imports/Ctx) и walk.NoopSchemaRenderer
// (остальные хуки не нужны). Skip не реализуется — walker и так не спускается
// в properties, т.к. StructRenderer реализует SkipDescendants и стоит первым
// в pack'е (после него walker не доходит до SetDefaultsRenderer по детям).
package schema

import (
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/generator/walk"
	"nschugorev/oapigenerator/internal/parser"
	"strings"
)

// SetDefaultsRenderer рендерит SetDefaults-метод для object-схем.
// Срабатывает на OnStruct (моно-режим, <Name>) и OnSplitStruct
// (<Name>Request + <Name>Response, если у варианта есть defaults).
type SetDefaultsRenderer struct {
	render.Base
	walk.NoopSchemaRenderer
}

// NewSetDefaultsRenderer возвращает SetDefaultsRenderer с нулевым состоянием.
// Buf и Imports вливаются через Base.Init в compose.FileComposer перед обходом.
func NewSetDefaultsRenderer() *SetDefaultsRenderer { return &SetDefaultsRenderer{} }

// OnStruct рендерит `func (m *<Name>) SetDefaults()` в моно-режиме (без split).
// Если в дереве схемы нет defaults — noop.
func (r *SetDefaultsRenderer) OnStruct(s *parser.Schema) error {
	// Гарантируем, что после возврата renderer'а mode сброшен в "", чтобы
	// последующие renderer'ы не видели stale modeRequest/modeResponse от
	// StructRenderer или от собственного рендера. defer срабатывает даже
	// при раннем return.
	defer r.Ctx.TypeMapper.SetMode("")

	// Явно сбрасываем режим на входе: walker вызывает OnSplitStruct/OnStruct
	// на каждом renderer'е последовательно, и StructRenderer может оставить
	// mode в modeRequest/modeResponse. SetDefaultsRenderer отвечает за свой
	// режим сам.
	r.Ctx.TypeMapper.SetMode("")

	if !treeHasDefaultsWithVisited(s, nil, map[string]bool{s.Name: true}, r.projectModel()) {
		return nil
	}

	r.renderSetDefaults(s, goName(s.Name), nil)

	return nil
}

// OnSplitStruct рендерит SetDefaults отдельно для <Name>Request и
// <Name>Response, когда включён GOLANG_SPLIT_REQUEST_RESPONSE. Каждый
// вариант рендерится только если среди отфильтрованных properties есть
// хотя бы один default.
//
// TypeMapper переключается между modeRequest/modeResponse, чтобы $ref на
// splittable-схемы разрешался корректно (Required-логика и Go-типы зависят
// от режима).
func (r *SetDefaultsRenderer) OnSplitStruct(s *parser.Schema) error {
	// Гарантируем, что после возврата renderer'а mode сброшен в "", чтобы
	// последующие renderer'ы не видели stale modeResponse от рендера Response
	// варианта. defer срабатывает даже при панике.
	defer r.Ctx.TypeMapper.SetMode("")

	name := goName(s.Name)

	reqKeep := func(p *parser.Property) bool { return p.Schema == nil || !p.Schema.ReadOnly }
	respKeep := func(p *parser.Property) bool { return p.Schema == nil || !p.Schema.WriteOnly }

	model := r.projectModel()

	r.Ctx.TypeMapper.SetMode(modeRequest)
	if treeHasDefaultsWithVisited(s, reqKeep, map[string]bool{s.Name: true}, model) {
		r.renderSetDefaults(s, name+"Request", reqKeep)
	}

	r.Ctx.TypeMapper.SetMode(modeResponse)
	if treeHasDefaultsWithVisited(s, respKeep, map[string]bool{s.Name: true}, model) {
		r.renderSetDefaults(s, name+"Response", respKeep)
	}

	return nil
}

// projectModel возвращает *parser.Model из контекста или nil, если контекст
// или Project не задан (тестовый сценарий без composer'а). nil-safe:
// treeHasDefaultsWithVisited и resolveRefSchema корректно обрабатывают
// nil-Model (пропускают $ref-разрешение).
func (r *SetDefaultsRenderer) projectModel() *parser.Model {
	if r.Ctx == nil || r.Ctx.Project == nil {
		return nil
	}

	return r.Ctx.Project.Model
}

// renderSetDefaults рендерит тело `func (m *<Name>) SetDefaults() { ... }`,
// проставляющий default-значения для полей с Default != nil, а также
// рекурсивно вызывающий SetDefaults для вложенных object-полей с $ref.
//
// keep фильтрует properties: для моно-режима nil, для split — фильтр
// Request/Response варианта.
//
// Тело перенесено из Generator.renderSetDefaultsMethod (set_defaults.go:22-53)
// с заменами: g.renderSetDefaultForField → r.renderSetDefaultForField,
// g.renderNestedSetDefaultsCall → r.renderNestedSetDefaultsCall,
// w.Print → r.Buf.Print.
func (r *SetDefaultsRenderer) renderSetDefaults(
	s *parser.Schema,
	name string,
	keep func(*parser.Property) bool,
) {
	r.Buf.Print("func (m *", name, ") SetDefaults() {\n")

	for _, p := range s.Properties {
		if keep != nil && !keep(p) {
			continue
		}

		if p.Schema == nil {
			continue
		}

		// Поле с собственным Default — рендерим присваивание литерала.
		if p.Schema.Default != nil {
			r.renderSetDefaultForField(p)

			continue
		}

		// Поле без Default, но с $ref на object-схему, у которой есть
		// defaults — рекурсивно вызываем <Field>.SetDefaults().
		r.renderNestedSetDefaultsCall(p, keep)
	}

	r.Buf.Print("}\n\n")
}

// renderSetDefaultForField рендерит блок `if m.<Field> == <zero> { m.<Field> = <literal> }`
// для одного property. Опциональные поля (pointer) проверяются на nil,
// required — на zero-value примитива.
//
// Для optional (pointer) полей literal оборачивается в локальную переменную
// и берётся указатель: `if m.Field == nil { v := <literal>; m.Field = &v }`,
// так как прямое присваивание `m.Field = <literal>` не компилируется для
// pointer-типов (*int, *string, *Code и т.д.).
//
// Тело перенесено из Generator.renderSetDefaultForField (set_defaults.go:63-108)
// с заменами: m.goType → r.Ctx.TypeMapper.GoType,
// g.requiredForMode(p, m.mode) → r.requiredForMode(p) (делегирует в package-level),
// fieldIsOptional → package-local, w.Print → r.Buf.Print.
func (r *SetDefaultsRenderer) renderSetDefaultForField(p *parser.Property) {
	// B2: форматы, маппящиеся на не-примитивные Go-типы (time.Time, []byte),
	// не поддерживаются — присваивание строкового литерала не компилируется.
	//
	// Future work: support date-time/date defaults via time.Parse.
	if isNonPrimitiveStringFormat(p.Schema) {
		return
	}

	fieldName := goName(p.Name)
	goType := r.Ctx.TypeMapper.GoType(p.Schema)

	optional := fieldIsOptional(r.requiredForMode(p), goType)
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
		r.Buf.Print("\tif m.", fieldName, " == nil {\n")
		r.Buf.Print("\t\tv := ", literal, "\n")
		r.Buf.Print("\t\tm.", fieldName, " = &v\n")
		r.Buf.Print("\t}\n")

		return
	}

	r.Buf.Print("\tif m.", fieldName, " == ", zeroValueLiteral(p.Schema), " {\n")
	r.Buf.Print("\t\tm.", fieldName, " = ", literal, "\n")
	r.Buf.Print("\t}\n")
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
//
// Тело перенесено из Generator.renderNestedSetDefaultsCall (set_defaults.go:119-147)
// с заменами: g.resolveRefSchema → resolveRefSchema (с r.Ctx.Project.Model),
// g.schemaTreeHasDefaults → treeHasDefaultsWithVisited (с r.Ctx.Project.Model),
// m.goType → r.Ctx.TypeMapper.GoType,
// g.requiredForMode(p, m.mode) → r.requiredForMode(p),
// fieldIsOptional → package-local, w.Print → r.Buf.Print.
func (r *SetDefaultsRenderer) renderNestedSetDefaultsCall(
	p *parser.Property,
	keep func(*parser.Property) bool,
) {
	model := r.projectModel()

	target := resolveRefSchema(p.Schema, model)
	if target == nil || len(target.Properties) == 0 {
		return
	}

	if !treeHasDefaultsWithVisited(target, keep, map[string]bool{target.Name: true}, model) {
		return
	}

	fieldName := goName(p.Name)
	fieldType := r.Ctx.TypeMapper.GoType(p.Schema)

	optional := fieldIsOptional(r.requiredForMode(p), fieldType)
	if optional {
		r.Buf.Print("\tif m.", fieldName, " != nil {\n")
		r.Buf.Print("\t\tm.", fieldName, ".SetDefaults()\n")
		r.Buf.Print("\t}\n")

		return
	}

	r.Buf.Print("\tm.", fieldName, ".SetDefaults()\n")
}

// requiredForMode делегирован в package-level requiredForMode (см. mode.go).
// Сохранён как метод-обёртка для симметрии с StructRenderer и удобства
// вызовов r.requiredForMode(p) внутри SetDefaultsRenderer.
func (r *SetDefaultsRenderer) requiredForMode(p *parser.Property) bool {
	return requiredForMode(r.Ctx, p)
}
