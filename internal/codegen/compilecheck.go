package codegen

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// CompileCheck запускает `go build ./...` для проверки, что сгенерированный
// код компилируется. Ищет go.mod начиная с outputDir и поднимаясь вверх до
// корня файловой системы: это позволяет держать сгенерированный output внутри
// существующего модуля (go.mod на уровень выше или выше), а не обязывает
// держать go.mod в самой output-директории.
//
// Если go.mod найден — `go build ./...` запускается из корня найденного модуля
// (компилируется весь модуль, включая output). Если go.mod отсутствует в output
// и во всех родительских каталогах — возвращает ошибку с подсказкой.
func CompileCheck(outputDir string) error {
	moduleRoot, ok := findGoModRoot(outputDir)
	if !ok {
		return fmt.Errorf(
			"go.mod not found in %s or any parent directory; "+
				"run `go mod init <import-prefix>` in your module root",
			outputDir,
		)
	}

	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = moduleRoot

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go build ./... failed (module root %s): %w\n%s", moduleRoot, err, out)
	}

	return nil
}

// findGoModRoot ищет go.mod начиная с dir и поднимаясь вверх до корня ФС.
// Возвращает (moduleRoot, true), если найден, иначе ("", false).
func findGoModRoot(dir string) (string, bool) {
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// достигли корня файловой системы
			return "", false
		}
		dir = parent
	}
}
