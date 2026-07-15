// Package parser — парсер стандартного OpenAPI 3.x в минимальный IR,
// используемый генератором. Поддерживаются только стандартные концепции:
// schemas (oneOf/anyOf/allOf/$ref/required/enum/format/default/nullable/
// deprecated), paths, operations, parameters, request bodies, responses.
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
	Operations []*Method
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
	Operations []*Method
}

// Method — HTTP-операция. Соответствует доменной модели Service/Method
// эталонного генератора (Operation в OpenAPI-терминологии).
type Method struct {
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

	service *Service // back-reference, выставляется при Paths.AddMethod
}

// ServiceName возвращает имя сервиса (тег), которому принадлежит метод.
// Nil-safe: возвращает "" если method ещё не привязан к Service.
func (m *Method) ServiceName() string {
	if m == nil || m.service == nil {
		return ""
	}

	return m.service.Name
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
	// RequestRequired true, если у property в spec стоит
	// x-request-required: true. Используется при USE_REQUIRED_V2.
	RequestRequired bool
	// ResponseRequired true, если у property в spec стоит
	// x-response-required: true. Используется при USE_REQUIRED_V2.
	ResponseRequired bool
	// Optional true, если у property в spec стоит x-optional: true.
	// Используется при GOLANG_USE_OPTIONAL для генерации
	// optional.Optional[T] вместо *T.
	Optional bool
	// Sensitive true, если у property в spec стоит x-sensitive: true.
	// Используется audit-data генератором: поле маскируется в audit-версии
	// через sensitive.Sensitive[T].
	Sensitive bool
	// Immutable true, если у property в spec стоит
	// x-validations: [Immutable]. Используется update-marker'ом: такие
	// поля не помечаются IsUsedInUpdate (кроме поля с именем "name").
	Immutable bool
	// IsUsedInUpdate true, если свойство участвует в PATCH/PUT-запросе.
	// Ставится update-marker'ом (см. update_marker.go).
	IsUsedInUpdate bool
	// Validations — правила валидации из x-validations (кроме Immutable,
	// который идёт в отдельное поле). Простые правила (">0", "Size <=10")
	// и именованные валидаторы ("cdn.EmailFormat").
	Validations []ValidationRule
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
	// AdditionalPropertiesFalse true, если в spec указано
	// additionalProperties: false — закрытая структура без доп. полей.
	AdditionalPropertiesFalse bool
	// IsUsedInUpdate true, если схема участвует в PATCH/PUT request body
	// какой-то операции. Ставится update-marker'ом (см. update_marker.go).
	IsUsedInUpdate bool
	// Validations — schema-level правила из x-validations. Только named
	// валидаторы (cross-field). Простые правила на уровне схемы не
	// поддерживаются — они идут на properties.
	Validations []ValidationRule
	// SourceFile — абсолютный путь к yaml-файлу, где определена top-level
	// схема (components.schemas). Заполняется в markExternalRefs. Пустой
	// для вложенных схем (properties, items, ...).
	SourceFile string
	// OwnerProject — проект-владелец top-level схемы. Заполняется в
	// markExternalRefs. Nil для вложенных схем.
	OwnerProject *Project
	// ExternalRef — заполнен, если эта Schema является $ref на схему из
	// другого сервиса. Содержит абсолютный путь к файлу целевой схемы +
	// фрагмент (например "/input/common/src/openapi/openapi.yaml#/components/schemas/User").
	// Пустой для локальных $ref.
	ExternalRef string
}

// ValidationRule — абстрактное правило валидации из x-validations.
// Реализации: SimpleRule (">0", "Size <=10") и NamedRule
// ("cdn.EmailFormat").
type ValidationRule interface {
	isValidationRule()
}

// Target — что сравнивать в SimpleRule.
type Target int

const (
	// TargetValue — сравнение самого значения (для чисел).
	TargetValue Target = iota
	// TargetSize — сравнение len() для slice/string/map. "Length" в spec
	// алиас для Size — нормализуется в TargetSize.
	TargetSize
)

// Operator — оператор сравнения в SimpleRule.
type Operator int

const (
	OpGT Operator = iota // >
	OpGE                 // >=
	OpLT                 // <
	OpLE                 // <=
	OpEQ                 // ==
	OpNE                 // !=
)

// SimpleRule — простое правило валидации: числовое сравнение или
// сравнение len(). Генерируется как inline if-проверка.
type SimpleRule struct {
	Target Target
	Op     Operator
	Value  float64
}

func (SimpleRule) isValidationRule() {}

// NamedRule — именованный валидатор. Имя вида "pkg.Name" — lookup в
// validator.Registry при вызове ValidateOwn.
type NamedRule struct {
	Name string
}

func (NamedRule) isValidationRule() {}

// Parse парсит OpenAPI 3.x документ из байтов.
func Parse(data []byte) (*Document, error) {
	return parseBytes(data, "", nil)
}

// ParseFile парсит OpenAPI 3.x файл из fsys. path — относительный путь.
func ParseFile(fsys fs.FS, path string) (*Document, error) {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, fmt.Errorf("read spec %q: %w", path, err)
	}

	return parseBytes(data, path, fsys)
}
