package generator

import (
	"nschugorev/oapigenerator/internal/parser"
)

// formURLContentType — media-type для URL-form encoded request body.
const formURLContentType = "application/x-www-form-urlencoded"

// schemeHasURLFormat сообщает, ссылается ли form-urlencoded request body
// какой-либо операции на схему sh (по $ref-имени). Inline-схемы без Name
// не поддерживаются — используйте $ref на components.schemas.
func schemeHasURLFormat(sh *parser.Schema, operations []*parser.Method) bool {
	if sh == nil || sh.Name == "" {
		return false
	}

	for _, op := range operations {
		if op.RequestBody == nil {
			continue
		}

		mt, ok := op.RequestBody.Content[formURLContentType]
		if !ok || mt.Schema == nil {
			continue
		}

		if mt.Schema.Ref != "" && refToName(mt.Schema.Ref) == sh.Name {
			return true
		}
	}

	return false
}

// requestBodyIsURLForm сообщает, использует ли операция form-urlencoded
// request body (вместо application/json).
func requestBodyIsURLForm(rb *parser.RequestBody) bool {
	if rb == nil || rb.Content == nil {
		return false
	}

	_, ok := rb.Content[formURLContentType]

	return ok
}
