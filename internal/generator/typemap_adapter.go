package generator

import (
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
)

// typeMapperAdapter bridg'ит внутренний *typeMapper пакета generator к
// интерфейсу render.TypeMapper. После каждого вызова GoType накопленные
// typeMapper'ом импорты дренажатся в tracker renderer'а — так рендер
// alias/enum/map-alias получает корректные импорты (time, model, optional и
// т.п.) без дублирования логики разрешения типов.
type typeMapperAdapter struct {
	tm      *typeMapper
	tracker *render.ImportTracker
}

// GoType делегирует вызов typeMapper.goType и переносит накопленные с момента
// предыдущего вызова импорты в общий ImportTracker renderer'а.
func (a *typeMapperAdapter) GoType(s *parser.Schema) string {
	before := len(a.tm.imports)
	result := a.tm.goType(s)

	for _, imp := range a.tm.imports[before:] {
		a.tracker.Add(imp)
	}

	return result
}

// newRenderTypeMapper строит свежий *typeMapper для пакета pkg и режима mode
// (modeRequest / modeResponse / "") и оборачивает его в adapter, привязанный
// к tracker renderer'а. Свежий *typeMapper нужен, потому что импорты
// накапливаются в нём локально — переиспользование между схемами привело бы к
// утечке импортов одной схемы в файл другой.
func (g *Generator) newRenderTypeMapper(
	pkg, mode string,
	tracker *render.ImportTracker,
) *typeMapperAdapter {
	m := g.newTypeMapper(pkg)
	m.mode = mode

	return &typeMapperAdapter{tm: m, tracker: tracker}
}

// compile-time conformance: adapter удовлетворяет интерфейсу render.TypeMapper.
var _ render.TypeMapper = (*typeMapperAdapter)(nil)
