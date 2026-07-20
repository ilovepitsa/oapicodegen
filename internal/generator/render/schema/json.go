// Package schema: JSONRenderer рендерит MarshalJSON/UnmarshalJSON для
// union-схем (oneOf/anyOf). Методы на *<Name> валидны, т.к. union рендерится
// как struct, не interface.
package schema

import (
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/generator/walk"
	"nschugorev/oapigenerator/internal/parser"
)

// JSONRenderer рендерит MarshalJSON/UnmarshalJSON для union-схем (oneOf/anyOf).
// Срабатывает на OnUnion-хук (см. walk.SchemaWalker — oneOf/anyOf диспатчатся
// в OnUnion). Для не-union схем все хуки noop (наследуются от NoopSchemaRenderer).
//
// Реализует SkipDescendants — walker не должен спускаться в варианты oneOf/
// anyOf: каждый вариант обычно $ref на другую схему, и спуск вызвал бы
// повторный рендер этой схемы в <name>_json.gen.go. JSONRenderer сам итерирует
// variants в collectVariants.
type JSONRenderer struct {
	render.Base
	walk.NoopSchemaRenderer
}

// NewJSONRenderer возвращает JSONRenderer с нулевым состоянием. Buf и Imports
// вливаются через Base.Init в compose.FileComposer перед обходом.
func NewJSONRenderer() *JSONRenderer { return &JSONRenderer{} }

// Skip реализует walk.SkipDescendants — JSONRenderer не требует спуска
// walker'а в варианты union: collectVariants итерирует их сама.
func (r *JSONRenderer) Skip(_ *parser.Schema) bool { return true }

// unionVariant описывает один вариант union-схемы: имя поля в struct
// (PascalCase, см. inlineVariantName) и Go-тип. Используется в MarshalJSON
// и UnmarshalJSON — вынесен в тип, чтобы не дублировать литерал.
type unionVariant struct {
	field string
	typ   string
}

// OnUnion рендерит MarshalJSON/UnmarshalJSON для oneOf/anyOf-схемы.
// kind не используется — для oneOf и anyOf логика одинакова (порядок
// вариантов сохраняется из spec).
//
// Импорты: encoding/json (json.Marshal/Unmarshal) и fmt (fmt.Errorf в
// no-variant-matched ошибке).
func (r *JSONRenderer) OnUnion(s *parser.Schema, _ walk.UnionKind) error {
	r.Imports.Add(gogen.Import{Path: "encoding/json"})
	r.Imports.Add(gogen.Import{Path: "fmt"})

	name := goName(s.Name)
	vs := r.collectVariants(s)

	r.renderUnmarshalJSON(name, vs)
	r.renderMarshalJSON(name, vs)

	return nil
}

// collectVariants возвращает список вариантов union-схемы (oneOf или anyOf),
// для которых удалось получить не-empty и не-any Go-тип. Имя поля: из $ref
// (если есть) или сгенерированное из типа (inlineVariantName).
func (r *JSONRenderer) collectVariants(s *parser.Schema) []unionVariant {
	variants := s.OneOf
	if len(variants) == 0 {
		variants = s.AnyOf
	}

	vs := make([]unionVariant, 0, len(variants))

	for _, v := range variants {
		variantType := r.Ctx.TypeMapper.GoType(v)
		if variantType == "" || variantType == goTypeAny {
			continue
		}

		fieldName := goName(refToName(v.Ref))
		if fieldName == "" {
			fieldName = inlineVariantName(variantType)
		}

		vs = append(vs, unionVariant{field: fieldName, typ: variantType})
	}

	return vs
}

// renderUnmarshalJSON рендерит `func (m *<Name>) UnmarshalJSON(data []byte) error`.
// Для каждого варианта декларация локальной переменной, попытка json.Unmarshal,
// при успехе — присваивание m.<Field> и return nil. Если ни один вариант не
// сматчился — fmt.Errorf.
func (r *JSONRenderer) renderUnmarshalJSON(name string, vs []unionVariant) {
	r.Buf.Print("func (m *", name, ") UnmarshalJSON(data []byte) error {\n")

	for i, v := range vs {
		r.Buf.Print("\tvar v_", i, " ", v.typ, "\n")
		r.Buf.Print("\tif err := json.Unmarshal(data, &v_", i, "); err == nil {\n")
		r.Buf.Print("\t\tm.", v.field, " = &v_", i, "\n")
		r.Buf.Print("\t\treturn nil\n")
		r.Buf.Print("\t}\n")
	}

	r.Buf.Print("\treturn fmt.Errorf(\"", name, ": no variant matched\")\n")
	r.Buf.Print("}\n\n")
}

// renderMarshalJSON рендерит `func (m <Name>) MarshalJSON() ([]byte, error)`.
// Для каждого варианта проверка m.<Field> != nil → json.Marshal. Если ни один
// не задан — json.Marshal(nil).
func (r *JSONRenderer) renderMarshalJSON(name string, vs []unionVariant) {
	r.Buf.Print("func (m ", name, ") MarshalJSON() ([]byte, error) {\n")

	for _, v := range vs {
		r.Buf.Print("\tif m.", v.field, " != nil {\n")
		r.Buf.Print("\t\treturn json.Marshal(m.", v.field, ")\n")
		r.Buf.Print("\t}\n")
	}

	r.Buf.Print("\treturn json.Marshal(nil)\n")
	r.Buf.Print("}\n")
}
