package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nschugorev/oapigenerator/internal/golden"
)

const (
	e2eSpecPath   = "../../testdata/minimal/spec.yaml"
	e2eGoldenPath = "../../testdata/minimal/golden"
	e2eImportPfx  = "nschugorev/oapigenerator/testdata/minimal/golden"
)

// TestE2E_Minimal проверяет полный пайплайн cmd/oapigen на минимальной спеке
// со стандартными конструкциями OpenAPI 3.x: object/array/oneOf/$ref/enum/
// nullable/additionalProperties + path/query/body параметры и CRUD-операции.
func TestE2E_Minimal(t *testing.T) {
	output := t.TempDir()
	stderr := os.NewFile(0, os.DevNull)
	t.Cleanup(func() { _ = stderr.Close() })

	err := run([]string{
		"-input", e2eSpecPath,
		"-output", output,
		"-import-prefix", e2eImportPfx,
		"-log-level", "error",
	}, stderr)
	require.NoError(t, err)

	gotFiles := walkFiles(t, output)
	require.NotEmpty(t, gotFiles, "no files generated")

	dir := golden.NewDir(t, golden.WithPath(e2eGoldenPath), golden.WithRecreateOnUpdate())

	for rel, content := range gotFiles {
		dir.Equals(rel, content)
	}

	if golden.Update() {
		return
	}

	wantFiles := walkFiles(t, e2eGoldenPath)
	for rel := range wantFiles {
		_, ok := gotFiles[rel]
		assert.True(t, ok, "golden file %q has no corresponding generated file", rel)
	}
}

func walkFiles(t *testing.T, root string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = data

		return nil
	})
	require.NoError(t, err)

	return files
}
