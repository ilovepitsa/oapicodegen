package schema

import (
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/generator/walk"
	"nschugorev/oapigenerator/internal/parser"
)

// AliasRenderer рендерит type-declarations для примитивных alias'ов
// (`type PetID string`) и map-alias'ов (`type StringMap map[string]string`,
// `type Empty struct{}`). Embed'ит walk.NoopSchemaRenderer — остальные хуки
// (OnStruct/OnEnum/...) не нужны, walker диспатчит в AliasRenderer только
// top-level alias/map-alias схемы.
type AliasRenderer struct {
	render.Base
	walk.NoopSchemaRenderer
}

// NewAliasRenderer возвращает AliasRenderer с нулевым состоянием. Buf и
// Imports вливаются через Base.Init в compose.FileComposer перед обходом.
func NewAliasRenderer() *AliasRenderer { return &AliasRenderer{} }

// OnAlias рендерит `type <Name> <GoType>` с опциональным doc-комментарием.
// GoType берётся из TypeMapper'а контекста — это adapter к generator.typeMapper,
// который корректно разрешает $ref, format, nullable и копит импорты.
func (r *AliasRenderer) OnAlias(s *parser.Schema) error {
	name := goName(s.Name)

	if s.Description != "" {
		writeDocComment(r.Buf, s.Description)
	}

	r.Buf.Print("type ", name, " ", r.Ctx.TypeMapper.GoType(s), "\n")

	return nil
}

// OnMap рендерит map-alias: `type <Name> map[string]<Elem>` для схем с
// AdditionalProperties, или `type <Name> struct{}` для additionalProperties:
// false. Значение elem по умолчанию `any`, если AdditionalProperties не задан.
func (r *AliasRenderer) OnMap(s *parser.Schema) error {
	name := goName(s.Name)

	if s.AdditionalPropertiesFalse {
		r.Buf.Print("type ", name, " struct{}\n")

		return nil
	}

	elem := goTypeAny
	if s.AdditionalProperties != nil {
		elem = r.Ctx.TypeMapper.GoType(s.AdditionalProperties)
	}

	r.Buf.Print("type ", name, " map[string]", elem, "\n")

	return nil
}
