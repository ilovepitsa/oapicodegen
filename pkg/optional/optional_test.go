package optional

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOptional_New(t *testing.T) {
	t.Parallel()

	o := New(42)

	assert.True(t, o.IsSet())
	assert.Equal(t, 42, o.Value())
}

func TestOptional_Zero(t *testing.T) {
	t.Parallel()

	var o Optional[int]

	assert.False(t, o.IsSet())
	assert.Equal(t, 0, o.Value())
}

func TestOptional_NewEmpty(t *testing.T) {
	t.Parallel()

	o := NewEmpty[string]()

	assert.False(t, o.IsSet())
	assert.Equal(t, "", o.Value())
}

func TestOptional_SetTo(t *testing.T) {
	t.Parallel()

	var o Optional[string]
	o.SetTo("hello")

	assert.True(t, o.IsSet())
	assert.Equal(t, "hello", o.Value())
}

func TestOptional_SetToNil(t *testing.T) {
	t.Parallel()

	t.Run("int", func(t *testing.T) {
		t.Parallel()

		var o Optional[int]
		o.SetToNil()

		assert.True(t, o.IsSet(), "SetToNil must mark as set")
		assert.Equal(t, 0, o.Value(), "SetToNil must leave zero value")
	})

	t.Run("string", func(t *testing.T) {
		t.Parallel()

		var o Optional[string]
		o.SetToNil()

		assert.True(t, o.IsSet())
		assert.Equal(t, "", o.Value())
	})

	t.Run("pointer", func(t *testing.T) {
		t.Parallel()

		var o Optional[*int]
		o.SetToNil()

		assert.True(t, o.IsSet())
		assert.Nil(t, o.Value())
	})
}

func TestOptional_Unset(t *testing.T) {
	t.Parallel()

	t.Run("after SetTo", func(t *testing.T) {
		t.Parallel()

		o := New(42)
		o.Unset()

		assert.False(t, o.IsSet())
		assert.Equal(t, 0, o.Value())
	})

	t.Run("after SetToNil", func(t *testing.T) {
		t.Parallel()

		var o Optional[int]
		o.SetToNil()
		o.Unset()

		assert.False(t, o.IsSet())
		assert.Equal(t, 0, o.Value())
	})

	t.Run("double Unset is no-op", func(t *testing.T) {
		t.Parallel()

		o := New(7)
		o.Unset()
		o.Unset()

		assert.False(t, o.IsSet())
	})
}

func TestOptional_ValueOr(t *testing.T) {
	t.Parallel()

	t.Run("not set returns default", func(t *testing.T) {
		t.Parallel()

		var o Optional[int]
		assert.Equal(t, 99, o.ValueOr(99))
	})

	t.Run("set returns value", func(t *testing.T) {
		t.Parallel()

		o := New(7)
		assert.Equal(t, 7, o.ValueOr(99))
	})

	t.Run("SetToNil returns default (explicit null has no value)", func(t *testing.T) {
		t.Parallel()

		var o Optional[int]
		o.SetToNil()
		// SetToNil means "explicitly null"; ValueOr returns the fallback.
		assert.Equal(t, 99, o.ValueOr(99))
	})
}

func TestOptional_MarshalJSON(t *testing.T) {
	t.Parallel()

	t.Run("not set returns null", func(t *testing.T) {
		t.Parallel()

		var o Optional[int]
		data, err := json.Marshal(o)
		require.NoError(t, err)
		assert.JSONEq(t, "null", string(data))
	})

	t.Run("set int", func(t *testing.T) {
		t.Parallel()

		o := New(42)
		data, err := json.Marshal(o)
		require.NoError(t, err)
		assert.JSONEq(t, "42", string(data))
	})

	t.Run("set string", func(t *testing.T) {
		t.Parallel()

		o := New("hello")
		data, err := json.Marshal(o)
		require.NoError(t, err)
		assert.JSONEq(t, `"hello"`, string(data))
	})

	t.Run("set struct", func(t *testing.T) {
		t.Parallel()

		type person struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}
		o := New(person{Name: "Alice", Age: 30})
		data, err := json.Marshal(o)
		require.NoError(t, err)
		assert.JSONEq(t, `{"name":"Alice","age":30}`, string(data))
	})

	t.Run("SetToNil returns null", func(t *testing.T) {
		t.Parallel()

		var o Optional[int]
		o.SetToNil()
		data, err := json.Marshal(o)
		require.NoError(t, err)
		assert.JSONEq(t, "null", string(data))
	})

	t.Run("zero value Optional returns null", func(t *testing.T) {
		t.Parallel()

		var o Optional[int]
		data, err := json.Marshal(o)
		require.NoError(t, err)
		assert.JSONEq(t, "null", string(data))
	})
}

