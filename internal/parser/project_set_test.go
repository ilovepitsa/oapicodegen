package parser_test

import (
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/parser"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProjectSet_ByNameLookup(t *testing.T) {
	common := &parser.Project{Folder: "common"}
	userBackend := &parser.Project{Folder: "userBackend"}

	ps := &parser.ProjectSet{
		Common:   common,
		Projects: []*parser.Project{common, userBackend},
		ByName:   map[string]*parser.Project{"common": common, "userBackend": userBackend},
	}

	got, ok := ps.ByNameLookup("userBackend")
	assert.True(t, ok)
	assert.Same(t, userBackend, got)

	_, ok = ps.ByNameLookup("nonexistent")
	assert.False(t, ok)
}

func TestProject_Folder(t *testing.T) {
	p := &parser.Project{
		Folder:       "userBackend",
		SpecPath:     "/input/userBackend/src/openapi/openapi.yaml",
		FlagsPath:    "/input/userBackend/generation_flags.yaml",
		OutputDir:    "/go/userBackend",
		ImportPrefix: "nschugorev/oapigenerator/go/userBackend",
	}
	assert.Equal(t, "userBackend", p.Folder)
	assert.Equal(t, "/go/userBackend", p.OutputDir)
}

func TestProject_CreateModel(t *testing.T) {
	p := &parser.Project{Folder: "svc"}
	imp := gogen.Import{Path: "nschugorev/oapigenerator/go/svc/model", Package: "model"}
	m := p.CreateModel(imp)

	assert.NotNil(t, p.Model)
	assert.Same(t, m, p.Model)
	assert.Equal(t, "nschugorev/oapigenerator/go/svc/model", p.Model.Import.Path)
	assert.Equal(t, gogen.LocalImport, p.Model.Import.Type, "CreateModel must set Type to LocalImport")
}

func TestProject_CreatePaths(t *testing.T) {
	p := &parser.Project{Folder: "svc"}
	pi := p.CreatePaths("nschugorev/oapigenerator/go/svc")

	assert.NotNil(t, p.Paths)
	assert.Same(t, pi, p.Paths)

	assert.Equal(t, "nschugorev/oapigenerator/go/svc/interfaces/client", pi.Imports.ClientInterfaces.Path)
	assert.Equal(t, "nschugorev/oapigenerator/go/svc/interfaces/server", pi.Imports.ServerInterfaces.Path)
	assert.Equal(t, "nschugorev/oapigenerator/go/svc/impl/httpclient", pi.Imports.ClientHTTP.Path)
	assert.Equal(t, "nschugorev/oapigenerator/go/svc/impl/echoserver", pi.Imports.ServerHTTP.Path)
	assert.Equal(t, "nschugorev/oapigenerator/go/svc/impl/mocks/client", pi.Imports.ClientMocks.Path)
	assert.Equal(t, "nschugorev/oapigenerator/go/svc/impl/mocks/server", pi.Imports.ServerMocks.Path)
	assert.Equal(t, "nschugorev/oapigenerator/go/svc/model", pi.Imports.Model.Path)
	assert.Equal(t, "nschugorev/oapigenerator/go/svc/sdk", pi.Imports.SDK.Path)

	assert.Equal(t, "http", pi.Imports.ClientHTTP.Alias)
	assert.Equal(t, "http", pi.Imports.ServerHTTP.Alias)
	assert.Equal(t, "mock", pi.Imports.ClientMocks.Alias)
	assert.Equal(t, "mock", pi.Imports.ServerMocks.Alias)
	assert.Equal(t, "model", pi.Imports.Model.Alias)

	assert.Equal(t, "client", pi.Imports.ClientInterfaces.Package)
	assert.Equal(t, "client", pi.Imports.ClientHTTP.Package)

	assert.Equal(t, gogen.LocalImport, pi.Imports.SDK.Type)
}
