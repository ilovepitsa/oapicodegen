package codegen

import (
	"bytes"
	"testing"
)

func TestNewFile_Content(t *testing.T) {
	f := NewFile([]byte("hello"))
	if !bytes.Equal(f.Content(), []byte("hello")) {
		t.Fatalf("expected hello, got %q", f.Content())
	}
}

func TestNewFile_EmptyBytes(t *testing.T) {
	f := NewFile(nil)
	if len(f.Content()) != 0 {
		t.Fatalf("expected empty, got %q", f.Content())
	}
}

func TestBufferWriter_P_Variadic(t *testing.T) {
	b := NewBufferWriter()
	b.P("resp, err := ", "call()", "\n")
	if b.String() != "resp, err := call()\n" {
		t.Fatalf("unexpected content: %q", b.String())
	}
}

func TestBufferWriter_P_SingleString(t *testing.T) {
	b := NewBufferWriter()
	b.P("waitTimeout := ")
	if b.String() != "waitTimeout := " {
		t.Fatalf("unexpected: %q", b.String())
	}
}

func TestBufferWriter_W(t *testing.T) {
	b := NewBufferWriter()
	b.W("line1")
	b.W("line2")
	if b.String() != "line1line2" {
		t.Fatalf("unexpected: %q", b.String())
	}
}

func TestBufferWriter_NL(t *testing.T) {
	b := NewBufferWriter()
	b.W("line1")
	b.NL()
	b.W("line2")
	if b.String() != "line1\nline2" {
		t.Fatalf("unexpected: %q", b.String())
	}
}

func TestBufferWriter_Content_Bytes(t *testing.T) {
	b := NewBufferWriter()
	b.P("data")
	got := b.Content()
	if !bytes.Equal(got, []byte("data")) {
		t.Fatalf("expected []byte(data), got %q", got)
	}
}

func TestBufferWriter_ImplementsFile(t *testing.T) {
	var _ File = (*BufferWriter)(nil)
}

func TestBufferWriter_Len_Reset(t *testing.T) {
	b := NewBufferWriter()
	b.P("hello")
	if b.Len() != 5 {
		t.Fatalf("expected len 5, got %d", b.Len())
	}
	b.Reset()
	if b.Len() != 0 {
		t.Fatalf("expected len 0 after reset, got %d", b.Len())
	}
}

// captureWriter — тестовый FileWriter, сохраняющий все записи в map.
type captureWriter struct {
	files map[string][]byte
}

func (c *captureWriter) WriteFile(name string, file File) error {
	c.files[name] = file.Content()
	return nil
}

func (c *captureWriter) Close() error { return nil }

func TestWithPath_PrefixesName(t *testing.T) {
	cap := &captureWriter{files: map[string][]byte{}}
	fw := WithPath(cap, "model")

	b := NewBufferWriter()
	b.P("package model")
	if err := fw.WriteFile("user.gen.go", b); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, ok := cap.files["model/user.gen.go"]; !ok {
		t.Fatalf("expected file at model/user.gen.go; got keys: %v", keys(cap.files))
	}
}

func TestPathWriter_CleansPath(t *testing.T) {
	cap := &captureWriter{files: map[string][]byte{}}
	fw := WithPath(cap, "model/")

	if err := fw.WriteFile("user.gen.go", NewFile([]byte("x"))); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// path.Join нормализует trailing slash
	if _, ok := cap.files["model/user.gen.go"]; !ok {
		t.Fatalf("expected normalized path; got keys: %v", keys(cap.files))
	}
}

func TestWithPath_EmptyPathPassthrough(t *testing.T) {
	cap := &captureWriter{files: map[string][]byte{}}
	fw := WithPath(cap, "")

	if err := fw.WriteFile("a.go", NewFile([]byte("x"))); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, ok := cap.files["a.go"]; !ok {
		t.Fatalf("expected file at a.go; got keys: %v", keys(cap.files))
	}
}

func TestWithPath_Stacks(t *testing.T) {
	cap := &captureWriter{files: map[string][]byte{}}
	fw := WithPath(WithPath(cap, "a"), "b")

	if err := fw.WriteFile("c.go", NewFile([]byte("x"))); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, ok := cap.files["a/b/c.go"]; !ok {
		t.Fatalf("expected file at a/b/c.go; got keys: %v", keys(cap.files))
	}
}

func TestWithPath_CloseDelegatesToInner(t *testing.T) {
	cap := &captureWriter{files: map[string][]byte{}}
	fw := WithPath(cap, "model")
	if err := fw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestNoopFileWriter_WriteFile(t *testing.T) {
	w := NoopFileWriter{}
	if err := w.WriteFile("any.go", NewFile([]byte("anything"))); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestNoopFileWriter_Close(t *testing.T) {
	w := NoopFileWriter{}
	if err := w.Close(); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestNoopFileWriter_ImplementsFileWriter(t *testing.T) {
	var _ FileWriter = NoopFileWriter{}
}

func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
