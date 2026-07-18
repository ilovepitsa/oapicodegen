package walk

import (
	"fmt"
	"nschugorev/oapigenerator/internal/parser"
)

// OpenAPI type-строки, используемые walker'ом для диспатча. Вынесены в
// константы, чтобы удовлетворить goconst (3+ употребления каждой).
const (
	schemaTypeObject = "object"
	schemaTypeArray  = "array"
	schemaTypeMap    = "map"
)

// SchemaWalker рекурсивно обходит дерево *parser.Schema и диспатчит хуки в
// зарегистрированные renderer'ы. Для каждой схемы walker:
//  1. вызывает один из type-dispatch хуков (OnStruct/OnEnum/.../OnAlias);
//  2. вызывает per-child хуки (OnStructProperty/OnArrayItem/...);
//  3. если ни один renderer не попросил пропустить потомков через
//     SkipDescendants — рекурсивно обходит дочерние схемы.
//
// Ошибка из любого хука немедленно прерывает обход и оборачивается с указанием
// имени текущей схемы и дочернего элемента.
type SchemaWalker struct {
	renderers []SchemaRenderer
}

// NewSchemaWalker строит walker'а с набором renderer'ов. Порядок вызова хуков
// совпадает с порядком аргументов.
func NewSchemaWalker(r ...SchemaRenderer) *SchemaWalker {
	return &SchemaWalker{renderers: r}
}

// Walk запускает обход схемы s. nil-схема игнорируется.
func (w *SchemaWalker) Walk(s *parser.Schema) error {
	if s == nil {
		return nil
	}

	if err := w.dispatchType(s); err != nil {
		return fmt.Errorf("walk schema %q: %w", s.Name, err)
	}

	if err := w.dispatchChildren(s); err != nil {
		return err
	}

	if w.shouldDescend(s) {
		if err := w.descend(s); err != nil {
			return err
		}
	}

	return nil
}

// dispatchType вызывает один из type-dispatch хуков в зависимости от типа schema.
// Логика соответствует switch в Generator.renderSchema (после миграции будет в render/).
//
// Split-схемы (IsSplit=true) идут в OnSplitStruct, остальные object-схемы — в OnStruct.
func (w *SchemaWalker) dispatchType(s *parser.Schema) error {
	switch {
	case s.Type == schemaTypeObject:
		return w.dispatchObject(s)
	case s.Type == schemaTypeArray:
		return w.callEach(func(r SchemaRenderer) error { return r.OnArray(s) })
	case s.Type == schemaTypeMap || s.AdditionalProperties != nil:
		return w.callEach(func(r SchemaRenderer) error { return r.OnMap(s) })
	case len(s.OneOf) > 0:
		return w.callEach(func(r SchemaRenderer) error { return r.OnUnion(s, UnionOneOf) })
	case len(s.AnyOf) > 0:
		return w.callEach(func(r SchemaRenderer) error { return r.OnUnion(s, UnionAnyOf) })
	case len(s.AllOf) > 0:
		return w.callEach(func(r SchemaRenderer) error { return r.OnAllOf(s) })
	case len(s.Enum) > 0:
		return w.callEach(func(r SchemaRenderer) error { return r.OnEnum(s) })
	default:
		return w.callEach(func(r SchemaRenderer) error { return r.OnAlias(s) })
	}
}

// dispatchObject вызывает OnSplitStruct для split-схем, OnMap — для
// map-alias'ов (object без properties: дополнительная семантика через
// AdditionalProperties / AdditionalPropertiesFalse), иначе OnStruct.
//
// Map-alias выделен отдельной веткой, чтобы рендер мог обрабатывать его
// иначе, чем обычный struct (тип `map[string]X` или `struct{}` вместо
// struct-определения). Логика зеркалит switch в generator.renderSchema.
func (w *SchemaWalker) dispatchObject(s *parser.Schema) error {
	if s.IsSplit {
		return w.callEach(func(r SchemaRenderer) error { return r.OnSplitStruct(s) })
	}

	if len(s.Properties) == 0 {
		return w.callEach(func(r SchemaRenderer) error { return r.OnMap(s) })
	}

	return w.callEach(func(r SchemaRenderer) error { return r.OnStruct(s) })
}

// dispatchChildren вызывает per-child хуки (без рекурсивного спуска). Каждый
// контейнерный тип имеет свой хук: OnStructProperty для object, OnArrayItem
// для array и т.д. Ошибка хука оборачивается с указанием родителя и дочернего
// элемента, чтобы трассировка показывала, где именно упали.
func (w *SchemaWalker) dispatchChildren(s *parser.Schema) error {
	switch {
	case s.Type == schemaTypeObject && len(s.Properties) > 0:
		return w.dispatchStructProperties(s)
	case s.Type == schemaTypeArray && s.Items != nil:
		return w.dispatchArrayItem(s)
	case s.Type == schemaTypeMap && s.AdditionalProperties != nil:
		return w.dispatchMapValue(s)
	case len(s.OneOf) > 0:
		return w.dispatchUnionVariants(s, s.OneOf, "oneOf")
	case len(s.AnyOf) > 0:
		return w.dispatchUnionVariants(s, s.AnyOf, "anyOf")
	case len(s.AllOf) > 0:
		return w.dispatchAllOfMembers(s)
	}

	return nil
}

