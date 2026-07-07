package configurator

import (
	"flag"
	"nschugorev/oapigenerator/internal/codegen"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFileWriterConfiguratorFromFlags_ReturnsNonNil(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	c := NewFileWriterConfiguratorFromFlags(fs)
	require.NotNil(t, c)
	assert.Equal(t, os.FileMode(0o755), c.dirPerm)
}

func TestCreate_EmptyOutput_ReturnsNoop(t *testing.T) {
	c := NewFileWriterConfiguratorFromFlags(flag.NewFlagSet("test", flag.ContinueOnError))
	fw, err := c.Create("")
	require.NoError(t, err)
	assert.IsType(t, codegen.NoopFileWriter{}, fw)
}

func TestCreate_ValidOutput_CreatesDirAndWriter(t *testing.T) {
	tmp := t.TempDir()
	output := filepath.Join(tmp, "out")

	c := NewFileWriterConfiguratorFromFlags(flag.NewFlagSet("test", flag.ContinueOnError))
	fw, err := c.Create(output)
	require.NoError(t, err)
	assert.IsType(t, &fileWriter{}, fw)

	info, err := os.Stat(output)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestFileWriter_WriteFile_TopLevel(t *testing.T) {
	tmp := t.TempDir()
	output := filepath.Join(tmp, "out")

	c := NewFileWriterConfiguratorFromFlags(flag.NewFlagSet("test", flag.ContinueOnError))
	fw, err := c.Create(output)
	require.NoError(t, err)

	require.NoError(t, fw.WriteFile("main.go", codegen.NewFile([]byte("package main\n"))))
	got, err := os.ReadFile(filepath.Join(output, "main.go"))
	require.NoError(t, err)
	assert.Equal(t, "package main\n", string(got))
}

func TestFileWriter_WriteFile_CreatesParentDirs(t *testing.T) {
	tmp := t.TempDir()
	output := filepath.Join(tmp, "out")

	c := NewFileWriterConfiguratorFromFlags(flag.NewFlagSet("test", flag.ContinueOnError))
	fw, err := c.Create(output)
	require.NoError(t, err)

	require.NoError(t, fw.WriteFile("model/user.gen.go", codegen.NewFile([]byte("package model\n"))))

	got, err := os.ReadFile(filepath.Join(output, "model", "user.gen.go"))
	require.NoError(t, err)
	assert.Equal(t, "package model\n", string(got))

	info, err := os.Stat(filepath.Join(output, "model"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestFileWriter_WriteFile_DeeplyNested(t *testing.T) {
	tmp := t.TempDir()
	output := filepath.Join(tmp, "out")

	c := NewFileWriterConfiguratorFromFlags(flag.NewFlagSet("test", flag.ContinueOnError))
	fw, err := c.Create(output)
	require.NoError(t, err)

	require.NoError(t, fw.WriteFile("a/b/c/d.go", codegen.NewFile([]byte("package d\n"))))
	got, err := os.ReadFile(filepath.Join(output, "a", "b", "c", "d.go"))
	require.NoError(t, err)
	assert.Equal(t, "package d\n", string(got))
}

func TestFileWriter_WriteFile_OverwritesExisting(t *testing.T) {
	tmp := t.TempDir()
	output := filepath.Join(tmp, "out")

	c := NewFileWriterConfiguratorFromFlags(flag.NewFlagSet("test", flag.ContinueOnError))
	fw, err := c.Create(output)
	require.NoError(t, err)

	require.NoError(t, fw.WriteFile("f.go", codegen.NewFile([]byte("old"))))
	require.NoError(t, fw.WriteFile("f.go", codegen.NewFile([]byte("new content"))))

	got, err := os.ReadFile(filepath.Join(output, "f.go"))
	require.NoError(t, err)
	assert.Equal(t, "new content", string(got))
}

func TestFileWriter_Close_NoError(t *testing.T) {
	tmp := t.TempDir()
	output := filepath.Join(tmp, "out")

	c := NewFileWriterConfiguratorFromFlags(flag.NewFlagSet("test", flag.ContinueOnError))
	fw, err := c.Create(output)
	require.NoError(t, err)

	assert.NoError(t, fw.Close())
}

func TestFileWriter_ImplementsCodegenFileWriter(t *testing.T) {
	var _ codegen.FileWriter = (*fileWriter)(nil)
}

func TestCreate_RelativeOutputPath(t *testing.T) {
	// Относительный путь должен работать (через WithBaseDir + MkdirAll).
	// Используем t.TempDir как cwd, чтобы не засорять репозиторий.
	tmp := t.TempDir()
	t.Chdir(tmp)

	c := NewFileWriterConfiguratorFromFlags(flag.NewFlagSet("test", flag.ContinueOnError))
	fw, err := c.Create("out")
	require.NoError(t, err)
	require.NotNil(t, fw)

	require.NoError(t, fw.WriteFile("f.go", codegen.NewFile([]byte("package main\n"))))
	got, err := os.ReadFile(filepath.Join(tmp, "out", "f.go"))
	require.NoError(t, err)
	assert.Equal(t, "package main\n", string(got))
}

func TestCreate_MkdirAllFails_ReturnsError(t *testing.T) {
	// output под файлом — MkdirAll(".") не сможет создать каталог.
	tmp := t.TempDir()
	blocker := filepath.Join(tmp, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))

	c := NewFileWriterConfiguratorFromFlags(flag.NewFlagSet("test", flag.ContinueOnError))
	_, err := c.Create(filepath.Join(blocker, "out"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "configurator: create output dir")
}

func TestFileWriter_WriteFile_MkdirAllFails(t *testing.T) {
	tmp := t.TempDir()
	output := filepath.Join(tmp, "out")

	c := NewFileWriterConfiguratorFromFlags(flag.NewFlagSet("test", flag.ContinueOnError))
	fw, err := c.Create(output)
	require.NoError(t, err)

	// Создаём файл-блокер в output, чтобы MkdirAll для подкаталога с тем же именем упал.
	require.NoError(t, os.WriteFile(filepath.Join(output, "blocker"), []byte("x"), 0o644))

	err = fw.WriteFile("blocker/sub.go", codegen.NewFile([]byte("package main\n")))
	require.Error(t, err)
}
