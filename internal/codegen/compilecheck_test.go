package codegen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompileCheck_NoGoModFails(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	err := CompileCheck(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "go.mod not found")
	assert.Contains(t, err.Error(), "parent")
}

func TestCompileCheck_BuildErrorPropagated(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module test\n\ngo 1.21\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "broken.go"),
		[]byte("package test\n\nfunc main() {\n\tx :=  // syntax error\n}\n"), 0o644))

	err := CompileCheck(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "go build")
}

func TestCompileCheck_SuccessOnValidModule(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module test\n\ngo 1.21\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"),
		[]byte("package test\n\nfunc main() {}\n"), 0o644))

	err := CompileCheck(dir)
	assert.NoError(t, err)
}

func TestCompileCheck_FindsGoModInParent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"),
		[]byte("module test\n\ngo 1.21\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"),
		[]byte("package test\n\nfunc main() {}\n"), 0o644))

	output := filepath.Join(root, "gen")
	require.NoError(t, os.Mkdir(output, 0o755))

	err := CompileCheck(output)
	assert.NoError(t, err, "should find go.mod in parent and build the module")
}

func TestCompileCheck_WalksPastIntermediateDirs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"),
		[]byte("module test\n\ngo 1.21\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"),
		[]byte("package test\n\nfunc main() {}\n"), 0o644))

	output := filepath.Join(root, "a", "b")
	require.NoError(t, os.MkdirAll(output, 0o755))

	err := CompileCheck(output)
	assert.NoError(t, err, "should walk two levels up to find go.mod")
}
