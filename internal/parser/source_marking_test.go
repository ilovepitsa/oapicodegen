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
	project := &Project{Folder: "userBackend", ImportPrefix: "nschugorev/oapigenerator/go/userBackend"}
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
	project := &Project{Folder: "userBackend", ImportPrefix: "nschugorev/oapigenerator/go/userBackend"}
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

func TestMarkExternalRefs_NilProject(t *testing.T) {
	assert.NotPanics(t, func() {
		markExternalRefs(nil, "/input/svc/openapi.yaml")
	})
}

func TestBuildSchemaIndex_PopulatesFromProjectSet(t *testing.T) {
	common := &Project{Folder: "common", ImportPrefix: "nschugorev/oapigenerator/go/common"}
	const commonSpec = "/input/common/src/openapi/openapi.yaml"
	common.Model = &Model{project: common, schemas: []*Schema{
		{Name: "User", SourceFile: commonSpec, OwnerProject: common},
		{Name: "Profile", SourceFile: commonSpec, OwnerProject: common},
	}}

	userBackend := &Project{Folder: "userBackend", ImportPrefix: "nschugorev/oapigenerator/go/userBackend"}
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
	assert.Equal(t, "nschugorev/oapigenerator/go/common", entry.GoImport)
	assert.Equal(t, common, entry.Project)

	entry, ok = si.Lookup(userSpec, "UserList")
	require.True(t, ok)
	assert.Equal(t, "UserList", entry.GoType)
	assert.Equal(t, "nschugorev/oapigenerator/go/userBackend", entry.GoImport)

	_, ok = si.Lookup(commonSpec, "Profile")
	assert.True(t, ok)
}