func TestOptional_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	t.Run("null sets SetToNil", func(t *testing.T) {
		t.Parallel()

		var o Optional[int]
		err := json.Unmarshal([]byte("null"), &o)
		require.NoError(t, err)

		assert.True(t, o.IsSet(), "null must mark as set (SetToNil)")
		assert.Equal(t, 0, o.Value())
	})

	t.Run("value sets and decodes", func(t *testing.T) {
		t.Parallel()

		var o Optional[int]
		err := json.Unmarshal([]byte("42"), &o)
		require.NoError(t, err)

		assert.True(t, o.IsSet())
		assert.Equal(t, 42, o.Value())
	})

	t.Run("string value", func(t *testing.T) {
		t.Parallel()

		var o Optional[string]
		err := json.Unmarshal([]byte(`"hello"`), &o)
		require.NoError(t, err)

		assert.True(t, o.IsSet())
		assert.Equal(t, "hello", o.Value())
	})

	t.Run("struct value", func(t *testing.T) {
		t.Parallel()

		type person struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}
		var o Optional[person]
		err := json.Unmarshal([]byte(`{"name":"Bob","age":25}`), &o)
		require.NoError(t, err)

		assert.True(t, o.IsSet())
		assert.Equal(t, person{Name: "Bob", Age: 25}, o.Value())
	})

	t.Run("empty data via direct call leaves not set", func(t *testing.T) {
		t.Parallel()

		var o Optional[int]
		// json.Unmarshal returns "unexpected end of JSON input" before
		// invoking UnmarshalJSON for empty input, so call the method directly.
		err := o.UnmarshalJSON(nil)
		require.NoError(t, err)

		assert.False(t, o.IsSet())
	})

	t.Run("zero-length data via direct call leaves not set", func(t *testing.T) {
		t.Parallel()

		var o Optional[int]
		err := o.UnmarshalJSON([]byte{})
		require.NoError(t, err)

		assert.False(t, o.IsSet())
	})

	t.Run("json.Unmarshal with nil data returns error from stdlib", func(t *testing.T) {
		t.Parallel()

		// Standard json.Unmarshal rejects empty input before calling
		// UnmarshalJSON; document this behaviour so callers know.
		var o Optional[int]
		err := json.Unmarshal(nil, &o)
		require.Error(t, err)

		assert.False(t, o.IsSet())
	})

	t.Run("invalid json returns error", func(t *testing.T) {
		t.Parallel()

		var o Optional[int]
		err := json.Unmarshal([]byte("not-json"), &o)
		require.Error(t, err)
	})

	t.Run("type mismatch returns error", func(t *testing.T) {
		t.Parallel()

		var o Optional[int]
		err := json.Unmarshal([]byte(`"not-an-int"`), &o)
		require.Error(t, err)
	})
}

func TestOptional_RoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("int", func(t *testing.T) {
		t.Parallel()

		original := New(42)

		data, err := json.Marshal(original)
		require.NoError(t, err)

		var decoded Optional[int]
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.True(t, decoded.IsSet())
		assert.Equal(t, 42, decoded.Value())
	})

	t.Run("string", func(t *testing.T) {
		t.Parallel()

		original := New("hello")

		data, err := json.Marshal(original)
		require.NoError(t, err)

		var decoded Optional[string]
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.True(t, decoded.IsSet())
		assert.Equal(t, "hello", decoded.Value())
	})

	t.Run("struct", func(t *testing.T) {
		t.Parallel()

		type item struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		}
		original := New(item{ID: 1, Name: "x"})

		data, err := json.Marshal(original)
		require.NoError(t, err)

		var decoded Optional[item]
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.True(t, decoded.IsSet())
		assert.Equal(t, item{ID: 1, Name: "x"}, decoded.Value())
	})

	t.Run("SetToNil round-trips as null", func(t *testing.T) {
		t.Parallel()

		original := NewEmpty[int]()
		original.SetToNil()

		data, err := json.Marshal(original)
		require.NoError(t, err)
		assert.JSONEq(t, "null", string(data))

		var decoded Optional[int]
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.True(t, decoded.IsSet(), "null must round-trip as set=true")
		assert.Equal(t, 0, decoded.Value())
	})
}

func TestOptional_InStruct(t *testing.T) {
	t.Parallel()

	// Demonstrates typical usage: a struct with an optional field.
	type update struct {
		Name Optional[string] `json:"name"`
		Age  Optional[int]    `json:"age"`
	}

	t.Run("marshal with mixed set/unset", func(t *testing.T) {
		t.Parallel()

		u := update{
			Name: New("Alice"),
		}

		data, err := json.Marshal(u)
		require.NoError(t, err)
		assert.JSONEq(t, `{"name":"Alice","age":null}`, string(data))
	})

	t.Run("unmarshal with missing field stays unset", func(t *testing.T) {
		t.Parallel()

		var u update
		err := json.Unmarshal([]byte(`{"name":"Bob"}`), &u)
		require.NoError(t, err)

		assert.True(t, u.Name.IsSet())
		assert.Equal(t, "Bob", u.Name.Value())
		assert.False(t, u.Age.IsSet(), "missing field must stay unset")
	})

	t.Run("unmarshal with explicit null sets SetToNil", func(t *testing.T) {
		t.Parallel()

		var u update
		err := json.Unmarshal([]byte(`{"name":null,"age":5}`), &u)
		require.NoError(t, err)

		assert.True(t, u.Name.IsSet(), "explicit null must mark as set")
		assert.Equal(t, "", u.Name.Value())
		assert.True(t, u.Age.IsSet())
		assert.Equal(t, 5, u.Age.Value())
	})
}
