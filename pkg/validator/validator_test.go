package validator

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Registry tests ---

type fakeValidator struct {
	name string
	err  error
}

func (f fakeValidator) Name() string { return f.name }

func (f fakeValidator) Validate(_ any) error { return f.err }

func TestRegistry_RegisterAndGet(t *testing.T) {
	t.Parallel()

	r := New()
	v := fakeValidator{name: "test.EmailFormat"}
	r.Register(v)

	got, ok := r.Get("test.EmailFormat")
	require.True(t, ok)
	assert.Equal(t, v, got)

	_, ok = r.Get("missing")
	assert.False(t, ok)
}

func TestRegistry_Get_EmptyRegistry(t *testing.T) {
	t.Parallel()

	r := New()
	_, ok := r.Get("anything")
	assert.False(t, ok)
}

func TestRegistry_Register_Overwrites(t *testing.T) {
	t.Parallel()

	r := New()
	r.Register(fakeValidator{name: "x", err: errors.New("v1")})
	r.Register(fakeValidator{name: "x", err: errors.New("v2")})

	got, ok := r.Get("x")
	require.True(t, ok)
	assert.EqualError(t, got.Validate(nil), "v2")
}

func TestRegistry_Names_Sorted(t *testing.T) {
	t.Parallel()

	r := New()
	r.Register(fakeValidator{name: "c"})
	r.Register(fakeValidator{name: "a"})
	r.Register(fakeValidator{name: "b"})

	assert.Equal(t, []string{"a", "b", "c"}, r.Names())
}

func TestRegistry_Names_Empty(t *testing.T) {
	t.Parallel()

	r := New()
	assert.Empty(t, r.Names())
}

func TestRegistry_AssertExact_Match(t *testing.T) {
	t.Parallel()

	r := New()
	r.Register(fakeValidator{name: "a"})
	r.Register(fakeValidator{name: "b"})

	err := r.AssertExact([]string{"a", "b"})
	assert.NoError(t, err)
}

