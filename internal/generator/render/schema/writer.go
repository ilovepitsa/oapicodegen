package schema

// writer — минимальный интерфейс записи, который удовлетворяет
// *codegen.BufferWriter. Вынесен, чтобы naming.go не зависел от codegen.
type writer interface {
	Print(args ...any)
}
