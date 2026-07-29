package render

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCollector_AppendAndDrain(t *testing.T) {
	t.Parallel()

	c := NewCollector()
	c.Append(Diagnostic{Location: "s.P.tag", Reason: "empty schema"})
	c.Append(Diagnostic{Location: "paths./x", Reason: "unresolved ref"})

	got := c.Drain()
	assert.Len(t, got, 2)
	assert.Equal(t, "s.P.tag", got[0].Location)
	assert.Empty(t, c.Drain(), "drain clears the collector")
}
