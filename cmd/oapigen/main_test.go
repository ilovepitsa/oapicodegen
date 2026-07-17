package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nullFile открывает /dev/null как *os.File для использования в качестве
// stderr в тестах. В отличие от os.NewFile(0, ...) не занимает fd 0 (stdin),
// что избегает гонок с GC-финализатором, закрывающим fd 0 посреди теста.
func nullFile(t *testing.T) *os.File {
	t.Helper()
	f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })

	return f
}

// writeService creates a single-service project layout at dir/demo/src/openapi/openapi.yaml
// with the minimalSpec and returns the project root (dir) to pass as -input.
func writeService(t *testing.T, dir string) string {
	t.Helper()
	const service = "demo"
	require.NoError(t, os.MkdirAll(filepath.Join(dir, service, "src", "openapi"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, service, "src", "openapi", "openapi.yaml"), []byte(minimalSpec), 0o644,
	))

	return dir
}

func TestRun_GeneratesFiles(t *testing.T) {
	tmp := t.TempDir()
	input := writeService(t, tmp)
	output := filepath.Join(tmp, "gen")

	err := run([]string{
		"-input", input,
		"-output", output,
		"-import-prefix", "github.com/foo/bar/gen",
		"-skip-compile-check",
	}, nullFile(t))
	require.NoError(t, err)

	files, err := filepath.Glob(filepath.Join(output, "demo", "*", "*.gen.go"))
	require.NoError(t, err)
	assert.NotEmpty(t, files, "expected generated files")
}

// TestRun_CrossFileRef проверяет, что CLI резолвит cross-file $ref через
// каталог спеки: openapi.yaml ссылается на pet.yaml#/components/schemas/Pet.
func TestRun_CrossFileRef(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "demo", "src", "openapi")
	require.NoError(t, os.MkdirAll(specDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(specDir, "openapi.yaml"), []byte(crossFileSpec), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(specDir, "pet.yaml"), []byte(petSpec), 0o644))

	output := filepath.Join(tmp, "gen")
	stderr := nullFile(t)
	err := run([]string{
		"-input", tmp,
		"-output", output,
		"-import-prefix", "github.com/foo/bar/gen",
		"-skip-compile-check",
	}, stderr)
	require.NoError(t, err)

	files, err := filepath.Glob(filepath.Join(output, "demo", "*", "*.gen.go"))
	require.NoError(t, err)
	assert.NotEmpty(t, files, "expected generated files despite cross-file $ref")
}

func TestRun_DryRunNoFiles(t *testing.T) {
	tmp := t.TempDir()
	input := writeService(t, tmp)
	output := filepath.Join(tmp, "gen")
	stderr := nullFile(t)
	err := run([]string{
		"-input", input,
		"-output", output,
		"-import-prefix", "github.com/foo/bar/gen",
		"-dry-run",
	}, stderr)
	require.NoError(t, err)

	_, err = os.Stat(output)
	assert.True(t, os.IsNotExist(err), "output dir should not exist in dry-run")
}

func TestRun_MissingInput(t *testing.T) {
	stderr := nullFile(t)
	err := run([]string{}, stderr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "-input is required")
}

func TestRun_MissingOutputWithoutDryRun(t *testing.T) {
	tmp := t.TempDir()
	input := writeService(t, tmp)

	stderr := nullFile(t)
	err := run([]string{
		"-input", input,
		"-import-prefix", "github.com/foo/bar/gen",
	}, stderr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "-output is required")
}

func TestRun_MissingImportPrefix(t *testing.T) {
	tmp := t.TempDir()
	input := writeService(t, tmp)

	stderr := nullFile(t)
	err := run([]string{
		"-input", input,
		"-output", filepath.Join(tmp, "gen"),
	}, stderr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "-import-prefix is required")
}

func TestRun_InputMustBeDirectory(t *testing.T) {
	tmp := t.TempDir()
	spec := filepath.Join(tmp, "spec.yaml")
	require.NoError(t, os.WriteFile(spec, []byte(minimalSpec), 0o644))

	stderr := nullFile(t)
	err := run([]string{
		"-input", spec,
		"-output", filepath.Join(tmp, "gen"),
		"-import-prefix", "github.com/foo/bar/gen",
	}, stderr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "-input must be a directory")
}

