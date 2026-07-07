package parser

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validGlobalConfig — эталонный глобальный конфиг со всеми 4 флагами.
// Используется в большинстве тестов как базовая фикстура.
const validGlobalConfig = `- name: GOLANG_SERVER_BODY_REQUEST_NO_AUTO_DEFAULTS
  description: "Don't auto-fill defaults in server request binding"
  enabled: true
  defaultValue: false
  targetValue: true
  affects: [golang]

- name: GOLANG_SPLIT_REQUEST_RESPONSE
  description: "Separate Request/Response models"
  enabled: true
  defaultValue: false
  targetValue: true
  affects: [golang]

- name: USE_REQUIRED_V2
  description: "Support x-request-required/x-response-required"
  enabled: true
  defaultValue: false
  targetValue: true
  dependsOn:
    GOLANG_SPLIT_REQUEST_RESPONSE: true
  affects: [golang]

- name: USE_UTC_FOR_DATE_TIME
  description: "Use UTC for date-time fields"
  enabled: false
  defaultValue: false
  targetValue: false
  affects: [golang]
`

func newTestFS(files map[string]string) fstest.MapFS {
	m := make(fstest.MapFS, len(files))
	for path, content := range files {
		m[path] = &fstest.MapFile{Data: []byte(content)}
	}

	return m
}

func TestGenerationFlagsLoader_Load_Success(t *testing.T) {
	fsys := newTestFS(map[string]string{"generation_flags.yaml": validGlobalConfig})

	loader := NewGenerationFlagsLoader(fsys)
	require.NoError(t, loader.Load("generation_flags.yaml"))

	assert.Len(t, loader.gfConfigs, 4)
	assert.Contains(t, loader.gfConfigs, FlagServerNoAutoDefaults)
	assert.Contains(t, loader.gfConfigs, FlagSplitRequestResponse)
	assert.Contains(t, loader.gfConfigs, FlagUseRequiredV2)
	assert.Contains(t, loader.gfConfigs, FlagUseUTCForDateTime)
}

func TestGenerationFlagsLoader_Load_MissingFlag(t *testing.T) {
	config := `- name: GOLANG_SERVER_BODY_REQUEST_NO_AUTO_DEFAULTS
  enabled: true
  defaultValue: false
  targetValue: true
  affects: [golang]
`
	fsys := newTestFS(map[string]string{"generation_flags.yaml": config})

	loader := NewGenerationFlagsLoader(fsys)
	err := loader.Load("generation_flags.yaml")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "GOLANG_SPLIT_REQUEST_RESPONSE")
}

func TestGenerationFlagsLoader_Load_MissingGolangInAffects(t *testing.T) {
	config := `- name: GOLANG_SERVER_BODY_REQUEST_NO_AUTO_DEFAULTS
  enabled: true
  defaultValue: false
  targetValue: true
  affects: [kotlin]

- name: GOLANG_SPLIT_REQUEST_RESPONSE
  enabled: true
  defaultValue: false
  targetValue: true
  affects: [golang]

- name: USE_REQUIRED_V2
  enabled: true
  defaultValue: false
  targetValue: true
  affects: [golang]

- name: USE_UTC_FOR_DATE_TIME
  enabled: false
  defaultValue: false
  targetValue: false
  affects: [golang]
`
	fsys := newTestFS(map[string]string{"generation_flags.yaml": config})

	loader := NewGenerationFlagsLoader(fsys)
	err := loader.Load("generation_flags.yaml")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "affects")
}

func TestGenerationFlagsLoader_Load_DefaultEqualsTargetWithDependsOn(t *testing.T) {
	config := `- name: GOLANG_SERVER_BODY_REQUEST_NO_AUTO_DEFAULTS
  enabled: true
  defaultValue: false
  targetValue: false
  dependsOn:
    GOLANG_SPLIT_REQUEST_RESPONSE: true
  affects: [golang]

- name: GOLANG_SPLIT_REQUEST_RESPONSE
  enabled: true
  defaultValue: false
  targetValue: true
  affects: [golang]

- name: USE_REQUIRED_V2
  enabled: true
  defaultValue: false
  targetValue: true
  affects: [golang]

- name: USE_UTC_FOR_DATE_TIME
  enabled: false
  defaultValue: false
  targetValue: false
  affects: [golang]
`
	fsys := newTestFS(map[string]string{"generation_flags.yaml": config})

	loader := NewGenerationFlagsLoader(fsys)
	err := loader.Load("generation_flags.yaml")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not contain dependsOn")
}

func TestGenerationFlagsLoader_GetProjectFeatures_NoOverride(t *testing.T) {
	fsys := newTestFS(map[string]string{
		"generation_flags.yaml": validGlobalConfig,
	})

	loader := NewGenerationFlagsLoader(fsys)
	require.NoError(t, loader.Load("generation_flags.yaml"))

	features, err := loader.GetProjectFeatures("project_flags.yaml")
	require.NoError(t, err)

	assert.False(t, features.ServerNoAutoDefaults.Value)
	assert.False(t, features.SplitRequestResponse.Value)
	assert.False(t, features.UseRequiredV2.Value)
	assert.False(t, features.UseUTCForDateTime.Value)
}

