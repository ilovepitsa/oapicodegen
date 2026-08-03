package parser

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveExternalRef_LocalRef(t *testing.T) {
	got := resolveExternalRef("#/components/schemas/User", "/input/svc/openapi.yaml")
	assert.Equal(t, "", got, "local $ref must not produce ExternalRef")
}

func TestResolveExternalRef_ExternalRef(t *testing.T) {
	specPath := "/input/userBackend/src/openapi/openapi.yaml"
	ref := "../../../common/src/openapi/openapi.yaml#/components/schemas/User"

	got := resolveExternalRef(ref, specPath)

	expected := filepath.Clean("/input/common/src/openapi/openapi.yaml") + "#/components/schemas/User"
	assert.Equal(t, expected, got)
}

func TestResolveExternalRef_EmptyRef(t *testing.T) {
	got := resolveExternalRef("", "/input/svc/openapi.yaml")
	assert.Equal(t, "", got)
}

func TestMarkExternalRefs_TopLevelFields(t *testing.T) {
	project := &Project{Folder: "userBackend", ImportPrefix: "github.com/ilovepitsa/oapicodegen/go/userBackend"}
	schemas := []*Schema{
		{Name: "UserList", Type: "array"},
		{Name: "CreateUserRequest", Type: "object"},
	}
	project.Model = &Model{project: project, schemas: schemas}

	const specPath = "/input/userBackend/src/openapi/openapi.yaml"
	markExternalRefs(project, specPath)

	for _, s := range schemas {
		assert.Equal(t, specPath, s.SourceFile, "schema %q must have SourceFile", s.Name)
		assert.Equal(t, project, s.OwnerProject, "schema %q must have OwnerProject", s.Name)
	}
}

func TestMarkExternalRefs_NestedExternalRef(t *testing.T) {
	project := &Project{Folder: "userBackend", ImportPrefix: "github.com/ilovepitsa/oapicodegen/go/userBackend"}
	externalSchema := &Schema{
		Ref:  "../../../common/src/openapi/openapi.yaml#/components/schemas/User",
		Name: "User",
	}
	topSchema := &Schema{
		Name: "CreateUserRequest",
		Type: "object",
		Properties: []*Property{
			{Name: "user", Schema: externalSchema},
		},
	}
	project.Model = &Model{project: project, schemas: []*Schema{topSchema}}

	const specPath = "/input/userBackend/src/openapi/openapi.yaml"
	markExternalRefs(project, specPath)

	expected := filepath.Clean("/input/common/src/openapi/openapi.yaml") + "#/components/schemas/User"
	assert.Equal(t, expected, externalSchema.ExternalRef,
		"nested schema with external $ref must have ExternalRef set")
	assert.Equal(t, "", externalSchema.SourceFile,
		"nested schema must not have SourceFile set")
	assert.Nil(t, externalSchema.OwnerProject,
		"nested schema must not have OwnerProject set")
}

func TestMarkExternalRefs_LocalRefNoExternal(t *testing.T) {
	project := &Project{Folder: "userBackend"}
	localRef := &Schema{Ref: "#/components/schemas/UserList", Name: "UserList"}
	topSchema := &Schema{
		Name: "Wrapper",
		Properties: []*Property{
			{Name: "item", Schema: localRef},
		},
	}
	project.Model = &Model{project: project, schemas: []*Schema{topSchema}}

	markExternalRefs(project, "/input/userBackend/src/openapi/openapi.yaml")

	assert.Equal(t, "", localRef.ExternalRef,
		"local $ref must not set ExternalRef")
}

// TestMarkExternalRefs_PathOnlyCrossFileRef_NoExternal — path-only cross-file
// $ref (без #/-фрагмента, one-schema-per-file layout zvonilka) на схему,
// зарегистрированную в этом же сервисе, НЕ должен помечаться ExternalRef.
// До фикса refToSchemaName("../models/User.yaml") возвращал "User.yaml" →
// Lookup падал → ExternalRef выставлялся → qualifyExternalType давал any
// (дефект #2 из bug-report v1.3.0, блокер типизации #69).
func TestMarkExternalRefs_PathOnlyCrossFileRef_NoExternal(t *testing.T) {
	project := &Project{Folder: "svc"}
	userSchema := &Schema{
		Name: "User",
		Type: "object",
		Properties: []*Property{
			{Name: "id", Schema: &Schema{Type: "string", ReadOnly: true}},
		},
	}
	// Вложенный path-only $ref на User (zvonilka-формат, без #/).
	nestedUser := &Schema{Ref: "../models/User.yaml", Name: "User.yaml"}
	loginSchema := &Schema{
		Name: "LoginResponse",
		Type: "object",
		Properties: []*Property{
			{Name: "user", Schema: nestedUser},
		},
	}
	project.Model = &Model{project: project, schemas: []*Schema{userSchema, loginSchema}}
	project.Model.Index()

	markExternalRefs(project, "/input/svc/src/openapi/openapi.yaml")

	assert.Equal(t, "", nestedUser.ExternalRef,
		"path-only cross-file $ref to a registered intra-service schema must NOT be marked ExternalRef")
}

