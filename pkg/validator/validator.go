// Package validator предоставляет registry именованных валидаторов и
// reflection-walker для рекурсивной валидации struct-дерева.
//
// Паттерн использования (server-side):
//
//	reg := validator.New()
//	reg.Register(cdn.ArrayOfDomainNames{})
//	reg.Register(cdn.EmailFormat{})
//	if err := reg.AssertExact(gen.ExpectedValidatorNames()); err != nil {
//	    log.Fatal(err)  // fail-fast при старте
//	}
//
//	if err := validator.Validate(&req, reg); err != nil {
//	    writeError(w, 400, err)
//	    return
//	}
//
// Сгенерированные типы реализуют Validatable через метод ValidateOwn(reg),
// содержащий только правила этой структуры (без рекурсии в потомков —
// это делает walker). Структуры без правил не реализуют Validatable и
// прозрачно пропускаются walker'ом.
//
// Walker обходит struct/slice/array/map/ptr/interface через reflection,
// вызывает ValidateOwn на каждой структуре, реализующей Validatable, и
// при ошибке заворачивает её с путём вида "Owner.Pets[2].Name".
//
// Fail-fast: первая ошибка прерывает обход.
package validator

import (
	"fmt"
	"reflect" //nolint:depguard // walker принципиально на reflection
	"sort"
	"strings"
)

// Validator — именованная валидация значения. Name() уникально
// идентифицирует валидатор в Registry. Validate применяется к значению
// поля (или всей структуры для schema-level валидаторов).
type Validator interface {
	Name() string
	Validate(value any) error
}

// Validatable реализуется сгенерированными структурами, у которых есть
// собственные правила валидации. ValidateOwn содержит только правила
// этой структуры (scalar-правила, named-валидаторы на её полях,
// schema-level валидатор) и НЕ рекурсивит в потомков — это делает walker.
type Validatable interface {
	ValidateOwn(reg *Registry) error
}

// Registry хранит именованные валидаторы по имени. Нес thread-safe;
// предполагается заполнение один раз при старте сервера.
type Registry struct {
	validators map[string]Validator
}

// New создаёт пустой Registry.
func New() *Registry {
	return &Registry{validators: make(map[string]Validator)}
}

// Register добавляет валидатор в registry по его Name(). Если валидатор
// с таким именем уже есть — он молча перезаписывается.
func (r *Registry) Register(v Validator) {
	r.validators[v.Name()] = v
}

// Get возвращает валидатор по имени. ok=false, если не зарегистрирован.
func (r *Registry) Get(name string) (Validator, bool) {
	v, ok := r.validators[name]

	return v, ok
}

// Names возвращает отсортированный список имён зарегистрированных
// валидаторов.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.validators))
	for n := range r.validators {
		names = append(names, n)
	}

	sort.Strings(names)

	return names
}

// AssertExact проверяет, что набор зарегистрированных валидаторов
// точно совпадает с expected. Лишние или недостающие — ошибка.
// Используется при старте сервера для fail-fast проверки: если spec
// требует валидатор, которого нет в registry (или наоборот) — приложение
// не стартует.
func (r *Registry) AssertExact(expected []string) error {
	want := make(map[string]bool, len(expected))
	for _, n := range expected {
		want[n] = true
	}

	var missing, extra []string

	for n := range want {
		if _, ok := r.validators[n]; !ok {
			missing = append(missing, n)
		}
	}

	for n := range r.validators {
		if !want[n] {
			extra = append(extra, n)
		}
	}

	sort.Strings(missing)
	sort.Strings(extra)

	if len(missing) == 0 && len(extra) == 0 {
		return nil
	}

	var parts []string
	if len(missing) > 0 {
		parts = append(parts, "missing: "+strings.Join(missing, ", "))
	}

	if len(extra) > 0 {
		parts = append(parts, "extra: "+strings.Join(extra, ", "))
	}

	return fmt.Errorf("validator registry mismatch: %s", strings.Join(parts, "; "))
}

