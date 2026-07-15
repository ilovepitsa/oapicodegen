package parser

import (
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceDescriptor_Fields(t *testing.T) {
	d := serviceDescriptor{
		Folder:    "userBackend",
		SpecPath:  "/input/userBackend/src/openapi/openapi.yaml",
		FlagsPath: "/input/userBackend/generation_flags.yaml",
	}
	assert.Equal(t, "userBackend", d.Folder)
	assert.Equal(t, "/input/userBackend/src/openapi/openapi.yaml", d.SpecPath)
	assert.Equal(t, "/input/userBackend/generation_flags.yaml", d.FlagsPath)
}

func TestWalkServices_DiscoversAllServices(t *testing.T) {
	descs, err := walkServices("testdata/multiservice")
	require.NoError(t, err)
	assert.Len(t, descs, 3)

	byFolder := map[string]serviceDescriptor{}
	for _, d := range descs {
		byFolder[d.Folder] = d
	}

	assert.Contains(t, byFolder, "common")
	assert.Contains(t, byFolder, "userBackend")
	assert.Contains(t, byFolder, "authBackend")

	// SpecPath — путь к openapi.yaml внутри сервиса
	assert.True(t, strings.HasSuffix(byFolder["userBackend"].SpecPath, "userBackend/src/openapi/openapi.yaml"))
}

func TestWalkServices_NoServicesDir(t *testing.T) {
	_, err := walkServices("testdata/nonexistent")
	require.Error(t, err)
}

func TestNewProjectLoader(t *testing.T) {
	pl := NewProjectLoader()
	assert.NotNil(t, pl)
}

func TestProjectLoader_Load_SingleService(t *testing.T) {
	pl := NewProjectLoader()
	ps, si, err := pl.Load(
		"testdata/multiservice",
		nil,
		"nschugorev/oapigenerator/go",
		"/output",
	)
	require.NoError(t, err)
	assert.NotNil(t, ps)
	assert.NotNil(t, si)
	// Projects включает все сервисы, в том числе common (см. ProjectSet.Projects).
	assert.Len(t, ps.Projects, 3) // common + userBackend + authBackend
	assert.NotNil(t, ps.Common, "common project must be detected and separated")
	assert.Contains(t, ps.ByName, "common")
	assert.Contains(t, ps.ByName, "userBackend")
	assert.Contains(t, ps.ByName, "authBackend")
}

func TestProjectLoader_Load_ModelSchemasTransferred(t *testing.T) {
	pl := NewProjectLoader()
	ps, _, err := pl.Load("testdata/multiservice", nil,
		"nschugorev/oapigenerator/go", "/output")
	require.NoError(t, err)

	common := ps.Common
	assert.NotNil(t, common.Model)
	assert.Len(t, common.Model.Schemas(), 2, "common must have User + Profile")

	// Index построен — Lookup работает
	user, ok := common.Model.Lookup("User")
	assert.True(t, ok)
	assert.Equal(t, "User", user.Name)

	profile, ok := common.Model.Lookup("Profile")
	assert.True(t, ok)
	assert.Equal(t, "Profile", profile.Name)

	_, ok = common.Model.Lookup("Nonexistent")
	assert.False(t, ok)
}

func TestProjectLoader_Load_PathsServicesGroupedByTag(t *testing.T) {
	pl := NewProjectLoader()
	ps, _, err := pl.Load("testdata/multiservice", nil,
		"nschugorev/oapigenerator/go", "/output")
	require.NoError(t, err)

	userBackend := ps.ByName["userBackend"]
	assert.NotNil(t, userBackend.Paths)
	assert.Len(t, userBackend.Paths.Services, 1, "userBackend has one tag → one service")
	assert.Equal(t, "UserBackend", userBackend.Paths.Services[0].Name)
	assert.Len(t, userBackend.Paths.Services[0].Methods, 2, "ListUsers + CreateUser")

	authBackend := ps.ByName["authBackend"]
	assert.Len(t, authBackend.Paths.Services, 1)
	assert.Equal(t, "AuthBackend", authBackend.Paths.Services[0].Name)
	assert.Len(t, authBackend.Paths.Services[0].Methods, 1, "Login")
}

func TestProjectLoader_Load_CommonHasNoServices(t *testing.T) {
	pl := NewProjectLoader()
	ps, _, err := pl.Load("testdata/multiservice", nil,
		"nschugorev/oapigenerator/go", "/output")
	require.NoError(t, err)

	// common/spec имеет paths: {} — нет операций → нет сервисов
	assert.Empty(t, ps.Common.Paths.Services)
}

func TestProjectLoader_Load_PathImportsPopulated(t *testing.T) {
	pl := NewProjectLoader()
	ps, _, err := pl.Load("testdata/multiservice", nil,
		"nschugorev/oapigenerator/go", "/output")
	require.NoError(t, err)

	userBackend := ps.ByName["userBackend"]
	pi := userBackend.Paths.Imports
	assert.Equal(t, "nschugorev/oapigenerator/go/userBackend/interfaces/client", pi.ClientInterfaces.Path)
	assert.Equal(t, "nschugorev/oapigenerator/go/userBackend/impl/httpclient", pi.ClientHTTP.Path)
	assert.Equal(t, "nschugorev/oapigenerator/go/userBackend/model", pi.Model.Path)
	assert.Equal(t, "nschugorev/oapigenerator/go/userBackend/sdk", pi.SDK.Path)

	// Model.Import
	assert.Equal(t, "nschugorev/oapigenerator/go/userBackend/model", userBackend.Model.Import.Path)
	assert.Equal(t, gogen.LocalImport, userBackend.Model.Import.Type)
}

func TestProjectLoader_Load_CommonPrefixSet(t *testing.T) {
	pl := NewProjectLoader()
	ps, _, err := pl.Load("testdata/multiservice", nil,
		"nschugorev/oapigenerator/go", "/output")
	require.NoError(t, err)

	assert.Equal(t, "common", ps.Common.Model.Prefix,
		"common project must have Model.Prefix set for cross-service aliasing")
}

func TestProjectLoader_Load_InputNotFound(t *testing.T) {
	pl := NewProjectLoader()
	_, _, err := pl.Load("testdata/does-not-exist", nil,
		"nschugorev/oapigenerator/go", "/output")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "walk services")
}

