package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nschugorev/oapigenerator/internal/generator/render"
)

func TestUTCTimeRenderer_FilePath(t *testing.T) {
	t.Parallel()

	r := NewUTCTimeRenderer()
	assert.Equal(t, "model/utc_time.gen.go", r.FilePath())
}

func TestUTCTimeRenderer_RenderBodyAndImports(t *testing.T) {
	t.Parallel()

	r := NewUTCTimeRenderer()
	ctx := &render.RenderContext{}

	body, imps, err := r.Render(ctx)
	require.NoError(t, err)

	got := string(body)
	assert.Contains(t, got, "type UTCTime time.Time")
	assert.Contains(t, got, "func (u UTCTime) MarshalJSON() ([]byte, error) {")
	assert.Contains(t, got, "func (u *UTCTime) UnmarshalJSON(data []byte) error {")
	assert.Contains(t, got, "return json.Marshal(time.Time(u).UTC())")
	assert.Contains(t, got, "*u = UTCTime(t.UTC())")

	paths := importPaths(imps)
	assert.Contains(t, paths, "encoding/json")
	assert.Contains(t, paths, "time")
}

func TestUTCTimeRenderer_RenderIsDeterministic(t *testing.T) {
	t.Parallel()

	r := NewUTCTimeRenderer()
	ctx := &render.RenderContext{}

	body1, _, err := r.Render(ctx)
	require.NoError(t, err)

	body2, _, err := r.Render(ctx)
	require.NoError(t, err)

	assert.Equal(t, body1, body2)
}

// importPaths extracts .Path from each import for easy assertion.
func importPaths(imps *render.ImportTracker) []string {
	out := make([]string, 0, len(imps.Imports()))
	for _, imp := range imps.Imports() {
		out = append(out, imp.Path)
	}

	return out
}
