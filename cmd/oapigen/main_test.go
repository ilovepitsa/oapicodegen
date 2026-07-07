package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_GeneratesFiles(t *testing.T) {
	tmp := t.TempDir()
	output := filepath.Join(tmp, "gen")
	spec := filepath.Join(tmp, "spec.yaml")
	require.NoError(t, os.WriteFile(spec, []byte(minimalSpec), 0o644))

	stderr := os.NewFile(0, "/dev/null")
	err := run([]string{
		"-input", spec,
		"-output", output,
		"-import-prefix", "github.com/foo/bar/gen",
	}, stderr)
	require.NoError(t, err)

	files, err := filepath.Glob(filepath.Join(output, "*", "*.gen.go"))
	require.NoError(t, err)
	assert.NotEmpty(t, files, "expected generated files")
}

func TestRun_DryRunNoFiles(t *testing.T) {
	tmp := t.TempDir()
	spec := filepath.Join(tmp, "spec.yaml")
	require.NoError(t, os.WriteFile(spec, []byte(minimalSpec), 0o644))

	output := filepath.Join(tmp, "gen")
	stderr := os.NewFile(0, "/dev/null")
	err := run([]string{
		"-input", spec,
		"-output", output,
		"-import-prefix", "github.com/foo/bar/gen",
		"-dry-run",
	}, stderr)
	require.NoError(t, err)

	_, err = os.Stat(output)
	assert.True(t, os.IsNotExist(err), "output dir should not exist in dry-run")
}

func TestRun_MissingInput(t *testing.T) {
	stderr := os.NewFile(0, "/dev/null")
	err := run([]string{}, stderr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "-input is required")
}

func TestRun_MissingOutputWithoutDryRun(t *testing.T) {
	tmp := t.TempDir()
	spec := filepath.Join(tmp, "spec.yaml")
	require.NoError(t, os.WriteFile(spec, []byte(minimalSpec), 0o644))

	stderr := os.NewFile(0, "/dev/null")
	err := run([]string{
		"-input", spec,
		"-import-prefix", "github.com/foo/bar/gen",
	}, stderr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "-output is required")
}

func TestRun_MissingImportPrefix(t *testing.T) {
	tmp := t.TempDir()
	spec := filepath.Join(tmp, "spec.yaml")
	require.NoError(t, os.WriteFile(spec, []byte(minimalSpec), 0o644))

	stderr := os.NewFile(0, "/dev/null")
	err := run([]string{
		"-input", spec,
		"-output", filepath.Join(tmp, "gen"),
	}, stderr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "-import-prefix is required")
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
`

func TestRun_GenerationFlagsConfig_LoadsDefaults(t *testing.T) {
	tmp := t.TempDir()
	spec := filepath.Join(tmp, "spec.yaml")
	require.NoError(t, os.WriteFile(spec, []byte(minimalSpec), 0o644))

	cfgPath := filepath.Join(tmp, "generation_flags.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(validGlobalFlagsConfig), 0o644))

	output := filepath.Join(tmp, "gen")
	stderr := os.NewFile(0, "/dev/null")
	err := run([]string{
		"-input", spec,
		"-output", output,
		"-import-prefix", "github.com/foo/bar/gen",
		"-generation-flags-config-path", cfgPath,
	}, stderr)
	require.NoError(t, err)

	files, err := filepath.Glob(filepath.Join(output, "*", "*.gen.go"))
	require.NoError(t, err)
	assert.NotEmpty(t, files)
}

func TestRun_GenerationFlagsConfig_WithProjectOverride(t *testing.T) {
	tmp := t.TempDir()
	spec := filepath.Join(tmp, "spec.yaml")
	require.NoError(t, os.WriteFile(spec, []byte(minimalSpec), 0o644))

	cfgPath := filepath.Join(tmp, "generation_flags.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(validGlobalFlagsConfig), 0o644))

	projectFlags := "GOLANG_SERVER_BODY_REQUEST_NO_AUTO_DEFAULTS: true\nGOLANG_SPLIT_REQUEST_RESPONSE: true\nUSE_REQUIRED_V2: true\n"
	pfPath := filepath.Join(tmp, "project_flags.yaml")
	require.NoError(t, os.WriteFile(pfPath, []byte(projectFlags), 0o644))

	output := filepath.Join(tmp, "gen")
	stderr := os.NewFile(0, "/dev/null")
	err := run([]string{
		"-input", spec,
		"-output", output,
		"-import-prefix", "github.com/foo/bar/gen",
		"-generation-flags-config-path", cfgPath,
		"-project-flags-path", pfPath,
	}, stderr)
	require.NoError(t, err)
}

func TestRun_ProjectFlagsPathRequiresGlobalConfig(t *testing.T) {
	tmp := t.TempDir()
	spec := filepath.Join(tmp, "spec.yaml")
	require.NoError(t, os.WriteFile(spec, []byte(minimalSpec), 0o644))

	stderr := os.NewFile(0, "/dev/null")
	err := run([]string{
		"-input", spec,
		"-output", filepath.Join(tmp, "gen"),
		"-import-prefix", "github.com/foo/bar/gen",
		"-project-flags-path", filepath.Join(tmp, "project_flags.yaml"),
	}, stderr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "-project-flags-path requires -generation-flags-config-path")
}

func TestRun_GenerationFlagsConfig_BadConfig(t *testing.T) {
	tmp := t.TempDir()
	spec := filepath.Join(tmp, "spec.yaml")
	require.NoError(t, os.WriteFile(spec, []byte(minimalSpec), 0o644))

	cfgPath := filepath.Join(tmp, "generation_flags.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("not valid yaml {{{"), 0o644))

	stderr := os.NewFile(0, "/dev/null")
	err := run([]string{
		"-input", spec,
		"-output", filepath.Join(tmp, "gen"),
		"-import-prefix", "github.com/foo/bar/gen",
		"-generation-flags-config-path", cfgPath,
	}, stderr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load generation flags")
}
