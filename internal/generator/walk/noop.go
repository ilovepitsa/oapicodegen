package walk

import "nschugorev/oapigenerator/internal/parser"

// Compile-time conformance checks: гарантируют, что noop-реализации
// удовлетворяют интерфейсам renderer'ов. Без этих строк линтер unused
// помечает типы как неиспользуемые (первые embed'еры появятся в Task 2/3).
var (
	_ SchemaRenderer = NoopSchemaRenderer{}
	_ MethodRenderer = NoopMethodRenderer{}
)

// NoopSchemaRenderer даёт пустые реализации для неиспользуемых хуков.
// Renderer'ы embed'ят его и реализуют только нужные методы.
type NoopSchemaRenderer struct{}

func (NoopSchemaRenderer) OnStruct(_ *parser.Schema) error             { return nil }
func (NoopSchemaRenderer) OnEnum(_ *parser.Schema) error               { return nil }
func (NoopSchemaRenderer) OnAlias(_ *parser.Schema) error              { return nil }
func (NoopSchemaRenderer) OnArray(_ *parser.Schema) error              { return nil }
func (NoopSchemaRenderer) OnMap(_ *parser.Schema) error                { return nil }
func (NoopSchemaRenderer) OnUnion(_ *parser.Schema, _ UnionKind) error { return nil }
func (NoopSchemaRenderer) OnAllOf(_ *parser.Schema) error              { return nil }
func (NoopSchemaRenderer) OnSplitStruct(_ *parser.Schema) error        { return nil }

func (NoopSchemaRenderer) OnStructProperty(_ *parser.Schema, _ string, _ *parser.Schema) error {
	return nil
}

func (NoopSchemaRenderer) OnArrayItem(_ *parser.Schema, _ int, _ *parser.Schema) error {
	return nil
}

func (NoopSchemaRenderer) OnMapValue(_, _ *parser.Schema) error {
	return nil
}

func (NoopSchemaRenderer) OnUnionVariant(_ *parser.Schema, _ int, _ *parser.Schema) error {
	return nil
}

func (NoopSchemaRenderer) OnAllOfMember(_ *parser.Schema, _ int, _ *parser.Schema) error {
	return nil
}

// NoopMethodRenderer — аналогично для method-walker'а.
type NoopMethodRenderer struct{}

func (NoopMethodRenderer) OnMethod(_ *parser.Method) error { return nil }

func (NoopMethodRenderer) OnPathParameter(_ *parser.Method, _ *parser.Parameter) error {
	return nil
}

func (NoopMethodRenderer) OnQueryParameter(_ *parser.Method, _ *parser.Parameter) error {
	return nil
}

func (NoopMethodRenderer) OnHeaderParameter(_ *parser.Method, _ *parser.Parameter) error {
	return nil
}

func (NoopMethodRenderer) OnCookieParameter(_ *parser.Method, _ *parser.Parameter) error {
	return nil
}

func (NoopMethodRenderer) OnRequestBody(_ *parser.Method, _ *parser.RequestBody) error {
	return nil
}

func (NoopMethodRenderer) OnResponse(_ *parser.Method, _ string, _ *parser.Response) error {
	return nil
}

//nolint:lll // noop signature must match MethodRenderer.OnResponseHeader; intrinsic length
func (NoopMethodRenderer) OnResponseHeader(_ *parser.Method, _, _ string, _ *parser.Parameter) error {
	return nil
}
