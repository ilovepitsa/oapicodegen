package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersion_SetGetRoundTrip(t *testing.T) {
	t.Parallel()

	original := Version
	t.Cleanup(func() { Version = original })

	Set("v9.9.9")
	assert.Equal(t, "v9.9.9", Get())
	assert.Equal(t, "v9.9.9", Version)
}

func TestVersion_GetReturnsCurrent(t *testing.T) {
	t.Parallel()

	original := Version
	t.Cleanup(func() { Version = original })

	Set("v1.2.3")
	assert.Equal(t, "v1.2.3", Get())
}
