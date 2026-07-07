// Package configurator строит codegen.FileWriter из CLI-флагов и runtime
// output-пути. Замена git.mws-team.ru/mws/devp/platform-go/pkg/codegen/configurator.
//
// Конвейер: NewFileWriterConfiguratorFromFlags(flagSet) → Create(output) →
// fileWriter, который использует internal/fs.RealFS с WithBaseDir(output) и
// автоматически создаёт родительские каталоги при WriteFile.
//
// Dry-run НЕ обрабатывается здесь (как и в оригинале) — caller (cmd/oapigen)
// должен сам вернуть codegen.NoopFileWriter при dry-run.
package configurator

import (
	"flag"
	"fmt"
	"nschugorev/oapigenerator/internal/codegen"
	"os"
	"path"

	realfs "nschugorev/oapigenerator/internal/fs"
)

// Configurator хранит настройки FileWriter, зарегистрированные на FlagSet.
type Configurator struct {
	flagSet *flag.FlagSet
	dirPerm os.FileMode
}

const defaultDirPerm os.FileMode = 0o755

// NewFileWriterConfiguratorFromFlags создаёт Configurator и сохраняет
// FlagSet для будущего расширения (например, -fw-dir-perm, -fw-file-perm).
// В первой итерации флаги не регистрируются; используются дефолты:
// dirPerm=0755.
func NewFileWriterConfiguratorFromFlags(fs *flag.FlagSet) *Configurator {
	return &Configurator{
		flagSet: fs,
		dirPerm: defaultDirPerm,
	}
}

// Create возвращает FileWriter, пишущий в output. Если output пустой —
// возвращает NoopFileWriter. Гарантирует существование output-каталога
// через MkdirAll.
func (c *Configurator) Create(output string) (codegen.FileWriter, error) {
	if output == "" {
		return codegen.NoopFileWriter{}, nil
	}

	fs := realfs.NewRecommendedReal(realfs.WithBaseDir(output))
	if err := fs.MkdirAll(".", c.dirPerm); err != nil {
		return nil, fmt.Errorf("configurator: create output dir %q: %w", output, err)
	}

	return &fileWriter{fs: fs, dirPerm: c.dirPerm}, nil
}

// fileWriter — codegen.FileWriter поверх internal/fs.FS. Создаёт родительские
// каталоги файла по требованию.
type fileWriter struct {
	fs      realfs.FS
	dirPerm os.FileMode
}

func (w *fileWriter) WriteFile(name string, file codegen.File) error {
	if dir := path.Dir(name); dir != "" && dir != "." {
		if err := w.fs.MkdirAll(dir, w.dirPerm); err != nil {
			return fmt.Errorf("mkdir %q: %w", dir, err)
		}
	}

	if err := w.fs.WriteFile(name, file.Content()); err != nil {
		return fmt.Errorf("write %q: %w", name, err)
	}

	return nil
}

func (w *fileWriter) Close() error { return nil }
