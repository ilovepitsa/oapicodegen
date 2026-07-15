package parser_test

import (
	"nschugorev/oapigenerator/internal/parser"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSchemaIndex_Lookup(t *testing.T) {
	common := &parser.Project{Folder: "common", ImportPrefix: "nschugorev/oapigenerator/go/common"}
	const absPath = "/input/common/src/openapi/openapi.yaml"
	si := &parser.SchemaIndex{
		Schemas: map[string]*parser.SchemaEntry{
			absPath + "#/components/schemas/User": {
				Project:    common,
				SchemaName: "User",
				GoImport:   "nschugorev/oapigenerator/go/common",
				GoType:     "User",
			},
		},
	}

	got, ok := si.Lookup(absPath, "User")
	assert.True(t, ok)
	assert.Equal(t, "User", got.GoType)
	assert.Equal(t, "nschugorev/oapigenerator/go/common", got.GoImport)

	_, ok = si.Lookup("/nonexistent.yaml", "User")
	assert.False(t, ok)

	_, ok = si.Lookup(absPath, "Nonexistent")
	assert.False(t, ok)
}

func TestSchemaIndex_LookupForMode_NoSplit(t *testing.T) {
	common := &parser.Project{Folder: "common", ImportPrefix: "nschugorev/oapigenerator/go/common"}
	const absPath = "/input/common/src/openapi/openapi.yaml"
	si := &parser.SchemaIndex{
		Schemas: map[string]*parser.SchemaEntry{
			absPath + "#/components/schemas/User": {
				Project:    common,
				SchemaName: "User",
				GoImport:   "nschugorev/oapigenerator/go/common",
				GoType:     "User",
			},
		},
	}

	got, ok := si.LookupForMode(absPath, "User", "")
	assert.True(t, ok)
	assert.Equal(t, "User", got.GoType)

	got, ok = si.LookupForMode(absPath, "User", parser.ModeRequest)
	assert.True(t, ok)
	assert.Equal(t, "User", got.GoType)
}

func TestSchemaIndex_LookupForMode_SplitEnabled(t *testing.T) {
	common := &parser.Project{
		Folder:       "common",
		ImportPrefix: "nschugorev/oapigenerator/go/common",
		Features: parser.ProjectFeatures{
			SplitRequestResponse: parser.ProjectFeature{Value: true},
		},
	}

	const absPath = "/input/common/src/openapi/openapi.yaml"
	key := absPath + "#/components/schemas/User"
	si := &parser.SchemaIndex{
		Schemas: map[string]*parser.SchemaEntry{
			key: {
				Project:    common,
				SchemaName: "User",
				GoImport:   "nschugorev/oapigenerator/go/common",
				GoType:     "User",
			},
		},
	}

	got, ok := si.LookupForMode(absPath, "User", parser.ModeRequest)
	assert.True(t, ok)
	assert.Equal(t, "UserRequest", got.GoType)

	got, ok = si.LookupForMode(absPath, "User", parser.ModeResponse)
	assert.True(t, ok)
	assert.Equal(t, "UserResponse", got.GoType)

	got, ok = si.LookupForMode(absPath, "User", "")
	assert.True(t, ok)
	assert.Equal(t, "User", got.GoType)

	assert.Equal(t, "User", si.Schemas[key].GoType,
		"LookupForMode must return a copy and not mutate the index entry")
}