func TestRegistry_AssertExact_Missing(t *testing.T) {
	t.Parallel()

	r := New()
	r.Register(fakeValidator{name: "a"})

	err := r.AssertExact([]string{"a", "b", "c"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing: b, c")
	assert.NotContains(t, err.Error(), "extra")
}

func TestRegistry_AssertExact_Extra(t *testing.T) {
	t.Parallel()

	r := New()
	r.Register(fakeValidator{name: "a"})
	r.Register(fakeValidator{name: "b"})

	err := r.AssertExact([]string{"a"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extra: b")
	assert.NotContains(t, err.Error(), "missing")
}

func TestRegistry_AssertExact_BothMissingAndExtra(t *testing.T) {
	t.Parallel()

	r := New()
	r.Register(fakeValidator{name: "a"})
	r.Register(fakeValidator{name: "extra1"})

	err := r.AssertExact([]string{"a", "missing1", "missing2"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing: missing1, missing2")
	assert.Contains(t, err.Error(), "extra: extra1")
}

func TestRegistry_AssertExact_EmptyBoth(t *testing.T) {
	t.Parallel()

	r := New()
	err := r.AssertExact(nil)
	assert.NoError(t, err)
}

// --- Walker tests ---
//
// Тестовые типы с value-receiver ValidateOwn. Имитация сгенерированного кода.

type leafValidatable struct {
	Name string
	Err  error
}

func (l leafValidatable) ValidateOwn(_ *Registry) error { return l.Err }

type leafValidatablePtr struct {
	Name string
	Err  error
}

func (l *leafValidatablePtr) ValidateOwn(_ *Registry) error { return l.Err }

type nonValidatable struct {
	Name string
}

type parentStruct struct {
	Child  leafValidatable
	Child2 leafValidatable
}

type sliceParent struct {
	Items []leafValidatable
}

type mapParent struct {
	Items map[string]leafValidatable
}

type ptrParent struct {
	Child *leafValidatable
}

type deepNested struct {
	Owner parentStruct
	Pets  []leafValidatable
}

func TestValidate_NoRules_NoError(t *testing.T) {
	t.Parallel()

	r := New()
	err := Validate(nonValidatable{Name: "x"}, r)
	assert.NoError(t, err)
}

func TestValidate_SimpleStruct_CallsValidateOwn(t *testing.T) {
	t.Parallel()

	r := New()

	// Встраиваемый leafValidatable даёт ValidateOwn — walker должен его вызвать.
	v := struct {
		leafValidatable
	}{
		leafValidatable: leafValidatable{
			Err: errors.New("validation failed"),
		},
	}

	err := Validate(v, r)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")
}

func TestValidate_NestedStruct_BothCalled(t *testing.T) {
	t.Parallel()

	r := New()
	p := parentStruct{
		Child:  leafValidatable{Err: nil},
		Child2: leafValidatable{Err: errors.New("child2 error")},
	}

	err := Validate(p, r)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Child2")
	assert.Contains(t, err.Error(), "child2 error")
}

func TestValidate_Slice_EachElementValidated(t *testing.T) {
	t.Parallel()

	r := New()
	p := sliceParent{
		Items: []leafValidatable{
			{Err: nil},
			{Err: nil},
			{Err: errors.New("slice[2] error")},
		},
	}

	err := Validate(p, r)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Items[2]")
	assert.Contains(t, err.Error(), "slice[2] error")
}

func TestValidate_Map_EachValueValidated(t *testing.T) {
	t.Parallel()

	r := New()
	p := mapParent{
		Items: map[string]leafValidatable{
			"a":   {Err: nil},
			"bad": {Err: errors.New("map[bad] error")},
		},
	}

	err := Validate(p, r)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Items[bad]")
	assert.Contains(t, err.Error(), "map[bad] error")
}

func TestValidate_PointerToStruct_UnwrapsAndCalls(t *testing.T) {
	t.Parallel()

	r := New()
	v := &leafValidatable{Err: errors.New("ptr error")}

	err := Validate(v, r)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ptr error")
}

func TestValidate_NilPointer_Skipped(t *testing.T) {
	t.Parallel()

	r := New()
	p := ptrParent{Child: nil}

	err := Validate(p, r)
	assert.NoError(t, err)
}

func TestValidate_NilSlice_Skipped(t *testing.T) {
	t.Parallel()

	r := New()
	p := sliceParent{Items: nil}

	err := Validate(p, r)
	assert.NoError(t, err)
}

func TestValidate_EmptySlice_Skipped(t *testing.T) {
	t.Parallel()

	r := New()
	p := sliceParent{Items: []leafValidatable{}}

	err := Validate(p, r)
	assert.NoError(t, err)
}

func TestValidate_DeepNested_PathInError(t *testing.T) {
	t.Parallel()

	r := New()
	p := deepNested{
		Owner: parentStruct{
			Child:  leafValidatable{Err: nil},
			Child2: leafValidatable{Err: nil},
		},
		Pets: []leafValidatable{
			{Err: nil},
			{Err: errors.New("deep error")},
		},
	}

	err := Validate(p, r)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Pets[1]")
	assert.Contains(t, err.Error(), "deep error")
}

func TestValidate_FailFast_FirstErrorReturns(t *testing.T) {
	t.Parallel()

	r := New()
	p := sliceParent{
		Items: []leafValidatable{
			{Err: errors.New("first")},
			{Err: errors.New("second")},
		},
	}

	err := Validate(p, r)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "first")
	assert.NotContains(t, err.Error(), "second")
}

func TestValidate_PointerReceiver_OnAddressable(t *testing.T) {
	t.Parallel()

	r := New()
	v := &leafValidatablePtr{Err: errors.New("ptr-recv error")}

	err := Validate(v, r)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ptr-recv error")
}

func TestValidate_PointerReceiver_InSlice(t *testing.T) {
	t.Parallel()

	r := New()
	type wrapper struct {
		Items []*leafValidatablePtr
	}
	w := wrapper{
		Items: []*leafValidatablePtr{
			{Err: nil},
			{Err: errors.New("ptr-in-slice error")},
		},
	}

	err := Validate(w, r)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Items[1]")
	assert.Contains(t, err.Error(), "ptr-in-slice error")
}

func TestValidate_NilInterface_Skipped(t *testing.T) {
	t.Parallel()

	r := New()
	var v any = nil

	err := Validate(v, r)
	assert.NoError(t, err)
}

func TestValidate_NestedStruct_PathTopLevelHasNoPrefix(t *testing.T) {
	t.Parallel()

	r := New()
	// Top-level validatable — path="", ошибка без префикса.
	v := leafValidatable{Err: errors.New("top error")}

	err := Validate(v, r)
	require.Error(t, err)
	assert.Equal(t, "top error", err.Error())
}

func TestValidate_UnexportedField_Skipped(t *testing.T) {
	t.Parallel()

	r := New()
	type withPrivate struct {
		secret leafValidatable // unexported — walker должен пропустить
		Public leafValidatable
	}

	v := withPrivate{
		secret: leafValidatable{Err: errors.New("secret error")},
		Public: leafValidatable{Err: nil},
	}

	err := Validate(v, r)
	assert.NoError(t, err, "unexported field must be skipped")
}

// --- Validator interface usage ---

func TestValidator_Validate_CalledWithFieldValue(t *testing.T) {
	t.Parallel()

	r := New()

	// Используем тестовый валидатор с tracked-вызовом.
	tracked := &trackingValidator{name: "test.NonEmpty"}
	r.Register(tracked)

	// Прямой вызов валидатора через registry.
	got, ok := r.Get("test.NonEmpty")
	require.True(t, ok)
	require.NoError(t, got.Validate("hello"))
	assert.True(t, tracked.called)
	assert.Equal(t, "hello", tracked.gotValue)
}

type trackingValidator struct {
	name     string
	called   bool
	gotValue any
	err      error
}

func (t *trackingValidator) Name() string { return t.name }

func (t *trackingValidator) Validate(value any) error {
	t.called = true
	t.gotValue = value

	return t.err
}

// --- Edge cases ---

func TestValidate_Primitive_NoError(t *testing.T) {
	t.Parallel()

	r := New()
	err := Validate(42, r)
	assert.NoError(t, err)
}

func TestValidate_String_NoError(t *testing.T) {
	t.Parallel()

	r := New()
	err := Validate("hello", r)
	assert.NoError(t, err)
}

func TestValidate_ArrayOfValidatables(t *testing.T) {
	t.Parallel()

	r := New()
	arr := [3]leafValidatable{
		{Err: nil},
		{Err: nil},
		{Err: errors.New("array[2] failed")},
	}

	err := Validate(arr, r)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "[2]")
	assert.Contains(t, err.Error(), "array[2] failed")
}
