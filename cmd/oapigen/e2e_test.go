package main

import (
	"nschugorev/oapigenerator/internal/golden"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	e2eInputDir   = "../../testdata/project"
	e2eGoldenPath = "../../testdata/project/golden"
	e2eImportPfx  = "nschugorev/oapigenerator/testdata/project/golden"
)

// TestE2E_Minimal проверяет полный пайплайн cmd/oapigen на проекте из одного
// сервиса testdata/project/minimal. CLI обходит каталог, находит сервис,
// генерирует пакеты в <output>/minimal/... и сравнивает с golden.
//
// Compile-check пропущен (-skip-compile-check): в tmp-каталоге нет go.mod,
// настройка отдельного модуля для каждого запуска выходит за рамки e2e.
func TestE2E_Minimal(t *testing.T) {
	output := t.TempDir()
	stderr := nullFile(t)

	err := run([]string{
		"-input", e2eInputDir,
		"-output", output,
		"-import-prefix", e2eImportPfx,
		"-skip-compile-check",
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
		// skip non-generated files (go.mod, etc.)
		if filepath.Ext(rel) != ".go" {
			continue
		}
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
