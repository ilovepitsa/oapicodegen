package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestService_LowerName(t *testing.T) {
	s := &Service{Name: "UserBackend"}
	assert.Equal(t, "userbackend", s.LowerName())
}

func TestService_Empty(t *testing.T) {
	s := &Service{}
	assert.Equal(t, "", s.LowerName())
}
