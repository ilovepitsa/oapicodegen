package parser

import "strings"

// Service — группа методов, объединённых по тегу (op.Tags[0]).
// Используется генератором для эмитции server interfaces по сервисам.
type Service struct {
	Name    string
	Methods []*Method

	paths *Paths // back-reference
}

// LowerName возвращает имя сервиса в нижнем регистре — используется для
// имен файлов и Go-идентификаторов где важен lowercase.
func (s *Service) LowerName() string {
	return strings.ToLower(s.Name)
}
