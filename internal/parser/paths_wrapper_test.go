package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPaths_ImportsZeroValue(t *testing.T) {
	p := &Paths{}
	assert.Equal(t, "", p.Imports.ClientInterfaces.Path)
}

func TestPaths_ImportsAssigned(t *testing.T) {
	pi := PathImports{}
	pi.ClientHTTP.Path = "nschugorev/oapigenerator/go/svc/impl/httpclient"
	p := &Paths{Imports: pi}
	assert.Equal(t, "nschugorev/oapigenerator/go/svc/impl/httpclient", p.Imports.ClientHTTP.Path)
}

func TestPaths_AddMethod_NewService(t *testing.T) {
	p := &Paths{}
	m1 := &Method{OperationID: "GetUser"}
	m2 := &Method{OperationID: "CreateUser"}

	p.AddMethod("UserBackend", m1)
	p.AddMethod("UserBackend", m2)

	assert.Len(t, p.Services, 1)
	assert.Equal(t, "UserBackend", p.Services[0].Name)
	assert.Len(t, p.Services[0].Methods, 2)
	assert.Same(t, p.Services[0], m1.service, "AddMethod must set method.service back-ref")
}

func TestPaths_AddMethod_MultipleServices(t *testing.T) {
	p := &Paths{}
	p.AddMethod("UserBackend", &Method{OperationID: "GetUser"})
	p.AddMethod("AuthBackend", &Method{OperationID: "Login"})

	assert.Len(t, p.Services, 2)
	assert.Equal(t, "UserBackend", p.Services[0].Name)
	assert.Equal(t, "AuthBackend", p.Services[1].Name)
}

func TestPaths_DeleteService(t *testing.T) {
	p := &Paths{}
	p.AddMethod("UserBackend", &Method{OperationID: "GetUser"})
	p.AddMethod("AuthBackend", &Method{OperationID: "Login"})

	p.DeleteService("UserBackend")

	assert.Len(t, p.Services, 1)
	assert.Equal(t, "AuthBackend", p.Services[0].Name)
	assert.NotContains(t, p.servicesMap, "UserBackend")
}
