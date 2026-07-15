package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/fs"
)

// serviceDescriptor описывает один обнаруженный сервис во входном каталоге.
type serviceDescriptor struct {
	Folder    string // относительный путь от input ("userBackend", "common")
	SpecPath  string // абсолютный путь к src/openapi/openapi.yaml
	FlagsPath string // абсолютный путь к generation_flags.yaml (может быть пустым)
}

// walkServices обходит input-каталог и находит все сервисы по наличию
// src/openapi/openapi.yaml. Возвращает дескрипторы, отсортированные по Folder
// для детерминированной обработки. Каталог "common" (если есть) попадает в
// общий список — ProjectLoader.Load выделит его отдельно.
func walkServices(input string) ([]serviceDescriptor, error) {
	entries, err := os.ReadDir(input)
	if err != nil {
		return nil, fmt.Errorf("read input dir %q: %w", input, err)
	}

	var out []serviceDescriptor
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		folder := entry.Name()
		specPath := filepath.Join(input, folder, "src", "openapi", "openapi.yaml")
		if _, err := os.Stat(specPath); err != nil {
			continue // не сервис — пропускаем
		}
		desc := serviceDescriptor{
			Folder:   folder,
			SpecPath: specPath,
		}
		flagsPath := filepath.Join(input, folder, "generation_flags.yaml")
		if _, err := os.Stat(flagsPath); err == nil {
			desc.FlagsPath = flagsPath
		}
		out = append(out, desc)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Folder < out[j].Folder
	})
	return out, nil
}

// ProjectLoader собирает ProjectSet из входного каталога: обнаруживает
// сервисы, парсит спеки, загружает generation flags, переносит схемы и
// операции в Model/Paths.
type ProjectLoader struct{}

// NewProjectLoader создаёт ProjectLoader. Stateless — все параметры
// передаются в Load.
func NewProjectLoader() *ProjectLoader {
	return &ProjectLoader{}
}

// Load собирает ProjectSet из input-каталога.
//
// input — абсолютный или относительный путь к корню проекта (содержит
// подпапки сервисов: common/, userBackend/, ...).
// flagsLoader — загрузчик generation flags (может быть nil — используются
// дефолтные значения для всех проектов).
// importPrefix — Go import path корня (например "nschugorev/oapigenerator/go").
// output — абсолютный путь к output-каталогу.
//
// Возвращает ProjectSet (с заполненными Projects, Common, ByName) и
// SchemaIndex (пустой — заполняется в T26.3 после source-marking).
func (pl *ProjectLoader) Load(
	input string,
	flagsLoader *GenerationFlagsLoader,
	importPrefix, output string,
) (*ProjectSet, *SchemaIndex, error) {
	descs, err := walkServices(input)
	if err != nil {
		return nil, nil, fmt.Errorf("walk services: %w", err)
	}

	ps := &ProjectSet{
		ByName: map[string]*Project{},
	}
	si := &SchemaIndex{Schemas: map[string]*SchemaEntry{}}

	for _, desc := range descs {
		project, err := pl.loadProject(desc, flagsLoader, importPrefix, output)
		if err != nil {
			return nil, nil, fmt.Errorf("load project %q: %w", desc.Folder, err)
		}

		if desc.Folder == "common" {
			ps.Common = project
			project.Model.Prefix = "common"
		}
		ps.Projects = append(ps.Projects, project)
		ps.ByName[project.Folder] = project
	}

	return ps, si, nil
}

// loadProject парсит один сервис и строит Project с Model и Paths.
func (pl *ProjectLoader) loadProject(
	desc serviceDescriptor,
	flagsLoader *GenerationFlagsLoader,
	importPrefix, output string,
) (*Project, error) {
	fsys := fs.NewRealFS()
	doc, err := ParseFile(fsys, desc.SpecPath)
	if err != nil {
		return nil, fmt.Errorf("parse spec: %w", err)
	}

	features, err := pl.resolveFeatures(flagsLoader, desc.FlagsPath)
	if err != nil {
		return nil, fmt.Errorf("resolve features: %w", err)
	}

	folder := desc.Folder
	project := &Project{
		Folder:       folder,
		SpecPath:     desc.SpecPath,
		FlagsPath:    desc.FlagsPath,
		Features:     features,
		OutputDir:    filepath.Join(output, folder),
		ImportPrefix: filepath.Join(importPrefix, folder),
	}

	modelImp := gogen.Import{
		Path:    project.ImportPrefix + "/model",
		Alias:   "model",
		Package: "",
	}
	project.CreateModel(modelImp)
	project.CreatePaths(project.ImportPrefix)

	// Перенос схем
	project.Model.schemas = doc.Schemas
	project.Model.Index()

	// Перенос операций
	for _, op := range doc.Operations {
		svcName, err := serviceName(op.Method, op.Path, op.Tags)
		if err != nil {
			return nil, fmt.Errorf("service name for %s %s: %w", op.Method, op.Path, err)
		}
		project.Paths.AddMethod(svcName, op)
	}

	return project, nil
}

// resolveFeatures возвращает ProjectFeatures для сервиса. Если flagsLoader
// nil или FlagsPath пустой — возвращает дефолтные значения.
func (pl *ProjectLoader) resolveFeatures(
	flagsLoader *GenerationFlagsLoader,
	flagsPath string,
) (ProjectFeatures, error) {
	if flagsLoader == nil || flagsPath == "" {
		return ProjectFeatures{}, nil
	}
	return flagsLoader.GetProjectFeatures(flagsPath)
}
