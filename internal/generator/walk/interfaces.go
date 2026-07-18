// Package walk содержит recursive walker'ы для доменной модели parser.Schema
// и parser.Method. Walker'ы не делают рендер — только обход и диспатч хуков
// в зарегистрированные renderer'ы.
package walk

import (
	"nschugorev/oapigenerator/internal/parser"
)

// UnionKind различает oneOf и anyOf.
type UnionKind int

const (
	UnionOneOf UnionKind = iota
	UnionAnyOf
)

// SchemaRenderer — reactive-интерфейс: renderer'ы реализуют нужные методы,
// остальные наследуют от noopSchemaRenderer.
//
//nolint:interfacebloat // 13 hooks mandated by spec; splitting breaks the reactive pattern
type SchemaRenderer interface {
	OnStruct(s *parser.Schema) error
	OnEnum(s *parser.Schema) error
	OnAlias(s *parser.Schema) error
	OnArray(s *parser.Schema) error
	OnMap(s *parser.Schema) error
	OnUnion(s *parser.Schema, kind UnionKind) error
	OnAllOf(s *parser.Schema) error
	OnSplitStruct(s *parser.Schema) error

	OnStructProperty(s *parser.Schema, name string, prop *parser.Schema) error
	OnArrayItem(s *parser.Schema, idx int, item *parser.Schema) error
	OnMapValue(s *parser.Schema, value *parser.Schema) error
	OnUnionVariant(s *parser.Schema, idx int, variant *parser.Schema) error
	OnAllOfMember(s *parser.Schema, idx int, member *parser.Schema) error
}

// SkipDescendants — optional-интерфейс. Renderer реализует его, чтобы
// попросить walker не спускаться в дочерние схемы (например, для external $ref).
type SkipDescendants interface {
	Skip(s *parser.Schema) bool
}

// MethodRenderer — reactive-интерфейс для обхода операций.
//
// OnRequestBody получает *parser.RequestBody целиком: тело содержит
// Content map[mediaType]*MediaType, и выбор media-type — ответственность
// renderer'а (walker не знает, какой тип предпочтителен). Renderer сам
// достаёт Schema из body.Content[mt].Schema и при необходимости передаёт
// её в SchemaWalker.
//
// OnResponse/OnResponseHeader получают code как string: parser.Response.
// StatusCode хранит строку из spec ("200", "4XX", "default"), и приведение
// к int потеряло бы "default" и шаблоны. Renderer сам парсит код, если ему
// нужен int.
//
// OnResponseHeader получает response header как *parser.Parameter: в текущей
// модели parser.Response.Headers имеет тип map[string]*parser.Parameter
// (отдельного типа parser.Header нет).
type MethodRenderer interface {
	OnMethod(m *parser.Method) error
	OnPathParameter(m *parser.Method, p *parser.Parameter) error
	OnQueryParameter(m *parser.Method, p *parser.Parameter) error
	OnHeaderParameter(m *parser.Method, p *parser.Parameter) error
	OnCookieParameter(m *parser.Method, p *parser.Parameter) error
	OnRequestBody(m *parser.Method, body *parser.RequestBody) error
	OnResponse(m *parser.Method, code string, resp *parser.Response) error
	OnResponseHeader(m *parser.Method, code string, name string, h *parser.Parameter) error
}
