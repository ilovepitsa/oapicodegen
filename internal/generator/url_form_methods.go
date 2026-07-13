package generator

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/parser"
	"strconv"
	"strings"
)

// formURLContentType — media-type для URL-form encoded request body.
const formURLContentType = "application/x-www-form-urlencoded"

// urlFormMethodsFile генерирует model/<name>_url_form.gen.go с методами
// MarshalURLForm / UnmarshalURLForm для object-схемы, на которую ссылается
// form-urlencoded request body какой-то операции.
//
// Поддерживаются только примитивные поля (string/integer/number/boolean +
// date-time/date). Arrays/maps/$ref/optional.Optional[T]/binary →
// сгенерированный метод возвращает runtime-ошибку.
func (g *Generator) urlFormMethodsFile(sh *parser.Schema) codegen.File {
	m := g.newTypeMapper("model")
	w := codegen.NewBufferWriter()

	name := goName(sh.Name)
	g.renderMarshalURLForm(w, sh, m, name)
	g.renderUnmarshalURLForm(w, sh, m, name)

	return g.factory.Create(&gogen.File{
		Package: "model",
		Imports: m.imports,
		Body:    w.Content(),
	})
}

// schemeHasURLFormat сообщает, ссылается ли form-urlencoded request body
// какой-либо операции на схему sh (по $ref-имени). Inline-схемы без Name
// не поддерживаются — используйте $ref на components.schemas.
func schemeHasURLFormat(sh *parser.Schema, doc *parser.Document) bool {
	if sh == nil || sh.Name == "" {
		return false
	}

	for _, op := range doc.Operations {
		if op.RequestBody == nil {
			continue
		}

		mt, ok := op.RequestBody.Content[formURLContentType]
		if !ok || mt.Schema == nil {
			continue
		}

		if mt.Schema.Ref != "" && refToName(mt.Schema.Ref) == sh.Name {
			return true
		}
	}

	return false
}

// requestBodyIsURLForm сообщает, использует ли операция form-urlencoded
// request body (вместо application/json).
func requestBodyIsURLForm(rb *parser.RequestBody) bool {
	if rb == nil || rb.Content == nil {
		return false
	}

	_, ok := rb.Content[formURLContentType]

	return ok
}

// renderMarshalURLForm рендерит `func (m <Name>) MarshalURLForm() (url.Values, error)`.
//
// Если хотя бы одно поле не поддерживается url-form encoding — метод сразу
// возвращает ошибку с именем первого unsupported поля (dead-code-безопасно:
// return стоит до encode-блока).
func (g *Generator) renderMarshalURLForm(
	w *codegen.BufferWriter,
	sh *parser.Schema,
	m *typeMapper,
	name string,
) {
	m.addImport("net/url", "")
	m.addImport("fmt", "")

	w.Print("func (m ", name, ") MarshalURLForm() (url.Values, error) {\n")

	if unsupported := g.firstUnsupportedURLFormField(sh); unsupported != "" {
		msg := "field " + unsupported + ": url-form encoding not supported"
		w.Print("\treturn nil, fmt.Errorf(", strconv.Quote(msg), ")\n")
		w.Print("}\n\n")

		return
	}

	w.Print("\tvalues := url.Values{}\n")

	for _, p := range sh.Properties {
		if p.Schema == nil {
			continue
		}

		g.renderMarshalField(w, p, m)
	}

	w.Print("\treturn values, nil\n")
	w.Print("}\n\n")
}

// renderMarshalField рендерит encoding одного поля.
//
//	required value: values.Set("<name>", <converter>(m.Field))
//	pointer (*T):   if m.Field != nil { values.Set("<name>", <converter>(*m.Field)) }
//
// Pointer возникает для nullable-полей и optional non-nullable полей.
func (g *Generator) renderMarshalField(
	w *codegen.BufferWriter,
	p *parser.Property,
	m *typeMapper,
) {
	fieldName := goName(p.Name)
	fieldType := m.goType(p.Schema)
	required := g.requiredForMode(p, m.mode)

	// Replicate renderField's pointer-wrapping: optional non-nullable → *T.
	if fieldIsOptional(required, fieldType) {
		fieldType = "*" + fieldType
	}

	pointer := strings.HasPrefix(fieldType, "*")
	converter := marshalConverter(p.Schema, m, fieldName, pointer)

	if pointer {
		w.Print("\tif m.", fieldName, " != nil {\n")
		w.Print("\t\tvalues.Set(", strconv.Quote(p.Name), ", ", converter, ")\n")
		w.Print("\t}\n")

		return
	}

	w.Print("\tvalues.Set(", strconv.Quote(p.Name), ", ", converter, ")\n")
}

