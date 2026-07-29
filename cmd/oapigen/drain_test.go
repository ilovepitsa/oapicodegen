package main

import (
	"testing"

	"github.com/ilovepitsa/oapicodegen/internal/generator/render"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDrainDiagnostics_SilentNoOp(t *testing.T) {
	t.Parallel()

	col := render.NewCollector()
	col.Append(render.Diagnostic{Location: "x", Reason: "r"})
	err := drainDiagnostics(col, "p", "silent", zap.NewNop().Sugar())
	require.NoError(t, err)
}

func TestDrainDiagnostics_WarnReturnsNil(t *testing.T) {
	t.Parallel()

	col := render.NewCollector()
	col.Append(render.Diagnostic{Location: "x", Reason: "r"})
	err := drainDiagnostics(col, "p", "warn", zap.NewNop().Sugar())
	require.NoError(t, err)
}

func TestDrainDiagnostics_ErrorReturnsError(t *testing.T) {
	t.Parallel()

	col := render.NewCollector()
	col.Append(render.Diagnostic{Location: "x", Reason: "r"})
	err := drainDiagnostics(col, "p", "error", zap.NewNop().Sugar())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GOLANG_SCHEMA_ANY=error")
}

func TestDrainDiagnostics_EmptyNoOp(t *testing.T) {
	t.Parallel()

	err := drainDiagnostics(render.NewCollector(), "p", "error", zap.NewNop().Sugar())
	require.NoError(t, err)
}
