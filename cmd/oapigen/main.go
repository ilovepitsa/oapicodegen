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
	"nschugorev/oapigenerator/internal/cli/logging"
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/configurator"
	"nschugorev/oapigenerator/internal/fs"
	"nschugorev/oapigenerator/internal/generator"
	"nschugorev/oapigenerator/internal/parser"
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

	logCfg := logging.NewLoggerConfiguratorFromFlags(flagSet)
	fwCfg := configurator.NewFileWriterConfiguratorFromFlags(flagSet)

	if err := flagSet.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
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

	for _, project := range ps.Projects {
		projectFW := codegen.WithPath(fw, project.Folder)
		if err := generator.Generate(projectFW, project, si); err != nil {
			return fmt.Errorf("generate project %q: %w", project.Folder, err)
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
