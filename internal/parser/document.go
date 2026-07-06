// Package parser — парсер стандартного OpenAPI 3.x в минимальный IR,
// используемый генератором. Поддерживаются только стандартные концепции:
// schemas (oneOf/anyOf/allOf/$ref/required/enum/format/default/nullable/
// deprecated), paths, operations, parameters, request bodies, responses.
//
// MWS-специфика (x-* расширения, audit-data, split Request/Response,
// update-схемы, common-схемы) намеренно отсутствует — бэклог.
package parser

import (
	"fmt"
	"io/fs"
)

// Document — распарсенный OpenAPI 3.x документ.
type Document struct {
	OpenAPI    string
	Info       Info
	Servers    []Server
	Paths      []*PathItem
	Schemas    []*Schema
	Operations []*Operation
}

// Info — секция info.
type Info struct {
	Title       string
	Description string
	Version     string
}

// Server — секция servers.
type Server struct {
	URL         string
	Description string
}

// PathItem — один path со всеми его операциями.
type PathItem struct {
	Path       string
	Operations []*Operation
}

// Operation — HTTP-операция.
type Operation struct {
	Method      string
	Path        string
	OperationID string
	Summary     string
	Description string
	Tags        []string
	Deprecated  bool
	Parameters  []*Parameter
	RequestBody *RequestBody
	Responses   []*Response
}

// Parameter — query/path/header/cookie-параметр.
type Parameter struct {
	Name        string
	In          string
	Description string
	Required    bool
	Deprecated  bool
	Schema      *Schema
}

// RequestBody — тело запроса.
type RequestBody struct {
	Description string
	Required    bool
	Content     map[string]*MediaType
}

// Response — HTTP-ответ. StatusCode — строка из spec ("200", "4XX", "default").
type Response struct {
	StatusCode  string
	Description string
	Content     map[string]*MediaType
	Headers     map[string]*Parameter
}

// MediaType — содержимое тела/ответа по media-type.
type MediaType struct {
	Schema *Schema
}

// Property — свойство объектного типа.
type Property struct {
	Name     string
	Schema   *Schema
	Required bool
}

// Schema — минимальное представление JSON Schema / OpenAPI Schema.
//
// Ref непустой, если схема — ссылка на другую ($ref). При этом остальные
// поля могут быть заполнены из целевой схемы (если удалось разрешить).
type Schema struct {
	Name                 string
	Description          string
	Type                 string
	Format               string
	Properties           []*Property
	Required             []string
	Items                *Schema
	Enum                 []any
	Default              any
	Nullable             bool
	Deprecated           bool
	ReadOnly             bool
	WriteOnly            bool
	AllOf                []*Schema
	OneOf                []*Schema
	AnyOf                []*Schema
	Ref                  string
	AdditionalProperties *Schema
}

// Parse парсит OpenAPI 3.x документ из байтов.
func Parse(data []byte) (*Document, error) {
	return parseBytes(data, "")
}

// ParseFile парсит OpenAPI 3.x файл из fsys. path — относительный путь.
func ParseFile(fsys fs.FS, path string) (*Document, error) {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, fmt.Errorf("read spec %q: %w", path, err)
	}

	return parseBytes(data, path)
}
