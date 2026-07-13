// Example: pattern регистрации валидаторов + startup-check.
//
// Демонстрирует полный lifecycle работы с x-validations:
//
//  1. Регистрация всех именованных валидаторов в Registry.
//  2. AssertExact при старте — fail-fast, если spec требует валидатор,
//     которого нет в registry (или наоборот).
//  3. Вызов validator.Validate на входящем объекте — walker обходит
//     struct-дерево и вызывает ValidateOwn на каждой структуре.
//
// Модель Pet ниже рукописная, но повторяет структуру сгенерированного кода
// (см. testdata/minimal/golden/model/item.gen.go для реального примера).
// В реальном приложении Pet генерируется oapigen из OpenAPI-спеки с
// x-validations, а валидаторы пишет пользователь.
package main

import (
	"errors"
	"fmt"
	"log"

	"nschugorev/oapigenerator/pkg/validator"
)

// Pet — пример сгенерированной модели. ValidateOwn содержит:
//   - inline-проверки (Size >=1, >0 и т.п.) — генерируются из простых правил;
//   - вызовы named-валидаторов через reg.Get(name) — генерируются из "pkg.Name".
//
// В реальном коде этот метод генерирует oapigen; здесь он написан руками
// для демонстрации pattern'а.
type Pet struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Tag  string `json:"tag"`
}

func (p Pet) ValidateOwn(reg *validator.Registry) error {
	if p.ID <= 0 {
		return fmt.Errorf("field ID: must be > 0")
	}

	if len(p.Name) < 1 {
		return fmt.Errorf("field Name: must be >= 1")
	}

	v, ok := reg.Get("app.NonEmptyName")
	if !ok {
		return fmt.Errorf("validator %q not registered", "app.NonEmptyName")
	}

	if err := v.Validate(p.Name); err != nil {
		return fmt.Errorf("field Name: %w", err)
	}

	consistency, ok := reg.Get("app.PetConsistency")
	if !ok {
		return fmt.Errorf("validator %q not registered", "app.PetConsistency")
	}

	if err := consistency.Validate(p); err != nil {
		return err
	}

	return nil
}

// ExpectedValidatorNames — пример сгенерированной функции (см.
// testdata/minimal/golden/model/expected_validators.gen.go). В реальном
// коде генерируется oapigen из всех x-validations в спеке.
func ExpectedValidatorNames() []string {
	return []string{
		"app.NonEmptyName",
		"app.PetConsistency",
	}
}

// --- Пользовательские валидаторы ---

// NonEmptyName — property-level валидатор: получает значение поля.
type NonEmptyName struct{}

func (NonEmptyName) Name() string { return "app.NonEmptyName" }

func (NonEmptyName) Validate(value any) error {
	s, ok := value.(string)
	if !ok {
		return fmt.Errorf("expected string, got %T", value)
	}

	if s == "" {
		return errors.New("name must not be empty")
	}

	return nil
}

// PetConsistency — schema-level (cross-field) валидатор: получает всю структуру.
type PetConsistency struct{}

func (PetConsistency) Name() string { return "app.PetConsistency" }

func (PetConsistency) Validate(value any) error {
	p, ok := value.(Pet)
	if !ok {
		return fmt.Errorf("expected Pet, got %T", value)
	}

	// Cross-field rule: ID и Name не должны совпадать.
	if fmt.Sprintf("%d", p.ID) == p.Name {
		return errors.New("ID must not equal Name")
	}

	return nil
}

func main() {
	// 1. Регистрация валидаторов.
	reg := validator.New()
	reg.Register(NonEmptyName{})
	reg.Register(PetConsistency{})

	// 2. Startup-check: набор валидаторов должен точно совпадать с тем,
	// что требует spec (ExpectedValidatorNames). Лишний или недостающий
	// валидатор — fail-fast, приложение не стартует.
	if err := reg.AssertExact(ExpectedValidatorNames()); err != nil {
		log.Fatalf("startup check failed: %v", err)
	}

	// 3. Валидация входящего объекта. Walker обходит Pet, вызывает
	// ValidateOwn, при ошибке заворачивает с путём "Owner.Pets[2].Name".
	examples := []struct {
		label string
		pet   Pet
	}{
		{
			label: "valid pet",
			pet:   Pet{ID: 42, Name: "Rex", Tag: "friendly"},
		},
		{
			label: "empty name (simple rule fails)",
			pet:   Pet{ID: 42, Name: "", Tag: ""},
		},
		{
			label: "id equals name (schema-level fails)",
			pet:   Pet{ID: 7, Name: "7", Tag: ""},
		},
		{
			label: "negative id (simple rule fails)",
			pet:   Pet{ID: -1, Name: "Rex", Tag: ""},
		},
	}

	for _, ex := range examples {
		fmt.Printf("— %s: ", ex.label)

		if err := validator.Validate(ex.pet, reg); err != nil {
			fmt.Println("FAIL:", err)
		} else {
			fmt.Println("OK")
		}
	}
}
