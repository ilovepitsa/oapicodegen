package codegen

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// CompileCheck запускает `go build ./...` в outputDir для проверки, что
// сгенерированный код компилируется. Требует наличия go.mod в outputDir.
//
// Если go.mod отсутствует — возвращает ошибку с подсказкой инициализировать
// модуль. Если `go build` завершается с ошибкой — возвращает ошибку,
// включающую stderr компилятора.
func CompileCheck(outputDir string) error {
	goModPath := filepath.Join(outputDir, "go.mod")
	if _, err := os.Stat(goModPath); err != nil {
		return fmt.Errorf(
			"output must be a Go module root (go.mod not found at %s); "+
				"run `go mod init <import-prefix>`",
			goModPath,
		)
	}

	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = outputDir

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go build ./... failed: %w\n%s", err, out)
	}

	return nil
}
