package walk

import (
	"fmt"
	"nschugorev/oapigenerator/internal/parser"
	"sort"
)

// MethodWalker обходит *parser.Method плоским списком: метод → параметры по
// категориям (path/query/header/cookie) → тело запроса → ответы с заголовками.
// В отличие от SchemaWalker, не рекурсивный — не спускается в body/response
// как schema-дерево; для этого renderer'ы вызывают SchemaWalker сами.
type MethodWalker struct {
	renderers []MethodRenderer
}

// NewMethodWalker строит walker'а с набором renderer'ов. Порядок вызова хуков
// совпадает с порядком аргументов.
func NewMethodWalker(r ...MethodRenderer) *MethodWalker {
	return &MethodWalker{renderers: r}
}

// Walk запускает обход метода m. nil-метод игнорируется.
func (w *MethodWalker) Walk(m *parser.Method) error {
	if m == nil {
		return nil
	}

	if err := w.callEach(w.onMethod(m)); err != nil {
		return fmt.Errorf("walk method %q: %w", m.OperationID, err)
	}

	for _, p := range m.Parameters {
		if err := w.dispatchParameter(m, p); err != nil {
			return err
		}
	}

	if m.RequestBody != nil {
		if err := w.callEach(w.onRequestBody(m)); err != nil {
			return fmt.Errorf("method %q request body: %w", m.OperationID, err)
		}
	}

	for _, resp := range m.Responses {
		if err := w.dispatchResponse(m, resp); err != nil {
			return err
		}
	}

	return nil
}

// dispatchParameter выбирает хук по категории параметра (p.In) и вызывает его
// для каждого renderer'а. Неизвестная категория молча пропускается —
// extractor'ы parser'а заполняют In только для path/query/header/cookie.
func (w *MethodWalker) dispatchParameter(m *parser.Method, p *parser.Parameter) error {
	fn, ok := w.parameterHook(m, p)
	if !ok {
		return nil
	}

	if err := w.callEach(fn); err != nil {
		return fmt.Errorf("method %q parameter %q (%s): %w", m.OperationID, p.Name, p.In, err)
	}

	return nil
}

// dispatchResponse вызывает OnResponse, затем OnResponseHeader для каждого
// заголовка ответа. Заголовки сортируются по имени для детерминированного
// обхода (map итерируется случайно).
func (w *MethodWalker) dispatchResponse(m *parser.Method, resp *parser.Response) error {
	if err := w.callEach(w.onResponse(m, resp)); err != nil {
		return fmt.Errorf("method %q response %s: %w", m.OperationID, resp.StatusCode, err)
	}

	for _, name := range sortedHeaderNames(resp.Headers) {
		h := resp.Headers[name]
		if err := w.callEach(w.onResponseHeader(m, resp, name, h)); err != nil {
			return fmt.Errorf(
				"method %q response %s header %q: %w",
				m.OperationID, resp.StatusCode, name, err,
			)
		}
	}

	return nil
}

// parameterHook возвращает замыкание хука параметра по категории p.In.
// Второе возвращаемое значение false — категория неизвестна, пропуск.
func (w *MethodWalker) parameterHook(
	m *parser.Method, p *parser.Parameter,
) (func(MethodRenderer) error, bool) {
	switch p.In {
	case "path":
		return func(r MethodRenderer) error { return r.OnPathParameter(m, p) }, true
	case "query":
		return func(r MethodRenderer) error { return r.OnQueryParameter(m, p) }, true
	case "header":
		return func(r MethodRenderer) error { return r.OnHeaderParameter(m, p) }, true
	case "cookie":
		return func(r MethodRenderer) error { return r.OnCookieParameter(m, p) }, true
	default:
		return nil, false
	}
}

func (w *MethodWalker) onMethod(m *parser.Method) func(MethodRenderer) error {
	return func(r MethodRenderer) error { return r.OnMethod(m) }
}

func (w *MethodWalker) onRequestBody(m *parser.Method) func(MethodRenderer) error {
	return func(r MethodRenderer) error { return r.OnRequestBody(m, m.RequestBody) }
}

func (w *MethodWalker) onResponse(
	m *parser.Method, resp *parser.Response,
) func(MethodRenderer) error {
	return func(r MethodRenderer) error { return r.OnResponse(m, resp.StatusCode, resp) }
}

func (w *MethodWalker) onResponseHeader(
	m *parser.Method, resp *parser.Response, name string, h *parser.Parameter,
) func(MethodRenderer) error {
	return func(r MethodRenderer) error { return r.OnResponseHeader(m, resp.StatusCode, name, h) }
}

// callEach вызывает fn для каждого renderer'а, возвращая первую ошибку.
func (w *MethodWalker) callEach(fn func(MethodRenderer) error) error {
	for _, r := range w.renderers {
		if err := fn(r); err != nil {
			return err
		}
	}

	return nil
}

// sortedHeaderNames возвращает имена заголовков ответа в лексикографическом
// порядке для детерминированного обхода (map итерируется случайно).
func sortedHeaderNames(headers map[string]*parser.Parameter) []string {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}
