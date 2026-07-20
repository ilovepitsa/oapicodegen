package generator

import (
	"nschugorev/oapigenerator/internal/parser"
)

// schemaHasSharedFields сообщает, есть ли у схемы хотя бы одно shared-поле
// (не readOnly && не writeOnly) — поле, существующее в обоих split-вариантах.
// Используется Generator'ом в shouldGenerateConverters для условия генерации
// <Name>_converters.gen.go. Само рендеринг-тело переезжает в
// render/schema.ConvertersRenderer (Task 7).
func schemaHasSharedFields(sh *parser.Schema) bool {
	for _, p := range sh.Properties {
		if p.Schema == nil {
			continue
		}

		if !p.Schema.ReadOnly && !p.Schema.WriteOnly {
			return true
		}
	}

	return false
}
