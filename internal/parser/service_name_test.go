package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestServiceName_NoTags(t *testing.T) {
	got, err := serviceName("GET", "/users", nil)
	assert.NoError(t, err)
	assert.Equal(t, "Service", got)
}

func TestServiceName_SingleTag(t *testing.T) {
	got, err := serviceName("GET", "/users", []string{"UserBackend"})
	assert.NoError(t, err)
	assert.Equal(t, "UserBackend", got)
}

func TestServiceName_MultipleTags(t *testing.T) {
	_, err := serviceName("GET", "/users", []string{"UserBackend", "Admin"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be exactly one tag")
}

func TestServiceName_EmptyTag(t *testing.T) {
	_, err := serviceName("GET", "/users", []string{""})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tag must not be empty")
}
