package parser

import (
	"slices"
	"strings"

	highbase "github.com/pb33f/libopenapi/datamodel/high/base"
	"github.com/pb33f/libopenapi/orderedmap"
	"gopkg.in/yaml.v3"
)

// Имена x-* расширений, читаемых парсером.
const (
	extRequestRequired  = "x-request-required"
	extResponseRequired = "x-response-required"
	extOptional         = "x-optional"
)

// schemaFromProxy конвертирует *highbase.SchemaProxy в наш *Schema.
// Если proxy — $ref, возвращается Schema с заполненным Ref и (по возможности)
// разрешёнными полями целевой схемы.
func schemaFromProxy(proxy *highbase.SchemaProxy) *Schema {
	if proxy == nil {
		return nil
	}

	if proxy.IsReference() {
		ref := proxy.GetReference()
		s := &Schema{Ref: ref}
		s.Name = refToSchemaName(ref)

		if target := proxy.Schema(); target != nil {
			fillSchema(s, target)
		}

		return s
	}

	s := &Schema{}
	fillSchema(s, proxy.Schema())

	return s
}

// fillSchema заполняет поля s из *highbase.Schema.
//
//nolint:gocyclo,cyclop // field-by-field mapping, linear
func fillSchema(s *Schema, sh *highbase.Schema) {
	s.Description = sh.Description
	s.Format = sh.Format
	s.Nullable = boolPtrOrFalse(sh.Nullable)
	s.Deprecated = boolPtrOrFalse(sh.Deprecated)
	s.ReadOnly = boolPtrOrFalse(sh.ReadOnly)
	s.WriteOnly = boolPtrOrFalse(sh.WriteOnly)

	if len(sh.Type) > 0 {
		s.Type = sh.Type[0]
	}

	s.Required = append(s.Required, sh.Required...)

	if sh.Default != nil {
		s.Default = decodeNode(sh.Default)
	}

	for _, n := range sh.Enum {
		s.Enum = append(s.Enum, decodeNode(n))
	}

	if sh.Items != nil && sh.Items.IsA() && sh.Items.A != nil {
		s.Items = schemaFromProxy(sh.Items.A)
	}

	ap := sh.AdditionalProperties
	if ap != nil && ap.IsA() && ap.A != nil {
		s.AdditionalProperties = schemaFromProxy(ap.A)
	}

	if ap != nil && ap.IsB() && !ap.B {
		s.AdditionalPropertiesFalse = true
	}

	if sh.Properties != nil {
		for pair := sh.Properties.First(); pair != nil; pair = pair.Next() {
			propProxy := pair.Value()
			propHigh := schemaProxyHigh(propProxy)

			s.Properties = append(s.Properties, &Property{
				Name:             pair.Key(),
				Schema:           schemaFromProxy(propProxy),
				Required:         containsString(s.Required, pair.Key()),
				RequestRequired:  readBoolExtension(propHigh, extRequestRequired),
				ResponseRequired: readBoolExtension(propHigh, extResponseRequired),
				Optional:         readBoolExtension(propHigh, extOptional),
			})
		}
	}

	s.AllOf = appendComposites(s.AllOf, sh.AllOf)
	s.OneOf = appendComposites(s.OneOf, sh.OneOf)
	s.AnyOf = appendComposites(s.AnyOf, sh.AnyOf)
}

func appendComposites(dst []*Schema, proxies []*highbase.SchemaProxy) []*Schema {
	for _, p := range proxies {
		dst = append(dst, schemaFromProxy(p))
	}

	return dst
}

// extractComponentsSchemas проходит components.schemas и наполняет doc.Schemas.
func extractComponentsSchemas(doc *Document, schemas *orderedmap.Map[string, *highbase.SchemaProxy]) { //nolint:lll // generic type signature
	if schemas == nil {
		return
	}

	for pair := schemas.First(); pair != nil; pair = pair.Next() {
		s := schemaFromProxy(pair.Value())
		s.Name = pair.Key()
		doc.Schemas = append(doc.Schemas, s)
	}
}

func refToSchemaName(ref string) string {
	if ref == "" {
		return ""
	}

	if idx := strings.LastIndex(ref, "/"); idx >= 0 {
		return ref[idx+1:]
	}

	return strings.TrimPrefix(ref, "#/components/schemas/")
}

func boolPtrOrFalse(b *bool) bool {
	return b != nil && *b
}

// decodeNode декодирует yaml.Node в any (string/int/float/bool/...).
// yaml.v3 не возвращает ошибку для any-таргета (паникует на невалидных узлах),
// поэтому ветки ошибок нет — узлы приходят от libopenapi, уже валидные.
func decodeNode(n *yaml.Node) any {
	if n == nil {
		return nil
	}

	var v any
	if err := n.Decode(&v); err != nil {
		return nil
	}

	return v
}

func containsString(slice []string, v string) bool {
	return slices.Contains(slice, v)
}

// schemaProxyHigh возвращает *highbase.Schema из proxy. Для $ref-proxy
// возвращается разрешённая целевая схема; если proxy nil или не разрешается —
// nil. Используется для чтения per-property x-* расширений.
func schemaProxyHigh(proxy *highbase.SchemaProxy) *highbase.Schema {
	if proxy == nil {
		return nil
	}

	return proxy.Schema()
}

// readBoolExtension читает scalar-bool расширение (x-optional,
// x-request-required, x-response-required) из Extensions high-схемы
// свойства. Возвращает false, если расширение отсутствует или его значение
// не bool.
func readBoolExtension(sh *highbase.Schema, key string) bool {
	if sh == nil || sh.Extensions == nil {
		return false
	}

	node := sh.Extensions.GetOrZero(key)
	if node == nil || node.Kind != yaml.ScalarNode {
		return false
	}

	var b bool
	if err := node.Decode(&b); err != nil {
		return false
	}

	return b
}
