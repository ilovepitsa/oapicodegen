package fs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestNewRealFS_NoBaseDir(t *testing.T) {
	rfs := NewRealFS()
	if rfs.baseDir != "" {
		t.Fatalf("expected empty baseDir, got %q", rfs.baseDir)
	}
}

func TestNewRecommendedReal_WithBaseDir(t *testing.T) {
	dir := t.TempDir()
	rfs := NewRecommendedReal(WithBaseDir(dir))
	if rfs.baseDir != dir {
		t.Fatalf("expected baseDir %q, got %q", dir, rfs.baseDir)
	}
}

func TestRealFS_WriteReadRoundtrip(t *testing.T) {
	rfs := NewRealFS()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "out.txt")

	if err := rfs.WriteFile(path, []byte("hello")); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := ReadFile(rfs, path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("expected hello, got %q", got)
	}
}

func TestRealFS_Stat(t *testing.T) {
	rfs := NewRealFS()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "f.txt")
	_ = rfs.WriteFile(path, []byte("x"))

	info, err := rfs.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Size() != 1 {
		t.Fatalf("expected size 1, got %d", info.Size())
	}
}

func TestRealFS_ReadDir(t *testing.T) {
	rfs := NewRealFS()
	tmp := t.TempDir()
	_ = rfs.WriteFile(filepath.Join(tmp, "a.txt"), []byte("a"))
	_ = rfs.WriteFile(filepath.Join(tmp, "b.txt"), []byte("b"))

	entries, err := rfs.ReadDir(tmp)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestRealFS_MkdirAll_Remove(t *testing.T) {
	rfs := NewRealFS()
	tmp := t.TempDir()
	nested := filepath.Join(tmp, "a", "b", "c")

	if err := rfs.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	info, err := rfs.Stat(nested)
	if err != nil {
		t.Fatalf("Stat after MkdirAll: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected directory")
	}

	if err := rfs.Remove(nested); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := rfs.Stat(nested); !os.IsNotExist(err) {
		t.Fatalf("expected NotExist after Remove, got %v", err)
	}
}

func TestRealFS_Open_ReadsContent(t *testing.T) {
	rfs := NewRealFS()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "f.txt")
	_ = rfs.WriteFile(path, []byte("content"))

	f, err := rfs.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	buf := make([]byte, 7)
	n, err := f.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if n != 7 || string(buf) != "content" {
		t.Fatalf("expected content, got %q", buf[:n])
	}
}

func TestRealFS_BaseDir_Sandbox(t *testing.T) {
	base := t.TempDir()
	rfs := NewRecommendedReal(WithBaseDir(base))

	if err := rfs.WriteFile("file.txt", []byte("data")); err != nil {
		t.Fatalf("WriteFile within base: %v", err)
	}
	got, err := ReadFile(rfs, "file.txt")
	if err != nil {
		t.Fatalf("ReadFile within base: %v", err)
	}
	if string(got) != "data" {
		t.Fatalf("expected data, got %q", got)
	}
}

func TestRealFS_BaseDir_RejectsEscape(t *testing.T) {
	base := t.TempDir()
	rfs := NewRecommendedReal(WithBaseDir(base))

	if err := rfs.WriteFile("../escape.txt", []byte("x")); err == nil {
		t.Fatal("expected error escaping base dir, got nil")
	}
	if err := rfs.MkdirAll("../escape", 0o755); err == nil {
		t.Fatal("expected error escaping base dir via MkdirAll, got nil")
	}
}

func TestRealFS_BaseDir_NestedDirsCreated(t *testing.T) {
	base := t.TempDir()
	rfs := NewRecommendedReal(WithBaseDir(base))

	if err := rfs.WriteFile(filepath.Join("a", "b", "c.txt"), []byte("x")); err != nil {
		// WriteFile doesn't create parent dirs; expect failure
		if !isPathError(err) {
			t.Fatalf("expected path error for missing parent, got %v", err)
		}
	} else {
		t.Fatal("expected WriteFile to fail without parent dirs")
	}

	if err := rfs.MkdirAll(filepath.Join("a", "b"), 0o755); err != nil {
		t.Fatalf("MkdirAll nested: %v", err)
	}
	if err := rfs.WriteFile(filepath.Join("a", "b", "c.txt"), []byte("x")); err != nil {
		t.Fatalf("WriteFile after MkdirAll: %v", err)
	}
}

func TestRealFS_Resolve_EmptyName(t *testing.T) {
	rfs := NewRealFS()
	if _, err := rfs.Stat(""); err == nil {
		t.Fatal("expected error on empty name, got nil")
	}
}

func TestRealFS_ReadOnlyFS_InterfaceAcceptance(t *testing.T) {
	// RealFS должен удовлетворять и FS, и ReadOnlyFS.
	var _ FS = (*RealFS)(nil)
	var _ ReadOnlyFS = (*RealFS)(nil)
}

func isPathError(err error) bool {
	var pe *fs.PathError
	return errors.As(err, &pe)
}
