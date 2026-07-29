package parser

import (
	"errors"
	"fmt"
	"github.com/ilovepitsa/oapicodegen/internal/genflags"
	"io/fs"

	realfs "github.com/ilovepitsa/oapicodegen/internal/fs"

	"gopkg.in/yaml.v3"
)

// GenerationFlagsLoader грузит глобальный конфиг флагов и per-project override,
// валидирует правила и резолвит финальные значения ProjectFeatures.
//
// Двухфазный:
//  1. Load(source) — парсит глобальный generation_flags.yaml, валидирует наличие
//     всех поддерживаемых флагов и корректность их конфигов через genflags.Registry.
//  2. GetProjectFeatures(projectPath) — парсит override проекта (map flag→bool),
//     резолвит каждый флаг через registry.
type GenerationFlagsLoader struct {
	fsys     realfs.ReadOnlyFS
	registry *genflags.Registry

	gfConfigs       map[string]GenerationFlagConfig
	projectFlagsAny map[string]any
}

// NewGenerationFlagsLoader создаёт loader поверх readonly FS с стандартным
// набором зарегистрированных флагов.
func NewGenerationFlagsLoader(fsys realfs.ReadOnlyFS) *GenerationFlagsLoader {
	return &GenerationFlagsLoader{
		fsys:     fsys,
		registry: newDefaultRegistry(),
	}
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

	for _, name := range l.registry.Names() {
		cfg, ok := l.gfConfigs[name]
		if !ok {
			return fmt.Errorf("validate flag %q: generation flag not found in config", name)
		}

		if err := l.registry.ValidateConfig(name, cfg); err != nil {
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
	setters := map[string]func(ProjectFeature, SchemaAnyFeature){
		FlagServerNoAutoDefaults: func(f ProjectFeature, _ SchemaAnyFeature) { features.ServerNoAutoDefaults = f },
		FlagSplitRequestResponse: func(f ProjectFeature, _ SchemaAnyFeature) { features.SplitRequestResponse = f },
		FlagUseRequiredV2:        func(f ProjectFeature, _ SchemaAnyFeature) { features.UseRequiredV2 = f },
		FlagUseUTCForDateTime:    func(f ProjectFeature, _ SchemaAnyFeature) { features.UseUTCForDateTime = f },
		FlagUseOptional:          func(f ProjectFeature, _ SchemaAnyFeature) { features.UseOptional = f },
		FlagSchemaAny:            func(_ ProjectFeature, e SchemaAnyFeature) { features.SchemaAny = e },
	}

	for _, name := range l.registry.Names() {
		pf, ef, err := l.resolveFlag(projectFlags, name)
		if err != nil {
			return ProjectFeatures{}, err
		}

		setters[name](pf, ef)
	}

	return features, nil
}

func (l *GenerationFlagsLoader) resolveFlag(
	projectFlags map[string]bool,
	name string,
) (ProjectFeature, SchemaAnyFeature, error) {
	cfg := l.gfConfigs[name]

	var override any
	if v, has := l.projectFlagsAny[name]; has {
		override = v
	}

	value, err := l.registry.Resolve(name, override, projectFlags, cfg)
	if err != nil {
		return ProjectFeature{}, SchemaAnyFeature{}, fmt.Errorf("resolve flag %q: %w", name, err)
	}

	switch v := value.(type) {
	case bool:
		return ProjectFeature{Value: v}, SchemaAnyFeature{}, nil
	case string:
		return ProjectFeature{}, SchemaAnyFeature{Mode: v}, nil
	default:
		return ProjectFeature{}, SchemaAnyFeature{}, fmt.Errorf("resolve flag %q: unexpected type %T", name, value)
	}
}

// loadProjectFlags читает per-project override как map[string]any, сохраняет
// результат в l.projectFlagsAny и возвращает bool-only подмножество для
// использования в роли `resolved` (DependsOn ссылается только на bool-флаги).
func (l *GenerationFlagsLoader) loadProjectFlags(path string) (map[string]bool, error) {
	anyMap, err := l.loadProjectFlagsAny(path)
	if err != nil {
		return nil, err
	}

	l.projectFlagsAny = anyMap

	boolMap := make(map[string]bool, len(anyMap))
	for name, v := range anyMap {
		b, ok := v.(bool)
		if !ok {
			// Не-bool значения (enum-override) пропускаем: они разрешаются
			// отдельной веткой в resolveFlag и не нужны для DependsOn.
			continue
		}

		boolMap[name] = b
	}

	return boolMap, nil
}

// loadProjectFlagsAny декодирует per-project override в map[string]any.
// Семантика отсутствия файла/пустого файла совпадает с исходной: пустой путь
// или отсутствующий файл → пустой map (без ошибки); пустой файл → пустой map.
func (l *GenerationFlagsLoader) loadProjectFlagsAny(path string) (map[string]any, error) {
	if path == "" {
		return map[string]any{}, nil
	}

	file, err := l.fsys.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]any{}, nil
		}

		return nil, fmt.Errorf("open project generation flags %q: %w", path, err)
	}

	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat project generation flags: %w", err)
	}

	if info.Size() == 0 {
		return map[string]any{}, nil
	}

	var flags map[string]any
	if err := yaml.NewDecoder(file).Decode(&flags); err != nil {
		return nil, fmt.Errorf("decode project generation flags: %w", err)
	}

	return flags, nil
}
