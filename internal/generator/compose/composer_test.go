package compose

import (
	"errors"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/generator/walk"
	"nschugorev/oapigenerator/internal/parser"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// schemaRecorder — тестовый SchemaRenderer, embed'ящий render.Base, чтобы
// получать shared Buf через injectableRenderer.Init. На OnStruct пишет в Buf
// маркер "RECORD:<name>", что позволяет проверить, что composer вливает Buf
// до обхода и собирает байты в файл.
//
// Все 13 хуков реализованы явно (без embed walk.NoopSchemaRenderer, т.к. он
// не используется) — только OnStruct пишет, остальные no-op.
type schemaRecorder struct {
	render.Base
	written []string
}

func (r *schemaRecorder) OnStruct(s *parser.Schema) error {
	r.written = append(r.written, s.Name)
	r.Buf.WriteString("RECORD:" + s.Name + "\n")

	return nil
}

func (r *schemaRecorder) OnEnum(*parser.Schema) error                  { return nil }
func (r *schemaRecorder) OnAlias(*parser.Schema) error                 { return nil }
func (r *schemaRecorder) OnArray(*parser.Schema) error                 { return nil }
func (r *schemaRecorder) OnMap(*parser.Schema) error                   { return nil }
func (r *schemaRecorder) OnUnion(*parser.Schema, walk.UnionKind) error { return nil }
func (r *schemaRecorder) OnAllOf(*parser.Schema) error                 { return nil }
func (r *schemaRecorder) OnSplitStruct(*parser.Schema) error           { return nil }

func (r *schemaRecorder) OnStructProperty(*parser.Schema, string, *parser.Schema) error {
	return nil
}

func (r *schemaRecorder) OnArrayItem(*parser.Schema, int, *parser.Schema) error { return nil }
func (r *schemaRecorder) OnMapValue(*parser.Schema, *parser.Schema) error       { return nil }
func (r *schemaRecorder) OnUnionVariant(*parser.Schema, int, *parser.Schema) error {
	return nil
}

func (r *schemaRecorder) OnAllOfMember(*parser.Schema, int, *parser.Schema) error { return nil }

// methodRecorder — аналог schemaRecorder для MethodRenderer. Пишет маркер
// "RECORD:<opID>" в OnMethod.
type methodRecorder struct {
	render.Base
	written []string
}

func (r *methodRecorder) OnMethod(m *parser.Method) error {
	r.written = append(r.written, m.OperationID)
	r.Buf.WriteString("RECORD:" + m.OperationID + "\n")

	return nil
}

func (r *methodRecorder) OnPathParameter(*parser.Method, *parser.Parameter) error  { return nil }
func (r *methodRecorder) OnQueryParameter(*parser.Method, *parser.Parameter) error { return nil }
func (r *methodRecorder) OnHeaderParameter(*parser.Method, *parser.Parameter) error {
	return nil
}
func (r *methodRecorder) OnCookieParameter(*parser.Method, *parser.Parameter) error { return nil }
func (r *methodRecorder) OnRequestBody(*parser.Method, *parser.RequestBody) error   { return nil }
func (r *methodRecorder) OnResponse(*parser.Method, string, *parser.Response) error {
	return nil
}

func (r *methodRecorder) OnResponseHeader(*parser.Method, string, string, *parser.Parameter) error {
	return nil
}

// fakeSingleton — тестовый SingletonRenderer с фиксированными body+imports.
type fakeSingleton struct {
	path string
	body []byte
	imps []gogen.Import
	err  error
}

func (f *fakeSingleton) Render(*render.RenderContext) ([]byte, *render.ImportTracker, error) {
	if f.err != nil {
		return nil, nil, f.err
	}

	tr := render.NewImportTracker()
	for _, imp := range f.imps {
		tr.Add(imp)
	}

	return f.body, tr, nil
}

func (f *fakeSingleton) FilePath() string { return f.path }

func TestComposeSchemaFile_RecordsToBuf(t *testing.T) {
	ff := gogen.NewFileFactory("oapigen-test")
	c := NewFileComposer(ff)

	s := &parser.Schema{Name: "Pet", Type: "object"}
	s.Properties = []*parser.Property{
		{Name: "id", Schema: &parser.Schema{Name: "string", Type: "string"}},
	}
	rec := &schemaRecorder{}
	ctx := &render.RenderContext{}

	file, err := c.ComposeSchemaFile(s, []walk.SchemaRenderer{rec}, ctx)
	require.NoError(t, err)
	require.NotNil(t, file)

	assert.Equal(t, []string{"Pet"}, rec.written)
	assert.Contains(t, string(file.Content()), "RECORD:Pet")
	assert.Contains(t, string(file.Content()), "package model")
}

func TestComposeSchemaFile_WalkerErrorWrapped(t *testing.T) {
	ff := gogen.NewFileFactory("oapigen-test")
	c := NewFileComposer(ff)

	s := &parser.Schema{Name: "Boom", Type: "object"}
	s.Properties = []*parser.Property{
		{Name: "id", Schema: &parser.Schema{Name: "string", Type: "string"}},
	}
	rec := &failingSchemaRenderer{failOn: "Boom"}
	ctx := &render.RenderContext{}

	_, err := c.ComposeSchemaFile(s, []walk.SchemaRenderer{rec}, ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `compose schema "Boom"`)
	assert.Contains(t, err.Error(), "boom: Boom")
}

type failingSchemaRenderer struct {
	render.Base
	failOn string
}

func (f *failingSchemaRenderer) OnStruct(s *parser.Schema) error {
	if s.Name == f.failOn {
		return errors.New("boom: " + s.Name)
	}

	return nil
}

func (f *failingSchemaRenderer) OnEnum(*parser.Schema) error                  { return nil }
func (f *failingSchemaRenderer) OnAlias(*parser.Schema) error                 { return nil }
func (f *failingSchemaRenderer) OnArray(*parser.Schema) error                 { return nil }
func (f *failingSchemaRenderer) OnMap(*parser.Schema) error                   { return nil }
func (f *failingSchemaRenderer) OnUnion(*parser.Schema, walk.UnionKind) error { return nil }
func (f *failingSchemaRenderer) OnAllOf(*parser.Schema) error                 { return nil }
func (f *failingSchemaRenderer) OnSplitStruct(*parser.Schema) error           { return nil }

func (f *failingSchemaRenderer) OnStructProperty(*parser.Schema, string, *parser.Schema) error {
	return nil
}

func (f *failingSchemaRenderer) OnArrayItem(*parser.Schema, int, *parser.Schema) error { return nil }
func (f *failingSchemaRenderer) OnMapValue(*parser.Schema, *parser.Schema) error       { return nil }
func (f *failingSchemaRenderer) OnUnionVariant(*parser.Schema, int, *parser.Schema) error {
	return nil
}

func (f *failingSchemaRenderer) OnAllOfMember(*parser.Schema, int, *parser.Schema) error {
	return nil
}

func TestComposeMethodFile_RecordsEachMethod(t *testing.T) {
	ff := gogen.NewFileFactory("oapigen-test")
	c := NewFileComposer(ff)

	methods := []*parser.Method{
		{Method: "GET", Path: "/pets", OperationID: "ListPets"},
		{Method: "POST", Path: "/pets", OperationID: "CreatePet"},
	}
	rec := &methodRecorder{}
	ctx := &render.RenderContext{}

	file, err := c.ComposeMethodFile("client", methods, []walk.MethodRenderer{rec}, ctx)
	require.NoError(t, err)
	require.NotNil(t, file)

	assert.Equal(t, []string{"ListPets", "CreatePet"}, rec.written)
	assert.Contains(t, string(file.Content()), "RECORD:ListPets")
	assert.Contains(t, string(file.Content()), "RECORD:CreatePet")
	assert.Contains(t, string(file.Content()), "package client")
}

func TestComposeMethodFile_WalkerErrorWrapped(t *testing.T) {
	ff := gogen.NewFileFactory("oapigen-test")
	c := NewFileComposer(ff)

	methods := []*parser.Method{
		{Method: "GET", Path: "/pets", OperationID: "BoomOp"},
	}
	rec := &failingMethodRenderer{failOn: "BoomOp"}
	ctx := &render.RenderContext{}

	_, err := c.ComposeMethodFile("client", methods, []walk.MethodRenderer{rec}, ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `compose method "BoomOp"`)
	assert.Contains(t, err.Error(), "boom: BoomOp")
}

type failingMethodRenderer struct {
	render.Base
	failOn string
}

func (f *failingMethodRenderer) OnMethod(m *parser.Method) error {
	if m.OperationID == f.failOn {
		return errors.New("boom: " + m.OperationID)
	}

	return nil
}

func (f *failingMethodRenderer) OnPathParameter(*parser.Method, *parser.Parameter) error  { return nil }
func (f *failingMethodRenderer) OnQueryParameter(*parser.Method, *parser.Parameter) error { return nil }
func (f *failingMethodRenderer) OnHeaderParameter(*parser.Method, *parser.Parameter) error {
	return nil
}

func (f *failingMethodRenderer) OnCookieParameter(*parser.Method, *parser.Parameter) error {
	return nil
}
func (f *failingMethodRenderer) OnRequestBody(*parser.Method, *parser.RequestBody) error { return nil }
func (f *failingMethodRenderer) OnResponse(*parser.Method, string, *parser.Response) error {
	return nil
}

func (f *failingMethodRenderer) OnResponseHeader(*parser.Method, string, string, *parser.Parameter) error {
	return nil
}

func TestComposeSingletonFile_AssemblesBodyAndImports(t *testing.T) {
	ff := gogen.NewFileFactory("oapigen-test")
	c := NewFileComposer(ff)

	r := &fakeSingleton{
		path: "model/utc_time.gen.go",
		body: []byte("type Time time.Time\n"),
		imps: []gogen.Import{
			{Path: "time", Package: "time"},
			{Path: "encoding/json", Package: "json"},
		},
	}
	ctx := &render.RenderContext{}

	file, err := c.ComposeSingletonFile(r, ctx)
	require.NoError(t, err)
	require.NotNil(t, file)

	content := string(file.Content())
	assert.Contains(t, content, "package model")
	assert.Contains(t, content, "type Time time.Time")
	assert.Contains(t, content, `"encoding/json"`)
	assert.Contains(t, content, `"time"`)
}

func TestComposeSingletonFile_RenderErrorWrapped(t *testing.T) {
	ff := gogen.NewFileFactory("oapigen-test")
	c := NewFileComposer(ff)

	r := &fakeSingleton{
		path: "model/utc_time.gen.go",
		err:  errors.New("render boom"),
	}
	ctx := &render.RenderContext{}

	_, err := c.ComposeSingletonFile(r, ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "singleton model/utc_time.gen.go")
	assert.Contains(t, err.Error(), "render boom")
}

func TestPackageOf(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"nested path", "model/utc_time.gen.go", "model"},
		{"deep path", "client/v2/pets.gen.go", "v2"},
		{"no slash", "root.gen.go", "root.gen.go"},
		{"empty string", "", ""},
		{"interfaces under client", "interfaces/client/client.gen.go", "client"},
		{"interfaces under server", "interfaces/server/server.gen.go", "server"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, packageOf(tc.in))
		})
	}
}
