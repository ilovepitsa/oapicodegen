// Package optional предоставляет обёртку Optional[T] над значениями, которые
// могут быть явно заданы или отсутствовать. Используется сгенерированным кодом
// для optional-полей (x-optional): отличает "поле не задано" от "поле задано
// значением" и "поле явно задано как null".
//
// Семантика JSON:
//   - не set → MarshalJSON возвращает null.
//   - set с значением → MarshalJSON возвращает JSON-представление value.
//   - SetToNil → MarshalJSON возвращает null, IsSet() == true.
//
// UnmarshalJSON:
//   - null → SetToNil (set=true, value=zero, isNil=true).
//   - не null → декодирует в value, set=true, isNil=false.
//   - пустой вызов (len(data)==0) → не set.
package optional

import (
	"encoding/json"
	"fmt"
)

// Optional — обёртка над значением T, различающая три состояния:
//   - не задано (zero value): set==false.
//   - задано значением: set==true, isNil==false.
//   - явно задано как null: set==true, isNil==true.
type Optional[T any] struct {
	set   bool
	isNil bool
	value T
}

// New создаёт Optional с явно заданным значением v: IsSet()==true, Value()==v.
func New[T any](v T) Optional[T] {
	return Optional[T]{
		set:   true,
		isNil: false,
		value: v,
	}
}

// NewEmpty создаёт Optional в состоянии "не задано". Эквивалентно zero value
// Optional[T]; предоставлен для явности.
func NewEmpty[T any]() Optional[T] {
	return Optional[T]{}
}

// IsSet возвращает true, если значение было явно задано (через New, SetTo или
// SetToNil, либо через успешный UnmarshalJSON).
func (o Optional[T]) IsSet() bool {
	return o.set
}

// IsNil возвращает true, если значение было явно задано как null (SetToNil
// или JSON null при UnmarshalJSON). Для не-set состояния возвращает false.
// Используется update-getter'ами (Get<Field>) чтобы отличать "поле
// прислали null" от "поле не прислали".
func (o Optional[T]) IsNil() bool {
	return o.set && o.isNil
}

// Value возвращает хранимое значение. Если Optional не set или SetToNil,
// возвращает zero T.
func (o Optional[T]) Value() T {
	return o.value
}

// ValueOr возвращает хранимое значение, если set и не isNil; иначе — def.
// Для SetToNil возвращает def, так как значение явно null.
func (o Optional[T]) ValueOr(def T) T {
	if !o.set || o.isNil {
		return def
	}

	return o.value
}

// SetTo явно задаёт значение v: set=true, isNil=false, value=v.
func (o *Optional[T]) SetTo(v T) {
	o.set = true
	o.isNil = false
	o.value = v
}

// SetToNil помечает Optional как set=true с zero-значением T и isNil=true.
// Семантика: "поле присутствует, но явно задано как null".
func (o *Optional[T]) SetToNil() {
	o.set = true
	o.isNil = true

	var zero T
	o.value = zero
}

// Unset сбрасывает Optional: set=false, isNil=false, value=zero T.
func (o *Optional[T]) Unset() {
	o.set = false
	o.isNil = false

	var zero T
	o.value = zero
}

// MarshalJSON сериализует Optional.
//   - не set → null.
//   - SetToNil (isNil) → null.
//   - иначе — JSON-представление value.
func (o Optional[T]) MarshalJSON() ([]byte, error) {
	if !o.set || o.isNil {
		return []byte("null"), nil
	}

	data, err := json.Marshal(o.value)
	if err != nil {
		return nil, fmt.Errorf("optional.Optional.MarshalJSON: %w", err)
	}

	return data, nil
}

// UnmarshalJSON десериализует Optional.
//   - пустой вызов (len(data)==0) → не set.
//   - null → SetToNil (set=true, value=zero, isNil=true).
//   - не null → декодирует в value, set=true, isNil=false.
func (o *Optional[T]) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		o.Unset()

		return nil
	}

	if string(data) == "null" {
		o.SetToNil()

		return nil
	}

	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("optional.Optional.UnmarshalJSON: %w", err)
	}

	o.SetTo(v)

	return nil
}
