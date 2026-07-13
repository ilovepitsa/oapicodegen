// Package main implements the oapigen CLI entry point.
//
// Связывает parser → generator и пишет Go-пакеты в output-каталог.
//
//	usage: oapigen -input spec.yaml -output ./gen -import-prefix github.com/foo/bar/gen
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
	"path/filepath"
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
		generationFlagsConfig string
		projectFlagsPath      string
	)

	flagSet.StringVar(&input, "input", "", "path to OpenAPI 3.x spec file")
	flagSet.StringVar(&output, "output", "", "output directory for generated Go packages")
	flagSet.StringVar(&importPrefix, "import-prefix", "",
		"Go import path prefix for generated packages")
	flagSet.BoolVar(&dryRun, "dry-run", false, "parse and generate without writing to filesystem")
	flagSet.StringVar(
		&generationFlagsConfig, "generation-flags-config-path", "",
		"path to global generation_flags.yaml",
	)
	flagSet.StringVar(
		&projectFlagsPath, "project-flags-path", "",
		"path to per-project generation flags override",
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

	if projectFlagsPath != "" && generationFlagsConfig == "" {
		return errors.New("-project-flags-path requires -generation-flags-config-path")
	}

	logger, err := logCfg.Create()
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}

	defer func() { _ = logger.Sync() }() //nolint:errcheck // zap.Sync often fails on stderr/stdout

	sugar := logger.Sugar()

	// ParseFile (а не os.ReadFile + Parse) прокидывает os.DirFS в libopenpi,
	// чтобы резолвить cross-file $ref из каталога спеки.
	doc, err := parser.ParseFile(os.DirFS(filepath.Dir(input)), filepath.Base(input))
	if err != nil {
		return fmt.Errorf("parse spec %q: %w", input, err)
	}

	sugar.Infof("parsed spec: %d schemas, %d operations", len(doc.Schemas), len(doc.Operations))

	var fw codegen.FileWriter
	if dryRun {
		fw = codegen.NoopFileWriter{}

		sugar.Info("dry-run mode: no files will be written")
	} else {
		fw, err = fwCfg.Create(output)
		if err != nil {
			return fmt.Errorf("create file writer: %w", err)
		}
	}

	defer func() {
		if cerr := fw.Close(); cerr != nil {
			sugar.Errorf("close file writer: %v", cerr)
		}
	}()

	genOpts := []generator.Option{generator.WithModulePath(importPrefix)}

	if generationFlagsConfig != "" {
		pf, err := loadProjectFeatures(generationFlagsConfig, projectFlagsPath)
		if err != nil {
			return fmt.Errorf("load generation flags: %w", err)
		}

		genOpts = append(genOpts, generator.WithProjectFeatures(pf))
		sugar.Infof(
			"generation flags: no-auto-defaults=%v split=%v required-v2=%v utc=%v",
			pf.ServerNoAutoDefaults.Value, pf.SplitRequestResponse.Value,
			pf.UseRequiredV2.Value, pf.UseUTCForDateTime.Value,
		)
	}

	if err := generator.Generate(fw, doc, genOpts...); err != nil {
		return fmt.Errorf("generate: %w", err)
	}

	sugar.Infof("generation complete: output=%s import-prefix=%s", output, importPrefix)

	return nil
}

func loadProjectFeatures(configPath, projectFlagsPath string) (parser.ProjectFeatures, error) {
	loader := parser.NewGenerationFlagsLoader(fs.NewRealFS())
	if err := loader.Load(configPath); err != nil {
		return parser.ProjectFeatures{}, fmt.Errorf(
			"load generation flags config %q: %w", configPath, err,
		)
	}

	pf, err := loader.GetProjectFeatures(projectFlagsPath)
	if err != nil {
		return parser.ProjectFeatures{}, fmt.Errorf("resolve project features: %w", err)
	}

	return pf, nil
}
