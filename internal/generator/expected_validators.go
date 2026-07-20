package generator

import (
	"fmt"
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/parser"
	"strconv"
)

// writeExpectedValidatorsFile пишет model/expected_validators.gen.go, если
// в документе есть хотя бы один named-валидатор. Вызывается один раз за
// генерацию.
func (g *Generator) writeExpectedValidatorsFile(fw codegen.FileWriter) error {
	file, ok := g.expectedValidatorsFile()
	if !ok {
		return nil
	}

	const fname = "model/expected_validators.gen.go"

	if err := fw.WriteFile(fname, file); err != nil {
		return fmt.Errorf("write %s: %w", fname, err)
	}

	return nil
}

// expectedValidatorsFile генерирует model/expected_validators.gen.go —
// функцию ExpectedValidatorNames(), возвращающую отсортированный
// уникальный список имён named-валидаторов, на которые ссылаются
// x-validations во всех схемах документа.
//
// Файл генерируется только если есть хотя бы один named-валидатор.
// Используется приложением при старте: validator.Registry.AssertExact(
// model.ExpectedValidatorNames()) падает, если какой-то валидатор не
// зарегистрирован или зарегистрирован лишний.
func (g *Generator) expectedValidatorsFile() (codegen.File, bool) {
	names := collectExpectedValidatorNames(g.project.Model.Schemas())
	if len(names) == 0 {
		return nil, false
	}

	w := codegen.NewBufferWriter()
	w.Print("// ExpectedValidatorNames возвращает отсортированный уникальный список\n")
	w.Print("// имён named-валидаторов, на которые ссылаются x-validations схем.\n")
	w.Print("// Используется с validator.Registry.AssertExact при старте приложения\n")
	w.Print("// для проверки, что все валидаторы зарегистрированы.\n")
	w.Print("func ExpectedValidatorNames() []string {\n")
	w.Print("\treturn []string{\n")

	for _, n := range names {
		w.Print("\t\t", strconv.Quote(n), ",\n")
	}

	w.Print("\t}\n")
	w.Print("}\n")

	return g.factory.Create(&gogen.File{
		Package: "model",
		Body:    w.Content(),
	}), true
}

// collectExpectedValidatorNames собирает уникальные имена именованных
// валидаторов со всех схем документа (property-level + schema-level).
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

func sortStrings(s []string) []string {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}

	return s
}
