package parser

import "slices"

// Paths — доменная модель операций сервиса. Содержит PathImports (типизированные
// Go-импорты артефактов) и Services (методы, сгруппированные по тегу).
type Paths struct {
	Imports  PathImports
	Services []*Service

	servicesMap map[string]*Service
	project     *Project
}

// AddMethod привязывает метод к сервису с заданным именем. Если сервис
// ещё не существует — создаёт. Выставляет method.service back-reference.
func (p *Paths) AddMethod(serviceName string, m *Method) {
	if p.servicesMap == nil {
		p.servicesMap = map[string]*Service{}
	}
	s := p.servicesMap[serviceName]
	if s == nil {
		s = &Service{Name: serviceName, paths: p}
		p.servicesMap[serviceName] = s
		p.Services = append(p.Services, s)
	}
	s.Methods = append(s.Methods, m)
	m.service = s
}

// DeleteService удаляет сервис по имени. No-op если сервис не найден.
func (p *Paths) DeleteService(name string) {
	delete(p.servicesMap, name)
	p.Services = slices.DeleteFunc(p.Services, func(s *Service) bool {
		return s.Name == name
	})
}
