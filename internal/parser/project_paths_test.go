package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"nschugorev/oapigenerator/internal/codegen/gogen"
)

func TestPathImports_Fields(t *testing.T) {
	pi := PathImports{
		ClientInterfaces: gogen.Import{Path: "nschugorev/oapigenerator/go/svc/interfaces/client", Alias: "", Package: "client"},
		ServerInterfaces: gogen.Import{Path: "nschugorev/oapigenerator/go/svc/interfaces/server", Alias: "", Package: "server"},
		ClientHTTP:       gogen.Import{Path: "nschugorev/oapigenerator/go/svc/impl/httpclient", Alias: "http", Package: "client"},
		ServerHTTP:       gogen.Import{Path: "nschugorev/oapigenerator/go/svc/impl/echoserver", Alias: "http", Package: "server"},
		ClientMocks:      gogen.Import{Path: "nschugorev/oapigenerator/go/svc/impl/mocks/client", Alias: "mock", Package: "client"},
		ServerMocks:      gogen.Import{Path: "nschugorev/oapigenerator/go/svc/impl/mocks/server", Alias: "mock", Package: "server"},
		Model:            gogen.Import{Path: "nschugorev/oapigenerator/go/svc/model", Alias: "model", Package: ""},
		SDK:              gogen.Import{Path: "nschugorev/oapigenerator/go/svc/sdk", Alias: "", Package: ""},
	}
	assert.Equal(t, "nschugorev/oapigenerator/go/svc/interfaces/client", pi.ClientInterfaces.Path)
	assert.Equal(t, "http", pi.ClientHTTP.Alias)
	assert.Equal(t, "client", pi.ClientInterfaces.Package)
	assert.Equal(t, "nschugorev/oapigenerator/go/svc/model", pi.Model.Path)
}
