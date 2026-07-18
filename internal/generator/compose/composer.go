// Package compose — FileComposer, оркестрирующий walker+renderer прогоны и
// собирающий выходные файлы. Это фундамент для миграции renderer'ов (Tasks 7+).
package compose

import (
	"fmt"
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/generator/walk"
	"nschugorev/oapigenerator/internal/parser"
	"strings"
)

// FileComposer оркестрирует walker-прогоны и сборку codegen.File через
// gogen.FileFactory. Shared Buf и ImportTracker вливаются в каждый renderer
// через optional-интерфейс injectableRenderer (см. render.Base.Init) — это
// позволяет нескольким renderer'ам писать в один файл без явной координации.
type FileComposer struct {
	FF *gogen.FileFactory
}

// NewFileComposer строит composer с заданной FileFactory.
func NewFileComposer(ff *gogen.FileFactory) *FileComposer {
	return &FileComposer{FF: ff}
}

// injectableRenderer — optional-интерфейс. Renderer'ы, embed'ящие render.Base,
// удовлетворяют ему и получают shared Buf/Imports через Init перед обходом.
// Сигнатура зеркалит render.Base.Init один-в-один.
type injectableRenderer interface {
	Init(buf *codegen.BufferWriter, imports *render.ImportTracker, ctx *render.RenderContext)
}

// ComposeSchemaFile обходит схему s с набором schema-renderer'ов, собирает
// накопленные renderer'ами байты и импорты в один codegen.File пакета "model".
//
// Пакет жёстко зафиксирован как "model": schema-файлы всегда пишутся в model/.
// Имя файла определяет вызывающая сторона (composer не знает имени — это
// ответственность file-write слоя), поэтому возвращается только File.
func (c *FileComposer) ComposeSchemaFile(
	s *parser.Schema,
	renderers []walk.SchemaRenderer,
	ctx *render.RenderContext,
) (codegen.File, error) {
	buf, imports := c.injectAll(schemaInjectable(renderers), ctx)

	walker := walk.NewSchemaWalker(renderers...)
	if err := walker.Walk(s); err != nil {
		return nil, fmt.Errorf("compose schema %q: %w", s.Name, err)
	}

	return c.assemble("model", buf, imports), nil
}

// ComposeMethodFile обходит каждый метод из methods с набором
// method-renderer'ов, собирает накопленные байты и импорты в один codegen.File
// пакета pkgPath. Все методы складываются в один файл (типичный сценарий —
// client.go или server.go для набора операций).
func (c *FileComposer) ComposeMethodFile(
	pkgPath string,
	methods []*parser.Method,
	renderers []walk.MethodRenderer,
	ctx *render.RenderContext,
) (codegen.File, error) {
	buf, imports := c.injectAll(methodInjectable(renderers), ctx)

	walker := walk.NewMethodWalker(renderers...)
	for _, m := range methods {
		if err := walker.Walk(m); err != nil {
			return nil, fmt.Errorf("compose method %q: %w", m.OperationID, err)
		}
	}

	return c.assemble(pkgPath, buf, imports), nil
}

// ComposeSingletonFile вызывает singleton-renderer напрямую (без walker — у
// singleton нет структуры для обхода), получает готовые body+imports и
// собирает codegen.File. Пакет выводится из FilePath() как первый сегмент пути
// до '/' (например, "model/utc_time.gen.go" → "model").
func (c *FileComposer) ComposeSingletonFile(
	r render.SingletonRenderer,
	ctx *render.RenderContext,
) (codegen.File, error) {
	body, imports, err := r.Render(ctx)
	if err != nil {
		return nil, fmt.Errorf("singleton %s: %w", r.FilePath(), err)
	}

	pkg := packageOf(r.FilePath())

	return c.assembleBytes(pkg, body, imports), nil
}

// injectAll создаёт shared Buf и ImportTracker и вливает их в каждый renderer
// из rs (уже отфильтрованный optional-интерфейс). Возвращает те же Buf/Imports
// для последующей сборки файла.
func (c *FileComposer) injectAll(
	rs []injectableRenderer, ctx *render.RenderContext,
) (*codegen.BufferWriter, *render.ImportTracker) {
	buf := codegen.NewBufferWriter()
	imports := render.NewImportTracker()

	for _, r := range rs {
		r.Init(buf, imports, ctx)
	}

	return buf, imports
}

// assemble собирает codegen.File из накопленного Buf и ImportTracker.
// Используется walker-методами (Schema/Method), где renderer'ы писали в Buf.
func (c *FileComposer) assemble(
	pkg string, buf *codegen.BufferWriter, imports *render.ImportTracker,
) codegen.File {
	return c.assembleBytes(pkg, buf.Content(), imports)
}

// assembleBytes собирает codegen.File из готовых body+imports. Используется
// singleton-методом, где renderer вернул body напрямую (без walker'а).
func (c *FileComposer) assembleBytes(
	pkg string, body []byte, imports *render.ImportTracker,
) codegen.File {
	return c.FF.Create(&gogen.File{Package: pkg, Imports: imports.Imports(), Body: body})
}

// schemaInjectable приводит slice SchemaRenderer к slice injectableRenderer,
// фильтруя те, что не реализуют optional-интерфейс. Отдельная функция нужна
// потому, что дженерики с интерфейсным ограничением T any + any().(interface)
// не позволяют сохранить статическую типизацию источника.
func schemaInjectable(rs []walk.SchemaRenderer) []injectableRenderer {
	out := make([]injectableRenderer, 0, len(rs))

	for _, r := range rs {
		if inj, ok := r.(injectableRenderer); ok {
			out = append(out, inj)
		}
	}

	return out
}

// methodInjectable — аналог schemaInjectable для MethodRenderer.
func methodInjectable(rs []walk.MethodRenderer) []injectableRenderer {
	out := make([]injectableRenderer, 0, len(rs))

	for _, r := range rs {
		if inj, ok := r.(injectableRenderer); ok {
			out = append(out, inj)
		}
	}

	return out
}

// packageOf извлекает имя пакета из пути файла: берёт подстроку до первого '/'.
// "model/utc_time.gen.go" → "model". Путь без '/' трактуется как имя пакета.
func packageOf(filePath string) string {
	pkg, _, _ := strings.Cut(filePath, "/")

	return pkg
}
