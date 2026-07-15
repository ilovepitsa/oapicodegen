Инкpackage parser_test

import (
	"testing"

	"nschugorev/oapigenerator/internal/parser"

	"github.com/stretchr/testify/assert"
)

func TestResourcesSet_LookupByFile(t *testing.T) {
	common := &parser.Project{Name: "common", ImportPrefix: "nschugorev/oapigenerator/go/common"}
	rs := &parser.ResourcesSet{
		Schemas: map[string]*parser.ResourceSchema{
			"/input/common/src/openapi/schemas/User.yaml": {
				Project:    common,
				SchemaName: "User",
				GoImport:   "nschugorev/oapigenerator/go/common",
				GoType:     "User",
			},
		},
	}

	got, ok := rs.LookupByFile("/input/common/src/openapi/schemas/User.yaml")
	assert.True(t, ok)
	assert.Equal(t, "User", got.GoType)
	assert.Equal(t, "nschugorev/oapigenerator/go/common", got.GoImport)

	_, ok = rs.LookupByFile("/nonexistent.yaml")
	assert.False(t, ok)
}

func TestResourcesSet_LookupForMode_SplitAware(t *testing.T) {
	common := &parser.Project{Name: "common", ImportPrefix: "nschugorev/oapigenerator/go/common"}
	rs := &parser.ResourcesSet{
		Schemas: map[string]*parser.ResourceSchema{
			"/input/common/src/openapi/schemas/User.yaml": {
				Project:    common,
				SchemaName: "User",
				GoImport:   "nschugorev/oapigenerator/go/common",
				GoType:     "User",
			},
		},
	}

	// Моно режим → User.
	got, ok := rs.LookupForMode("/input/common/src/openapi/schemas/User.yaml", "")
	assert.True(t, ok)
	assert.Equal(t, "User", got.GoType)

	// Request режим → UserRequest (если split включён во владельце — здесь нет, fallback на User).
	got, ok = rs.LookupForMode("/input/common/src/openapi/schemas/User.yaml", "modeRequest")
	assert.True(t, ok)
	// В этой версии без split во владельце — GoType не меняется.
	assert.Equal(t, "User", got.GoType)
}

func TestResourcesSet_LookupForMode_SplitEnabled(t *testing.T) {
	common := &parser.Project{
		Name:         "common",
		ImportPrefix: "nschugorev/oapigenerator/go/common",
		Features: parser.ProjectFeatures{
			SplitRequestResponse: parser.ProjectFeature{Value: true},
		},
	}
	const key = "/input/common/src/openapi/schemas/User.yaml"
	rs := &parser.ResourcesSet{
		Schemas: map[string]*parser.ResourceSchema{
			key: {
				Project:    common,
				SchemaName: "User",
				GoImport:   "nschugorev/oapigenerator/go/common",
				GoType:     "User",
			},
		},
	}

	// Request mode → UserRequest.
	got, ok := rs.LookupForMode(key, parser.ModeRequest)
	assert.True(t, ok)
	assert.Equal(t, "UserRequest", got.GoType)

	// Response mode → UserResponse.
	got, ok = rs.LookupForMode(key, parser.ModeResponse)
	assert.True(t, ok)
	assert.Equal(t, "UserResponse", got.GoType)

	// Mono mode → User (no suffix).
	got, ok = rs.LookupForMode(key, "")
	assert.True(t, ok)
	assert.Equal(t, "User", got.GoType)

	// Index entry must not be mutated.
	assert.Equal(t, "User", rs.Schemas[key].GoType,
		"LookupForMode must return a copy and not mutate the index entry")
}
