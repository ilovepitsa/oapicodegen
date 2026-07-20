// Package schema: URLFormRenderer рендерит MarshalURLForm/UnmarshalURLForm методы
// для object-схем, на которые ссылается form-urlencoded request body какой-то
// операции.
//
// Поддерживаются только примитивные поля (string/integer/number/boolean +
// date-time/date). Arrays/maps/$ref/optional.Optional[T]/binary →
// сгенерированный метод возвращает runtime-ошибку.
//
// Портирован из Generator.urlFormMethodsFile + renderMarshalURLForm +
// renderUnmarshalURLForm + хелперов (internal/generator/url_form_methods.go).
// Старый путь удаляется из generator.go (urlFormMethodsFile), сам файл
// url_form_methods.go остаётся — schemeHasURLFormat/requestBodyIsURLForm
// используются Generator'ом для условия и impl-генераторами.
//
// Renderer embed'ит render.Base (Buf/Imports/Ctx) и walk.NoopSchemaRenderer.
// Skip не реализуется — aux-файл рендерится отдельным pack'ом из одного
// URLFormRenderer, descendants не обходятся.
package schema

import (
	"strconv"
	"strings"

	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/generator/walk"
	"nschugorev/oapigenerator/internal/parser"
)

// URLFormRenderer рендерит MarshalURLForm/UnmarshalURLForm методы для object-схем.
// Срабатывает на OnStruct/OnSplitStruct — оба делегируют в renderURLForm с
// базовым именем <Name>: form body ссылается на схему по её базовому $ref-имени,
// суффиксы Request/Response для url_form не нужны.
type URLFormRenderer struct {
	render.Base
	walk.NoopSchemaRenderer
}

// NewURLFormRenderer возвращает URLFormRenderer с нулевым состоянием. Buf и
// Imports вливаются через Base.Init в compose.FileComposer перед обходом.
func NewURLFormRenderer() *URLFormRenderer { return &URLFormRenderer{} }

// OnStruct рендерит MarshalURLForm + UnmarshalURLForm для основной <Name>-
// структуры. mode typeMapper'а выставляется в "" — url_form рендерится для
// базового имени, суффиксы splittable-схем не нужны.
func (r *URLFormRenderer) OnStruct(s *parser.Schema) error {
	defer r.Ctx.TypeMapper.SetMode("")
	r.Ctx.TypeMapper.SetMode("")

	r.renderURLForm(s, goName(s.Name))

	return nil
}

// OnSplitStruct делегирует в renderURLForm с базовым именем <Name>. Form body
// ссылается на схему по её базовому $ref-имени — рендерить методы на
// <Name>Request/<Name>Response не нужно (тела формы парсятся в моно-структуру).
func (r *URLFormRenderer) OnSplitStruct(s *parser.Schema) error {
	defer r.Ctx.TypeMapper.SetMode("")
	r.Ctx.TypeMapper.SetMode("")

	r.renderURLForm(s, goName(s.Name))

	return nil
}

// renderURLForm рендерит MarshalURLForm и UnmarshalURLForm последовательно
// в общий Buf.
func (r *URLFormRenderer) renderURLForm(s *parser.Schema, name string) {
	r.renderMarshalURLForm(s, name)
	r.renderUnmarshalURLForm(s, name)
}

// renderMarshalURLForm рендерит `func (m <Name>) MarshalURLForm() (url.Values, error)`.
//
// Если хотя бы одно поле не поддерживается url-form encoding — метод сразу
// возвращает ошибку с именем первого unsupported поля (dead-code-безопасно:
// return стоит до encode-блока).
func (r *URLFormRenderer) renderMarshalURLForm(s *parser.Schema, name string) {
	r.Imports.Add(gogen.Import{Path: "net/url"})
	r.Imports.Add(gogen.Import{Path: "fmt"})

	r.Buf.Print("func (m ", name, ") MarshalURLForm() (url.Values, error) {\n")

	if unsupported := r.firstUnsupportedURLFormField(s); unsupported != "" {
		msg := "field " + unsupported + ": url-form encoding not supported"
		r.Buf.Print("\treturn nil, fmt.Errorf(", strconv.Quote(msg), ")\n")
		r.Buf.Print("}\n\n")

		return
	}

	r.Buf.Print("\tvalues := url.Values{}\n")

	for _, p := range s.Properties {
		if p.Schema == nil {
			continue
		}

		r.renderMarshalField(p)
	}

	r.Buf.Print("\treturn values, nil\n")
	r.Buf.Print("}\n\n")
}

