package ptr

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGet(t *testing.T) {
	t.Run("bool literal", func(t *testing.T) {
		p := Get(true)
		if !assert.NotNil(t, p) {
			return
		}
		assert.True(t, *p)
	})

	t.Run("string", func(t *testing.T) {
		p := Get("hello")
		assert.Equal(t, "hello", *p)
	})

	t.Run("int", func(t *testing.T) {
		p := Get(42)
		assert.Equal(t, 42, *p)
	})

	t.Run("struct", func(t *testing.T) {
		type s struct{ X int }
		p := Get(s{X: 7})
		assert.Equal(t, 7, p.X)
	})

	t.Run("each call returns distinct pointer", func(t *testing.T) {
		a := Get(1)
		b := Get(1)
		assert.NotSame(t, a, b)
	})
}

func TestValue(t *testing.T) {
	t.Run("nil returns zero", func(t *testing.T) {
		var p *int
		assert.Equal(t, 0, Value(p))
	})

	t.Run("nil struct returns zero", func(t *testing.T) {
		type s struct {
			X int
			Y string
		}
		var p *s
		v := Value(p)
		assert.Equal(t, s{}, v)
	})

	t.Run("non-nil dereferences", func(t *testing.T) {
		v := 5
		p := &v
		assert.Equal(t, 5, Value(p))
	})

	t.Run("does not panic on nil pointer", func(t *testing.T) {
		var p *string
		assert.NotPanics(t, func() {
			_ = Value(p)
		})
	})
}

func TestClone(t *testing.T) {
	t.Run("nil returns nil", func(t *testing.T) {
		var p *int
		assert.Nil(t, Clone(p))
	})

	t.Run("returns new pointer with same value", func(t *testing.T) {
		v := 10
		p := &v
		c := Clone(p)
		if !assert.NotNil(t, c) {
			return
		}
		assert.Equal(t, 10, *c)
		assert.NotSame(t, p, c)
	})

	t.Run("clone is independent of source", func(t *testing.T) {
		v := 1
		p := &v
		c := Clone(p)
		*c = 99
		assert.Equal(t, 1, *p, "source mutated after clone change")
	})
}

func TestEqual(t *testing.T) {
	t.Run("both nil", func(t *testing.T) {
		assert.True(t, Equal[int](nil, nil))
	})

	t.Run("first nil", func(t *testing.T) {
		b := 1
		assert.False(t, Equal(nil, &b))
	})

	t.Run("second nil", func(t *testing.T) {
		a := 1
		assert.False(t, Equal(&a, nil))
	})

	t.Run("same values", func(t *testing.T) {
		a, b := 5, 5
		assert.True(t, Equal(&a, &b))
	})

	t.Run("different values", func(t *testing.T) {
		a, b := 5, 6
		assert.False(t, Equal(&a, &b))
	})

	t.Run("same pointer", func(t *testing.T) {
		a := 5
		assert.True(t, Equal(&a, &a))
	})
}