// dispatchStructProperties вызывает OnStructProperty для каждого свойства object-схемы.
func (w *SchemaWalker) dispatchStructProperties(s *parser.Schema) error {
	for _, p := range s.Properties {
		for _, r := range w.renderers {
			if err := r.OnStructProperty(s, p.Name, p.Schema); err != nil {
				return fmt.Errorf("schema %q property %q: %w", s.Name, p.Name, err)
			}
		}
	}

	return nil
}

// dispatchArrayItem вызывает OnArrayItem для единственного элемента array-схемы.
// parser.Schema.Items — одиночная *Schema (не slice/tuple); индекс всегда 0.
func (w *SchemaWalker) dispatchArrayItem(s *parser.Schema) error {
	for _, r := range w.renderers {
		if err := r.OnArrayItem(s, 0, s.Items); err != nil {
			return fmt.Errorf("schema %q item 0: %w", s.Name, err)
		}
	}

	return nil
}

// dispatchMapValue вызывает OnMapValue для value-схемы map'а.
func (w *SchemaWalker) dispatchMapValue(s *parser.Schema) error {
	for _, r := range w.renderers {
		if err := r.OnMapValue(s, s.AdditionalProperties); err != nil {
			return fmt.Errorf("schema %q map value: %w", s.Name, err)
		}
	}

	return nil
}

// dispatchUnionVariants вызывает OnUnionVariant для каждого варианта oneOf/anyOf.
// kindLabel — "oneOf" или "anyOf" — используется в сообщении об ошибке.
func (w *SchemaWalker) dispatchUnionVariants(s *parser.Schema, variants []*parser.Schema, kindLabel string) error { //nolint:lll // function signature with typed slice + label params
	for i, v := range variants {
		for _, r := range w.renderers {
			if err := r.OnUnionVariant(s, i, v); err != nil {
				return fmt.Errorf("schema %q %s[%d]: %w", s.Name, kindLabel, i, err)
			}
		}
	}

	return nil
}

// dispatchAllOfMembers вызывает OnAllOfMember для каждого члена allOf.
func (w *SchemaWalker) dispatchAllOfMembers(s *parser.Schema) error {
	for i, m := range s.AllOf {
		for _, r := range w.renderers {
			if err := r.OnAllOfMember(s, i, m); err != nil {
				return fmt.Errorf("schema %q allOf[%d]: %w", s.Name, i, err)
			}
		}
	}

	return nil
}

// shouldDescend проверяет, не попросил ли какой-нибудь renderer пропустить
// потомков текущей схемы (через optional-интерфейс SkipDescendants).
func (w *SchemaWalker) shouldDescend(s *parser.Schema) bool {
	for _, r := range w.renderers {
		if sd, ok := r.(SkipDescendants); ok && sd.Skip(s) {
			return false
		}
	}

	return true
}

// descend рекурсивно обходит дочерние схемы. Порядок соответствует
// dispatchChildren: properties → items → additionalProperties → oneOf →
// anyOf → allOf. Ошибка из Walk потомка уже обёрнута именем схемы —
// дополнительная обёртка не нужна.
func (w *SchemaWalker) descend(s *parser.Schema) error {
	switch {
	case s.Type == schemaTypeObject:
		return w.descendProperties(s)
	case s.Type == schemaTypeArray && s.Items != nil:
		return w.Walk(s.Items)
	case s.Type == schemaTypeMap && s.AdditionalProperties != nil:
		return w.Walk(s.AdditionalProperties)
	case len(s.OneOf) > 0:
		return w.descendSchemas(s.OneOf)
	case len(s.AnyOf) > 0:
		return w.descendSchemas(s.AnyOf)
	case len(s.AllOf) > 0:
		return w.descendSchemas(s.AllOf)
	}

	return nil
}

// descendProperties рекурсивно обходит схемы свойств object-схемы.
func (w *SchemaWalker) descendProperties(s *parser.Schema) error {
	for _, p := range s.Properties {
		if err := w.Walk(p.Schema); err != nil {
			return err
		}
	}

	return nil
}

// descendSchemas рекурсивно обходит slice дочерних схем (oneOf/anyOf/allOf).
func (w *SchemaWalker) descendSchemas(schemas []*parser.Schema) error {
	for _, child := range schemas {
		if err := w.Walk(child); err != nil {
			return err
		}
	}

	return nil
}

// callEach вызывает fn для каждого renderer'а, возвращая первую ошибку.
// Ошибка от интерфейсного хука не оборачивается здесь — вызывающая сторона
// (dispatchType → Walk) добавляет контекст имени схемы.
func (w *SchemaWalker) callEach(fn func(SchemaRenderer) error) error {
	for _, r := range w.renderers {
		if err := fn(r); err != nil {
			return err
		}
	}

	return nil
}
