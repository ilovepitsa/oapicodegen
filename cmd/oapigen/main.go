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
	"nschugorev/oapigenerator/internal/generator"
	"nschugorev/oapigenerator/internal/parser"
	"os"
)

func Main() int {
	if err := run(os.Args[1:], os.Stderr); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "oapigen: %v\n", err)

		return 1
	}

	return 0
}

func run(args []string, stderr *os.File) error {
	fs := flag.NewFlagSet("oapigen", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		input        string
		output       string
		importPrefix string
		dryRun       bool
	)

	fs.StringVar(&input, "input", "", "path to OpenAPI 3.x spec file")
	fs.StringVar(&output, "output", "", "output directory for generated Go packages")
	fs.StringVar(&importPrefix, "import-prefix", "", "Go import path prefix for generated packages (e.g. github.com/foo/bar/gen)")
	fs.BoolVar(&dryRun, "dry-run", false, "parse and generate without writing to filesystem")

	logCfg := logging.NewLoggerConfiguratorFromFlags(fs)
	fwCfg := configurator.NewFileWriterConfiguratorFromFlags(fs)

	if err := fs.Parse(args); err != nil {
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

	defer func() { _ = logger.Sync() }() //nolint:errcheck // zap.Sync часто падает на stderr/stdout — намеренно игнорируем

	sugar := logger.Sugar()

	data, err := os.ReadFile(input)
	if err != nil {
		return fmt.Errorf("read spec %q: %w", input, err)
	}

	doc, err := parser.Parse(data)
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

	if err := generator.Generate(fw, doc, generator.WithModulePath(importPrefix)); err != nil {
		return fmt.Errorf("generate: %w", err)
	}

	sugar.Infof("generation complete: output=%s import-prefix=%s", output, importPrefix)

	return nil
}
