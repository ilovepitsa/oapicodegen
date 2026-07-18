package walk

import (
	"errors"
	"nschugorev/oapigenerator/internal/parser"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingRenderer записывает последовательность хуков для проверки порядка обхода.
// Все 13 хуков SchemaRenderer переопределены, поэтому NoopSchemaRenderer не
// embed'ится (иначе unused-линтер падает на неиспользуемом поле).
type recordingRenderer struct {
	calls []string
}

func (r *recordingRenderer) record(name string, s *parser.Schema) error {
	r.calls = append(r.calls, name+":"+s.Name)

	return nil
}

func (r *recordingRenderer) OnStruct(s *parser.Schema) error { return r.record("OnStruct", s) }
func (r *recordingRenderer) OnEnum(s *parser.Schema) error   { return r.record("OnEnum", s) }
func (r *recordingRenderer) OnAlias(s *parser.Schema) error  { return r.record("OnAlias", s) }
func (r *recordingRenderer) OnArray(s *parser.Schema) error  { return r.record("OnArray", s) }
func (r *recordingRenderer) OnMap(s *parser.Schema) error    { return r.record("OnMap", s) }
func (r *recordingRenderer) OnUnion(s *parser.Schema, _ UnionKind) error {
	return r.record("OnUnion", s)
}
func (r *recordingRenderer) OnAllOf(s *parser.Schema) error { return r.record("OnAllOf", s) }
func (r *recordingRenderer) OnSplitStruct(s *parser.Schema) error {
	return r.record("OnSplitStruct", s)
}

func (r *recordingRenderer) OnStructProperty(s *parser.Schema, name string, prop *parser.Schema) error {
	r.calls = append(r.calls, "OnStructProperty:"+s.Name+"."+name)

	return nil
}

func (r *recordingRenderer) OnArrayItem(s *parser.Schema, idx int, item *parser.Schema) error {
	r.calls = append(r.calls, "OnArrayItem:"+s.Name+"["+strconv.Itoa(idx)+"]="+item.Name)

	return nil
}

func (r *recordingRenderer) OnMapValue(s *parser.Schema, value *parser.Schema) error {
	r.calls = append(r.calls, "OnMapValue:"+s.Name+"="+value.Name)

	return nil
}

func (r *recordingRenderer) OnUnionVariant(s *parser.Schema, idx int, variant *parser.Schema) error {
	r.calls = append(r.calls, "OnUnionVariant:"+s.Name+"["+strconv.Itoa(idx)+"]="+variant.Name)

	return nil
}

func (r *recordingRenderer) OnAllOfMember(s *parser.Schema, idx int, member *parser.Schema) error {
	r.calls = append(r.calls, "OnAllOfMember:"+s.Name+"["+strconv.Itoa(idx)+"]="+member.Name)

	return nil
}

func TestSchemaWalker_StructWithProperty(t *testing.T) {
	parent := &parser.Schema{Name: "Pet", Type: "object"}
	parent.Properties = []*parser.Property{
		{Name: "name", Schema: &parser.Schema{Name: "string", Type: "string"}},
	}

	rec := &recordingRenderer{}
	w := NewSchemaWalker(rec)
	require.NoError(t, w.Walk(parent))

	assert.Equal(t, []string{
		"OnStruct:Pet",
		"OnStructProperty:Pet.name",
		"OnAlias:string", // descend в string-схему — она type=string, рендерится как alias
	}, rec.calls)
}

func TestSchemaWalker_ArrayWithItem(t *testing.T) {
	arr := &parser.Schema{
		Name:  "PetList",
		Type:  "array",
		Items: &parser.Schema{Name: "Pet", Type: "object", Properties: []*parser.Property{}},
	}
	rec := &recordingRenderer{}
	w := NewSchemaWalker(rec)
	require.NoError(t, w.Walk(arr))

	assert.Equal(t, []string{
		"OnArray:PetList",
		"OnArrayItem:PetList[0]=Pet",
		"OnMap:Pet",
	}, rec.calls)
}

func TestSchemaWalker_ErrorStopsWalk(t *testing.T) {
	parent := &parser.Schema{Name: "Pet", Type: "object"}
	parent.Properties = []*parser.Property{
		{Name: "id", Schema: &parser.Schema{Name: "string", Type: "string"}},
	}
	failing := &failingRenderer{failOn: "Pet"}
	w := NewSchemaWalker(failing)
	err := w.Walk(parent)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "walk schema \"Pet\"")
	assert.Contains(t, err.Error(), "boom: Pet")
}

func TestSchemaWalker_SplitStructDispatches(t *testing.T) {
	split := &parser.Schema{Name: "Pet", Type: "object", IsSplit: true}
	split.Properties = []*parser.Property{
		{Name: "id", Schema: &parser.Schema{Name: "int", Type: "integer"}},
	}

	rec := &recordingRenderer{}
	w := NewSchemaWalker(rec)
	require.NoError(t, w.Walk(split))

	assert.Equal(t, []string{
		"OnSplitStruct:Pet",
		"OnStructProperty:Pet.id",
		"OnAlias:int",
	}, rec.calls)
}

