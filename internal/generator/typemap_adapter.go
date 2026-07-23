package generator

import (
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
)

// typeMapperAdapter bridg'ит внутренний *typeMapper пакета generator к
// интерфейсу render.TypeMapper. После каждого вызова GoType/BaseType
// накопленные typeMapper'ом импорты дренажатся в ctx.Imports — общий
// ImportTracker, установленный compose.FileComposer через Base.Init.
// Так рендер alias/enum/struct/json получает корректные импорты (time,
// model, optional, validator и т.п.) без дублирования логики разрешения
// типов. До Init (renderer'ы вне composer-пути) ctx.Imports может быть nil —
// в этом случае импорты накапливаются в typeMapper.imports и теряются
// (тестовые fakes не используют импорты).
type typeMapperAdapter struct {
	tm  *typeMapper
	ctx *render.RenderContext
}

// tracker возвращает общий ImportTracker из ctx. nil-safe: если ctx.Imports
// не установлен (тестовый сценарий без composer), импорты не дренажатся.
func (a *typeMapperAdapter) tracker() *render.ImportTracker {
	if a.ctx == nil {
		return nil
	}

	return a.ctx.Imports
}

// drain переносит накопленные с момента before импорты из typeMapper.imports
// в общий ImportTracker (если он установлен).
func (a *typeMapperAdapter) drain(before int) {
	t := a.tracker()
	if t == nil {
		return
	}

	for _, imp := range a.tm.imports[before:] {
		t.Add(imp)
	}
}

// GoType делегирует вызов typeMapper.goType и переносит накопленные с момента
// предыдущего вызова импорты в общий ImportTracker renderer'а.
func (a *typeMapperAdapter) GoType(s *parser.Schema) string {
	before := len(a.tm.imports)
	result := a.tm.goType(s)
	a.drain(before)

	return result
}

// SetMode переключает режим typeMapper'а ("", "Request", "Response").
// StructRenderer использует это при split-рендере: для <Name>Request —
// mode="Request", для <Name>Response — mode="Response", чтобы $ref на
// splittable-схемы разрешались в правильный идентификатор.
func (a *typeMapperAdapter) SetMode(mode string) {
	a.tm.mode = mode
}

// Mode возвращает текущий режим typeMapper'а. StructRenderer использует это
// для передачи mode в callbacks (SetDefaults/ValidateOwn) — последний
// установленный через SetMode режим и есть "текущий".
func (a *typeMapperAdapter) Mode() string {
	return a.tm.mode
}

// BaseType делегирует вызов typeMapper.baseType и дренажит накопленные
// импорты в tracker. Возвращает Go-тип без nullable-указателя —
// StructRenderer.updateFieldType использует это для Update<Name> полей
// (Optional[T] уже различает null, pointer не нужен).
func (a *typeMapperAdapter) BaseType(s *parser.Schema) string {
	before := len(a.tm.imports)
	result := a.tm.baseType(s)
	a.drain(before)

	return result
}

// newRenderTypeMapper строит свежий *typeMapper для пакета pkg и режима mode
// (modeRequest / modeResponse / "") и оборачивает его в adapter, привязанный
// к ctx. Свежий *typeMapper нужен, потому что импорты накапливаются в нём
// локально — переиспользование между схемами привело бы к утечке импортов
// одной схемы в файл другой. ctx.Imports проставляется compose.FileComposer
// через Base.Init после конструирования adapter'а.
// subPkg — SubPackage схемы для cross-subpackage import'ов.
func (g *Generator) newRenderTypeMapper(
	pkg, mode, subPkg string,
	ctx *render.RenderContext,
) *typeMapperAdapter {
	m := g.newTypeMapper(pkg)
	m.mode = mode
	m.subPkg = subPkg

	return &typeMapperAdapter{tm: m, ctx: ctx}
}

// compile-time conformance: adapter удовлетворяет интерфейсу render.TypeMapper.
var _ render.TypeMapper = (*typeMapperAdapter)(nil)
