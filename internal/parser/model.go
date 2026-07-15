package parser

import "nschugorev/oapigenerator/internal/codegen/gogen"

// Model — доменная модель схем сервиса. Содержит schemas, Import (Go-импорт
// model-пакета), Prefix (alias для common-проекта), и schemasIndex для
// быстрого Lookup по имени.
type Model struct {
	project      *Project
	Import       gogen.Import
	schemas      []*Schema
	schemasIndex map[string]*Schema // строится Index()
	Prefix       string             // для common: "common" alias в импортах
}

// Schemas возвращает схемы сервиса. До заполнения ProjectLoader'ом — nil.
func (m *Model) Schemas() []*Schema { return m.schemas }

// Index строит schemasIndex по m.schemas. Идемпотентен. Должен вызываться
// после заполнения schemas (в ProjectLoader.Load).
func (m *Model) Index() {
	m.schemasIndex = make(map[string]*Schema, len(m.schemas))
	for _, s := range m.schemas {
		if s.Name == "" {
			continue
		}

		m.schemasIndex[s.Name] = s
	}
}

// Lookup возвращает схему по имени. Требует предварительного вызова Index().
// Второе возвращаемое — false если Index не вызван или имя не найдено.
func (m *Model) Lookup(name string) (*Schema, bool) {
	if m.schemasIndex == nil {
		return nil, false
	}

	s, ok := m.schemasIndex[name]

	return s, ok
}
