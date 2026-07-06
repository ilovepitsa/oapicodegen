package fs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRealFS_NoBaseDir(t *testing.T) {
	rfs := NewRealFS()
	assert.Empty(t, rfs.baseDir)
}

func TestNewRecommendedReal_WithBaseDir(t *testing.T) {
	dir := t.TempDir()
	rfs := NewRecommendedReal(WithBaseDir(dir))
	assert.Equal(t, dir, rfs.baseDir)
}

func TestRealFS_WriteReadRoundtrip(t *testing.T) {
	rfs := NewRealFS()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "out.txt")

	require.NoError(t, rfs.WriteFile(path, []byte("hello")))
	got, err := ReadFile(rfs, path)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(got))
}

func TestRealFS_Stat(t *testing.T) {
	rfs := NewRealFS()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "f.txt")
	require.NoError(t, rfs.WriteFile(path, []byte("x")))

	info, err := rfs.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, int64(1), info.Size())
}

func TestRealFS_ReadDir(t *testing.T) {
	rfs := NewRealFS()
	tmp := t.TempDir()
	require.NoError(t, rfs.WriteFile(filepath.Join(tmp, "a.txt"), []byte("a")))
	require.NoError(t, rfs.WriteFile(filepath.Join(tmp, "b.txt"), []byte("b")))

	entries, err := rfs.ReadDir(tmp)
	require.NoError(t, err)
	assert.Len(t, entries, 2)
}

func TestRealFS_MkdirAll_Remove(t *testing.T) {
	rfs := NewRealFS()
	tmp := t.TempDir()
	nested := filepath.Join(tmp, "a", "b", "c")

	require.NoError(t, rfs.MkdirAll(nested, 0o755))
	info, err := rfs.Stat(nested)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	require.NoError(t, rfs.Remove(nested))
	_, err = rfs.Stat(nested)
	assert.True(t, os.IsNotExist(err))
}

func TestRealFS_Open_ReadsContent(t *testing.T) {
	rfs := NewRealFS()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "f.txt")
	require.NoError(t, rfs.WriteFile(path, []byte("content")))

	f, err := rfs.Open(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	buf := make([]byte, 7)
	n, err := f.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 7, n)
	assert.Equal(t, "content", string(buf))
}

func TestRealFS_BaseDir_Sandbox(t *testing.T) {
	base := t.TempDir()
	rfs := NewRecommendedReal(WithBaseDir(base))

	require.NoError(t, rfs.WriteFile("file.txt", []byte("data")))
	got, err := ReadFile(rfs, "file.txt")
	require.NoError(t, err)
	assert.Equal(t, "data", string(got))
}

func TestRealFS_BaseDir_RejectsEscape(t *testing.T) {
	base := t.TempDir()
	rfs := NewRecommendedReal(WithBaseDir(base))

	err := rfs.WriteFile("../escape.txt", []byte("x"))
	assert.Error(t, err)
	err = rfs.MkdirAll("../escape", 0o755)
	assert.Error(t, err)
}

func TestRealFS_BaseDir_RejectsEscape_AllOps(t *testing.T) {
	base := t.TempDir()
	rfs := NewRecommendedReal(WithBaseDir(base))

	_, err := rfs.Open("../escape")
	assert.Error(t, err)
	_, err = rfs.ReadDir("../escape")
	assert.Error(t, err)
	assert.Error(t, rfs.Remove("../escape"))
	_, err = rfs.Stat("../escape")
	assert.Error(t, err)
}

func TestRealFS_BaseDir_NestedDirsCreated(t *testing.T) {
	base := t.TempDir()
	rfs := NewRecommendedReal(WithBaseDir(base))

	// WriteFile не создаёт родительские каталоги
	err := rfs.WriteFile(filepath.Join("a", "b", "c.txt"), []byte("x"))
	assert.Error(t, err, "expected failure without parent dirs")
	require.True(t, isPathError(err), "expected path error for missing parent, got %v", err)

	require.NoError(t, rfs.MkdirAll(filepath.Join("a", "b"), 0o755))
	require.NoError(t, rfs.WriteFile(filepath.Join("a", "b", "c.txt"), []byte("x")))
}

func TestRealFS_Resolve_EmptyName(t *testing.T) {
	rfs := NewRealFS()
	_, err := rfs.Stat("")
	assert.Error(t, err)
}

func TestRealFS_ReadOnlyFS_InterfaceAcceptance(t *testing.T) {
	var _ FS = (*RealFS)(nil)
	var _ ReadOnlyFS = (*RealFS)(nil)
}

func isPathError(err error) bool {
	var pe *fs.PathError

	return errors.As(err, &pe)
}