// TestSchemaWalker_EmptyObjectDispatchesToMap проверяет, что object-схема
// без properties (map-alias: object + AdditionalProperties / AdditionalPropertiesFalse)
// диспатчится в OnMap, а не OnStruct — это позволяет AliasRenderer'у
// рендерить `type X map[string]Y` вместо struct-определения.
//
// OnMapValue не вызывается: walker диспатчит его только для Type=="map"
// (не standard OpenAPI). Map-alias имеет Type=="object", поэтому значение
// AdditionalProperties рендерится внутри OnMap через TypeMapper.GoType.
func TestSchemaWalker_EmptyObjectDispatchesToMap(t *testing.T) {
	empty := &parser.Schema{
		Name:                 "StringMap",
		Type:                 "object",
		AdditionalProperties: &parser.Schema{Name: "string", Type: "string"},
	}

	rec := &recordingRenderer{}
	w := NewSchemaWalker(rec)
	require.NoError(t, w.Walk(empty))

	assert.Equal(t, []string{"OnMap:StringMap"}, rec.calls)
}

type failingRenderer struct {
	NoopSchemaRenderer
	failOn string
}

func (f *failingRenderer) OnStruct(s *parser.Schema) error {
	if s.Name == f.failOn {
		return errors.New("boom: " + s.Name)
	}

	return nil
}

// recordingMethodRenderer записывает последовательность хуков method-walker'а.
// Все 8 хуков MethodRenderer переопределены, поэтому noopMethodRenderer не
// embed'ится (иначе unused-линтер падает на неиспользуемом поле).
type recordingMethodRenderer struct {
	calls []string
}

func (r *recordingMethodRenderer) OnMethod(m *parser.Method) error {
	r.calls = append(r.calls, "OnMethod:"+m.OperationID)

	return nil
}

func (r *recordingMethodRenderer) OnPathParameter(m *parser.Method, p *parser.Parameter) error {
	r.calls = append(r.calls, "OnPathParameter:"+p.Name)

	return nil
}

func (r *recordingMethodRenderer) OnQueryParameter(m *parser.Method, p *parser.Parameter) error {
	r.calls = append(r.calls, "OnQueryParameter:"+p.Name)

	return nil
}

func (r *recordingMethodRenderer) OnHeaderParameter(m *parser.Method, p *parser.Parameter) error {
	r.calls = append(r.calls, "OnHeaderParameter:"+p.Name)

	return nil
}

func (r *recordingMethodRenderer) OnCookieParameter(m *parser.Method, p *parser.Parameter) error {
	r.calls = append(r.calls, "OnCookieParameter:"+p.Name)

	return nil
}

func (r *recordingMethodRenderer) OnRequestBody(m *parser.Method, body *parser.RequestBody) error {
	r.calls = append(r.calls, "OnRequestBody:"+body.Content["application/json"].Schema.Name)

	return nil
}

func (r *recordingMethodRenderer) OnResponse(m *parser.Method, code string, _ *parser.Response) error {
	r.calls = append(r.calls, "OnResponse:"+code)

	return nil
}

func (r *recordingMethodRenderer) OnResponseHeader(_ *parser.Method, code string, name string, _ *parser.Parameter) error {
	r.calls = append(r.calls, "OnResponseHeader:"+code+":"+name)

	return nil
}

func TestMethodWalker_Order(t *testing.T) {
	method := &parser.Method{
		Method:      "GET",
		Path:        "/pets",
		OperationID: "ListPets",
		Parameters: []*parser.Parameter{
			{In: "path", Name: "id"},
			{In: "query", Name: "limit"},
			{In: "header", Name: "X-Trace"},
			{In: "cookie", Name: "session"},
		},
		RequestBody: &parser.RequestBody{
			Content: map[string]*parser.MediaType{
				"application/json": {Schema: &parser.Schema{Name: "Pet", Type: "object"}},
			},
		},
		Responses: []*parser.Response{
			{
				StatusCode: "200",
				Headers: map[string]*parser.Parameter{
					"X-Rate-Limit": {Name: "X-Rate-Limit", In: "header"},
				},
				Content: map[string]*parser.MediaType{
					"application/json": {Schema: &parser.Schema{Name: "PetList", Type: "array"}},
				},
			},
		},
	}

	rec := &recordingMethodRenderer{}
	w := NewMethodWalker(rec)
	require.NoError(t, w.Walk(method))

	assert.Equal(t, []string{
		"OnMethod:ListPets",
		"OnPathParameter:id",
		"OnQueryParameter:limit",
		"OnHeaderParameter:X-Trace",
		"OnCookieParameter:session",
		"OnRequestBody:Pet",
		"OnResponse:200",
		"OnResponseHeader:200:X-Rate-Limit",
	}, rec.calls)
}

type failingMethodRenderer struct {
	NoopMethodRenderer
	failOn string
}

func (f *failingMethodRenderer) OnMethod(m *parser.Method) error {
	if m.OperationID == f.failOn {
		return errors.New("boom: " + m.OperationID)
	}

	return nil
}

func TestMethodWalker_ErrorStopsWalk(t *testing.T) {
	method := &parser.Method{Method: "GET", Path: "/pets", OperationID: "ListPets"}
	w := NewMethodWalker(&failingMethodRenderer{failOn: "ListPets"})
	err := w.Walk(method)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "walk method")
	assert.Contains(t, err.Error(), "boom: ListPets")
}
