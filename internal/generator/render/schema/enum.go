package schema

import (
	"fmt"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/generator/walk"
	"nschugorev/oapigenerator/internal/parser"
)

// EnumRenderer рендерит type-declaration для enum-схемы и const-блок с
// поименованными константами для каждого уникального enum-значения.
// Embed'ит walk.NoopSchemaRenderer — остальные хуки не нужны.
type EnumRenderer struct {
	render.Base
	walk.NoopSchemaRenderer
}

// NewEnumRenderer возвращает EnumRenderer с нулевым состоянием. Buf и
// Imports вливаются через Base.Init в compose.FileComposer перед обходом.
func NewEnumRenderer() *EnumRenderer { return &EnumRenderer{} }

// OnEnum рендерит:
//
//	type <Name> <BaseGo>
//
//	const (
//	    <Name><ValueName> <Name> = <literal>
//	    ...
//	)
//
// Базовый Go-тип определяется по Type+Format схемы. Одинаковые значения
// дедуплицируются (по строковому представлению) — порядок первых
// вхождений сохраняется.
func (r *EnumRenderer) OnEnum(s *parser.Schema) error {
	baseGo := enumBaseType(s)
	r.Buf.Print("type ", goName(s.Name), " ", baseGo, "\n\n")

	r.Buf.Print("const (\n")

	seen := make(map[string]bool, len(s.Enum))

	for i, v := range s.Enum {
		valStr := enumStringValue(v)
		if seen[valStr] {
			continue
		}

		seen[valStr] = true

		r.Buf.Print("\t", enumValueName(goName(s.Name), valStr, i), " ", goName(s.Name), " = ", enumLiteral(v, baseGo), "\n") //nolint:lll // const declaration line
	}

	r.Buf.Print(")\n")

	return nil
}

// enumBaseType возвращает Go-тип, на котором строится enum (string по
// умолчанию, int/int32/int64 для integer, float32/float64 для number).
func enumBaseType(s *parser.Schema) string {
	switch s.Type {
	case oapiTypeInteger:
		switch s.Format {
		case oapiFormatInt32:
			return oapiFormatInt32
		case oapiFormatInt64:
			return oapiFormatInt64
		default:
			return "int"
		}
	case oapiTypeNumber:
		switch s.Format {
		case oapiFormatFloat:
			return goTypeFloat32
		default:
			return goTypeFloat64
		}
	default:
		return oapiTypeString
	}
}

// enumStringValue возвращает строковое представление enum-значения —
// используется как ключ дедупликации и как основа для имени константы.
func enumStringValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}

	return fmt.Sprint(v)
}

// enumLiteral рендерит Go-литерал для enum-значения: quoted string для
// строковых enum'ов, raw Sprint для числовых.
func enumLiteral(v any, baseGo string) string {
	switch baseGo {
	case oapiTypeString:
		return fmt.Sprintf("%q", fmt.Sprint(v))
	default:
		return fmt.Sprint(v)
	}
}
