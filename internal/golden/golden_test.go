package golden

import (
	"flag"
	"fmt"
	"nschugorev/oapigenerator/internal/codegen"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeT — собирает вызовы Fatal/Fatalf без реального падения теста.
// Fatal имитирует поведение testing.T.Fatal: останавливает выполнение
// через panic, который перехватывается runCatchingFatal.
type fakeT struct {
	fatalCalls []string
}

type fakeTFatal struct{}

func (f *fakeT) Fatal(args ...any) {
	f.fatalCalls = append(f.fatalCalls, fmt.Sprint(args...))
	panic(fakeTFatal{})
}

func (f *fakeT) Fatalf(format string, args ...any) {
	f.fatalCalls = append(f.fatalCalls, fmt.Sprintf(format, args...))
	panic(fakeTFatal{})
}

// runCatchingFatal выполняет fn, перехватывая panic от fakeT.Fatal.
func runCatchingFatal(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(fakeTFatal); !ok {
				panic(r)
			}
		}
	}()
	fn()
}

// setUpdate устанавливает флаг -update на время теста и сбрасывает в конце.
func setUpdate(t *testing.T, on bool) {
	t.Helper()
	orig := *updateFlag
	*updateFlag = on
	t.Cleanup(func() { *updateFlag = orig })
}

func TestUpdate_FlagDefault(t *testing.T) {
	// Сбрасываем на случай, если другие тесты установили.
	*updateFlag = false
	assert.False(t, Update())
}

func TestUpdate_FlagOn(t *testing.T) {
	setUpdate(t, true)
	assert.True(t, Update())
}

func TestNewDir_DefaultPath(t *testing.T) {
	d := NewDir(t)
	assert.Equal(t, "testdata/golden", d.Path())
}

func TestNewDir_WithPath(t *testing.T) {
	d := NewDir(t, WithPath("testdata/foo.golden"))
	assert.Equal(t, "testdata/foo.golden", d.Path())
}

func TestNewDir_WithRecreateOnUpdate(t *testing.T) {
	d := NewDir(t, WithRecreateOnUpdate())
	assert.True(t, d.recreateOnUpdate)
}

func TestEquals_MatchingContent_NoFailure(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "f.txt"), []byte("hello"), 0o644))

	d := NewDir(t, WithPath(tmp))
	d.Equals("f.txt", []byte("hello"))
}

func TestEquals_Mismatch_Fails(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "f.txt"), []byte("hello"), 0o644))

	ft := &fakeT{}
	d := NewDir(ft, WithPath(tmp))
	runCatchingFatal(t, func() { d.Equals("f.txt", []byte("world")) })
	assert.Len(t, ft.fatalCalls, 1, "expected Fatal to be called on mismatch")
	assert.Contains(t, ft.fatalCalls[0], "golden mismatch")
}

func TestEquals_MissingGoldenFile_Fails(t *testing.T) {
	tmp := t.TempDir()

	ft := &fakeT{}
	d := NewDir(ft, WithPath(tmp))
	runCatchingFatal(t, func() { d.Equals("nonexistent.txt", []byte("anything")) })
	assert.Len(t, ft.fatalCalls, 1, "expected Fatal to be called on missing golden file")
	assert.Contains(t, ft.fatalCalls[0], "golden: read")
}

func TestEquals_UpdateMode_CreatesGoldenFile(t *testing.T) {
	setUpdate(t, true)
	tmp := t.TempDir()
	d := NewDir(t, WithPath(tmp))

	d.Equals("new.txt", []byte("fresh content"))

	got, err := os.ReadFile(filepath.Join(tmp, "new.txt"))
	require.NoError(t, err)
	assert.Equal(t, "fresh content", string(got))
}

func TestEquals_UpdateMode_OverwritesExisting(t *testing.T) {
	setUpdate(t, true)
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "f.txt"), []byte("old"), 0o644))

	d := NewDir(t, WithPath(tmp))
	d.Equals("f.txt", []byte("new"))

	got, err := os.ReadFile(filepath.Join(tmp, "f.txt"))
	require.NoError(t, err)
	assert.Equal(t, "new", string(got))
}

func TestEquals_UpdateMode_CreatesNestedDirs(t *testing.T) {
	setUpdate(t, true)
	tmp := t.TempDir()
	d := NewDir(t, WithPath(tmp))

	d.Equals("a/b/c/deep.txt", []byte("nested"))

	got, err := os.ReadFile(filepath.Join(tmp, "a", "b", "c", "deep.txt"))
	require.NoError(t, err)
	assert.Equal(t, "nested", string(got))
}

