package parser

import (
	"strings"

	highv3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
)

// extractPaths проходит paths и наполняет doc.Paths и doc.Operations.
func extractPaths(doc *Document, paths *highv3.Paths) {
	if paths == nil || paths.PathItems == nil {
		return
	}

	for pair := paths.PathItems.First(); pair != nil; pair = pair.Next() {
		pathStr := pair.Key()
		item := pair.Value()

		pi := &PathItem{Path: pathStr}
		ops := item.GetOperations()
		if ops != nil {
			for opPair := ops.First(); opPair != nil; opPair = opPair.Next() {
				op := convertOperation(pathStr, opPair.Key(), opPair.Value())
				pi.Operations = append(pi.Operations, op)
				doc.Operations = append(doc.Operations, op)
			}
		}
		doc.Paths = append(doc.Paths, pi)
	}
}

func convertOperation(path, method string, op *highv3.Operation) *Operation {
	out := &Operation{
		Method:      strings.ToUpper(method),
		Path:        path,
		OperationID: op.OperationId,
		Summary:     op.Summary,
		Description: op.Description,
		Tags:        append([]string(nil), op.Tags...),
		Deprecated:  boolPtrOrFalse(op.Deprecated),
	}

	for _, p := range op.Parameters {
		out.Parameters = append(out.Parameters, convertParameter(p))
	}

	if op.RequestBody != nil {
		out.RequestBody = convertRequestBody(op.RequestBody)
	}

	if op.Responses != nil {
		if op.Responses.Default != nil {
			out.Responses = append(out.Responses, convertResponse("default", op.Responses.Default))
		}
		if op.Responses.Codes != nil {
			for codePair := op.Responses.Codes.First(); codePair != nil; codePair = codePair.Next() {
				out.Responses = append(out.Responses, convertResponse(codePair.Key(), codePair.Value()))
			}
		}
	}

	return out
}

func convertParameter(p *highv3.Parameter) *Parameter {
	out := &Parameter{
		Name:        p.Name,
		In:          p.In,
		Description: p.Description,
		Required:    boolPtrOrFalse(p.Required),
		Deprecated:  p.Deprecated,
	}
	if p.Schema != nil {
		out.Schema = schemaFromProxy(p.Schema)
	}
	return out
}

func convertRequestBody(rb *highv3.RequestBody) *RequestBody {
	return &RequestBody{
		Description: rb.Description,
		Required:    boolPtrOrFalse(rb.Required),
		Content:     convertContent(rb.Content),
	}
}

func convertResponse(code string, r *highv3.Response) *Response {
	return &Response{
		StatusCode:  code,
		Description: r.Description,
		Content:     convertContent(r.Content),
		Headers:     convertHeaders(r.Headers),
	}
}

func convertContent(content *orderedmap.Map[string, *highv3.MediaType]) map[string]*MediaType {
	if content == nil {
		return nil
	}
	out := make(map[string]*MediaType, content.Len())
	for pair := content.First(); pair != nil; pair = pair.Next() {
		mt := &MediaType{}
		if pair.Value() != nil && pair.Value().Schema != nil {
			mt.Schema = schemaFromProxy(pair.Value().Schema)
		}
		out[pair.Key()] = mt
	}
	return out
}

func convertHeaders(headers *orderedmap.Map[string, *highv3.Header]) map[string]*Parameter {
	if headers == nil {
		return nil
	}
	out := make(map[string]*Parameter, headers.Len())
	for pair := headers.First(); pair != nil; pair = pair.Next() {
		h := pair.Value()
		p := &Parameter{
			Name:        pair.Key(),
			In:          "header",
			Description: h.Description,
			Required:    h.Required,
			Deprecated:  h.Deprecated,
		}
		if h.Schema != nil {
			p.Schema = schemaFromProxy(h.Schema)
		}
		out[pair.Key()] = p
	}
	return out
}
