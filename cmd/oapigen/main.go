// Package main implements the oapigen CLI entry point.
//
// Связывает parser → generator и пишет Go-пакеты в output-каталог.
//
//	usage: oapigen -input ./input -output ./gen -import-prefix github.com/foo/bar/gen
//
// -input — каталог проекта, содержащий подпапки сервисов вида
// `<service>/src/openapi/openapi.yaml`. CLI обходит каталог, парсит каждую
// найденную спеку и генерирует пакеты в `<output>/<service>/...`.
package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/ilovepitsa/oapicodegen/internal/cli/logging"
	"github.com/ilovepitsa/oapicodegen/internal/codegen"
	"github.com/ilovepitsa/oapicodegen/internal/codegen/configurator"
	"github.com/ilovepitsa/oapicodegen/internal/fs"
	"github.com/ilovepitsa/oapicodegen/internal/generator"
	"github.com/ilovepitsa/oapicodegen/internal/generator/render"
	"github.com/ilovepitsa/oapicodegen/internal/parser"
	"github.com/ilovepitsa/oapicodegen/internal/version"
	"os"

	"go.uber.org/zap"
)

func Main() int {
	if err := run(os.Args[1:], os.Stderr); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "oapigen: %v\n", err)

		return 1
	}

	return 0
}

//nolint:gocyclo,cyclop,funlen // CLI pipeline, linear by nature
func run(args []string, stderr *os.File) error {
	flagSet := flag.NewFlagSet("oapigen", flag.ContinueOnError)
	flagSet.SetOutput(stderr)

	var (
		input                 string
		output                string
		importPrefix          string
		dryRun                bool
		skipCompileCheck      bool
		generationFlagsConfig string
		showVersion           bool
	)

	flagSet.StringVar(&input, "input", "", "path to project root (directory with service subfolders)")
	flagSet.StringVar(&output, "output", "", "output directory for generated Go packages")
	flagSet.StringVar(&importPrefix, "import-prefix", "",
		"Go import path prefix for generated packages")
	flagSet.BoolVar(&dryRun, "dry-run", false, "parse and generate without writing to filesystem")
	flagSet.BoolVar(&skipCompileCheck, "skip-compile-check", false,
		"skip post-generation `go build ./...` check on output directory")
	flagSet.StringVar(
		&generationFlagsConfig, "generation-flags-config-path", "",
		"path to global generation_flags.yaml",
	)
	flagSet.BoolVar(&showVersion, "version", false, "print oapigen version and exit")

	logCfg := logging.NewLoggerConfiguratorFromFlags(flagSet)
	fwCfg := configurator.NewFileWriterConfiguratorFromFlags(flagSet)

	if err := flagSet.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	// --version обрабатывается до тяжёлой логики (загрузка проекта, генерация):
	// печатаем версию в stdout и выходим с кодом 0.
	if showVersion {
		fmt.Fprintf(os.Stdout, "oapigen (%s)\n", version.Get())

		return nil
	}

	if input == "" {
		return errors.New("-input is required")
	}

	if output == "" && !dryRun {
		return errors.New("-output is required (or use -dry-run)")
	}

	if importPrefix == "" {
		return errors.New("-import-prefix is required")
	}

	logger, err := logCfg.Create()
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}

	defer func() { _ = logger.Sync() }() //nolint:errcheck // zap.Sync often fails on stderr/stdout

	sugar := logger.Sugar()

	if err := validateInputDir(input); err != nil {
		return err
	}

	flagsLoader := parser.NewGenerationFlagsLoader(fs.NewRealFS())
	if err := loadGlobalFlags(flagsLoader, generationFlagsConfig, sugar); err != nil {
		return err
	}

	fw, err := buildFileWriter(fwCfg, output, dryRun, sugar)
	if err != nil {
		return err
	}

	defer func() {
		if cerr := fw.Close(); cerr != nil {
			sugar.Errorf("close file writer: %v", cerr)
		}
	}()

	ps, si, loadErr := parser.NewProjectLoader().Load(input, flagsLoader, importPrefix, output)
	if loadErr != nil {
		return fmt.Errorf("load project set: %w", loadErr)
	}

	sugar.Infof(
		"loaded %d projects (common=%v)",
		len(ps.Projects), ps.Common != nil,
	)

	// Precompute IsSplit для всех проектов до генерации: cross-service $ref
	// проверяет IsSplit target-схемы владельца, а Generate вычисляет splittable
	// только для текущего проекта (порядок генерации недетерминирован).
	generator.PrecomputeSplittable(ps)

	// Версия бинарника константа на запуск — логируем один раз на старте,
	// до начала per-project генерации.
	sugar.Infof("oapigen version %s", version.Get())

	for _, project := range ps.Projects {
		projectFW := codegen.WithPath(fw, project.Folder)
		col := render.NewCollector()
		if err := generator.Generate(projectFW, project, si, generator.WithDiagnostics(col)); err != nil {
			return fmt.Errorf("generate project %q: %w", project.Folder, err)
		}
		if err := drainDiagnostics(col, project.Folder, project.Features.SchemaAny.Mode, sugar); err != nil {
			return err
		}

		sugar.Infof("generated project: %s", project.Folder)
	}

	if !dryRun && !skipCompileCheck {
		if err := runCompileCheck(output, sugar); err != nil {
			return err
		}
	}

	sugar.Infof("generation complete: output=%s import-prefix=%s", output, importPrefix)

	return nil
}

