// Package sensitive предоставляет обёртку Sensitive[T] для полей, которые
// не должны попадать в audit-логи в открытом виде. MarshalJSON всегда
// возвращает маску "***" независимо от значения; UnmarshalJSON декодирует
// как обычно (чтобы тесты могли загружать реальные значения).
//
// Это маска, а не крипто-хеш: если потребуется хеширование или
// токенизация — расширить реализацию, не меняя API.
package sensitive

import (
	"encoding/json"
	"fmt"
)

// maskedValue — JSON-представление любого sensitive-поля.
const maskedValue = `"***"`

// Sensitive — обёртка над значением T, маскирующая его при JSON-маршалинге.
type Sensitive[T any] struct {
	value T
}

// New создаёт Sensitive с значением v.
func New[T any](v T) Sensitive[T] {
	return Sensitive[T]{value: v}
}

// Value возвращает исходное (немаскированное) значение.
func (s Sensitive[T]) Value() T {
	return s.value
}

// MarshalJSON всегда возвращает маску "***".
func (s Sensitive[T]) MarshalJSON() ([]byte, error) {
	return []byte(maskedValue), nil
}

// UnmarshalJSON декодирует data в исходное значение (без маскирования).
func (s *Sensitive[T]) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &s.value); err != nil {
		return fmt.Errorf("sensitive: unmarshal: %w", err)
	}

	return nil
}
