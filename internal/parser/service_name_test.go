package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceName_NoTags(t *testing.T) {
	got, err := serviceName("GET", "/users", nil)
	require.NoError(t, err)
	assert.Equal(t, "Service", got)
}

func TestServiceName_SingleTag(t *testing.T) {
	got, err := serviceName("GET", "/users", []string{"UserBackend"})
	require.NoError(t, err)
	assert.Equal(t, "UserBackend", got)
}

func TestServiceName_MultipleTags(t *testing.T) {
	_, err := serviceName("GET", "/users", []string{"UserBackend", "Admin"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be exactly one tag")
}

func TestServiceName_EmptyTag(t *testing.T) {
	_, err := serviceName("GET", "/users", []string{""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tag must not be empty")
}
