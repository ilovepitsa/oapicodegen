package render

import "sync"

// Diagnostic — одно сообщение аудита генерации (например, схема свелась к
// Go `any`). Severity проставляется потребителем (cmd/oapigen) по режиму
// флага GOLANG_SCHEMA_ANY (warn|error); здесь хранится только факт + место.
type Diagnostic struct {
	Location string // e.g. "components.schemas.Pet.properties.tag"
	Reason   string // e.g. "schema resolves to Go `any` (empty schema {})"
}

// Collector накапливает Diagnostic'и потокобезопасно (renderer'ы могут
// работать конкурентно). Drain возвращает копию и очищает накопленное.
type Collector struct {
	mu   sync.Mutex
	diag []Diagnostic
}

// NewCollector возвращает пустой Collector.
func NewCollector() *Collector { return &Collector{} }

// Append добавляет один Diagnostic.
func (c *Collector) Append(d Diagnostic) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.diag = append(c.diag, d)
}

// Drain возвращает накопленные Diagnostic'и и очищает коллектор.
func (c *Collector) Drain() []Diagnostic {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.diag
	c.diag = nil
	return out
}
