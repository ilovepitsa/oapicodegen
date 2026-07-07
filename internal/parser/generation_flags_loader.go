package parser

import (
	"errors"
	"fmt"
	"io/fs"
	"slices"

	"gopkg.in/yaml.v3"
	realfs "nschugorev/oapigenerator/internal/fs"
)

// GenerationFlagsLoader грузит глобальный конфиг флагов и per-project override,
// валидирует правила и резолвит финальные значения ProjectFeatures.
//
// Двухфазный:
//  1. Load(source) — парсит глобальный generation_flags.yaml, валидирует наличие
//     всех поддерживаемых флагов и корректность их конфигов.
//  2. GetProjectFeatures(projectPath) — парсит override проекта (map flag→bool),
//     резолвит каждый флаг против глобального конфига.
type GenerationFlagsLoader struct {
	fsys realfs.ReadOnlyFS

	gfConfigs map[string]GenerationFlagConfig
}

// NewGenerationFlagsLoader создаёт loader поверх readonly FS.
func NewGenerationFlagsLoader(fsys realfs.ReadOnlyFS) *GenerationFlagsLoader {
	return &GenerationFlagsLoader{fsys: fsys}
}

// Load парсит глобальный generation_flags.yaml и валидирует конфиги.
// Должен быть вызван до GetProjectFeatures.
func (l *GenerationFlagsLoader) Load(source string) error {
	b, err := realfs.ReadFile(l.fsys, source)
	if err != nil {
		return fmt.Errorf("read generation flags config %q: %w", source, err)
	}

	var flags []GenerationFlagConfig
	if err := yaml.Unmarshal(b, &flags); err != nil {
		return fmt.Errorf("decode generation flags config: %w", err)
	}

	l.gfConfigs = make(map[string]GenerationFlagConfig, len(flags))
	for _, f := range flags {
		l.gfConfigs[f.Name] = f
	}

	for _, name := range supportedFlags {
		if err := l.validateConfig(name); err != nil {
			return fmt.Errorf("validate flag %q: %w", name, err)
		}
	}

	return nil
}

// GetProjectFeatures резолвит финальные значения флагов для проекта.
// projectPath — путь к per-project override (map flag→bool). Файл может
// отсутствовать — тогда используются defaultValues из глобального конфига.
func (l *GenerationFlagsLoader) GetProjectFeatures(projectPath string) (ProjectFeatures, error) {
	if l.gfConfigs == nil {
		return ProjectFeatures{}, errors.New("Load must be called before GetProjectFeatures")
	}

	projectFlags, err := l.loadProjectFlags(projectPath)
	if err != nil {
		return ProjectFeatures{}, err
	}

	features := ProjectFeatures{}
	setters := map[string]func(ProjectFeature){
		FlagServerNoAutoDefaults: func(f ProjectFeature) { features.ServerNoAutoDefaults = f },
		FlagSplitRequestResponse: func(f ProjectFeature) { features.SplitRequestResponse = f },
		FlagUseRequiredV2:        func(f ProjectFeature) { features.UseRequiredV2 = f },
		FlagUseUTCForDateTime:    func(f ProjectFeature) { features.UseUTCForDateTime = f },
	}

	for _, name := range supportedFlags {
		feature, err := l.resolveFlag(projectFlags, name)
		if err != nil {
			return ProjectFeatures{}, fmt.Errorf("resolve flag %q: %w", name, err)
		}

		setters[name](feature)
	}

	return features, nil
}

func (l *GenerationFlagsLoader) validateConfig(name string) error {
	config, ok := l.gfConfigs[name]
	if !ok {
		return errors.New("generation flag not found in config")
	}

	if !slices.Contains(config.Affects, "golang") {
		return errors.New("'golang' is not found in the affects list")
	}

	if config.DefaultValue == config.TargetValue && len(config.DependsOn) != 0 {
		return errors.New("flag with same default and target values must not contain dependsOn")
	}

	return nil
}

func (l *GenerationFlagsLoader) resolveFlag(
	projectFlags map[string]bool,
	name string,
) (ProjectFeature, error) {
	config := l.gfConfigs[name]

	override, hasOverride := projectFlags[name]
	if !hasOverride {
		return ProjectFeature{Value: config.DefaultValue}, nil
	}

	value, err := validateOverride(config, override, projectFlags)
	if err != nil {
		return ProjectFeature{}, err
	}

	return ProjectFeature{Value: value}, nil
}

func validateOverride(
	config GenerationFlagConfig,
	value bool,
	projectFlags map[string]bool,
) (bool, error) {
	if !config.Enabled {
		if value != config.DefaultValue {
			return false, errors.New("flag is disabled, only default value is allowed")
		}

		return value, nil
	}

	if value == config.DefaultValue {
		return value, nil
	}

	for depName, depExpected := range config.DependsOn {
		depActual, ok := projectFlags[depName]
		if !ok {
			return false, fmt.Errorf("dependency %q is required but not found", depName)
		}

		if depActual != depExpected {
			return false, fmt.Errorf(
				"dependency %q: expected %v, got %v", depName, depExpected, depActual,
			)
		}
	}

	return value, nil
}

func (l *GenerationFlagsLoader) loadProjectFlags(path string) (map[string]bool, error) {
	if path == "" {
		return map[string]bool{}, nil
	}

	file, err := l.fsys.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]bool{}, nil
		}

		return nil, fmt.Errorf("open project generation flags %q: %w", path, err)
	}

	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat project generation flags: %w", err)
	}

	if info.Size() == 0 {
		return map[string]bool{}, nil
	}

	var flags map[string]bool
	if err := yaml.NewDecoder(file).Decode(&flags); err != nil {
		return nil, fmt.Errorf("decode project generation flags: %w", err)
	}

	return flags, nil
}
