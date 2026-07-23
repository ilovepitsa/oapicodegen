package generator

import (
	"nschugorev/oapigenerator/internal/parser"
	"sort"
)

// Ниже — общие хелперы, вынесенные из client.go (удалён в Phase 3).
// Они используются несколькими legacy-файлами (impl_client.go, client_sugar.go,
// impl_server.go, audit_server.go) и будут удалены после миграции этих файлов
// в singleton-renderer'ы.

// hasResponseHeaders сообщает, есть ли у ответа описанные headers.
func hasResponseHeaders(resp *parser.Response) bool {
	return resp != nil && len(resp.Headers) > 0
}

// bodySchema возвращает схему тела запроса (первый content-type).
func bodySchema(rb *parser.RequestBody) *parser.Schema {
	if rb == nil || rb.Content == nil {
		return nil
	}

	return firstContentSchema(rb.Content)
}

// responseSchema возвращает схему ответа (первый content-type).
func responseSchema(resp *parser.Response) *parser.Schema {
	if resp == nil || resp.Content == nil {
		return nil
	}

	return firstContentSchema(resp.Content)
}

func firstContentSchema(content map[string]*parser.MediaType) *parser.Schema {
	if _, ok := content["application/json"]; ok {
		return content["application/json"].Schema
	}

	keys := make([]string, 0, len(content))
	for k := range content {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	if len(keys) == 0 {
		return nil
	}

	return content[keys[0]].Schema
}

func sortedResponseCodes(responses []*parser.Response) []string {
	codes := make([]string, 0, len(responses))
	for _, r := range responses {
		codes = append(codes, r.StatusCode)
	}

	sort.Slice(codes, func(i, j int) bool {
		if codes[i] == oapiCodeDefault {
			return false
		}

		if codes[j] == oapiCodeDefault {
			return true
		}

		return codes[i] < codes[j]
	})

	return codes
}

func responseByCode(responses []*parser.Response, code string) *parser.Response {
	for _, r := range responses {
		if r.StatusCode == code {
			return r
		}
	}

	return nil
}

// resolveRefSchema возвращает схему из project.Model по $ref.
// Если s — не $ref, cross-service $ref (ExternalRef), или имя не найдено — возвращает nil.
func (g *Generator) resolveRefSchema(s *parser.Schema) *parser.Schema {
	if s == nil || s.ExternalRef != "" || s.Ref == "" {
		return nil
	}

	name := refToName(s.Ref)
	if sh, ok := g.project.Model.Lookup(name); ok {
		return sh
	}

	return nil
}