// marshalConverter возвращает Go-выражение, конвертирующее поле в string.
// pointer=true — поле *T, converter должен разыменовать (*m.Field).
func marshalConverter(s *parser.Schema, m *typeMapper, fieldName string, pointer bool) string {
	accessor := "m." + fieldName
	if pointer {
		accessor = "*" + accessor
	}

	switch s.Type {
	case oapiTypeString:
		return marshalStringConverter(s, m, accessor)
	case oapiTypeInteger:
		m.addImport("strconv", "")

		return "strconv.FormatInt(int64(" + accessor + "), 10)"
	case oapiTypeNumber:
		m.addImport("strconv", "")

		if s.Format == oapiFormatFloat {
			return "strconv.FormatFloat(float64(" + accessor + "), 'f', -1, 32)"
		}

		return "strconv.FormatFloat(float64(" + accessor + "), 'f', -1, 64)"
	case oapiTypeBoolean:
		m.addImport("strconv", "")

		return "strconv.FormatBool(" + accessor + ")"
	}

	return accessor
}

func marshalStringConverter(s *parser.Schema, m *typeMapper, accessor string) string {
	switch s.Format {
	case "date-time":
		m.addImport("time", "")

		if m.utcTime {
			return "time.Time(" + accessor + ").UTC().Format(time.RFC3339)"
		}

		return accessor + ".Format(time.RFC3339)"
	case "date":
		m.addImport("time", "")

		if m.utcTime {
			return "time.Time(" + accessor + ").UTC().Format(time.DateOnly)"
		}

		return accessor + ".Format(time.DateOnly)"
	default:
		return accessor
	}
}

// firstUnsupportedURLFormField возвращает Go-имя первого поля, не
// поддерживаемого url-form encoding. "" — все поля поддерживаются.
func (g *Generator) firstUnsupportedURLFormField(sh *parser.Schema) string {
	for _, p := range sh.Properties {
		if p.Schema == nil {
			continue
		}

		if !g.urlFormFieldSupported(p) {
			return goName(p.Name)
		}
	}

	return ""
}

// urlFormFieldSupported проверяет, поддерживается ли поле url-form encoding.
//
// Поддерживаются: string (включая date-time/date), integer, number, boolean.
// Не поддерживаются: array, object, $ref, optional.Optional[T], binary ([]byte),
// oneOf/anyOf/allOf, additionalProperties (map).
func (g *Generator) urlFormFieldSupported(p *parser.Property) bool {
	if p.Schema == nil {
		return false
	}

	if g.features.UseOptional.Value && p.Optional {
		return false
	}

	if !urlFormSchemaSupported(p.Schema) {
		return false
	}

	return urlFormPrimitiveSupported(p.Schema)
}

func urlFormSchemaSupported(s *parser.Schema) bool {
	if s.Ref != "" || len(s.OneOf) > 0 || len(s.AnyOf) > 0 || len(s.AllOf) > 0 {
		return false
	}

	if s.Type == "array" || s.AdditionalProperties != nil || s.AdditionalPropertiesFalse {
		return false
	}

	if s.Type == oapiTypeObject && len(s.Properties) > 0 {
		return false
	}

	return true
}

func urlFormPrimitiveSupported(s *parser.Schema) bool {
	switch s.Type {
	case oapiTypeString:
		return s.Format != "binary"
	case oapiTypeInteger, oapiTypeNumber, oapiTypeBoolean:
		return true
	}

	return false
}

// renderUnmarshalURLForm рендерит `func (m *<Name>) UnmarshalURLForm(values url.Values) error`.
//
// Если хотя бы одно поле не поддерживается — метод сразу возвращает ошибку
// (dead-code-безопасно). Для pointer-полей (nullable/optional) пустое значение
// в форме пропускается (поле остаётся nil); для required-полей парсится
// напрямую.
func (g *Generator) renderUnmarshalURLForm(
	w *codegen.BufferWriter,
	sh *parser.Schema,
	m *typeMapper,
	name string,
) {
	m.addImport("net/url", "")
	m.addImport("fmt", "")

	w.Print("func (m *", name, ") UnmarshalURLForm(values url.Values) error {\n")

	if unsupported := g.firstUnsupportedURLFormField(sh); unsupported != "" {
		msg := "field " + unsupported + ": url-form decoding not supported"
		w.Print("\treturn fmt.Errorf(", strconv.Quote(msg), ")\n")
		w.Print("}\n\n")

		return
	}

	for _, p := range sh.Properties {
		if p.Schema == nil {
			continue
		}

		g.renderUnmarshalField(w, p, m)
	}

	w.Print("\treturn nil\n")
	w.Print("}\n\n")
}

// renderUnmarshalField рендерит декодирование одного поля из url.Values.
func (g *Generator) renderUnmarshalField(
	w *codegen.BufferWriter,
	p *parser.Property,
	m *typeMapper,
) {
	fieldName := goName(p.Name)
	fieldType := m.goType(p.Schema)
	required := g.requiredForMode(p, m.mode)

	if fieldIsOptional(required, fieldType) {
		fieldType = "*" + fieldType
	}

	if strings.HasPrefix(fieldType, "*") {
		g.renderUnmarshalPointerField(w, p, m, fieldName)

		return
	}

	g.renderUnmarshalValueField(w, p, m, fieldName)
}

