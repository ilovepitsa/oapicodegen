package codegen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompileCheck_NoGoModFails(t *testing.T) {
	dir := t.TempDir()

	err := CompileCheck(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "go.mod not found")
}

func TestCompileCheck_BuildErrorPropagated(t *testing.T) {
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
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module test\n\ngo 1.21\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"),
		[]byte("package test\n\nfunc main() {}\n"), 0o644))

	err := CompileCheck(dir)
	assert.NoError(t, err)
}
