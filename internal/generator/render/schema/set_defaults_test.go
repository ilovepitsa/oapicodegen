// Package schema: tests for SetDefaultsRenderer. Renderer НЕ подключён к
// pack'у (Task 3 делает wiring) — тесты вызывают OnStruct/OnSplitStruct
// напрямую, проверяя рендер SetDefaults-метода.
package schema

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newSetDefaultsTestRenderer строит SetDefaultsRenderer с shared Buf/Imports
// и фейковым TypeMapper, привязанным к RenderContext. Project в ctx — non-nil
// с пустой Model (проиндексированной): это упражирует не-nil путь
// projectModel() и подготовит почву для $ref-тестов (Issue 1 из code review
// Task 2: nil-Project маскировал бы wiring-баги в Task 3).
//
// Модель пуста, но её schemasIndex инициализирован (Index() вызван в
// newTestProject) — Lookup возвращает false для любого имени, что безопасно
// для тестов без $ref, и позволяет тестам с $ref добавить свои схемы через
// SetSchemas.
func newSetDefaultsTestRenderer(t *testing.T, tm render.TypeMapper) *SetDefaultsRenderer {
	t.Helper()

	ctx := &render.RenderContext{
		TypeMapper: tm,
		Project:    newTestProjectWithEmptyModel(),
	}
	r := NewSetDefaultsRenderer()
	r.Init(codegen.NewBufferWriter(), render.NewImportTracker(), ctx)

	return r
}

// newTestProjectWithEmptyModel возвращает минимальный *parser.Project с
// проиндексированной пустой Model. Используется в тестах SetDefaultsRenderer,
// чтобы projectModel() возвращал non-nil Model и resolveRefSchema доходил
// до Lookup (а не возвращал nil сразу).
func newTestProjectWithEmptyModel() *parser.Project {
	p := &parser.Project{}
	m := &parser.Model{}
	m.SetSchemas(nil) // вызывает Index() → schemasIndex = empty map (non-nil).
	p.Model = m

	return p
}

func TestSetDefaultsRenderer_NoDefaults_NoOutput(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newSetDefaultsTestRenderer(t, tm)

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name: "Pet",
		Type: "object",
		Properties: []*parser.Property{
			{Name: "Tag", Schema: &parser.Schema{Type: "string"}},
		},
	}))

	got := string(r.Buf.Content())
	assert.Empty(t, got, "schema without defaults must produce no output")
}

func TestSetDefaultsRenderer_WithDefault_RendersMethod(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newSetDefaultsTestRenderer(t, tm)

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name: "Pet",
		Type: "object",
		Properties: []*parser.Property{
			{Name: "Tag", Required: true, Schema: &parser.Schema{Type: "string", Default: "none"}},
		},
	}))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "func (m *Pet) SetDefaults() {")
	assert.Contains(t, got, `m.Tag = "none"`)
}

func TestSetDefaultsRenderer_WithDefaultInt_RendersZeroCheck(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "int"}
	r := newSetDefaultsTestRenderer(t, tm)

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name: "Pet",
		Type: "object",
		Properties: []*parser.Property{
			{Name: "ID", Required: true, Schema: &parser.Schema{Type: "integer", Default: 5}},
		},
	}))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "func (m *Pet) SetDefaults() {")
	assert.Contains(t, got, "if m.ID == 0 {")
	assert.Contains(t, got, "m.ID = 5")
}

func TestSetDefaultsRenderer_OptionalField_RendersNilCheck(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newSetDefaultsTestRenderer(t, tm)

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name: "Pet",
		Type: "object",
		Properties: []*parser.Property{
			{Name: "Tag", Schema: &parser.Schema{Type: "string", Default: "none"}},
		},
	}))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "func (m *Pet) SetDefaults() {")
	assert.Contains(t, got, "if m.Tag == nil {")
	assert.Contains(t, got, "v := ")
	assert.Contains(t, got, "m.Tag = &v")
}

