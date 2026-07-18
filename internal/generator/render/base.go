// Package render содержит renderer'ы — реактивные компоненты, подписанные
// на хуки из package walk. Каждый renderer пишет в собственный BufferWriter.
package render

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/parser"
)

// RenderContext — общий контекст для всех renderer'ов в рамках одного проекта.
type RenderContext struct {
	Project      *parser.Project
	SchemaIndex  *parser.SchemaIndex
	Features     parser.ProjectFeatures
	Splittable   map[string]bool
	ModulePath   string
	ImportPrefix string
	TypeMapper   TypeMapper
}

// ImportTracker оборачивает []gogen.Import и дедуплицирует по Path+Alias.
// Определён здесь, а не в gogen, чтобы оставить gogen чистым структурным
// описанием файла без состояния.
type ImportTracker struct {
	imports []gogen.Import
}

// NewImportTracker возвращает пустой трекер.
func NewImportTracker() *ImportTracker {
	return &ImportTracker{imports: make([]gogen.Import, 0)}
}

// Add добавляет imp, если ещё нет такой пары Path+Alias.
func (t *ImportTracker) Add(imp gogen.Import) {
	for _, existing := range t.imports {
		if existing.Path == imp.Path && existing.Alias == imp.Alias {
			return
		}
	}

	t.imports = append(t.imports, imp)
}

// Imports возвращает накопленный срез импортов.
func (t *ImportTracker) Imports() []gogen.Import { return t.imports }

// Base — общий встраиваемый тип для renderer'ов.
type Base struct {
	Buf     *codegen.BufferWriter
	Imports *ImportTracker
	Ctx     *RenderContext
}

// NewBase создаёт Base с свежими Buf и ImportTracker и привязанным ctx.
func NewBase(ctx *RenderContext) Base {
	return Base{
		Buf:     codegen.NewBufferWriter(),
		Imports: NewImportTracker(),
		Ctx:     ctx,
	}
}

// Init перезаписывает все три поля на ресивере. Используется compose-пакетом
// (Task 6), чтобы влить shared Buf/Imports в каждый renderer через embed Base.
func (b *Base) Init(buf *codegen.BufferWriter, imports *ImportTracker, ctx *RenderContext) {
	b.Buf = buf
	b.Imports = imports
	b.Ctx = ctx
}

// SingletonRenderer — renderer, производящий один файл. Возвращает тело,
// трекер импортов и путь для записи.
type SingletonRenderer interface {
	Render(ctx *RenderContext) (body []byte, imports *ImportTracker, err error)
	FilePath() string
}