// renderUnmarshalValueField рендерит декодирование required (non-pointer) поля.
func (g *Generator) renderUnmarshalValueField(
	w *codegen.BufferWriter,
	p *parser.Property,
	m *typeMapper,
	fieldName string,
) {
	getCall := "values.Get(" + strconv.Quote(p.Name) + ")"

	if p.Schema.Type == oapiTypeString && !stringHasFormat(p.Schema) {
		w.Print("\tm.", fieldName, " = ", getCall, "\n")

		return
	}

	dec := unmarshalDecoder(p.Schema, m, getCall)
	w.Print("\tparsed, err := ", dec.parseCall, "\n")
	w.Print("\tif err != nil {\n")
	w.Print("\t\treturn err\n")
	w.Print("\t}\n")
	w.Print("\tm.", fieldName, " = ", dec.assignExpr, "\n")
}

// renderUnmarshalPointerField рендерит декодирование nullable/optional поля:
// пустое значение в форме пропускается (поле остаётся nil).
func (g *Generator) renderUnmarshalPointerField(
	w *codegen.BufferWriter,
	p *parser.Property,
	m *typeMapper,
	fieldName string,
) {
	w.Print("\tif v := values.Get(", strconv.Quote(p.Name), "); v != \"\" {\n")

	if p.Schema.Type == oapiTypeString && !stringHasFormat(p.Schema) {
		w.Print("\t\tm.", fieldName, " = &v\n")
		w.Print("\t}\n")

		return
	}

	dec := unmarshalDecoder(p.Schema, m, "v")
	w.Print("\t\tparsed, err := ", dec.parseCall, "\n")
	w.Print("\t\tif err != nil {\n")
	w.Print("\t\t\treturn err\n")
	w.Print("\t\t}\n")

	if dec.assignExpr == "parsed" {
		w.Print("\t\tm.", fieldName, " = &parsed\n")
	} else {
		w.Print("\t\tconverted := ", dec.assignExpr, "\n")
		w.Print("\t\tm.", fieldName, " = &converted\n")
	}

	w.Print("\t}\n")
}

// stringHasFormat сообщает, есть ли у string-схемы format, требующий
// специального декодирования (date-time/date).
func stringHasFormat(s *parser.Schema) bool {
	return s.Type == oapiTypeString && (s.Format == "date-time" || s.Format == "date")
}

// unmarshalDecoder возвращает компоненты декодирования поля из string.
// parseCall — Go-выражение вида "<func>(<arg>)", возвращающее (T, error);
// arg подставляется из valueSource (values.Get(...) для value-field, v — для pointer-field).
// assignExpr — выражение для присваивания полю (может включать cast),
// использует переменную parsed, объявленную вызывающим кодом.
type unmarshalParts struct {
	parseCall  string
	assignExpr string
}

func unmarshalDecoder(s *parser.Schema, m *typeMapper, valueSource string) unmarshalParts {
	switch s.Type {
	case oapiTypeString:
		return unmarshalStringDecoder(s, m, valueSource)
	case oapiTypeInteger:
		m.addImport("strconv", "")

		switch s.Format {
		case oapiFormatInt32:
			return unmarshalParts{
				parseCall:  "strconv.ParseInt(" + valueSource + ", 10, 32)",
				assignExpr: "int32(parsed)",
			}
		case oapiFormatInt64:
			return unmarshalParts{
				parseCall:  "strconv.ParseInt(" + valueSource + ", 10, 64)",
				assignExpr: "parsed",
			}
		default:
			return unmarshalParts{
				parseCall:  "strconv.Atoi(" + valueSource + ")",
				assignExpr: "parsed",
			}
		}
	case oapiTypeNumber:
		m.addImport("strconv", "")

		if s.Format == oapiFormatFloat {
			return unmarshalParts{
				parseCall:  "strconv.ParseFloat(" + valueSource + ", 32)",
				assignExpr: "float32(parsed)",
			}
		}

		return unmarshalParts{
			parseCall:  "strconv.ParseFloat(" + valueSource + ", 64)",
			assignExpr: "parsed",
		}
	case oapiTypeBoolean:
		m.addImport("strconv", "")

		return unmarshalParts{
			parseCall:  "strconv.ParseBool(" + valueSource + ")",
			assignExpr: "parsed",
		}
	}

	return unmarshalParts{parseCall: valueSource, assignExpr: "parsed"}
}

func unmarshalStringDecoder(s *parser.Schema, m *typeMapper, valueSource string) unmarshalParts {
	m.addImport("time", "")

	switch s.Format {
	case "date-time":
		if m.utcTime {
			return unmarshalParts{
				parseCall:  "time.Parse(time.RFC3339, " + valueSource + ")",
				assignExpr: "UTCTime(parsed)",
			}
		}

		return unmarshalParts{
			parseCall:  "time.Parse(time.RFC3339, " + valueSource + ")",
			assignExpr: "parsed",
		}
	case "date":
		return unmarshalParts{
			parseCall:  "time.Parse(time.DateOnly, " + valueSource + ")",
			assignExpr: "parsed",
		}
	}

	return unmarshalParts{parseCall: valueSource, assignExpr: "parsed"}
}
