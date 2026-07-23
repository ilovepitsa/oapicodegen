package operations

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/generator/render"
)

// MockClientRenderer рендерит impl/mocks/client/mocks.gen.go.
// Заменяет Generator.mockClientFile (internal/generator/mocks.go).
type MockClientRenderer struct{}

func NewMockClientRenderer() *MockClientRenderer { return &MockClientRenderer{} }

func (MockClientRenderer) FilePath() string    { return "impl/mocks/client/mocks.gen.go" }
func (MockClientRenderer) PackageName() string { return "mock_client" }

func (r *MockClientRenderer) Render(ctx *render.RenderContext) ([]byte, *render.ImportTracker, error) {
	return renderMockFile(ctx, "client", "MockClient", "apiclient.Client")
}

// MockServerRenderer рендерит impl/mocks/server/mocks.gen.go.
// Заменяет Generator.mockServerFile (internal/generator/mocks.go).
type MockServerRenderer struct{}

func NewMockServerRenderer() *MockServerRenderer { return &MockServerRenderer{} }

func (MockServerRenderer) FilePath() string    { return "impl/mocks/server/mocks.gen.go" }
func (MockServerRenderer) PackageName() string { return "mock_server" }

func (r *MockServerRenderer) Render(ctx *render.RenderContext) ([]byte, *render.ImportTracker, error) {
	return renderMockFile(ctx, "server", "MockServer", "apiserver.Server")
}

func renderMockFile(ctx *render.RenderContext, side, mockName, ifaceRef string) ([]byte, *render.ImportTracker, error) {
	ops := allOperations(ctx.Project)
	if len(ops) == 0 {
		return nil, nil, nil
	}

	imps := render.NewImportTracker()
	ctx.Imports = imps

	imps.Add(gogen.Import{Path: "context"})
	imps.Add(gogen.Import{Path: "reflect"})
	imps.Add(gogen.Import{Path: "go.uber.org/mock/gomock"})

	if ctx.Project != nil && ctx.Project.Paths != nil {
		imps.Add(gogen.Import{Path: ctx.Project.Paths.Imports.ClientInterfaces.Path, Alias: "apiclient"})
		if side == "server" {
			imps.Add(gogen.Import{Path: ctx.Project.Paths.Imports.ServerInterfaces.Path, Alias: "apiserver"})
		}
	}

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

	for _, op := range ops {
		methodName := operationMethodName(op)
		renderGomockMethod(w, mockName, recorderName, methodName)
	}

	return w.Content(), imps, nil
}

func renderGomockMethod(w *codegen.BufferWriter, mockName, recorderName, methodName string) {
	w.Print("func (m *", mockName, ") ", methodName, "(arg0 context.Context, arg1 *apiclient.", methodName, "Request) ")
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

var _ render.SingletonRenderer = (*MockClientRenderer)(nil)
var _ render.SingletonRenderer = (*MockServerRenderer)(nil)
