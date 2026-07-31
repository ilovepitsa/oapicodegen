package main

import (
	"github.com/ilovepitsa/oapicodegen/internal/codegen"
	"github.com/ilovepitsa/oapicodegen/internal/golden"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	e2eInputDir   = "../../testdata/project"
	e2eGoldenPath = "../../testdata/project/golden"
	e2eImportPfx  = "github.com/ilovepitsa/oapicodegen/testdata/project/golden"
)

// onefile — multi-file spec с subpackage-дроблением (schemas/users → model/users,
// schemas/common → model/common) и cross-subpackage $ref. Глобальный
// generation_flags.yaml лежит рядом с каталогом сервиса; per-project override —
// в testdata/onefile/onefile/generation_flags.yaml.
const (
	onefileInputDir    = "../../testdata/onefile"
	onefileGoldenPath  = "../../testdata/onefile/golden"
	onefileImportPfx   = "github.com/ilovepitsa/oapicodegen/testdata/onefile/golden"
	onefileFlagsConfig = "../../testdata/onefile/generation_flags.yaml"
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

// TestE2E_GoldenCompiles проверяет, что зафиксированные golden-файлы
// компилируются `go build ./...` в testdata/project/golden/ (отдельный module
// с go.mod и replace на основной модуль). Это единственный end-to-end
// тест compile-check: генератор → файлы → go build.
func TestE2E_GoldenCompiles(t *testing.T) {
	err := codegen.CompileCheck(e2eGoldenPath)
	require.NoError(t, err, "golden directory must compile with go build ./...")
}

// TestE2E_Onefile проверяет полный пайплайн на multi-file spec с subpackage-
// дроблением: схемы из schemas/users и schemas/common раскладываются в
// model/users и model/common, cross-subpackage $ref квалифицируются через
// алиас-импорт, split-конвертеры и url_form рендерятся на Request-вариант.
// Отличается от TestE2E_Minimal передачей -generation-flags-config-path
// (глобальный generation_flags.yaml нужен, чтобы per-project override из
// testdata/onefile/onefile/generation_flags.yaml резолвился во флаги).
func TestE2E_Onefile(t *testing.T) {
	output := t.TempDir()
	stderr := nullFile(t)

	err := run([]string{
		"-input", onefileInputDir,
		"-output", output,
		"-import-prefix", onefileImportPfx,
		"-generation-flags-config-path", onefileFlagsConfig,
		"-skip-compile-check",
		"-log-level", "error",
	}, stderr)
	require.NoError(t, err)

	gotFiles := walkFiles(t, output)
	require.NotEmpty(t, gotFiles, "no files generated")

	dir := golden.NewDir(t, golden.WithPath(onefileGoldenPath), golden.WithRecreateOnUpdate())

	for rel, content := range gotFiles {
		dir.Equals(rel, content)
	}

	if golden.Update() {
		return
	}

	wantFiles := walkFiles(t, onefileGoldenPath)
	for rel := range wantFiles {
		// skip non-generated files (go.mod, go.sum, etc.)
		if filepath.Ext(rel) != ".go" {
			continue
		}
		_, ok := gotFiles[rel]
		assert.True(t, ok, "golden file %q has no corresponding generated file", rel)
	}
}

// TestE2E_OnefileGoldenCompiles проверяет, что зафиксированные golden-файлы
// onefile (cross-subpackage код) компилируются go build ./... — главный
// приёмочный тест T28 subpackage-splitting: сгенерированные cross-package
// refs/imports должны быть синтаксически и типно корректны.
func TestE2E_OnefileGoldenCompiles(t *testing.T) {
	err := codegen.CompileCheck(onefileGoldenPath)
	require.NoError(t, err, "onefile golden directory must compile with go build ./...")
}
