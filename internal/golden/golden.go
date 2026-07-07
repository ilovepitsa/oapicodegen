// Package golden — golden-тесты: сравнение сгенерированного вывода с
// эталонными файлами в testdata/. Замена
// git.mws-team.ru/mws/devp/platform-go/pkg/golden.
//
// Конвейер:
//
//	dir := golden.NewDir(t, golden.WithPath("testdata/foo.golden"))
//	fw := golden.NewCodegenFS(t, dir)
//	generator.Generate(fw, ...)   // fw.WriteFile сравнивает/обновляет golden
//
// Режим -update: golden-файлы перезаписываются вместо сравнения.
package golden

import (
	"flag"
	"fmt"
	"nschugorev/oapigenerator/internal/codegen"
	"os"
	"path/filepath"

	"github.com/google/go-cmp/cmp"
)

var updateFlag = flag.Bool("update", false, "update golden files instead of comparing")

// Update сообщает, установлен ли флаг -update.
func Update() bool { return *updateFlag }

// Dir — каталог golden-файлов на диске.
type Dir struct {
	t interface {
		Fatal(args ...any)
		Fatalf(format string, args ...any)
	}
	path             string
	recreateOnUpdate bool
}

// Option настраивает Dir.
type Option func(*Dir)

// WithPath задаёт путь к golden-каталогу (по умолчанию "testdata/golden").
func WithPath(p string) Option {
	return func(d *Dir) { d.path = p }
}

// WithRecreateOnUpdate заставляет NewDir удалить каталог при -update перед
// записью новых golden-файлов (чтобы не накапливались устаревшие эталоны).
func WithRecreateOnUpdate() Option {
	return func(d *Dir) { d.recreateOnUpdate = true }
}

// NewDir создаёт golden-каталог. При -update и WithRecreateOnUpdate удаляет
// существующий каталог.
func NewDir(t interface {
	Fatal(args ...any)
	Fatalf(format string, args ...any)
}, opts ...Option,
) *Dir {
	d := &Dir{t: t, path: "testdata/golden"}
	for _, opt := range opts {
		opt(d)
	}
	if Update() && d.recreateOnUpdate {
		_ = os.RemoveAll(d.path)
	}
	return d
}

// Path возвращает путь к golden-каталогу.
func (d *Dir) Path() string { return d.path }

// Equals сравнивает got с содержимым golden-файла name (относительно Path).
// При -update: записывает got в golden-файл (создавая родительские каталоги).
// Без -update: при расхождении или отсутствии файла — t.Fatal с diff.
func (d *Dir) Equals(name string, got []byte) {
	goldenPath := filepath.Join(d.path, name)
	if Update() {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			d.t.Fatalf("golden: create parent dir for %s: %v", goldenPath, err)
		}
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			d.t.Fatalf("golden: write %s: %v", goldenPath, err)
		}
		return
	}
	if err := d.compare(goldenPath, got); err != nil {
		d.t.Fatal(err)
	}
}

func (d *Dir) compare(goldenPath string, got []byte) error {
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		return fmt.Errorf("golden: read %s: %v (run with -update to create)", goldenPath, err)
	}
	if diff := cmp.Diff(string(want), string(got)); diff != "" {
		return fmt.Errorf("golden mismatch for %s:\n%s", goldenPath, diff)
	}
	return nil
}

// CodegenFS — codegen.FileWriter, который делегирует каждый WriteFile в
// Dir.Equals (сравнение или обновление golden). Используется в e2e-тестах
// генератора: генератор пишет в CodegenFS, а тот автоматически сверяет
// каждый файл с эталоном.
type CodegenFS struct {
	t interface {
		Fatal(args ...any)
		Fatalf(format string, args ...any)
	}
	dir *Dir
}

// NewCodegenFS создаёт CodegenFS для заданного golden-каталога.
func NewCodegenFS(t interface {
	Fatal(args ...any)
	Fatalf(format string, args ...any)
}, dir *Dir,
) *CodegenFS {
	return &CodegenFS{t: t, dir: dir}
}

// WriteFile сравнивает file.Content() с golden-файлом name (или обновляет
// при -update). Реализует codegen.FileWriter.
func (c *CodegenFS) WriteFile(name string, file codegen.File) error {
	c.dir.Equals(name, file.Content())
	return nil
}

// Close — no-op. Реализует codegen.FileWriter.
func (c *CodegenFS) Close() error { return nil }