func TestGenerationFlagsLoader_GetProjectFeatures_WithOverride(t *testing.T) {
	fsys := newTestFS(map[string]string{
		"generation_flags.yaml": validGlobalConfig,
		"project_flags.yaml": `GOLANG_SERVER_BODY_REQUEST_NO_AUTO_DEFAULTS: true
GOLANG_SPLIT_REQUEST_RESPONSE: true
USE_REQUIRED_V2: true
`,
	})

	loader := NewGenerationFlagsLoader(fsys)
	require.NoError(t, loader.Load("generation_flags.yaml"))

	features, err := loader.GetProjectFeatures("project_flags.yaml")
	require.NoError(t, err)

	assert.True(t, features.ServerNoAutoDefaults.Value)
	assert.True(t, features.SplitRequestResponse.Value)
	assert.True(t, features.UseRequiredV2.Value)
	// USE_UTC_FOR_DATE_TIME не в override → default false
	assert.False(t, features.UseUTCForDateTime.Value)
}

func TestGenerationFlagsLoader_GetProjectFeatures_DisabledFlagNonDefaultOverride(t *testing.T) {
	fsys := newTestFS(map[string]string{
		"generation_flags.yaml": validGlobalConfig,
		// USE_UTC_FOR_DATE_TIME disabled в конфиге, default false
		// пробуем поставить true → должна быть ошибка
		"project_flags.yaml": "USE_UTC_FOR_DATE_TIME: true\n",
	})

	loader := NewGenerationFlagsLoader(fsys)
	require.NoError(t, loader.Load("generation_flags.yaml"))

	_, err := loader.GetProjectFeatures("project_flags.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "USE_UTC_FOR_DATE_TIME")
	assert.Contains(t, err.Error(), "disabled")
}

func TestGenerationFlagsLoader_GetProjectFeatures_DependencyMissing(t *testing.T) {
	fsys := newTestFS(map[string]string{
		"generation_flags.yaml": validGlobalConfig,
		// USE_REQUIRED_V2 зависит от GOLANG_SPLIT_REQUEST_RESPONSE=true,
		// но в override его нет → ошибка
		"project_flags.yaml": "USE_REQUIRED_V2: true\n",
	})

	loader := NewGenerationFlagsLoader(fsys)
	require.NoError(t, loader.Load("generation_flags.yaml"))

	_, err := loader.GetProjectFeatures("project_flags.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dependency")
	assert.Contains(t, err.Error(), "GOLANG_SPLIT_REQUEST_RESPONSE")
}

func TestGenerationFlagsLoader_GetProjectFeatures_DependencyWrongValue(t *testing.T) {
	fsys := newTestFS(map[string]string{
		"generation_flags.yaml": validGlobalConfig,
		// USE_REQUIRED_V2 зависит от GOLANG_SPLIT_REQUEST_RESPONSE=true,
		// но в override ставим false → ошибка
		"project_flags.yaml": `USE_REQUIRED_V2: true
GOLANG_SPLIT_REQUEST_RESPONSE: false
`,
	})

	loader := NewGenerationFlagsLoader(fsys)
	require.NoError(t, loader.Load("generation_flags.yaml"))

	_, err := loader.GetProjectFeatures("project_flags.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dependency")
}

func TestGenerationFlagsLoader_GetProjectFeatures_LoadNotCalled(t *testing.T) {
	fsys := newTestFS(map[string]string{})
	loader := NewGenerationFlagsLoader(fsys)

	_, err := loader.GetProjectFeatures("project_flags.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Load must be called")
}

func TestGenerationFlagsLoader_GetProjectFeatures_EmptyProjectFile(t *testing.T) {
	fsys := newTestFS(map[string]string{
		"generation_flags.yaml": validGlobalConfig,
		"project_flags.yaml":    "",
	})

	loader := NewGenerationFlagsLoader(fsys)
	require.NoError(t, loader.Load("generation_flags.yaml"))

	features, err := loader.GetProjectFeatures("project_flags.yaml")
	require.NoError(t, err)

	// Пустой файл = нет override → defaults
	assert.False(t, features.ServerNoAutoDefaults.Value)
	assert.False(t, features.SplitRequestResponse.Value)
}

func TestGenerationFlagsLoader_Load_InvalidYAML(t *testing.T) {
	fsys := newTestFS(map[string]string{
		"generation_flags.yaml": "{{{{invalid yaml",
	})

	loader := NewGenerationFlagsLoader(fsys)
	err := loader.Load("generation_flags.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode")
}

func TestGenerationFlagsLoader_Load_FileNotFound(t *testing.T) {
	fsys := newTestFS(map[string]string{})

	loader := NewGenerationFlagsLoader(fsys)
	err := loader.Load("nonexistent.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read")
}
