package operations

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/generator/render"
)

// SDKRenderer рендерит sdk/sdk.gen.go: consumer-facing facade over apiclient.Client.
// Заменяет Generator.sdkFile (internal/generator/sdk.go).
type SDKRenderer struct{}

func NewSDKRenderer() *SDKRenderer { return &SDKRenderer{} }

func (SDKRenderer) FilePath() string { return "sdk/sdk.gen.go" }

func (r *SDKRenderer) Render(ctx *render.RenderContext) ([]byte, *render.ImportTracker, error) {
	ops := allOperations(ctx.Project)
	if len(ops) == 0 {
		return nil, nil, nil
	}

	imps := render.NewImportTracker()
	ctx.Imports = imps

	imps.Add(gogen.Import{Path: "fmt"})

	const httpclientPkg = "nschugorev/oapigenerator/pkg/httpclient"
	imps.Add(gogen.Import{Path: httpclientPkg, Alias: "httpclient"})

	if ctx.Project != nil && ctx.Project.Paths != nil {
		imps.Add(gogen.Import{Path: ctx.Project.Paths.Imports.ClientInterfaces.Path, Alias: "apiclient"})
		imps.Add(gogen.Import{Path: ctx.Project.Paths.Imports.ClientHTTP.Path, Alias: "implclient"})
	}

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

	return w.Content(), imps, nil
}

var _ render.SingletonRenderer = (*SDKRenderer)(nil)
