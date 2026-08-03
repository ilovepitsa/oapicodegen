package parser_test

import (
	"github.com/ilovepitsa/oapicodegen/internal/parser"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSchemaIndex_Lookup(t *testing.T) {
	common := &parser.Project{Folder: "common", ImportPrefix: "github.com/ilovepitsa/oapicodegen/go/common"}
	const absPath = "/input/common/src/openapi/openapi.yaml"
	si := &parser.SchemaIndex{
		Schemas: map[string]*parser.SchemaEntry{
			absPath + "#/components/schemas/User": {
				Project:    common,
				SchemaName: "User",
				GoImport:   "github.com/ilovepitsa/oapicodegen/go/common",
				GoType:     "User",
			},
		},
	}

	got, ok := si.Lookup(absPath, "User")
	assert.True(t, ok)
	assert.Equal(t, "User", got.GoType)
	assert.Equal(t, "github.com/ilovepitsa/oapicodegen/go/common", got.GoImport)

	_, ok = si.Lookup("/nonexistent.yaml", "User")
	assert.False(t, ok)

	_, ok = si.Lookup(absPath, "Nonexistent")
	assert.False(t, ok)
}

func TestSchemaIndex_LookupForMode_NoSplit(t *testing.T) {
	common := &parser.Project{Folder: "common", ImportPrefix: "github.com/ilovepitsa/oapicodegen/go/common"}
	const absPath = "/input/common/src/openapi/openapi.yaml"
	si := &parser.SchemaIndex{
		Schemas: map[string]*parser.SchemaEntry{
			absPath + "#/components/schemas/User": {
				Project:    common,
				SchemaName: "User",
				GoImport:   "github.com/ilovepitsa/oapicodegen/go/common",
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
		ImportPrefix: "github.com/ilovepitsa/oapicodegen/go/common",
		Features: parser.ProjectFeatures{
			SplitRequestResponse: parser.ProjectFeature{Value: true},
		},
	}

	// User — splittable (IsSplit=true): cross-service ref получает суффикс.
	userSchema := &parser.Schema{Name: "User", IsSplit: true}
	common.Model = &parser.Model{}
	common.Model.SetSchemas([]*parser.Schema{userSchema})

	const absPath = "/input/common/src/openapi/openapi.yaml"
	key := absPath + "#/components/schemas/User"
	si := &parser.SchemaIndex{
		Schemas: map[string]*parser.SchemaEntry{
			key: {
				Project:    common,
				SchemaName: "User",
				GoImport:   "github.com/ilovepitsa/oapicodegen/go/common",
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

// TestSchemaIndex_LookupForMode_NonSplittableTarget_NoSuffix — cross-service
// ref на non-splittable схему (primitive alias UUIDv7, enum) НЕ получает
// Request/Response-суффикс, даже если проект-владелец split-enabled: у таких
// схем нет Request/Response-вариантов → суффикс дал бы undefined-тип.
func TestSchemaIndex_LookupForMode_NonSplittableTarget_NoSuffix(t *testing.T) {
	common := &parser.Project{
		Folder:       "common",
		ImportPrefix: "github.com/ilovepitsa/oapicodegen/go/common",
		Features: parser.ProjectFeatures{
			SplitRequestResponse: parser.ProjectFeature{Value: true},
		},
	}

	// UUIDv7 — primitive alias, IsSplit=false.
	uuidSchema := &parser.Schema{Name: "UUIDv7", IsSplit: false}
	common.Model = &parser.Model{}
	common.Model.SetSchemas([]*parser.Schema{uuidSchema})

	const absPath = "/input/common/src/openapi/openapi.yaml"
	si := &parser.SchemaIndex{
		Schemas: map[string]*parser.SchemaEntry{
			absPath + "#/components/schemas/UUIDv7": {
				Project:    common,
				SchemaName: "UUIDv7",
				GoImport:   "github.com/ilovepitsa/oapicodegen/go/common",
				GoType:     "UUIDv7",
			},
		},
	}

	got, ok := si.LookupForMode(absPath, "UUIDv7", parser.ModeRequest)
	assert.True(t, ok)
	assert.Equal(t, "UUIDv7", got.GoType, "non-splittable target must not get Request suffix")
}
