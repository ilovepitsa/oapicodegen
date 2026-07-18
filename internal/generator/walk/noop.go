package walk

import "nschugorev/oapigenerator/internal/parser"

// Compile-time conformance checks: гарантируют, что noop-реализации
// удовлетворяют интерфейсам renderer'ов. Без этих строк линтер unused
// помечает типы как неиспользуемые (первые embed'еры появятся в Task 2/3).
var (
	_ SchemaRenderer = noopSchemaRenderer{}
	_ MethodRenderer = noopMethodRenderer{}
)

// noopSchemaRenderer даёт пустые реализации для неиспользуемых хуков.
// Renderer'ы embed'ят его и реализуют только нужные методы.
type noopSchemaRenderer struct{}

func (noopSchemaRenderer) OnStruct(_ *parser.Schema) error             { return nil }
func (noopSchemaRenderer) OnEnum(_ *parser.Schema) error               { return nil }
func (noopSchemaRenderer) OnAlias(_ *parser.Schema) error              { return nil }
func (noopSchemaRenderer) OnArray(_ *parser.Schema) error              { return nil }
func (noopSchemaRenderer) OnMap(_ *parser.Schema) error                { return nil }
func (noopSchemaRenderer) OnUnion(_ *parser.Schema, _ UnionKind) error { return nil }
func (noopSchemaRenderer) OnAllOf(_ *parser.Schema) error              { return nil }
func (noopSchemaRenderer) OnSplitStruct(_ *parser.Schema) error        { return nil }
func (noopSchemaRenderer) OnStructProperty(_ *parser.Schema, _ string, _ *parser.Schema) error {
	return nil
}

func (noopSchemaRenderer) OnArrayItem(_ *parser.Schema, _ int, _ *parser.Schema) error {
	return nil
}

func (noopSchemaRenderer) OnMapValue(_, _ *parser.Schema) error {
	return nil
}

func (noopSchemaRenderer) OnUnionVariant(_ *parser.Schema, _ int, _ *parser.Schema) error {
	return nil
}

func (noopSchemaRenderer) OnAllOfMember(_ *parser.Schema, _ int, _ *parser.Schema) error {
	return nil
}

// noopMethodRenderer — аналогично для method-walker'а.
type noopMethodRenderer struct{}

func (noopMethodRenderer) OnMethod(_ *parser.Method) error { return nil }
func (noopMethodRenderer) OnPathParameter(_ *parser.Method, _ *parser.Parameter) error {
	return nil
}

func (noopMethodRenderer) OnQueryParameter(_ *parser.Method, _ *parser.Parameter) error {
	return nil
}

func (noopMethodRenderer) OnHeaderParameter(_ *parser.Method, _ *parser.Parameter) error {
	return nil
}

func (noopMethodRenderer) OnCookieParameter(_ *parser.Method, _ *parser.Parameter) error {
	return nil
}

func (noopMethodRenderer) OnRequestBody(_ *parser.Method, _ *parser.RequestBody) error {
	return nil
}

func (noopMethodRenderer) OnResponse(_ *parser.Method, _ string, _ *parser.Response) error {
	return nil
}

//nolint:lll // noop signature must match MethodRenderer.OnResponseHeader; intrinsic length
func (noopMethodRenderer) OnResponseHeader(_ *parser.Method, _, _ string, _ *parser.Parameter) error {
	return nil
}