const minimalSpec = `openapi: 3.0.3
info:
  title: t
  version: '1'
paths:
  /pets:
    get:
      operationId: listPets
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema: {$ref: '#/components/schemas/Pets'}
components:
  schemas:
    Pet: {type: object, properties: {name: {type: string}}}
    Pets: {type: array, items: {$ref: '#/components/schemas/Pet'}}
`

const crossFileSpec = `openapi: 3.0.3
info: {title: t, version: '1'}
paths: {}
components:
  schemas:
    PetList:
      type: object
      properties:
        items:
          type: array
          items: {$ref: 'pet.yaml#/components/schemas/Pet'}
`

const petSpec = `openapi: 3.0.3
info: {title: pets, version: '1'}
paths: {}
components:
  schemas:
    Pet:
      type: object
      required: [id]
      properties:
        id: {type: integer, format: int64}
        name: {type: string}
`

const validGlobalFlagsConfig = `- name: GOLANG_SERVER_BODY_REQUEST_NO_AUTO_DEFAULTS
  enabled: true
  defaultValue: false
  targetValue: true
  affects: [golang]

- name: GOLANG_SPLIT_REQUEST_RESPONSE
  enabled: true
  defaultValue: false
  targetValue: true
  affects: [golang]

- name: USE_REQUIRED_V2
  enabled: true
  defaultValue: false
  targetValue: true
  dependsOn:
    GOLANG_SPLIT_REQUEST_RESPONSE: true
  affects: [golang]

- name: USE_UTC_FOR_DATE_TIME
  enabled: false
  defaultValue: false
  targetValue: false
  affects: [golang]

- name: GOLANG_USE_OPTIONAL
  enabled: true
  defaultValue: false
  targetValue: true
  affects: [golang]
`

func TestRun_GenerationFlagsConfig_LoadsDefaults(t *testing.T) {
	tmp := t.TempDir()
	input := writeService(t, tmp)

	cfgPath := filepath.Join(tmp, "generation_flags.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(validGlobalFlagsConfig), 0o644))

	output := filepath.Join(tmp, "gen")
	stderr := nullFile(t)
	err := run([]string{
		"-input", input,
		"-output", output,
		"-import-prefix", "github.com/foo/bar/gen",
		"-generation-flags-config-path", cfgPath,
		"-skip-compile-check",
	}, stderr)
	require.NoError(t, err)

	files, err := filepath.Glob(filepath.Join(output, "demo", "*", "*.gen.go"))
	require.NoError(t, err)
	assert.NotEmpty(t, files)
}

// TestRun_PerServiceFlagsOverride проверяет, что generation_flags.yaml
// размещённый рядом с сервисом (в <service>/generation_flags.yaml),
// применяется как per-service override. Это замена старому -project-flags-path.
func TestRun_PerServiceFlagsOverride(t *testing.T) {
	tmp := t.TempDir()
	input := writeService(t, tmp)

	cfgPath := filepath.Join(tmp, "generation_flags.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(validGlobalFlagsConfig), 0o644))

	projectFlags := "GOLANG_SERVER_BODY_REQUEST_NO_AUTO_DEFAULTS: true\nGOLANG_SPLIT_REQUEST_RESPONSE: true\nUSE_REQUIRED_V2: true\n"
	require.NoError(t, os.WriteFile(
		filepath.Join(tmp, "demo", "generation_flags.yaml"), []byte(projectFlags), 0o644,
	))

	output := filepath.Join(tmp, "gen")
	stderr := nullFile(t)
	err := run([]string{
		"-input", input,
		"-output", output,
		"-import-prefix", "github.com/foo/bar/gen",
		"-generation-flags-config-path", cfgPath,
		"-skip-compile-check",
	}, stderr)
	require.NoError(t, err)
}

func TestRun_GenerationFlagsConfig_BadConfig(t *testing.T) {
	tmp := t.TempDir()
	input := writeService(t, tmp)

	cfgPath := filepath.Join(tmp, "generation_flags.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("not valid yaml {{{"), 0o644))

	stderr := nullFile(t)
	err := run([]string{
		"-input", input,
		"-output", filepath.Join(tmp, "gen"),
		"-import-prefix", "github.com/foo/bar/gen",
		"-generation-flags-config-path", cfgPath,
	}, stderr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load generation flags")
}
