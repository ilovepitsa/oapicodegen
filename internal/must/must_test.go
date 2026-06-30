package must

import (
	"errors"
	"testing"
)

func TestValue_NoError(t *testing.T) {
	t.Run("returns value when err is nil", func(t *testing.T) {
		v := Value(42, nil)
		if v != 42 {
			t.Fatalf("expected 42, got %d", v)
		}
	})

	t.Run("returns pointer value when err is nil", func(t *testing.T) {
		x := 7
		p := Value(&x, nil)
		if p == nil || *p != 7 {
			t.Fatalf("expected pointer to 7, got %v", p)
		}
	})

	t.Run("returns zero value when v is zero and err is nil", func(t *testing.T) {
		v := Value("", nil)
		if v != "" {
			t.Fatalf("expected empty string, got %q", v)
		}
	})

	t.Run("returns nil pointer when v is nil and err is nil", func(t *testing.T) {
		var p *int
		got := Value(p, nil)
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})
}

func TestValue_Error(t *testing.T) {
	t.Run("panics when err is non-nil", func(t *testing.T) {
		err := errors.New("boom")
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected panic, got none")
			}
			if r != err {
				t.Fatalf("expected panic with err %v, got %v", err, r)
			}
		}()
		_ = Value(42, err)
	})

	t.Run("panics with wrapped error", func(t *testing.T) {
		base := errors.New("base")
		wrapped := errors.Join(base, errors.New("context"))
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected panic, got none")
			}
			if r != wrapped {
				t.Fatalf("expected panic with wrapped err, got %v", r)
			}
		}()
		_ = Value("ignored", wrapped)
	})
}
