package generator

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/parser"
	"strings"
)

const sensitivePkg = "nschugorev/oapigenerator/pkg/sensitive"

// auditModelFile генерирует model/<name>_audit_data.gen.go с audit-версией
// схемы и методом GetAuditData. Sensitive-поля (x-sensitive: true) маскируются
// через sensitive.Sensitive[T]; остальные копируются как есть.
//
// Триггер: схема referenced из request body или response любой операции —
// см. schemaReferencedByOperation.
func (g *Generator) auditModelFile(sh *parser.Schema) codegen.File {
	m := g.newTypeMapper("model")
	w := codegen.NewBufferWriter()

	name := goName(sh.Name)
	g.renderAuditStruct(w, sh, m, name)
	g.renderGetAuditData(w, sh, m, name)

	return g.factory.Create(&gogen.File{
		Package: "model",
		Imports: m.imports,
		Body:    w.Content(),
	})
}

// schemaReferencedByOperation сообщает, ссылается ли request body или
// response любой операции на схему sh (по $ref-имени).
func schemaReferencedByOperation(sh *parser.Schema, operations []*parser.Method) bool {
	if sh == nil || sh.Name == "" {
		return false
	}

	for _, op := range operations {
		if schemaInRequest(op.RequestBody, sh.Name) || schemaInResponses(op.Responses, sh.Name) {
			return true
		}
	}

	return false
}

func schemaInRequest(rb *parser.RequestBody, name string) bool {
	if rb == nil {
		return false
	}

	for _, mt := range rb.Content {
		if mt.Schema != nil && mt.Schema.Ref != "" && refToName(mt.Schema.Ref) == name {
			return true
		}
	}

	return false
}

func schemaInResponses(responses []*parser.Response, name string) bool {
	for _, resp := range responses {
		for _, mt := range resp.Content {
			if mt.Schema != nil && mt.Schema.Ref != "" && refToName(mt.Schema.Ref) == name {
				return true
			}
		}
	}

	return false
}

// renderAuditStruct рендерит `type <Name>AuditData struct { ... }`.
//
// Sensitive-поля получают тип sensitive.Sensitive[T] (или *sensitive.Sensitive[T]
// для pointer-полей); остальные — тот же тип, что в оригинале.
func (g *Generator) renderAuditStruct(
	w *codegen.BufferWriter,
	sh *parser.Schema,
	m *typeMapper,
	name string,
) {
	w.Print("type ", name, "AuditData struct {\n")

	for _, p := range sh.Properties {
		if p.Schema == nil {
			continue
		}

		g.renderAuditField(w, p, m)
	}

	w.Print("}\n\n")
}

// renderAuditField рендерит одно поле audit-struct'а.
func (g *Generator) renderAuditField(
	w *codegen.BufferWriter,
	p *parser.Property,
	m *typeMapper,
) {
	fieldName := goName(p.Name)
	fieldType := m.goType(p.Schema)
	required := g.requiredForMode(p, m.mode)

	if fieldIsOptional(required, fieldType) {
		fieldType = "*" + fieldType
	}

	pointer := strings.HasPrefix(fieldType, "*")

	if p.Sensitive {
		m.addImport(sensitivePkg, "sensitive")

		fieldType = auditSensitiveType(fieldType, pointer)
	}

	omitEmpty := ""
	if !required {
		omitEmpty = ",omitempty"
	}

	w.Print(fieldName, " ", fieldType, " `json:\"", p.Name, omitEmpty, "\" yaml:\"", p.Name, omitEmpty, "\"`\n") //nolint:lll // struct tag line
}

// auditSensitiveType возвращает sensitive-обёрнутый тип для audit-поля.
// pointer=true → *sensitive.Sensitive[T], иначе sensitive.Sensitive[T].
func auditSensitiveType(fieldType string, pointer bool) string {
	baseType := fieldType
	if pointer {
		baseType = strings.TrimPrefix(fieldType, "*")
	}

	if pointer {
		return "*sensitive.Sensitive[" + baseType + "]"
	}

	return "sensitive.Sensitive[" + baseType + "]"
}

// renderGetAuditData рендерит `func (m <Name>) GetAuditData() any { ... }`.
//
// Для каждого поля:
//   - non-sensitive value:  am.Field = m.Field
//   - non-sensitive pointer: am.Field = m.Field
//   - sensitive value:       am.Field = sensitive.New(m.Field)
//   - sensitive pointer:     if m.Field != nil { v := sensitive.New(*m.Field); am.Field = &v }
func (g *Generator) renderGetAuditData(
	w *codegen.BufferWriter,
	sh *parser.Schema,
	m *typeMapper,
	name string,
) {
	w.Print("func (m ", name, ") GetAuditData() any {\n")
	w.Print("\tvar am ", name, "AuditData\n")

	for _, p := range sh.Properties {
		if p.Schema == nil {
			continue
		}

		fieldName := goName(p.Name)
		required := g.requiredForMode(p, m.mode)
		fieldType := m.goType(p.Schema)

		if fieldIsOptional(required, fieldType) {
			fieldType = "*" + fieldType
		}

		pointer := strings.HasPrefix(fieldType, "*")

		g.renderAuditCopyStmt(w, m, p, fieldName, pointer)
	}

	w.Print("\treturn am\n")
	w.Print("}\n\n")
}

// renderAuditCopyStmt рендерит оператор копирования одного поля в audit-struct.
func (g *Generator) renderAuditCopyStmt(
	w *codegen.BufferWriter,
	m *typeMapper,
	p *parser.Property,
	fieldName string,
	pointer bool,
) {
	if !p.Sensitive {
		w.Print("\tam.", fieldName, " = m.", fieldName, "\n")

		return
	}

	m.addImport(sensitivePkg, "sensitive")

	if pointer {
		w.Print("\tif m.", fieldName, " != nil {\n")
		w.Print("\t\tv := sensitive.New(*m.", fieldName, ")\n")
		w.Print("\t\tam.", fieldName, " = &v\n")
		w.Print("\t}\n")

		return
	}

	w.Print("\tam.", fieldName, " = sensitive.New(m.", fieldName, ")\n")
}
