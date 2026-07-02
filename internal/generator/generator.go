// Package generator генерирует Go-код из parser.Document. Первая итерация:
// только стандартный OpenAPI 3.x (без x-* расширений, audit-data, split
// Request/Response, update-схем, URL-form-encoding — всё это в бэклоге).
//
// Для каждой схемы из components.schemas генерируется <name>.gen.go с
// определением Go-типа (struct / interface / type alias). Для oneOf/anyOf
// дополнительно генерируется <name>_json.gen.go с UnmarshalJSON.
// Для операций из paths генерируется client.gen.go (интерфейс Client +
// request/response-структуры) и client_sugar.gen.go (ClientSugared обёртка).
package generator

import (
	"fmt"

	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/parser"
)

// Generator конфигурируется через Option-ы и хранит общее состояние генерации.
type Generator struct {
	doc         *parser.Document
	packageName string
	factory     *gogen.FileFactory
}

// Option настраивает Generator.
type Option func(*Generator)

// WithPackage задаёт имя Go-пакета для генерируемых файлов (по умолчанию "model").
func WithPackage(pkg string) Option {
	return func(g *Generator) { g.packageName = pkg }
}

// Generate обходит все схемы и операции, пишет Go-файлы через fw.
func Generate(fw codegen.FileWriter, doc *parser.Document, opts ...Option) error {
	g := &Generator{
		doc:         doc,
		packageName: "model",
		factory:     gogen.NewFileFactory("oapigen"),
	}
	for _, opt := range opts {
		opt(g)
	}

	for _, sh := range doc.Schemas {
		if sh.Name == "" {
			continue
		}
		sf := g.schemaFile(sh)
		fname := fileName(sh.Name) + ".gen.go"
		if err := fw.WriteFile(fname, sf); err != nil {
			return fmt.Errorf("write %s: %w", fname, err)
		}

		if needsJSONMethods(sh) {
			jf := g.jsonMethodsFile(sh)
			jname := fileName(sh.Name) + "_json.gen.go"
			if err := fw.WriteFile(jname, jf); err != nil {
				return fmt.Errorf("write %s: %w", jname, err)
			}
		}
	}

	if len(doc.Operations) > 0 {
		cf := g.clientFile()
		if err := fw.WriteFile("client.gen.go", cf); err != nil {
			return fmt.Errorf("write client.gen.go: %w", err)
		}

		sf := g.clientSugarFile()
		if err := fw.WriteFile("client_sugar.gen.go", sf); err != nil {
			return fmt.Errorf("write client_sugar.gen.go: %w", err)
		}

		srvf := g.serverFile()
		if err := fw.WriteFile("server.gen.go", srvf); err != nil {
			return fmt.Errorf("write server.gen.go: %w", err)
		}
	}

	return nil
}