// Validate обходит obj через reflection и вызывает ValidateOwn(reg) на
// каждой структуре, реализующей Validatable. Fail-fast: первая ошибка
// прерывает обход.
//
// Поддерживаемые типы: struct (включая fields), slice/array, map (по
// values), pointer, interface. nil-указатели и nil-интерфейсы
// пропускаются. Unexported поля пропускаются.
//
// Path в ошибке имеет вид "Owner.Pets[2].Name" — для понимания, какое
// именно поле невалидно.
func Validate(obj any, reg *Registry) error {
	if obj == nil {
		return nil
	}

	return walkValidate(reflect.ValueOf(obj), reg, "")
}

// walkValidate — рекурсивная реализация Validate. path — путь до
// текущего значения от корня (для сообщений об ошибках).
//
//nolint:exhaustive // reflect.Kind: интересны только struct/slice/array/map/ptr/interface
func walkValidate(v reflect.Value, reg *Registry, path string) error {
	if !v.IsValid() {
		return nil
	}

	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		return walkPointer(v, reg, path)
	case reflect.Struct:
		return walkStruct(v, reg, path)
	case reflect.Slice, reflect.Array:
		return walkSlice(v, reg, path)
	case reflect.Map:
		return walkMap(v, reg, path)
	default:
		// Примитивы (bool/int/float/string/...) — не содержат Validatable.
		return nil
	}
}

func walkPointer(v reflect.Value, reg *Registry, path string) error {
	if v.IsNil() {
		return nil
	}

	return walkValidate(v.Elem(), reg, path)
}

func walkStruct(v reflect.Value, reg *Registry, path string) error {
	if err := callValidateOwn(v, reg, path); err != nil {
		return err
	}

	for i := range v.NumField() {
		if !v.Field(i).CanInterface() {
			continue // skip unexported
		}

		fieldPath := joinPath(path, v.Type().Field(i).Name)
		if err := walkValidate(v.Field(i), reg, fieldPath); err != nil {
			return err
		}
	}

	return nil
}

func walkSlice(v reflect.Value, reg *Registry, path string) error {
	for i := range v.Len() {
		elemPath := fmt.Sprintf("%s[%d]", path, i)
		if err := walkValidate(v.Index(i), reg, elemPath); err != nil {
			return err
		}
	}

	return nil
}

func walkMap(v reflect.Value, reg *Registry, path string) error {
	for _, k := range v.MapKeys() {
		elemPath := fmt.Sprintf("%s[%v]", path, k.Interface())
		if err := walkValidate(v.MapIndex(k), reg, elemPath); err != nil {
			return err
		}
	}

	return nil
}

// callValidateOwn пытается вызвать ValidateOwn на v. Поддерживает как
// value-receiver, так и pointer-receiver реализации Validatable. Для
// pointer-receiver требует addressable value (map values, например, не
// addressable — для них используйте value-receiver в сгенерированном коде).
func callValidateOwn(v reflect.Value, reg *Registry, path string) error {
	obj, ok := lookupValidatable(v)
	if !ok {
		return nil
	}

	if err := obj.ValidateOwn(reg); err != nil {
		return wrapPath(path, err)
	}

	return nil
}

// lookupValidatable пытается получить Validatable из v. Сначала пробует
// value-receiver (работает для addressable и не-addressable values),
// затем pointer-receiver (требует CanAddr).
func lookupValidatable(v reflect.Value) (Validatable, bool) {
	if v.CanInterface() {
		if obj, ok := v.Interface().(Validatable); ok {
			return obj, true
		}
	}

	if v.CanAddr() && v.CanInterface() {
		if obj, ok := v.Addr().Interface().(Validatable); ok {
			return obj, true
		}
	}

	return nil, false
}

func joinPath(base, field string) string {
	if base == "" {
		return field
	}

	return base + "." + field
}

func wrapPath(path string, err error) error {
	if path == "" {
		return err
	}

	return fmt.Errorf("%s: %w", path, err)
}
