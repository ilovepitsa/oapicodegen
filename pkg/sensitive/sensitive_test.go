package sensitive

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSensitive_MarshalJSON_AlwaysMasked(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		s    any
	}{
		{"string", New("super-secret-password")},
		{"bytes", New([]byte{0xDE, 0xAD, 0xBE, 0xEF})},
		{"int", New(42)},
		{"empty string", New("")},
		{"nil bytes", New([]byte(nil))},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := json.Marshal(tc.s)
			require.NoError(t, err)
			assert.JSONEq(t, `"***"`, string(got))
		})
	}
}

func TestSensitive_Value_ReturnsOriginal(t *testing.T) {
	t.Parallel()

	s := New("my-secret")
	assert.Equal(t, "my-secret", s.Value())
}

func TestSensitive_UnmarshalJSON_DecodesValue(t *testing.T) {
	t.Parallel()

	var s Sensitive[string]
	require.NoError(t, json.Unmarshal([]byte(`"hello"`), &s))
	assert.Equal(t, "hello", s.Value())
}

func TestSensitive_UnmarshalJSON_Bytes(t *testing.T) {
	t.Parallel()

	var s Sensitive[[]byte]
	require.NoError(t, json.Unmarshal([]byte(`"aGVsbG8="`), &s))
	assert.Equal(t, []byte("hello"), s.Value())
}

func TestSensitive_RoundTrip_LosesValue(t *testing.T) {
	t.Parallel()

	original := New("super-secret")
	marshaled, err := json.Marshal(original)
	require.NoError(t, err)

	assert.JSONEq(t, `"***"`, string(marshaled), "marshal must mask")

	var restored Sensitive[string]
	require.NoError(t, json.Unmarshal(marshaled, &restored))
	assert.Equal(t, "***", restored.Value(), "unmarshal of mask gives literal ***")
}

func TestSensitive_ZeroValue(t *testing.T) {
	t.Parallel()

	var s Sensitive[string]
	assert.Equal(t, "", s.Value())

	got, err := json.Marshal(s)
	require.NoError(t, err)
	assert.JSONEq(t, `"***"`, string(got))
}

func TestSensitive_PointerReceiver_Unmarshal(t *testing.T) {
	t.Parallel()

	s := New(100)
	require.NoError(t, json.Unmarshal([]byte(`200`), &s))
	assert.Equal(t, 200, s.Value())
}