func validateInputDir(input string) error {
	info, err := os.Stat(input)
	if err != nil {
		return fmt.Errorf("stat input %q: %w", input, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("-input must be a directory (project root), got file: %s", input)
	}

	return nil
}

func loadGlobalFlags(
	flagsLoader *parser.GenerationFlagsLoader,
	configPath string,
	sugar *zap.SugaredLogger,
) error {
	if configPath == "" {
		return nil
	}

	if err := flagsLoader.Load(configPath); err != nil {
		return fmt.Errorf("load generation flags config %q: %w", configPath, err)
	}

	sugar.Infof("loaded generation flags config: %s", configPath)

	return nil
}

func buildFileWriter(
	fwCfg *configurator.Configurator,
	output string,
	dryRun bool,
	sugar *zap.SugaredLogger,
) (codegen.FileWriter, error) {
	if dryRun {
		sugar.Info("dry-run mode: no files will be written")

		return codegen.NoopFileWriter{}, nil
	}

	fw, err := fwCfg.Create(output)
	if err != nil {
		return nil, fmt.Errorf("create file writer: %w", err)
	}

	return fw, nil
}

func runCompileCheck(output string, sugar *zap.SugaredLogger) error {
	sugar.Infof("running compile check: %s", output)

	if err := codegen.CompileCheck(output); err != nil {
		return fmt.Errorf("compile check: %w", err)
	}

	sugar.Info("compile check passed")

	return nil
}

// drainDiagnostics обрабатывает накопленные аудитом GOLANG_SCHEMA_ANY
// диагностики по режиму флага:
//   - silent: ничего не делает;
//   - warn:   логирует каждую через sugar.Warnf и продолжает;
//   - error:  логирует каждую, затем возвращает aggregated error —
//     генерация прерывается (cmd завершается с non-zero exit).
//
// Логирование в error-режиме тоже происходит — пользователь видит список
// проблем до сообщения о прерывании. Пустой коллектор — no-op для любого режима.
func drainDiagnostics(col *render.Collector, project, mode string, sugar *zap.SugaredLogger) error {
	diag := col.Drain()
	if len(diag) == 0 || mode == "silent" {
		return nil
	}

	for _, d := range diag {
		sugar.Warnf("schema-any [%s] %s: %s", project, d.Location, d.Reason)
	}

	if mode == "error" {
		return fmt.Errorf(
			"project %q: %d schema(s) resolve to Go `any` (GOLANG_SCHEMA_ANY=error); "+
				"fix the schemas or set the flag to warn/silent",
			project, len(diag),
		)
	}

	return nil
}
