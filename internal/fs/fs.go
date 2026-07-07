// Package fs — обёртка над os-файловай системой с опцией sandbox-базового
// каталога (WithBaseDir). Замена git.mws-team.ru/mws/devp/platform-go/pkg/fs.
//
// В первой итерации поддерживается только RealFS (op на реальной FS). In-memory
// MapFS будет добавлен, когда задача (например, e2e-тесты T24) его реально
// потребует.
package fs

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// FS — читаемая и записываемая файловая система. Расширяет стандартные
// io/fs интерфейсы методами записи.
type FS interface {
	fs.FS
	fs.StatFS
	fs.ReadDirFS
	WriteFile(name string, data []byte) error
	MkdirAll(path string, perm os.FileMode) error
	Remove(name string) error
}

// ReadOnlyFS — читаемое подмножество FS. Используется там, где запись
// запрещена по контракту (например, загрузчик generation flags).
type ReadOnlyFS interface {
	fs.FS
	fs.StatFS
	fs.ReadDirFS
}

// ReadFile читает содержимое файла name из fsys. Удобная top-level функция
// поверх fs.ReadFile, повторяет сигнатуру platform-go/pkg/fs.ReadFile.
func ReadFile(fsys ReadOnlyFS, name string) ([]byte, error) {
	return fs.ReadFile(fsys, name) //nolint:wrapcheck // thin delegator
}

// RealFS — файловая система поверх os.*. При заданном baseDir все пути
// вычисляются относительно baseDir и не могут выйти за его пределы через "..".
type RealFS struct {
	baseDir string
}

// Option настраивает RealFS.
type Option func(*RealFS)

// WithBaseDir устанавливает базовый каталог: пути резолвятся относительно dir
// и не могут его покинуть.
func WithBaseDir(dir string) Option {
	return func(r *RealFS) { r.baseDir = dir }
}

// NewRealFS возвращает RealFS без базового каталога (работает от cwd).
func NewRealFS() *RealFS { return &RealFS{} }

// NewRecommendedReal возвращает RealFS с применёнными опциями.
func NewRecommendedReal(opts ...Option) *RealFS {
	r := &RealFS{}
	for _, opt := range opts {
		opt(r)
	}

	return r
}

func (r *RealFS) resolve(name string) (string, error) {
	if name == "" {
		return "", fs.ErrInvalid
	}

	if r.baseDir == "" {
		return filepath.Clean(name), nil
	}

	base := filepath.Clean(r.baseDir)
	cleaned := filepath.Clean(filepath.Join(base, name))

	if cleaned != base && !strings.HasPrefix(cleaned, base+string(filepath.Separator)) {
		return "", fmt.Errorf("fs: path %q escapes base dir %q", name, r.baseDir)
	}

	return cleaned, nil
}

func (r *RealFS) Open(name string) (fs.File, error) {
	p, err := r.resolve(name)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(p)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", name, err)
	}

	return f, nil
}

func (r *RealFS) Stat(name string) (fs.FileInfo, error) {
	p, err := r.resolve(name)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(p)
	if err != nil {
		return nil, fmt.Errorf("stat %q: %w", name, err)
	}

	return info, nil
}

func (r *RealFS) ReadDir(name string) ([]fs.DirEntry, error) {
	p, err := r.resolve(name)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(p)
	if err != nil {
		return nil, fmt.Errorf("readdir %q: %w", name, err)
	}

	return entries, nil
}

func (r *RealFS) WriteFile(name string, data []byte) error {
	p, err := r.resolve(name)
	if err != nil {
		return err
	}

	if err := os.WriteFile(p, data, 0o644); err != nil { //nolint:gosec,mnd,lll // standard perm for generated Go files
		return fmt.Errorf("write %q: %w", name, err)
	}

	return nil
}

func (r *RealFS) MkdirAll(path string, perm os.FileMode) error {
	p, err := r.resolve(path)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(p, perm); err != nil {
		return fmt.Errorf("mkdir %q: %w", path, err)
	}

	return nil
}

func (r *RealFS) Remove(name string) error {
	p, err := r.resolve(name)
	if err != nil {
		return err
	}

	if err := os.Remove(p); err != nil {
		return fmt.Errorf("remove %q: %w", name, err)
	}

	return nil
}
