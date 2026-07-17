package generator

import (
	"fmt"
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
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
