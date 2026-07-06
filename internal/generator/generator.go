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
}

// Option настраивает Generator.
type Option func(*Generator)

// WithModulePath задаёт Go import-path корня генерируемого кода
// (например "github.com/foo/bar/gen/petstore"). От него строятся пути
// к пакетам model/, interfaces/client/, interfaces/server/.
func WithModulePath(p string) Option {
	return func(g *Generator) { g.modulePath = p }
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
	}

	if len(doc.Operations) > 0 {
		cf := g.clientFile()
		if err := fw.WriteFile("interfaces/client/client.gen.go", cf); err != nil {
			return fmt.Errorf("write interfaces/client/client.gen.go: %w", err)
		}

		sf := g.clientSugarFile()
		if err := fw.WriteFile("interfaces/client/client_sugar.gen.go", sf); err != nil {
			return fmt.Errorf("write interfaces/client/client_sugar.gen.go: %w", err)
		}

		srvf := g.serverFile()
		if err := fw.WriteFile("interfaces/server/server.gen.go", srvf); err != nil {
			return fmt.Errorf("write interfaces/server/server.gen.go: %w", err)
		}

		implf := g.implClientFile()
		if err := fw.WriteFile("impl/httpclient/client.gen.go", implf); err != nil {
			return fmt.Errorf("write impl/httpclient/client.gen.go: %w", err)
		}

		srvImplf := g.implServerFile()
		if err := fw.WriteFile("impl/echoserver/server.gen.go", srvImplf); err != nil {
			return fmt.Errorf("write impl/echoserver/server.gen.go: %w", err)
		}

		mockClientF := g.mockClientFile()
		if err := fw.WriteFile("impl/mocks/client/mocks.gen.go", mockClientF); err != nil {
			return fmt.Errorf("write impl/mocks/client/mocks.gen.go: %w", err)
		}

		mockServerF := g.mockServerFile()
		if err := fw.WriteFile("impl/mocks/server/mocks.gen.go", mockServerF); err != nil {
			return fmt.Errorf("write impl/mocks/server/mocks.gen.go: %w", err)
		}

		sdkF := g.sdkFile()
		if err := fw.WriteFile("sdk/sdk.gen.go", sdkF); err != nil {
			return fmt.Errorf("write sdk/sdk.gen.go: %w", err)
		}
	}

	return nil
}
