package generator

import (
	"github.com/ilovepitsa/oapicodegen/internal/codegen"
	"github.com/ilovepitsa/oapicodegen/internal/codegen/gogen"
	"github.com/ilovepitsa/oapicodegen/internal/parser"
	"strings"
)

const sensitivePkg = "github.com/ilovepitsa/oapicodegen/pkg/sensitive"

// auditModelFile генерирует model/<name>_audit_data.gen.go с audit-версией
// схемы и методом GetAuditData. Sensitive-поля (x-sensitive: true) маскируются
// через sensitive.Sensitive[T]; остальные копируются как есть.
//
// Триггер: схема referenced из request body или response любой операции —
// см. schemaReferencedByOperation.
//
// При включённом GOLANG_SPLIT_REQUEST_RESPONSE схема рендерится как
// <Name>Request + <Name>Response (см. StructRenderer.OnSplitStruct). Audit
// повторяет эту разбивку: для каждого варианта генерируется собственный
// <Name><Variant>AuditData-struct и метод GetAuditData на соответствующем
// типе (<Name>Request / <Name>Response). Поля фильтруются по варианту так же,
// как StructRenderer: Request — без readOnly, Response — без writeOnly.
// Это требуется, потому что interfaces/client/audit.gen.go зовёт
// GetAuditData и на request-body, и на response-типе операции.
func (g *Generator) auditModelFile(sh *parser.Schema) codegen.File {
	w := codegen.NewBufferWriter()

	baseName := goName(sh.Name)
	splittable := g.splittable != nil && g.splittable[sh.Name]

	if !splittable {
		m := g.newTypeMapper("model")
		g.renderAuditStruct(w, sh, m, baseName, nil)
		g.renderGetAuditData(w, sh, m, baseName, nil)

		return g.factory.Create(&gogen.File{
			Package: "model",
			Imports: m.imports,
			Body:    w.Content(),
		})
	}

	// Split: Request + Response варианты с фильтрами как у StructRenderer.
	reqM := g.newTypeMapper("model")
	reqM.mode = modeRequest
	reqName := baseName + modeRequest
	reqKeep := func(p *parser.Property) bool { return p.Schema == nil || !p.Schema.ReadOnly }
	g.renderAuditStruct(w, sh, reqM, reqName, reqKeep)
	g.renderGetAuditData(w, sh, reqM, reqName, reqKeep)

	respM := g.newTypeMapper("model")
	respM.mode = modeResponse
	respName := baseName + modeResponse
	respKeep := func(p *parser.Property) bool { return p.Schema == nil || !p.Schema.WriteOnly }
	g.renderAuditStruct(w, sh, respM, respName, respKeep)
	g.renderGetAuditData(w, sh, respM, respName, respKeep)

	imports := mergeImports(reqM.imports, respM.imports)

	return g.factory.Create(&gogen.File{
		Package: "model",
		Imports: imports,
		Body:    w.Content(),
	})
}

// mergeImports объединяет два списка импортов, удаляя дубликаты.
func mergeImports(a, b []gogen.Import) []gogen.Import {
	seen := make(map[gogen.Import]bool, len(a)+len(b))
	out := make([]gogen.Import, 0, len(a)+len(b))

	for _, imp := range append(append([]gogen.Import{}, a...), b...) {
		if seen[imp] {
			continue
		}

		seen[imp] = true
		out = append(out, imp)
	}

	return out
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
// keep != nil фильтрует свойства по split-варианту (совпадает с фильтрами
// StructRenderer.OnSplitStruct): nil = все поля (моно-режим).
//
// Sensitive-поля получают тип sensitive.Sensitive[T] (или *sensitive.Sensitive[T]
// для pointer-полей); остальные — тот же тип, что в оригинале.
func (g *Generator) renderAuditStruct(
	w *codegen.BufferWriter,
	sh *parser.Schema,
	m *typeMapper,
	name string,
	keep func(*parser.Property) bool,
) {
	w.Print("type ", name, "AuditData struct {\n")

	for _, p := range sh.Properties {
		if p.Schema == nil {
			continue
		}

		if keep != nil && !keep(p) {
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
// keep != nil фильтрует свойства по split-варианту (должен совпадать с keep,
// переданным в renderAuditStruct). Для каждого поля:
//   - non-sensitive value:  am.Field = m.Field
//   - non-sensitive pointer: am.Field = m.Field
//   - sensitive value:       am.Field = sensitive.New(m.Field)
//   - sensitive pointer:     if m.Field != nil { v := sensitive.New(*m.Field); am.Field = &v }
func (g *Generator) renderGetAuditData(
	w *codegen.BufferWriter,
	sh *parser.Schema,
	m *typeMapper,
	name string,
	keep func(*parser.Property) bool,
) {
	w.Print("func (m ", name, ") GetAuditData() any {\n")
	w.Print("\tvar am ", name, "AuditData\n")

	for _, p := range sh.Properties {
		if p.Schema == nil {
			continue
		}

		if keep != nil && !keep(p) {
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
