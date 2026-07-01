package must

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValue_NoError(t *testing.T) {
	t.Run("returns value when err is nil", func(t *testing.T) {
		assert.Equal(t, 42, Value(42, nil))
	})

	t.Run("returns pointer value when err is nil", func(t *testing.T) {
		x := 7
		p := Value(&x, nil)
		if !assert.NotNil(t, p) {
			return
		}
		assert.Equal(t, 7, *p)
	})

	t.Run("returns zero value when v is zero and err is nil", func(t *testing.T) {
		assert.Equal(t, "", Value("", nil))
	})

	t.Run("returns nil pointer when v is nil and err is nil", func(t *testing.T) {
		var p *int
		assert.Nil(t, Value(p, nil))
	})
}

func TestValue_Error(t *testing.T) {
	t.Run("panics when err is non-nil", func(t *testing.T) {
		err := errors.New("boom")
		assert.PanicsWithValue(t, err, func() {
			_ = Value(42, err)
		})
	})

	t.Run("panics with wrapped error", func(t *testing.T) {
		wrapped := errors.Join(errors.New("base"), errors.New("context"))
		assert.PanicsWithValue(t, wrapped, func() {
			_ = Value("ignored", wrapped)
		})
	})
}