// TestMarkExternalRefs_PathOnlyCrossServiceRef_FragmentAppended — path-only
// cross-service $ref (на схему ДРУГОГО сервиса, без #/-фрагмента) должен
// получить #/components/schemas/<Name> фрагмент в ExternalRef, иначе
// qualifyExternalType не найдёт разделитель → поле падает в any.
// Резолвится относительно SourceFile top-level схемы (файла, где лежит ref),
// не specPath (openapi.yaml) — иначе глубина ../ уходит неверно.
func TestMarkExternalRefs_PathOnlyCrossServiceRef_FragmentAppended(t *testing.T) {
	project := &Project{Folder: "users"}
	// User loaded via $ref from schemas/models/User.yaml → markTopLevel
	// выставит SourceFile = файл схемы (User.yaml), не specPath.
	userSchema := &Schema{
		Name:       "User",
		Type:       "object",
		Ref:        "./schemas/models/User.yaml",
	}
	// Nested path-only cross-service ref (на common/UUIDv7.yaml, нет в users).
	nestedID := &Schema{Ref: "../../../../../common/src/openapi/schemas/identifiers/UUIDv7.yaml"}
	userSchema.Properties = []*Property{
		{Name: "id", Schema: nestedID},
	}
	project.Model = &Model{project: project, schemas: []*Schema{userSchema}}
	project.Model.Index()

	markExternalRefs(project, "/input/users/src/openapi/openapi.yaml")

	// ExternalRef резолвится относительно User.yaml (SourceFile), не openapi.yaml:
	// 5 ../ от schemas/models/ → /input/ → common/.../UUIDv7.yaml, + фрагмент.
	assert.Equal(t,
		"/input/common/src/openapi/schemas/identifiers/UUIDv7.yaml#/components/schemas/UUIDv7",
		nestedID.ExternalRef,
		"path-only cross-service $ref must resolve relative to top-level SourceFile and append #/components/schemas/<Name> fragment")
}

func TestMarkExternalRefs_NilProject(t *testing.T) {
	assert.NotPanics(t, func() {
		markExternalRefs(nil, "/input/svc/openapi.yaml")
	})
}

func TestBuildSchemaIndex_PopulatesFromProjectSet(t *testing.T) {
	common := &Project{Folder: "common", ImportPrefix: "github.com/ilovepitsa/oapicodegen/go/common"}
	const commonSpec = "/input/common/src/openapi/openapi.yaml"
	common.Model = &Model{project: common, schemas: []*Schema{
		{Name: "User", SourceFile: commonSpec, OwnerProject: common},
		{Name: "Profile", SourceFile: commonSpec, OwnerProject: common},
	}}

	userBackend := &Project{Folder: "userBackend", ImportPrefix: "github.com/ilovepitsa/oapicodegen/go/userBackend"}
	const userSpec = "/input/userBackend/src/openapi/openapi.yaml"
	userBackend.Model = &Model{project: userBackend, schemas: []*Schema{
		{Name: "UserList", SourceFile: userSpec, OwnerProject: userBackend},
	}}

	ps := &ProjectSet{
		Projects: []*Project{common, userBackend},
	}
	si := &SchemaIndex{}

	buildSchemaIndex(si, ps)

	require.Len(t, si.Schemas, 3)

	entry, ok := si.Lookup(commonSpec, "User")
	require.True(t, ok)
	assert.Equal(t, "User", entry.GoType)
	assert.Equal(t, "github.com/ilovepitsa/oapicodegen/go/common", entry.GoImport)
	assert.Equal(t, common, entry.Project)

	entry, ok = si.Lookup(userSpec, "UserList")
	require.True(t, ok)
	assert.Equal(t, "UserList", entry.GoType)
	assert.Equal(t, "github.com/ilovepitsa/oapicodegen/go/userBackend", entry.GoImport)

	_, ok = si.Lookup(commonSpec, "Profile")
	assert.True(t, ok)
}
