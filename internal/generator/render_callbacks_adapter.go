package generator

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
)

// generatorCallbacks адаптирует Generator к render.SchemaCallbacks.
// Каждый вызов строит свежий typeMapper (режим копируется из mode),
// запускает существующий Generator-метод, который пишет в w и накапливает
// импорты в typeMapper.imports, затем дренажит импорты в ctx.Imports —
// общий ImportTracker renderer'а, установленный compose.FileComposer
// через Base.Init.
//
// mode нужен, потому что существующие методы используют m.mode для:
//   - qualifyModelType: суффикс "Request"/"Response" для splittable-схем;
//   - requiredForMode: выбор p.RequestRequired / p.ResponseRequired.
//
// Когда Tasks 10-11 переедут в render/, этот adapter можно будет удалить.
type generatorCallbacks struct {
	g   *Generator
	ctx *render.RenderContext
}

// tracker возвращает общий ImportTracker из ctx. nil-safe: если ctx.Imports
// не установлен (тестовый сценарий без composer), импорты не дренажатся.
func (c *generatorCallbacks) tracker() *render.ImportTracker {
	if c.ctx == nil {
		return nil
	}

	return c.ctx.Imports
}

// RenderSetDefaults рендерит SetDefaults для схемы sh. Свежий typeMapper
// инициализируется режимом mode (переданным StructRenderer'ом через
// currentMode), после рендера импорты дренажатся в ctx.Imports.
func (c *generatorCallbacks) RenderSetDefaults(
	w *codegen.BufferWriter,
	sh *parser.Schema,
	mode, name string,
	keep func(*parser.Property) bool,
) {
	m := c.g.newTypeMapper("model")
	m.mode = mode
	c.g.renderSetDefaultsMethod(w, sh, m, name, keep)
	c.drainImports(m)
}

// RenderValidateOwn рендерит ValidateOwn для схемы sh. isUpdate различает
// Update<Name>-вариант (поля обёрнуты в Optional[T], cross-field валидаторы
// пропускаются). Свежий typeMapper с режимом mode, дренаж импортов после.
func (c *generatorCallbacks) RenderValidateOwn(
	w *codegen.BufferWriter,
	sh *parser.Schema,
	mode, name string,
	isUpdate bool,
	keep func(*parser.Property) bool,
) {
	m := c.g.newTypeMapper("model")
	m.mode = mode
	c.g.renderValidateOwn(w, sh, m, name, isUpdate, keep)
	c.drainImports(m)
}

// SchemaTreeHasDefaults делегирует к g.schemaTreeHasDefaults с свежим
// visited-set (sh.Name помечается посещённым). Импортов не накапливает —
// метод только читает дерево схем.
func (c *generatorCallbacks) SchemaTreeHasDefaults(
	sh *parser.Schema,
	keep func(*parser.Property) bool,
) bool {
	if sh == nil {
		return false
	}

	return c.g.schemaTreeHasDefaults(sh, keep, map[string]bool{sh.Name: true})
}

// drainImports переносит накопленные в typeMapper.imports элементы в
// ctx.Imports. Дедупликация — на стороне ImportTracker.Add.
func (c *generatorCallbacks) drainImports(m *typeMapper) {
	t := c.tracker()
	if t == nil {
		return
	}

	for _, imp := range m.imports {
		t.Add(imp)
	}
}

// compile-time conformance: generatorCallbacks удовлетворяет интерфейсу.
var _ render.SchemaCallbacks = (*generatorCallbacks)(nil)
