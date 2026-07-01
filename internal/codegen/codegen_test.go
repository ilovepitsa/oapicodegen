package codegen

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFile_Content(t *testing.T) {
	f := NewFile([]byte("hello"))
	assert.True(t, bytes.Equal(f.Content(), []byte("hello")))
}

func TestNewFile_EmptyBytes(t *testing.T) {
	f := NewFile(nil)
	assert.Empty(t, f.Content())
}

func TestBufferWriter_P_Variadic(t *testing.T) {
	b := NewBufferWriter()
	b.P("resp, err := ", "call()", "\n")
	assert.Equal(t, "resp, err := call()\n", b.String())
}

func TestBufferWriter_P_SingleString(t *testing.T) {
	b := NewBufferWriter()
	b.P("waitTimeout := ")
	assert.Equal(t, "waitTimeout := ", b.String())
}

func TestBufferWriter_W(t *testing.T) {
	b := NewBufferWriter()
	b.W("line1")
	b.W("line2")
	assert.Equal(t, "line1line2", b.String())
}

func TestBufferWriter_NL(t *testing.T) {
	b := NewBufferWriter()
	b.W("line1")
	b.NL()
	b.W("line2")
	assert.Equal(t, "line1\nline2", b.String())
}

func TestBufferWriter_Content_Bytes(t *testing.T) {
	b := NewBufferWriter()
	b.P("data")
	assert.True(t, bytes.Equal([]byte("data"), b.Content()))
}

func TestBufferWriter_ImplementsFile(t *testing.T) {
	var _ File = (*BufferWriter)(nil)
}

func TestBufferWriter_Len_Reset(t *testing.T) {
	b := NewBufferWriter()
	b.P("hello")
	assert.Equal(t, 5, b.Len())
	b.Reset()
	assert.Zero(t, b.Len())
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
	require.NoError(t, fw.WriteFile("user.gen.go", b))
	_, ok := cap.files["model/user.gen.go"]
	assert.True(t, ok, "expected file at model/user.gen.go; got keys: %v", keys(cap.files))
}

func TestPathWriter_CleansPath(t *testing.T) {
	cap := &captureWriter{files: map[string][]byte{}}
	fw := WithPath(cap, "model/")

	require.NoError(t, fw.WriteFile("user.gen.go", NewFile([]byte("x"))))
	// path.Join нормализует trailing slash
	_, ok := cap.files["model/user.gen.go"]
	assert.True(t, ok, "expected normalized path; got keys: %v", keys(cap.files))
}

func TestWithPath_EmptyPathPassthrough(t *testing.T) {
	cap := &captureWriter{files: map[string][]byte{}}
	fw := WithPath(cap, "")

	require.NoError(t, fw.WriteFile("a.go", NewFile([]byte("x"))))
	_, ok := cap.files["a.go"]
	assert.True(t, ok, "expected file at a.go; got keys: %v", keys(cap.files))
}

func TestWithPath_Stacks(t *testing.T) {
	cap := &captureWriter{files: map[string][]byte{}}
	fw := WithPath(WithPath(cap, "a"), "b")

	require.NoError(t, fw.WriteFile("c.go", NewFile([]byte("x"))))
	_, ok := cap.files["a/b/c.go"]
	assert.True(t, ok, "expected file at a/b/c.go; got keys: %v", keys(cap.files))
}

func TestWithPath_CloseDelegatesToInner(t *testing.T) {
	cap := &captureWriter{files: map[string][]byte{}}
	fw := WithPath(cap, "model")
	assert.NoError(t, fw.Close())
}

func TestNoopFileWriter_WriteFile(t *testing.T) {
	w := NoopFileWriter{}
	assert.NoError(t, w.WriteFile("any.go", NewFile([]byte("anything"))))
}

func TestNoopFileWriter_Close(t *testing.T) {
	w := NoopFileWriter{}
	assert.NoError(t, w.Close())
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