// renderMarshalField рендерит encoding одного поля.
//
//	required value: values.Set("<name>", <converter>(m.Field))
//	pointer (*T):   if m.Field != nil { values.Set("<name>", <converter>(*m.Field)) }
//
// Pointer возникает для nullable-полей и optional non-nullable полей.
func (r *URLFormRenderer) renderMarshalField(p *parser.Property) {
	fieldName := goName(p.Name)
	fieldType := r.Ctx.TypeMapper.GoType(p.Schema)
	required := r.requiredForMode(p)

	// Replicate renderField's pointer-wrapping: optional non-nullable → *T.
	if fieldIsOptional(required, fieldType) {
		fieldType = "*" + fieldType
	}

	pointer := strings.HasPrefix(fieldType, "*")
	converter := r.marshalConverter(p.Schema, fieldName, pointer)

	if pointer {
		r.Buf.Print("\tif m.", fieldName, " != nil {\n")
		r.Buf.Print("\t\tvalues.Set(", strconv.Quote(p.Name), ", ", converter, ")\n")
		r.Buf.Print("\t}\n")

		return
	}

	r.Buf.Print("\tvalues.Set(", strconv.Quote(p.Name), ", ", converter, ")\n")
}

// marshalConverter возвращает Go-выражение, конвертирующее поле в string.
// pointer=true — поле *T, converter должен разыменовать (*m.Field).
func (r *URLFormRenderer) marshalConverter(s *parser.Schema, fieldName string, pointer bool) string {
	accessor := "m." + fieldName
	if pointer {
		accessor = "*" + accessor
	}

	switch s.Type {
	case oapiTypeString:
		return r.marshalStringConverter(s, accessor)
	case oapiTypeInteger:
		r.Imports.Add(gogen.Import{Path: "strconv"})

		return "strconv.FormatInt(int64(" + accessor + "), 10)"
	case oapiTypeNumber:
		r.Imports.Add(gogen.Import{Path: "strconv"})

		if s.Format == oapiFormatFloat {
			return "strconv.FormatFloat(float64(" + accessor + "), 'f', -1, 32)"
		}

		return "strconv.FormatFloat(float64(" + accessor + "), 'f', -1, 64)"
	case oapiTypeBoolean:
		r.Imports.Add(gogen.Import{Path: "strconv"})

		return "strconv.FormatBool(" + accessor + ")"
	}

	return accessor
}

// marshalStringConverter возвращает Go-выражение для string-поля с учётом
// format (date-time/date). Для date-time при включённом USE_UTC_FOR_DATE_TIME
// конвертация идёт через time.Time(...).UTC().Format(...).
func (r *URLFormRenderer) marshalStringConverter(s *parser.Schema, accessor string) string {
	switch s.Format {
	case oapiFormatDateTime:
		r.Imports.Add(gogen.Import{Path: "time"})

		if r.Ctx.Features.UseUTCForDateTime.Value {
			return "time.Time(" + accessor + ").UTC().Format(time.RFC3339)"
		}

		return accessor + ".Format(time.RFC3339)"
	case oapiFormatDate:
		r.Imports.Add(gogen.Import{Path: "time"})

		if r.Ctx.Features.UseUTCForDateTime.Value {
			return "time.Time(" + accessor + ").UTC().Format(time.DateOnly)"
		}

		return accessor + ".Format(time.DateOnly)"
	default:
		return accessor
	}
}