func TestNewDir_RecreateOnUpdate_RemovesOldFiles(t *testing.T) {
	setUpdate(t, true)
	tmp := t.TempDir()
	// Создаём старый golden-файл, который не должен остаться после recreate
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "stale.txt"), []byte("old"), 0o644))

	d := NewDir(t, WithPath(tmp), WithRecreateOnUpdate())
	d.Equals("fresh.txt", []byte("new"))

	// stale.txt должен быть удалён при recreate
	_, err := os.Stat(filepath.Join(tmp, "stale.txt"))
	assert.True(t, os.IsNotExist(err), "expected stale.txt to be removed")

	// fresh.txt должен существовать
	got, err := os.ReadFile(filepath.Join(tmp, "fresh.txt"))
	require.NoError(t, err)
	assert.Equal(t, "new", string(got))
}

func TestNewDir_NoRecreateOnUpdate_KeepsOldFiles(t *testing.T) {
	setUpdate(t, true)
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "stale.txt"), []byte("old"), 0o644))

	d := NewDir(t, WithPath(tmp))
	d.Equals("fresh.txt", []byte("new"))

	// stale.txt должен остаться
	got, err := os.ReadFile(filepath.Join(tmp, "stale.txt"))
	require.NoError(t, err)
	assert.Equal(t, "old", string(got))
}

func TestCodegenFS_ImplementsCodegenFileWriter(t *testing.T) {
	var _ codegen.FileWriter = (*CodegenFS)(nil)
}

func TestCodegenFS_WriteFile_DelegatesToDirEquals(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "f.gen.go"), []byte("package model\n"), 0o644))

	d := NewDir(t, WithPath(tmp))
	fw := NewCodegenFS(t, d)

	err := fw.WriteFile("f.gen.go", codegen.NewFile([]byte("package model\n")))
	assert.NoError(t, err)
}

func TestCodegenFS_WriteFile_MismatchFails(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "f.gen.go"), []byte("package old\n"), 0o644))

	ft := &fakeT{}
	d := NewDir(ft, WithPath(tmp))
	fw := NewCodegenFS(ft, d)
	runCatchingFatal(t, func() {
		_ = fw.WriteFile("f.gen.go", codegen.NewFile([]byte("package new\n")))
	})
	assert.Len(t, ft.fatalCalls, 1, "expected Fatal on mismatch")
	assert.Contains(t, ft.fatalCalls[0], "golden mismatch")
}

func TestCodegenFS_WriteFile_UpdateModeWritesGolden(t *testing.T) {
	setUpdate(t, true)
	tmp := t.TempDir()
	d := NewDir(t, WithPath(tmp))
	fw := NewCodegenFS(t, d)

	err := fw.WriteFile("model/user.gen.go", codegen.NewFile([]byte("package model\n")))
	require.NoError(t, err)

	got, err := os.ReadFile(filepath.Join(tmp, "model", "user.gen.go"))
	require.NoError(t, err)
	assert.Equal(t, "package model\n", string(got))
}

func TestCodegenFS_Close_NoError(t *testing.T) {
	d := NewDir(t, WithPath(t.TempDir()))
	fw := NewCodegenFS(t, d)
	assert.NoError(t, fw.Close())
}

func TestCompare_MissingFileReturnsError(t *testing.T) {
	d := &Dir{path: "/nonexistent/path/that/should/not/exist"}
	err := d.compare("/nonexistent/path/that/should/not/exist/f.txt", []byte("x"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "golden: read")
}

func TestCompare_MatchingReturnsNil(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "f.txt")
	require.NoError(t, os.WriteFile(p, []byte("same"), 0o644))

	d := &Dir{path: tmp}
	err := d.compare(p, []byte("same"))
	assert.NoError(t, err)
}

func TestFlagRegistration(t *testing.T) {
	// Флаг должен быть зарегистрирован в flag.CommandLine.
	f := flag.Lookup("update")
	require.NotNil(t, f)
	assert.Contains(t, f.Usage, "golden")
}

func TestEquals_UpdateMode_MkdirAllFails(t *testing.T) {
	setUpdate(t, true)
	tmp := t.TempDir()
	// blocker — файл; MkdirAll("blocker/...") упадёт.
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "blocker"), []byte("x"), 0o644))

	ft := &fakeT{}
	d := NewDir(ft, WithPath(tmp))
	runCatchingFatal(t, func() { d.Equals("blocker/nested.txt", []byte("x")) })
	require.Len(t, ft.fatalCalls, 1)
	assert.Contains(t, ft.fatalCalls[0], "golden: create parent dir")
}

func TestEquals_UpdateMode_WriteFileFails(t *testing.T) {
	setUpdate(t, true)
	tmp := t.TempDir()
	// target — каталог; WriteFile в каталог упадёт.
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "target"), 0o755))

	ft := &fakeT{}
	d := NewDir(ft, WithPath(tmp))
	runCatchingFatal(t, func() { d.Equals("target", []byte("x")) })
	require.Len(t, ft.fatalCalls, 1)
	assert.Contains(t, ft.fatalCalls[0], "golden: write")
}
