// Package codegen — ядро абстракций вывода сгенерированных файлов: интерфейс
// File, FileWriter (запись в FS), Writer (код-райтер с Print/WriteString/
// NewLine), BufferWriter (Writer + File), WithPath (префиксация путей),
// NoopFileWriter (dry-run).
// Замена git.mws-team.ru/mws/devp/platform-go/pkg/codegen (без gogen — он в T7).
package codegen

import (
	"bytes"
	"fmt"
	"path"
)

// File — содержимое файла, готовое к записи в FS.
type File interface {
	Content() []byte
}

// rawFile — File, backed напрямую []byte.
type rawFile struct {
	data []byte
}

func (r *rawFile) Content() []byte { return r.data }

// NewFile создаёт File из сырых байт. Удобно для отладочного JSON/YAML вывода.
func NewFile(data []byte) File { return &rawFile{data: data} }

// FileWriter — записывает File-ы в файловую систему.
type FileWriter interface {
	WriteFile(name string, file File) error
	Close() error
}

// Writer — строитель исходного кода с буфером. Не реализует File; для записи
// в FileWriter используйте BufferWriter.
type Writer struct {
	buf bytes.Buffer
}

// Print пишет args через fmt.Fprint (без разделителей между args).
// Удобно для кодогенерации: w.Print("resp, err := ", call, "\n").
func (w *Writer) Print(args ...any) {
	_, _ = fmt.Fprint(&w.buf, args...)
}

// WriteString пишет строку s как есть.
func (w *Writer) WriteString(s string) {
	w.buf.WriteString(s)
}

// NewLine пишет один перевод строки.
func (w *Writer) NewLine() {
	w.buf.WriteByte('\n')
}

// Len возвращает текущий размер буфера в байтах.
func (w *Writer) Len() int { return w.buf.Len() }

// Reset очищает буфер.
func (w *Writer) Reset() { w.buf.Reset() }

// String возвращает накопленное содержимое.
func (w *Writer) String() string { return w.buf.String() }

// BufferWriter — Writer, дополнительно реализующий File.
type BufferWriter struct {
	Writer
}

// NewBufferWriter возвращает новый пустой BufferWriter.
func NewBufferWriter() *BufferWriter { return &BufferWriter{} }

// Content возвращает накопленные байты. Реализует File.
func (b *BufferWriter) Content() []byte { return b.buf.Bytes() }

// WithPath оборачивает fw так, что все WriteFile получают префикс path/.
// Пустая path означает «без префикса» — возвращаемый FileWriter делегирует
// вызовы напрямую. Складывается: WithPath(WithPath(fw, "a"), "b") → "a/b/...".
func WithPath(fw FileWriter, p string) FileWriter {
	if p == "" {
		return fw
	}
	return &pathWriter{inner: fw, prefix: p}
}

type pathWriter struct {
	inner  FileWriter
	prefix string
}

func (p *pathWriter) WriteFile(name string, file File) error {
	return p.inner.WriteFile(path.Join(p.prefix, name), file)
}

func (p *pathWriter) Close() error { return p.inner.Close() }

// NoopFileWriter — FileWriter, отбрасывающий все записи. Используется для
// dry-run генерации.
type NoopFileWriter struct{}

// WriteFile игнорирует аргументы и возвращает nil.
func (NoopFileWriter) WriteFile(string, File) error { return nil }

// Close возвращает nil.
func (NoopFileWriter) Close() error { return nil }