// firstUnsupportedURLFormField возвращает Go-имя первого поля, не
// поддерживаемого url-form encoding. "" — все поля поддерживаются.
func (r *URLFormRenderer) firstUnsupportedURLFormField(s *parser.Schema) string {
	for _, p := range s.Properties {
		if p.Schema == nil {
			continue
		}

		if !r.urlFormFieldSupported(p) {
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
func (r *URLFormRenderer) urlFormFieldSupported(p *parser.Property) bool {
	if p.Schema == nil {
		return false
	}

	if r.Ctx.Features.UseOptional.Value && p.Optional {
		return false
	}

	if !urlFormSchemaSupported(p.Schema) {
		return false
	}

	return urlFormPrimitiveSupported(p.Schema)
}

// urlFormSchemaSupported — pure-проверка, что схема не ссылается на другую
// ($ref/composite) и не является array/object-with-properties/map.
func urlFormSchemaSupported(s *parser.Schema) bool {
	if s.Ref != "" || len(s.OneOf) > 0 || len(s.AnyOf) > 0 || len(s.AllOf) > 0 {
		return false
	}

	if s.Type == oapiTypeArray || s.AdditionalProperties != nil || s.AdditionalPropertiesFalse {
		return false
	}

	if s.Type == oapiTypeObject && len(s.Properties) > 0 {
		return false
	}

	return true
}

// urlFormPrimitiveSupported — pure-проверка примитивного типа. binary не
// поддерживается (форматирует []byte, для url-form нужен string).
func urlFormPrimitiveSupported(s *parser.Schema) bool {
	switch s.Type {
	case oapiTypeString:
		return s.Format != oapiFormatBinary
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
func (r *URLFormRenderer) renderUnmarshalURLForm(s *parser.Schema, name string) {
	r.Imports.Add(gogen.Import{Path: "net/url"})
	r.Imports.Add(gogen.Import{Path: "fmt"})

	r.Buf.Print("func (m *", name, ") UnmarshalURLForm(values url.Values) error {\n")

	if unsupported := r.firstUnsupportedURLFormField(s); unsupported != "" {
		msg := "field " + unsupported + ": url-form decoding not supported"
		r.Buf.Print("\treturn fmt.Errorf(", strconv.Quote(msg), ")\n")
		r.Buf.Print("}\n\n")

		return
	}

	for _, p := range s.Properties {
		if p.Schema == nil {
			continue
		}

		r.renderUnmarshalField(p)
	}

	r.Buf.Print("\treturn nil\n")
	r.Buf.Print("}\n\n")
}

// renderUnmarshalField рендерит декодирование одного поля из url.Values.
func (r *URLFormRenderer) renderUnmarshalField(p *parser.Property) {
	fieldName := goName(p.Name)
	fieldType := r.Ctx.TypeMapper.GoType(p.Schema)
	required := r.requiredForMode(p)

	if fieldIsOptional(required, fieldType) {
		fieldType = "*" + fieldType
	}

	if strings.HasPrefix(fieldType, "*") {
		r.renderUnmarshalPointerField(p, fieldName)

		return
	}

	r.renderUnmarshalValueField(p, fieldName)
}

// renderUnmarshalValueField рендерит декодирование required (non-pointer) поля.
func (r *URLFormRenderer) renderUnmarshalValueField(p *parser.Property, fieldName string) {
	getCall := "values.Get(" + strconv.Quote(p.Name) + ")"

	if p.Schema.Type == oapiTypeString && !stringHasFormat(p.Schema) {
		r.Buf.Print("\tm.", fieldName, " = ", getCall, "\n")

		return
	}

	dec := r.unmarshalDecoder(p.Schema, getCall)
	r.Buf.Print("\tparsed, err := ", dec.parseCall, "\n")
	r.Buf.Print("\tif err != nil {\n")
	r.Buf.Print("\t\treturn err\n")
	r.Buf.Print("\t}\n")
	r.Buf.Print("\tm.", fieldName, " = ", dec.assignExpr, "\n")
}

// renderUnmarshalPointerField рендерит декодирование nullable/optional поля:
// пустое значение в форме пропускается (поле остаётся nil).
func (r *URLFormRenderer) renderUnmarshalPointerField(p *parser.Property, fieldName string) {
	r.Buf.Print("\tif v := values.Get(", strconv.Quote(p.Name), "); v != \"\" {\n")

	if p.Schema.Type == oapiTypeString && !stringHasFormat(p.Schema) {
		r.Buf.Print("\t\tm.", fieldName, " = &v\n")
		r.Buf.Print("\t}\n")

		return
	}

	dec := r.unmarshalDecoder(p.Schema, "v")
	r.Buf.Print("\t\tparsed, err := ", dec.parseCall, "\n")
	r.Buf.Print("\t\tif err != nil {\n")
	r.Buf.Print("\t\t\treturn err\n")
	r.Buf.Print("\t\t}\n")

	if dec.assignExpr == "parsed" {
		r.Buf.Print("\t\tm.", fieldName, " = &parsed\n")
	} else {
		r.Buf.Print("\t\tconverted := ", dec.assignExpr, "\n")
		r.Buf.Print("\t\tm.", fieldName, " = &converted\n")
	}

	r.Buf.Print("\t}\n")
}

// stringHasFormat сообщает, есть ли у string-схемы format, требующий
// специального декодирования (date-time/date).
func stringHasFormat(s *parser.Schema) bool {
	return s.Type == oapiTypeString && (s.Format == oapiFormatDateTime || s.Format == oapiFormatDate)
}

// unmarshalParts — компоненты декодирования поля из string. parseCall —
// Go-выражение вида "<func>(<arg>)", возвращающее (T, error); arg подставляется
// из valueSource (values.Get(...) для value-field, v — для pointer-field).
// assignExpr — выражение для присваивания полю (может включать cast),
// использует переменную parsed, объявленную вызывающим кодом.
type unmarshalParts struct {
	parseCall  string
	assignExpr string
}

// unmarshalDecoder возвращает компоненты декодирования поля из string.
// valueSource — Go-выражение для исходной строки (values.Get(...) или v).
func (r *URLFormRenderer) unmarshalDecoder(s *parser.Schema, valueSource string) unmarshalParts {
	switch s.Type {
	case oapiTypeString:
		return r.unmarshalStringDecoder(s, valueSource)
	case oapiTypeInteger:
		r.Imports.Add(gogen.Import{Path: "strconv"})

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
		r.Imports.Add(gogen.Import{Path: "strconv"})

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
		r.Imports.Add(gogen.Import{Path: "strconv"})

		return unmarshalParts{
			parseCall:  "strconv.ParseBool(" + valueSource + ")",
			assignExpr: "parsed",
		}
	}

	return unmarshalParts{parseCall: valueSource, assignExpr: "parsed"}
}

// unmarshalStringDecoder возвращает компоненты для string-поля с учётом format
// (date-time/date). Для date-time при включённом USE_UTC_FOR_DATE_TIME
// результат оборачивается в UTCTime(parsed).
func (r *URLFormRenderer) unmarshalStringDecoder(s *parser.Schema, valueSource string) unmarshalParts {
	r.Imports.Add(gogen.Import{Path: "time"})

	switch s.Format {
	case oapiFormatDateTime:
		if r.Ctx.Features.UseUTCForDateTime.Value {
			return unmarshalParts{
				parseCall:  "time.Parse(time.RFC3339, " + valueSource + ")",
				assignExpr: "UTCTime(parsed)",
			}
		}

		return unmarshalParts{
			parseCall:  "time.Parse(time.RFC3339, " + valueSource + ")",
			assignExpr: "parsed",
		}
	case oapiFormatDate:
		return unmarshalParts{
			parseCall:  "time.Parse(time.DateOnly, " + valueSource + ")",
			assignExpr: "parsed",
		}
	}

	return unmarshalParts{parseCall: valueSource, assignExpr: "parsed"}
}

// requiredForMode делегирован в package-level requiredForMode (см. mode.go).
// Сохранён как метод-обёртка для симметрии с StructRenderer/SetDefaultsRenderer
// и удобства вызовов r.requiredForMode(p) внутри URLFormRenderer.
func (r *URLFormRenderer) requiredForMode(p *parser.Property) bool {
	return requiredForMode(r.Ctx, p)
}
