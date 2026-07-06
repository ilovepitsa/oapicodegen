// Package ptr предоставляет обёртки над указателями: создание указателя из
// значения (Get), разыменование с zero-фоллбэком (Value), копирование (Clone)
// и nil-aware сравнение (Equal). Замена git.mws-team.ru/mws/devp/platform-go/pkg/ptr.
package ptr

// Get возвращает указатель на v. Удобно для создания *T из литералов и
// значений, не подлежащих взятию адреса напрямую (например, ptr.Get(true)).
func Get[T any](v T) *T {
	return &v
}

// Value разыменовывает p. Если p == nil, возвращает zero-значение T.
func Value[T any](p *T) T {
	if p == nil {
		var zero T

		return zero
	}

	return *p
}

// Clone возвращает указатель на копию значения *p. Если p == nil, возвращает nil.
func Clone[T any](p *T) *T {
	if p == nil {
		return nil
	}

	v := *p

	return &v
}

// Equal сравнивает значения по указателям. Оба nil → true; ровно один nil →
// false; иначе сравниваются *a и *b.
func Equal[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}

	return *a == *b
}
