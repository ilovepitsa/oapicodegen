package generator

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
)

// mockClientFile генерирует impl/mocks/client/mocks.gen.go:
// gomock-совместимый MockClient, реализующий apiclient.Client.
func (g *Generator) mockClientFile() codegen.File {
	return g.mockFile("client", "MockClient", "apiclient.Client")
}

// mockServerFile генерирует impl/mocks/server/mocks.gen.go:
// gomock-совместимый MockServer, реализующий apiserver.Server.
func (g *Generator) mockServerFile() codegen.File {
	return g.mockFile("server", "MockServer", "apiserver.Server")
}

func (g *Generator) mockFile(side, mockName, ifaceRef string) codegen.File {
	m := g.newTypeMapper("mock" + side)
	m.addImport("context", "")
	m.addImport("reflect", "")
	m.addImport("go.uber.org/mock/gomock", "")

	if g.modulePath != "" {
		m.addImport(g.modulePath+"/interfaces/client", "apiclient")

		if side == "server" {
			m.addImport(g.modulePath+"/interfaces/server", "apiserver")
		}
	}

	pkgName := "mock_client"
	if side == "server" {
		pkgName = "mock_server"
	}

	body := g.renderGomockMock(mockName, ifaceRef)

	return g.factory.Create(&gogen.File{
		Package: pkgName,
		Imports: m.imports,
		Body:    body,
	})
}

func (g *Generator) renderGomockMock(mockName, ifaceRef string) []byte {
	w := codegen.NewBufferWriter()
	recorderName := mockName + "MockRecorder"

	w.Print("var _ ", ifaceRef, " = (*", mockName, ")(nil)\n\n")

	w.Print("type ", mockName, " struct {\n")
	w.Print("\tctrl     *gomock.Controller\n")
	w.Print("\trecorder *", recorderName, "\n")
	w.Print("}\n\n")

	w.Print("type ", recorderName, " struct {\n")
	w.Print("\tmock *", mockName, "\n")
	w.Print("}\n\n")

	w.Print("func New", mockName, "(ctrl *gomock.Controller) *", mockName, " {\n")
	w.Print("\tmock := &", mockName, "{ctrl: ctrl}\n")
	w.Print("\tmock.recorder = &", recorderName, "{mock}\n")
	w.Print("\treturn mock\n")
	w.Print("}\n\n")

	w.Print("func (m *", mockName, ") EXPECT() *", recorderName, " {\n")
	w.Print("\treturn m.recorder\n")
	w.Print("}\n\n")

	for _, op := range g.doc.Operations {
		name := operationMethodName(op)
		g.renderGomockMethod(w, mockName, recorderName, name)
	}

	return w.Content()
}

func (g *Generator) renderGomockMethod(w *codegen.BufferWriter, mockName, recorderName, methodName string) { //nolint:lll // function signature
	w.Print("func (m *", mockName, ") ", methodName, "(arg0 context.Context, arg1 *apiclient.", methodName, "Request) ") //nolint:lll // generated method signature
	w.Print("(*apiclient.", methodName, "Response, error) {\n")
	w.Print("\tm.ctrl.T.Helper()\n")
	w.Print("\tret := m.ctrl.Call(m, \"", methodName, "\", arg0, arg1)\n")
	w.Print("\tret0, _ := ret[0].(*apiclient.", methodName, "Response)\n")
	w.Print("\tret1, _ := ret[1].(error)\n")
	w.Print("\treturn ret0, ret1\n")
	w.Print("}\n\n")

	w.Print("func (mr *", recorderName, ") ", methodName, "(arg0, arg1 any) *gomock.Call {\n")
	w.Print("\tmr.mock.ctrl.T.Helper()\n")
	w.Print("\treturn mr.mock.ctrl.RecordCallWithMethodType(mr.mock, \"", methodName, "\", ")
	w.Print("reflect.TypeOf((*", mockName, ")(nil).", methodName, "), arg0, arg1)\n")
	w.Print("}\n\n")
}
