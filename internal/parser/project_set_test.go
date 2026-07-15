package parser_test

import (
	"testing"

	"nschugorev/oapigenerator/internal/parser"

	"github.com/stretchr/testify/assert"
)

func TestProjectSet_ByNameLookup(t *testing.T) {
	common := &parser.Project{Name: "common"}
	userBackend := &parser.Project{Name: "userBackend"}

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

func TestProject_Fields(t *testing.T) {
	p := &parser.Project{
		Name:         "userBackend",
		SpecPath:     "/input/userBackend/src/openapi/openapi.yaml",
		FlagsPath:    "/input/userBackend/generation_flags.yaml",
		OutputDir:    "/go/userBackend",
		ImportPrefix: "nschugorev/oapigenerator/go/userBackend",
	}
	assert.Equal(t, "userBackend", p.Name)
	assert.Equal(t, "/go/userBackend", p.OutputDir)
}
