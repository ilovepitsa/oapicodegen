// Package schema: ExpectedValidatorsRenderer — SingletonRenderer, рендерящий
// model/expected_validators.gen.go. Заменяет Generator.expectedValidatorsFile
// (internal/generator/expected_validators.go). Тело и хелперы
// (collectExpectedValidatorNames, sortStrings) портированы байт-в-байт.
//
// Renderer смотрит на r.Ctx.Project.Model.Schemas() — все схемы документа.
// Имена валидаторов собираются с schema-level (sh.Validations) и property-level
// (p.Validations). SimpleRule игнорируются, учитываются только NamedRule.
// Файл генерируется только если есть хотя бы один named-валидатор — иначе
// Render возвращает пустое body, а вызывающая сторона пропускает запись файла.
package schema

import (
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
	"strconv"
)

// ExpectedValidatorsRenderer рендерит model/expected_validators.gen.go.
type ExpectedValidatorsRenderer struct{}

// NewExpectedValidatorsRenderer строит ExpectedValidatorsRenderer.
func NewExpectedValidatorsRenderer() *ExpectedValidatorsRenderer {
	return &ExpectedValidatorsRenderer{}
}

// FilePath возвращает путь генерируемого файла.
func (ExpectedValidatorsRenderer) FilePath() string {
	return "model/expected_validators.gen.go"
}

// Render возвращает тело файла и (пустой) трекер импортов. Если ни одного
// named-валидатора нет — возвращает пустое body и nil-трекер; вызывающая
// сторона (Generator.writeExpectedValidatorsFile) пропускает запись файла.
func (ExpectedValidatorsRenderer) Render(ctx *render.RenderContext) ([]byte, *render.ImportTracker, error) {
	names := collectExpectedValidatorNames(schemasOf(ctx))
	if len(names) == 0 {
		return nil, nil, nil
	}

	var buf []byte
	buf = append(buf, []byte("// ExpectedValidatorNames возвращает отсортированный уникальный список\n")...)
	buf = append(buf, []byte("// имён named-валидаторов, на которые ссылаются x-validations схем.\n")...)
	buf = append(buf, []byte("// Используется с validator.Registry.AssertExact при старте приложения\n")...)
	buf = append(buf, []byte("// для проверки, что все валидаторы зарегистрированы.\n")...)
	buf = append(buf, []byte("func ExpectedValidatorNames() []string {\n")...)
	buf = append(buf, []byte("\treturn []string{\n")...)

	for _, n := range names {
		buf = append(buf, []byte("\t\t")...)
		buf = append(buf, []byte(strconv.Quote(n))...)
		buf = append(buf, ',', '\n')
	}

	buf = append(buf, []byte("\t}\n")...)
	buf = append(buf, []byte("}\n")...)

	return buf, render.NewImportTracker(), nil
}

// schemasOf безопасно достаёт схемы проекта из контекста. Возвращает nil при
// отсутствии Project или Model — покрывает тестовые ctx без проекта.
func schemasOf(ctx *render.RenderContext) []*parser.Schema {
	if ctx == nil || ctx.Project == nil || ctx.Project.Model == nil {
		return nil
	}

	return ctx.Project.Model.Schemas()
}

// collectExpectedValidatorNames собирает уникальные имена именованных
// валидаторов со всех схем документа (property-level + schema-level).
// Портировано из internal/generator/expected_validators.go.
func collectExpectedValidatorNames(schemas []*parser.Schema) []string {
	seen := make(map[string]bool)

	for _, sh := range schemas {
		for _, rule := range sh.Validations {
			if nr, ok := rule.(parser.NamedRule); ok {
				seen[nr.Name] = true
			}
		}

		for _, p := range sh.Properties {
			for _, rule := range p.Validations {
				if nr, ok := rule.(parser.NamedRule); ok {
					seen[nr.Name] = true
				}
			}
		}
	}

	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}

	return sortStrings(out)
}

// sortStrings сортирует срез строк in-place вставками. Портировано из
// internal/generator/expected_validators.go — сохранён алгоритм, чтобы
// гарантироать байт-в-байт совпадение вывода (стандартный sort.Strings может
// отличаться по стабильности для дубликатов, хотя здесь дубликатов нет).
func sortStrings(s []string) []string {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}

	return s
}

// compile-time guard: ExpectedValidatorsRenderer удовлетворяет SingletonRenderer.
var _ render.SingletonRenderer = (*ExpectedValidatorsRenderer)(nil)