func TestProjectLoader_Load_MalformedSpec(t *testing.T) {
	tmp := t.TempDir()
	svcDir := filepath.Join(tmp, "broken", "src", "openapi")
	_ = os.MkdirAll(svcDir, 0o755)
	_ = os.WriteFile(filepath.Join(svcDir, "openapi.yaml"), []byte("not: valid: openapi"), 0o644)

	pl := NewProjectLoader()
	_, _, err := pl.Load(tmp, nil,
		"nschugorev/oapigenerator/go", "/output")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load project")
}

func TestProjectLoader_Load_MultipleTagsError(t *testing.T) {
	tmp := t.TempDir()
	svcDir := filepath.Join(tmp, "multi", "src", "openapi")
	_ = os.MkdirAll(svcDir, 0o755)
	spec := []byte(`openapi: 3.0.3
info:
  title: Multi
  version: 1.0.0
paths:
  /x:
    get:
      operationId: X
      tags: [One, Two]
      responses:
        '200':
          description: ok
`)
	_ = os.WriteFile(filepath.Join(svcDir, "openapi.yaml"), spec, 0o644)

	pl := NewProjectLoader()
	_, _, err := pl.Load(tmp, nil,
		"nschugorev/oapigenerator/go", "/output")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be exactly one tag")
}

func TestProjectLoader_Load_WithFlagsLoader(t *testing.T) {
	flagsLoader := NewGenerationFlagsLoader(fs.NewRealFS())
	err := flagsLoader.Load("testdata/multiservice/generation_flags.yaml")
	require.NoError(t, err)

	pl := NewProjectLoader()
	ps, _, err := pl.Load("testdata/multiservice", flagsLoader,
		"nschugorev/oapigenerator/go", "/output")
	require.NoError(t, err)

	for _, p := range ps.Projects {
		assert.False(t, p.Features.SplitRequestResponse.Value,
			"project %q must have default features", p.Folder)
	}
}

func TestProjectLoader_Load_SchemaIndexPopulated(t *testing.T) {
	pl := NewProjectLoader()
	_, si, err := pl.Load("testdata/multiservice", nil,
		"nschugorev/oapigenerator/go", "/output")
	require.NoError(t, err)
	require.NotNil(t, si)
	require.NotEmpty(t, si.Schemas, "SchemaIndex must be populated after Load")

	const commonSpec = "testdata/multiservice/common/src/openapi/openapi.yaml"

	entry, ok := si.Lookup(commonSpec, "User")
	require.True(t, ok, "common.User must be in SchemaIndex")
	assert.Equal(t, "User", entry.GoType)
	assert.Equal(t, "nschugorev/oapigenerator/go/common", entry.GoImport)
	assert.NotNil(t, entry.Project)

	_, ok = si.Lookup(commonSpec, "Profile")
	require.True(t, ok, "common.Profile must be in SchemaIndex")

	const userSpec = "testdata/multiservice/userBackend/src/openapi/openapi.yaml"
	entry, ok = si.Lookup(userSpec, "UserList")
	require.True(t, ok, "userBackend.UserList must be in SchemaIndex")
	assert.Equal(t, "nschugorev/oapigenerator/go/userBackend", entry.GoImport)
}

func TestProjectLoader_Load_SourceMarkingFields(t *testing.T) {
	pl := NewProjectLoader()
	ps, _, err := pl.Load("testdata/multiservice", nil,
		"nschugorev/oapigenerator/go", "/output")
	require.NoError(t, err)

	const commonSpec = "testdata/multiservice/common/src/openapi/openapi.yaml"
	common := ps.ByName["common"]
	for _, s := range common.Model.Schemas() {
		assert.Equal(t, commonSpec, s.SourceFile,
			"schema %q must have SourceFile", s.Name)
		assert.Equal(t, common, s.OwnerProject,
			"schema %q must have OwnerProject", s.Name)
	}

	const userSpec = "testdata/multiservice/userBackend/src/openapi/openapi.yaml"
	userBackend := ps.ByName["userBackend"]

	var createReq *Schema
	for _, s := range userBackend.Model.Schemas() {
		assert.Equal(t, userSpec, s.SourceFile)
		assert.Equal(t, userBackend, s.OwnerProject)

		if s.Name == "CreateUserRequest" {
			createReq = s
		}
	}

	require.NotNil(t, createReq, "CreateUserRequest schema must exist")
	require.Len(t, createReq.Properties, 1)

	userProp := createReq.Properties[0]
	require.NotNil(t, userProp.Schema)
	assert.NotEmpty(t, userProp.Schema.ExternalRef,
		"nested $ref to common.User must have ExternalRef set")
	expectedExtRef := commonSpec + "#/components/schemas/User"
	assert.Equal(t, expectedExtRef, userProp.Schema.ExternalRef)
}