func TestSetDefaultsRenderer_SplitStruct_RendersRequestAndResponse(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newSetDefaultsTestRenderer(t, tm)

	require.NoError(t, r.OnSplitStruct(&parser.Schema{
		Name:    "Pet",
		Type:    "object",
		IsSplit: true,
		Properties: []*parser.Property{
			{Name: "w", Required: true, Schema: &parser.Schema{Type: "string", WriteOnly: true, Default: "w"}},
			{Name: "r", Required: true, Schema: &parser.Schema{Type: "string", ReadOnly: true, Default: "r"}},
		},
	}))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "func (m *PetRequest) SetDefaults() {")
	assert.Contains(t, got, `m.W = "w"`)
	assert.Contains(t, got, "func (m *PetResponse) SetDefaults() {")
	assert.Contains(t, got, `m.R = "r"`)
}

func TestSetDefaultsRenderer_SplitStruct_OnlyRequestHasDefaults(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newSetDefaultsTestRenderer(t, tm)

	require.NoError(t, r.OnSplitStruct(&parser.Schema{
		Name:    "Pet",
		Type:    "object",
		IsSplit: true,
		Properties: []*parser.Property{
			{Name: "w", Required: true, Schema: &parser.Schema{Type: "string", WriteOnly: true, Default: "w"}},
			{Name: "r", Required: true, Schema: &parser.Schema{Type: "string", ReadOnly: true}},
		},
	}))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "func (m *PetRequest) SetDefaults() {")
	assert.NotContains(t, got, "func (m *PetResponse) SetDefaults() {")
}

// TestSetDefaultsRenderer_NestedRefWithDefaults_RendersNestedCall покрывает
// highest-risk untested branch — renderNestedSetDefaultsCall. Pet имеет
// optional $ref-поле owner → Owner с property tag (Default: "none").
// Ожидаем: if m.Owner != nil { m.Owner.SetDefaults() } (optional → pointer →
// nil-check перед вызовом).
//
// Issue 3 из code review Task 2: $ref recursion в nested object с defaults
// не была покрыта тестами (projectModel() возвращал nil и $ref не разрешался).
func TestSetDefaultsRenderer_NestedRefWithDefaults_RendersNestedCall(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "Owner"}
	r := newSetDefaultsTestRenderer(t, tm)

	ownerSchema := &parser.Schema{
		Name: "Owner",
		Type: "object",
		Properties: []*parser.Property{
			{Name: "tag", Schema: &parser.Schema{Type: "string", Default: "none"}},
		},
	}
	petSchema := &parser.Schema{
		Name: "Pet",
		Type: "object",
		Properties: []*parser.Property{
			{Name: "owner", Schema: &parser.Schema{Ref: "#/components/schemas/Owner"}},
		},
	}

	// Зарегистрировать схемы в Model, чтобы resolveRefSchema нашёл Owner по $ref.
	r.Ctx.Project.Model.SetSchemas([]*parser.Schema{petSchema, ownerSchema})

	require.NoError(t, r.OnStruct(petSchema))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "func (m *Pet) SetDefaults() {")
	assert.Contains(t, got, "if m.Owner != nil {")
	assert.Contains(t, got, "m.Owner.SetDefaults()")
}

// TestSetDefaultsRenderer_EnumDefault_RendersEnumConst покрывает enum default:
// property с $ref на enum-схему (Type: string, Enum, Default). Литерал
// рендерится как <TypeName><ValueName> (см. defaultValueLiteral в
// default_literals.go). Поле status optional (не в Required) → pointer →
// паттерн `v := <const>; m.Status = &v`.
//
// В production doc-loader (parser.schemaFromProxy) разрешает $ref и копирует
// поля целевой схемы (Default, Enum, Type) в Schema свойства. Поэтому
// p.Schema.Default != nil, и treeHasDefaultsWithVisited находит default
// напрямую (без $ref-рекурсии). Тест зеркалирует это: на property's Schema
// выставлены и Ref, и Default, и Enum.
//
// Issue 3 из code review Task 2: enum default не была покрыта.
func TestSetDefaultsRenderer_EnumDefault_RendersEnumConst(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "Status"}
	r := newSetDefaultsTestRenderer(t, tm)

	// statusSchema — top-level enum-схема (Type+Enum+Default).
	statusSchema := &parser.Schema{
		Name:    "Status",
		Type:    "string",
		Enum:    []any{"active", "inactive"},
		Default: "active",
	}
	// Property's Schema: $ref на Status + inherited поля (как делает doc-loader).
	itemSchema := &parser.Schema{
		Name: "Item",
		Type: "object",
		Properties: []*parser.Property{
			{Name: "status", Schema: &parser.Schema{
				Ref:     "#/components/schemas/Status",
				Name:    "Status",
				Type:    "string",
				Enum:    []any{"active", "inactive"},
				Default: "active",
			}},
		},
	}

	// Регистрируем схемы в Model — хотя $ref-рекурсия не нужна (Default уже
	// на property's Schema), регистрация держит тест близко к production.
	r.Ctx.Project.Model.SetSchemas([]*parser.Schema{itemSchema, statusSchema})

	require.NoError(t, r.OnStruct(itemSchema))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "func (m *Item) SetDefaults() {")
	// enum-константа: defaultValueLiteral возвращает <TypeName><ValueName>.
	// status optional (не в Required) → pointer → v-then-and-v pattern.
	assert.Contains(t, got, "if m.Status == nil {")
	assert.Contains(t, got, "v := StatusActive")
	assert.Contains(t, got, "m.Status = &v")
}

