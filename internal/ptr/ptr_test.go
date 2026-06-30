package ptr

import (
	"testing"
)

func TestGet(t *testing.T) {
	t.Run("bool literal", func(t *testing.T) {
		p := Get(true)
		if p == nil {
			t.Fatal("expected non-nil pointer")
		}
		if *p != true {
			t.Fatalf("expected true, got %v", *p)
		}
	})

	t.Run("string", func(t *testing.T) {
		p := Get("hello")
		if *p != "hello" {
			t.Fatalf("expected hello, got %q", *p)
		}
	})

	t.Run("int", func(t *testing.T) {
		p := Get(42)
		if *p != 42 {
			t.Fatalf("expected 42, got %d", *p)
		}
	})

	t.Run("struct", func(t *testing.T) {
		type s struct{ X int }
		p := Get(s{X: 7})
		if p.X != 7 {
			t.Fatalf("expected 7, got %d", p.X)
		}
	})

	t.Run("each call returns distinct pointer", func(t *testing.T) {
		a := Get(1)
		b := Get(1)
		if a == b {
			t.Fatal("expected distinct pointers for distinct calls")
		}
	})
}

func TestValue(t *testing.T) {
	t.Run("nil returns zero", func(t *testing.T) {
		var p *int
		if v := Value(p); v != 0 {
			t.Fatalf("expected 0, got %d", v)
		}
	})

	t.Run("nil struct returns zero", func(t *testing.T) {
		type s struct {
			X int
			Y string
		}
		var p *s
		v := Value(p)
		if v.X != 0 || v.Y != "" {
			t.Fatalf("expected zero struct, got %+v", v)
		}
	})

	t.Run("non-nil dereferences", func(t *testing.T) {
		v := 5
		p := &v
		if got := Value(p); got != 5 {
			t.Fatalf("expected 5, got %d", got)
		}
	})

	t.Run("does not panic on nil interface-like pointer", func(t *testing.T) {
		var p *string
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("unexpected panic: %v", r)
			}
		}()
		_ = Value(p)
	})
}

func TestClone(t *testing.T) {
	t.Run("nil returns nil", func(t *testing.T) {
		var p *int
		if c := Clone(p); c != nil {
			t.Fatalf("expected nil, got %v", c)
		}
	})

	t.Run("returns new pointer with same value", func(t *testing.T) {
		v := 10
		p := &v
		c := Clone(p)
		if c == nil {
			t.Fatal("expected non-nil clone")
		}
		if *c != 10 {
			t.Fatalf("expected 10, got %d", *c)
		}
		if c == p {
			t.Fatal("expected distinct pointer from source")
		}
	})

	t.Run("clone is independent of source", func(t *testing.T) {
		v := 1
		p := &v
		c := Clone(p)
		*c = 99
		if *p != 1 {
			t.Fatalf("source mutated after clone change: expected 1, got %d", *p)
		}
	})
}

func TestEqual(t *testing.T) {
	t.Run("both nil", func(t *testing.T) {
		if !Equal[int](nil, nil) {
			t.Fatal("expected both nil to be equal")
		}
	})

	t.Run("first nil", func(t *testing.T) {
		b := 1
		if Equal(nil, &b) {
			t.Fatal("expected nil vs non-nil to be unequal")
		}
	})

	t.Run("second nil", func(t *testing.T) {
		a := 1
		if Equal(&a, nil) {
			t.Fatal("expected non-nil vs nil to be unequal")
		}
	})

	t.Run("same values", func(t *testing.T) {
		a, b := 5, 5
		if !Equal(&a, &b) {
			t.Fatal("expected equal values to be equal")
		}
	})

	t.Run("different values", func(t *testing.T) {
		a, b := 5, 6
		if Equal(&a, &b) {
			t.Fatal("expected different values to be unequal")
		}
	})

	t.Run("same pointer", func(t *testing.T) {
		a := 5
		if !Equal(&a, &a) {
			t.Fatal("expected same pointer to be equal")
		}
	})
}
