package generator

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
)

// sdkFile генерирует sdk/sdk.gen.go: consumer-facing facade over apiclient.Client.
// SDK embeds apiclient.Client, поэтому все операции доступны напрямую.
// NewSDK строит impl-клиент из baseURL + opts; NewSDKFromClient принимает
// готовую реализацию (например, mock) для тестов.
func (g *Generator) sdkFile() codegen.File {
	m := g.newTypeMapper("sdk")
	m.addImport("fmt", "")

	const httpclientPkg = "nschugorev/oapigenerator/pkg/httpclient"

	m.addImport(httpclientPkg, "httpclient")

	if g.project != nil {
		m.addImport(g.project.Paths.Imports.ClientInterfaces.Path, "apiclient")
		m.addImport(g.project.Paths.Imports.ClientHTTP.Path, "implclient")
	}

	body := g.renderSDK()

	return g.factory.Create(&gogen.File{
		Package: "sdk",
		Imports: m.imports,
		Body:    body,
	})
}

func (g *Generator) renderSDK() []byte {
	w := codegen.NewBufferWriter()

	w.Print("type SDK struct {\n")
	w.Print("\tapiclient.Client\n")
	w.Print("}\n\n")

	w.Print("func NewSDK(baseURL string, opts ...httpclient.Option) (*SDK, error) {\n")
	w.Print("\tc, err := implclient.NewClient(baseURL, opts...)\n")
	w.Print("\tif err != nil {\n")
	w.WriteString("\t\treturn nil, fmt.Errorf(\"init sdk client: %w\", err)\n")
	w.Print("\t}\n")
	w.Print("\treturn &SDK{Client: c}, nil\n")
	w.Print("}\n\n")

	w.Print("func NewSDKFromClient(c apiclient.Client) *SDK {\n")
	w.Print("\treturn &SDK{Client: c}\n")
	w.Print("}\n")

	return w.Content()
}
