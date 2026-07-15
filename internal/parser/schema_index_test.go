package parser_test

import (
	"testing"

	"nschugorev/oapigenerator/internal/parser"

	"github.com/stretchr/testify/assert"
)

func TestSchemaIndex_LookupByFile(t *testing.T) {
	common := &parser.Project{Folder: "common", ImportPrefix: "nschugorev/oapigenerator/go/common"}
	si := &parser.SchemaIndex{
		Schemas: map[string]*parser.SchemaEntry{
			"/input/common/src/openapi/schemas/User.yaml": {
				Project:    common,
				SchemaName: "User",
				GoImport:   "nschugorev/oapigenerator/go/common",
				GoType:     "User",
			},
		},
	}

	got, ok := si.LookupByFile("/input/common/src/openapi/schemas/User.yaml")
	assert.True(t, ok)
	assert.Equal(t, "User", got.GoType)
	assert.Equal(t, "nschugorev/oapigenerator/go/common", got.GoImport)

	_, ok = si.LookupByFile("/nonexistent.yaml")
	assert.False(t, ok)
}

func TestSchemaIndex_LookupForMode_NoSplit(t *testing.T) {
	common := &parser.Project{Folder: "common", ImportPrefix: "nschugorev/oapigenerator/go/common"}
	si := &parser.SchemaIndex{
		Schemas: map[string]*parser.SchemaEntry{
			"/input/common/src/openapi/schemas/User.yaml": {
				Project:    common,
				SchemaName: "User",
				GoImport:   "nschugorev/oapigenerator/go/common",
				GoType:     "User",
			},
		},
	}

	got, ok := si.LookupForMode("/input/common/src/openapi/schemas/User.yaml", "")
	assert.True(t, ok)
	assert.Equal(t, "User", got.GoType)

	got, ok = si.LookupForMode("/input/common/src/openapi/schemas/User.yaml", parser.ModeRequest)
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
	const key = "/input/common/src/openapi/schemas/User.yaml"
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

	got, ok := si.LookupForMode(key, parser.ModeRequest)
	assert.True(t, ok)
	assert.Equal(t, "UserRequest", got.GoType)

	got, ok = si.LookupForMode(key, parser.ModeResponse)
	assert.True(t, ok)
	assert.Equal(t, "UserResponse", got.GoType)

	got, ok = si.LookupForMode(key, "")
	assert.True(t, ok)
	assert.Equal(t, "User", got.GoType)

	assert.Equal(t, "User", si.Schemas[key].GoType,
		"LookupForMode must return a copy and not mutate the index entry")
}
