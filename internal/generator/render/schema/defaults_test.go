// Package schema: tests for treeHasDefaults — pure utility, портированная из
// Generator.schemaTreeHasDefaults. Тестирует поведение без $ref-разрешения
// (Task 1 не требует Model; ref-resolution будет покрыт в Task 2 через
// SetDefaultsRenderer).
package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"nschugorev/oapigenerator/internal/parser"
)

func TestTreeHasDefaults_NoProperties(t *testing.T) {
	t.Parallel()

	s := &parser.Schema{Name: "Empty", Type: "object"}
	assert.False(t, treeHasDefaults(s, nil))
}

func TestTreeHasDefaults_WithDefault(t *testing.T) {
	t.Parallel()

	s := &parser.Schema{
		Name: "Pet",
		Type: "object",
		Properties: []*parser.Property{
			{Name: "Tag", Schema: &parser.Schema{Type: "string", Default: "none"}},
		},
	}
	assert.True(t, treeHasDefaults(s, nil))
}

func TestTreeHasDefaults_NoDefault_False(t *testing.T) {
	t.Parallel()

	s := &parser.Schema{
		Name: "Pet",
		Type: "object",
		Properties: []*parser.Property{
			{Name: "Tag", Schema: &parser.Schema{Type: "string"}},
		},
	}
	assert.False(t, treeHasDefaults(s, nil))
}

func TestTreeHasDefaults_KeepFilter_ExcludesDefault(t *testing.T) {
	t.Parallel()

	s := &parser.Schema{
		Name: "Pet",
		Type: "object",
		Properties: []*parser.Property{
			{Name: "ReadOnly", Schema: &parser.Schema{Type: "string", ReadOnly: true, Default: "ro"}},
		},
	}
	keep := func(p *parser.Property) bool { return !p.Schema.ReadOnly }
	assert.False(t, treeHasDefaults(s, keep))
}

func TestTreeHasDefaults_NilSchema(t *testing.T) {
	t.Parallel()

	assert.False(t, treeHasDefaults(nil, nil))
}
