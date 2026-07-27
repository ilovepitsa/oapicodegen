package parser

import (
	"github.com/ilovepitsa/oapicodegen/internal/codegen/gogen"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPathImports_Fields(t *testing.T) {
	pi := PathImports{
		ClientInterfaces: gogen.Import{Path: "github.com/ilovepitsa/oapicodegen/go/svc/interfaces/client", Alias: "", Package: "client"},
		ServerInterfaces: gogen.Import{Path: "github.com/ilovepitsa/oapicodegen/go/svc/interfaces/server", Alias: "", Package: "server"},
		ClientHTTP:       gogen.Import{Path: "github.com/ilovepitsa/oapicodegen/go/svc/impl/httpclient", Alias: "http", Package: "client"},
		ServerHTTP:       gogen.Import{Path: "github.com/ilovepitsa/oapicodegen/go/svc/impl/echoserver", Alias: "http", Package: "server"},
		ClientMocks:      gogen.Import{Path: "github.com/ilovepitsa/oapicodegen/go/svc/impl/mocks/client", Alias: "mock", Package: "client"},
		ServerMocks:      gogen.Import{Path: "github.com/ilovepitsa/oapicodegen/go/svc/impl/mocks/server", Alias: "mock", Package: "server"},
		Model:            gogen.Import{Path: "github.com/ilovepitsa/oapicodegen/go/svc/model", Alias: "model", Package: ""},
		SDK:              gogen.Import{Path: "github.com/ilovepitsa/oapicodegen/go/svc/sdk", Alias: "", Package: ""},
	}
	assert.Equal(t, "github.com/ilovepitsa/oapicodegen/go/svc/interfaces/client", pi.ClientInterfaces.Path)
	assert.Equal(t, "http", pi.ClientHTTP.Alias)
	assert.Equal(t, "client", pi.ClientInterfaces.Package)
	assert.Equal(t, "github.com/ilovepitsa/oapicodegen/go/svc/model", pi.Model.Path)
}
