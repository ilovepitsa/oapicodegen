package generator

import (
	"strings"

	"nschugorev/oapigenerator/internal/parser"
)

// operationMethodName возвращает Go-имя метода интерфейса для операции.
// Если есть operationId — используется он, иначе имя выводится из method+path.
func operationMethodName(op *parser.Operation) string {
	if op.OperationID != "" {
		return goName(op.OperationID)
	}

	return deriveMethodName(op.Method, op.Path)
}

func deriveMethodName(method, path string) string {
	var b strings.Builder

	b.WriteString(strings.ToLower(method))

	for _, seg := range strings.Split(path, "/") {
		if seg == "" {
			continue
		}

		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			inner := seg[1 : len(seg)-1]

			b.WriteString("By")
			b.WriteString(goName(inner))
		} else {
			b.WriteString(goName(seg))
		}
	}

	return goName(b.String())
}

// responseFieldName строит имя поля Response-структуры для кода ответа.
// "200" → "Response200", "default" → "ResponseDefault", "4XX" → "Response4XX".
func responseFieldName(code string) string {
	return "Response" + goName(code)
}

// isSuccessCode сообщает, является ли код ответа 2xx.
func isSuccessCode(code string) bool {
	if code == oapiCodeDefault {
		return false
	}

	if len(code) < 3 {
		return false
	}

	return code[0] == '2'
}
