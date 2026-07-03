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
