package parser

import (
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModel_Schemas_Empty(t *testing.T) {
	m := &Model{}
	assert.Nil(t, m.Schemas())
}

func TestModel_Schemas_ReturnsAssigned(t *testing.T) {
	schemas := []*Schema{{Name: "User"}}
	m := &Model{schemas: schemas}
	got := m.Schemas()
	assert.Len(t, got, 1)
	assert.Equal(t, "User", got[0].Name)
}

func TestModel_Import(t *testing.T) {
	m := &Model{Import: gogen.Import{Path: "nschugorev/oapigenerator/go/svc/model", Package: "model"}}
	assert.Equal(t, "nschugorev/oapigenerator/go/svc/model", m.Import.Path)
}

func TestModel_Lookup_BeforeIndex(t *testing.T) {
	m := &Model{schemas: []*Schema{{Name: "User"}}}
	_, ok := m.Lookup("User")
	assert.False(t, ok, "Lookup must return false before Index() is called")
}

func TestModel_Index_AndLookup(t *testing.T) {
	m := &Model{schemas: []*Schema{{Name: "User"}, {Name: "Profile"}}}
	m.Index()

	got, ok := m.Lookup("User")
	assert.True(t, ok)
	assert.Equal(t, "User", got.Name)

	got, ok = m.Lookup("Profile")
	assert.True(t, ok)
	assert.Equal(t, "Profile", got.Name)

	_, ok = m.Lookup("Nonexistent")
	assert.False(t, ok)
}

func TestModel_Prefix(t *testing.T) {
	m := &Model{Prefix: "common"}
	assert.Equal(t, "common", m.Prefix)
}
