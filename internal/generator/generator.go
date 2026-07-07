// Package generator генерирует Go-код из parser.Document. Первая итерация:
// только стандартный OpenAPI 3.x (без x-* расширений, audit-data, split
// Request/Response, update-схем, URL-form-encoding — всё это в бэклоге).
//
// Layout (multi-package, как в mwsapi):
//
//	<modulePath>/model/              — schemas + JSON-методы
//	<modulePath>/interfaces/client/  — Client interface + Request/Response + sugar
//	<modulePath>/interfaces/server/  — Server interface (переиспользует Request/Response из client)
//
// Для каждой схемы из components.schemas генерируется <name>.gen.go с
// определением Go-типа (struct / alias). Для oneOf/anyOf дополнительно
// генерируется <name>_json.gen.go с MarshalJSON/UnmarshalJSON.
package generator

import (
	"fmt"
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/parser"
)

// Generator конфигурируется через Option-ы и хранит общее состояние генерации.
type Generator struct {
	doc        *parser.Document
	modulePath string
	factory    *gogen.FileFactory
	features   parser.ProjectFeatures
}

// Option настраивает Generator.
type Option func(*Generator)

// WithModulePath задаёт Go import-path корня генерируемого кода
// (например "github.com/foo/bar/gen/petstore"). От него строятся пути
// к пакетам model/, interfaces/client/, interfaces/server/.
func WithModulePath(p string) Option {
	return func(g *Generator) { g.modulePath = p }
}

// WithProjectFeatures прокидывает резолвнутые generation flags в Generator.
// Без вызова option все флаги остаются false (zero value ProjectFeatures).
func WithProjectFeatures(pf parser.ProjectFeatures) Option {
	return func(g *Generator) { g.features = pf }
}

// Generate обходит все схемы и операции, пишет Go-файлы через fw.
func Generate(fw codegen.FileWriter, doc *parser.Document, opts ...Option) error {
	g := &Generator{
		doc:     doc,
		factory: gogen.NewFileFactory("oapigen"),
	}
	for _, opt := range opts {
		opt(g)
	}

	for _, sh := range doc.Schemas {
		if sh.Name == "" {
			continue
		}

		if err := g.writeSchemaFiles(fw, sh); err != nil {
			return err
		}
	}

	if err := g.writeUTCTimeFile(fw); err != nil {
		return err
	}

	if len(doc.Operations) > 0 {
		if err := g.writeOperationFiles(fw); err != nil {
			return err
		}
	}

	return nil
}

func (g *Generator) writeSchemaFiles(fw codegen.FileWriter, sh *parser.Schema) error {
	sf := g.schemaFile(sh)
	fname := "model/" + fileName(sh.Name) + ".gen.go"

	if err := fw.WriteFile(fname, sf); err != nil {
		return fmt.Errorf("write %s: %w", fname, err)
	}

	if needsJSONMethods(sh) {
		jf := g.jsonMethodsFile(sh)
		jname := "model/" + fileName(sh.Name) + "_json.gen.go"

		if err := fw.WriteFile(jname, jf); err != nil {
			return fmt.Errorf("write %s: %w", jname, err)
		}
	}

	return nil
}

// writeUTCTimeFile пишет model/utc_time.gen.go, если включён флаг
// USE_UTC_FOR_DATE_TIME. Вызывается один раз за генерацию.
func (g *Generator) writeUTCTimeFile(fw codegen.FileWriter) error {
	if !g.features.UseUTCForDateTime.Value {
		return nil
	}

	const fname = "model/utc_time.gen.go"

	if err := fw.WriteFile(fname, g.utcTimeFile()); err != nil {
		return fmt.Errorf("write %s: %w", fname, err)
	}

	return nil
}

func (g *Generator) writeOperationFiles(fw codegen.FileWriter) error {
	files := []struct {
		path string
		gen  func() codegen.File
	}{
		{"interfaces/client/client.gen.go", g.clientFile},
		{"interfaces/client/client_sugar.gen.go", g.clientSugarFile},
		{"interfaces/server/server.gen.go", g.serverFile},
		{"impl/httpclient/client.gen.go", g.implClientFile},
		{"impl/echoserver/server.gen.go", g.implServerFile},
		{"impl/mocks/client/mocks.gen.go", g.mockClientFile},
		{"impl/mocks/server/mocks.gen.go", g.mockServerFile},
		{"sdk/sdk.gen.go", g.sdkFile},
	}

	for _, f := range files {
		if err := fw.WriteFile(f.path, f.gen()); err != nil {
			return fmt.Errorf("write %s: %w", f.path, err)
		}
	}

	return nil
}