// TestSetDefaultsRenderer_DateTimeFormat_Skipped покрывает
// isNonPrimitiveStringFormat: поле createdAt с Type: string, Format:
// date-time, Default не должно давать присваивание строкового литерала
// (time.Time не компилируется). Но т.к. у поля есть Default, treeHasDefaults
// = true → SetDefaults рендерится, но с пустым телом по этому полю.
//
// Чтобы видеть, что метод всё-таки отрендерился, добавляем второе поле name
// с default — тогда тело содержит name, но НЕ содержит m.CreatedAt.
//
// Issue 3 из code review Task 2: isNonPrimitiveStringFormat skip не покрыт.
func TestSetDefaultsRenderer_DateTimeFormat_Skipped(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newSetDefaultsTestRenderer(t, tm)

	require.NoError(t, r.OnStruct(&parser.Schema{
		Name: "Event",
		Type: "object",
		Properties: []*parser.Property{
			{Name: "createdAt", Required: true, Schema: &parser.Schema{
				Type: "string", Format: "date-time", Default: "2024-01-01T00:00:00Z",
			}},
			{Name: "name", Schema: &parser.Schema{Type: "string", Default: "unnamed"}},
		},
	}))

	got := string(r.Buf.Content())
	// Метод рендерится (есть name с default) — но createdAt пропущен.
	assert.Contains(t, got, "func (m *Event) SetDefaults() {")
	assert.NotContains(t, got, "m.CreatedAt")
	// name с default присутствует (optional → pointer → v-then-and-v pattern).
	assert.Contains(t, got, `v := "unnamed"`)
	assert.Contains(t, got, "m.Name = &v")
}

// TestSetDefaultsRenderer_SplitOnlyResponseHasDefaults — зеркало существующего
// теста SplitStruct_OnlyRequestHasDefaults. readOnly-поле с Default попадает
// только в Response вариант; request-вариант не имеет defaults и не должен
// рендерить SetDefaults.
//
// Issue 3 из code review Task 2: mirror test для симметрии (только Response
// имеет defaults).
func TestSetDefaultsRenderer_SplitOnlyResponseHasDefaults(t *testing.T) {
	t.Parallel()

	tm := &fakeTypeMapper{got: "string"}
	r := newSetDefaultsTestRenderer(t, tm)

	require.NoError(t, r.OnSplitStruct(&parser.Schema{
		Name:    "Pet",
		Type:    "object",
		IsSplit: true,
		Properties: []*parser.Property{
			// readOnly с Default → только Response вариант.
			{Name: "ro", Required: true, Schema: &parser.Schema{Type: "string", ReadOnly: true, Default: "ro"}},
			// Обычное поле без Default — попадает в оба варианта, но default'а не даёт.
			{Name: "name", Required: true, Schema: &parser.Schema{Type: "string"}},
		},
	}))

	got := string(r.Buf.Content())
	assert.Contains(t, got, "func (m *PetResponse) SetDefaults() {")
	assert.Contains(t, got, `m.Ro = "ro"`)
	assert.NotContains(t, got, "func (m *PetRequest) SetDefaults() {")
}
