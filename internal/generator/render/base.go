// Package render содержит renderer'ы — реактивные компоненты, подписанные
// на хуки из package walk. Каждый renderer пишет в собственный BufferWriter.
package render

import (
	"bytes"
	"path"

	"github.com/ilovepitsa/oapicodegen/internal/codegen"
	"github.com/ilovepitsa/oapicodegen/internal/codegen/gogen"
	"github.com/ilovepitsa/oapicodegen/internal/parser"
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
	// Imports — общий ImportTracker, устанавливаемый compose.FileComposer
	// через Base.Init. TypeMapper-adapter читает его для дренажа импортов
	// после каждого вызова GoType/BaseType. nil до Init (renderer'ы,
	// вызываемые вне composer-пути, должны установить его вручную —
	// см. alias_test.go newAliasTestRenderer).
	Imports *ImportTracker
	// Diagnostics — коллектор аудита генерации (Task C: GOLANG_SCHEMA_ANY).
	// Инициализируется Generator'ом в обоих render-context builder'ах.
	// nil допустим — renderer'ы обязаны проверять перед Append (см.
	// schema_any_audit.go reportIfAny в последующих задачах).
	Diagnostics *Collector
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

// PruneUnused удаляет импорты, чей квалификатор не встречается в body как
// `<qual>.` (Go-qualified identifier). Квалификатор — Alias, если задан, иначе
// последний сегмент Path (конвенция Go: имя пакета = последний сегмент).
//
// Применяется renderer'ами, которые вызывают TypeMapper.GoType для сравнения
// типов, но не всегда эмитят тип в тело (ConvertersRenderer для direct-copy
// полей): GoType добавляет import как side-effect (qualifyUTCTime/
// qualifyModelType), и для direct-copy полей import остаётся unused →
// compile-error "imported and not used". Prune убирает такие висячие импорты.
//
// Безопасно для сгенерированного кода: импорты всегда используются как
// `pkg.Identifier` (dot/blank-imports генератор не эмитит).
func (t *ImportTracker) PruneUnused(body []byte) {
	kept := make([]gogen.Import, 0, len(t.imports))

	for _, imp := range t.imports {
		qual := imp.Alias
		if qual == "" {
			qual = path.Base(imp.Path)
		}

		if bytes.Contains(body, []byte(qual+".")) {
			kept = append(kept, imp)
		}
	}

	t.imports = kept
}

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

// Init перезаписывает все три поля на ресивере. Используется compose-пакетом,
// чтобы влить shared Buf/Imports в каждый renderer через embed Base.
// Также прокидывает Imports в ctx — typeMapperAdapter читает ctx.Imports
// для дренажа (см. writeStructFileViaComposer).
func (b *Base) Init(buf *codegen.BufferWriter, imports *ImportTracker, ctx *RenderContext) {
	b.Buf = buf
	b.Imports = imports
	b.Ctx = ctx
	ctx.Imports = imports
}

// SingletonRenderer — renderer, производящий один файл. Возвращает тело,
// трекер импортов и путь для записи.
type SingletonRenderer interface {
	Render(ctx *RenderContext) (body []byte, imports *ImportTracker, err error)
	FilePath() string
}

// PackageNamer — optional-интерфейс для renderer'ов, у которых Go-имя пакета
// отличается от имени директории, выводимого из FilePath.
// Если SingletonRenderer реализует PackageNamer, ComposeSingletonFile
// использует PackageName() вместо вывода из FilePath.
type PackageNamer interface {
	PackageName() string
}
