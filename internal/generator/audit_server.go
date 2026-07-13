package generator

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/parser"
)

// auditClientFile генерирует interfaces/client/audit.gen.go с audit-версиями
// request/response-структур операций и методами GetAuditData.
//
// Для каждой операции с path/query-параметрами или body-схемой:
//   - <Op>RequestAuditData — path/query params + Body any
//   - func (req *<Op>Request) GetAuditData() any
//
// Для каждого response с schema (кроме default):
//   - <Op>Response<Code>AuditData — Payload any
//   - func (resp *<Op>Response) Response<Code>AuditData() <Op>Response<Code>AuditData
//
// Body field типа any заполняется через req.Body.GetAuditData(), если body —
// $ref на именованную схему (у которой T27.3 генерирует GetAuditData).
// Inline body-схемы не поддерживаются — для них Body field не генерируется.
func (g *Generator) auditClientFile() codegen.File {
	m := g.newTypeMapper("client")
	w := codegen.NewBufferWriter()

	for _, op := range g.doc.Operations {
		g.renderOpRequestAudit(w, op, m)
	}

	for _, op := range g.doc.Operations {
		g.renderOpResponseAudit(w, op, m)
	}

	return g.factory.Create(&gogen.File{
		Package: "client",
		Imports: m.imports,
		Body:    w.Content(),
	})
}

// renderOpRequestAudit рендерит <Op>RequestAuditData struct + метод
// GetAuditData на *<Op>Request. Пропускает операцию без path/query-параметров
// и без body со схемой.
func (g *Generator) renderOpRequestAudit(
	w *codegen.BufferWriter,
	op *parser.Operation,
	m *typeMapper,
) {
	name := operationMethodName(op)
	auditName := name + "RequestAuditData"

	pathParams := filterParamsByIn(op.Parameters, oapiParamPath)
	queryParams := filterParamsByIn(op.Parameters, oapiParamQuery)
	bodySchema := g.resolveBodySchema(op.RequestBody)
	hasBody := bodySchema != nil && bodySchema.Name != ""

	if len(pathParams) == 0 && len(queryParams) == 0 && !hasBody {
		return
	}

	g.renderRequestAuditStruct(w, auditName, pathParams, queryParams, hasBody, m)
	g.renderRequestAuditMethod(w, name, auditName, pathParams, queryParams, op.RequestBody, hasBody)
}

// renderRequestAuditStruct рендерит `type <AuditName> struct { ... }`.
// Path/query params копируются как есть; Body — any (audit-данные тела).
func (g *Generator) renderRequestAuditStruct(
	w *codegen.BufferWriter,
	auditName string,
	pathParams, queryParams []*parser.Parameter,
	hasBody bool,
	m *typeMapper,
) {
	w.Print("type ", auditName, " struct {\n")

	for _, p := range pathParams {
		g.renderAuditParamField(w, p, m)
	}

	for _, p := range queryParams {
		g.renderAuditParamField(w, p, m)
	}

	if hasBody {
		w.Print("\tBody any\n")
	}

	w.Print("}\n\n")
}

// renderAuditParamField рендерит поле path/query-параметра в audit-структуре.
// Тип совпадает с полем в <Op>Request — копирование через присваивание.
func (g *Generator) renderAuditParamField(
	w *codegen.BufferWriter,
	p *parser.Parameter,
	m *typeMapper,
) {
	fieldName := goName(p.Name)
	fieldType := m.goType(p.Schema)
	w.Print("\t", fieldName, " ", fieldType, "\n")
}

// renderRequestAuditMethod рендерит:
//
//	func (req *<Op>Request) GetAuditData() any {
//	    am := <Op>RequestAuditData{
//	        <PathField>: req.<PathField>,
//	        ...
//	    }
//	    if req.Body != nil {
//	        am.Body = req.Body.GetAuditData()
//	    }
//	    return am
//	}
//
// Для required body nil-проверка опускается (всегда присваиваем).
func (g *Generator) renderRequestAuditMethod(
	w *codegen.BufferWriter,
	opName, auditName string,
	pathParams, queryParams []*parser.Parameter,
	rb *parser.RequestBody,
	hasBody bool,
) {
	w.Print("func (req *", opName, "Request) GetAuditData() any {\n")
	w.Print("\tam := ", auditName, "{\n")

	for _, p := range pathParams {
		f := goName(p.Name)
		w.Print("\t\t", f, ": req.", f, ",\n")
	}

	for _, p := range queryParams {
		f := goName(p.Name)
		w.Print("\t\t", f, ": req.", f, ",\n")
	}

	w.Print("\t}\n")

	if hasBody {
		required := rb != nil && rb.Required
		if required {
			w.Print("\tam.Body = req.Body.GetAuditData()\n")
		} else {
			w.Print("\tif req.Body != nil {\n")
			w.Print("\t\tam.Body = req.Body.GetAuditData()\n")
			w.Print("\t}\n")
		}
	}

	w.Print("\treturn am\n")
	w.Print("}\n\n")
}

// renderOpResponseAudit рендерит для каждого response со schema (кроме default):
//   - <Op>Response<Code>AuditData struct { Payload any }
//   - func (resp *<Op>Response) Response<Code>AuditData() <Op>Response<Code>AuditData
func (g *Generator) renderOpResponseAudit(
	w *codegen.BufferWriter,
	op *parser.Operation,
	_ *typeMapper,
) {
	opName := operationMethodName(op)

	for _, r := range op.Responses {
		if r.StatusCode == oapiCodeDefault {
			continue
		}

		schema := responseSchema(r)
		if schema == nil {
			continue
		}

		codeName := goName(r.StatusCode)
		auditName := opName + "Response" + codeName + "AuditData"
		methodName := "Response" + codeName + "AuditData"
		fieldName := responseFieldName(r.StatusCode)

		g.renderResponseAuditStruct(w, auditName)
		g.renderResponseAuditMethod(w, opName, auditName, methodName, fieldName, hasResponseHeaders(r))
	}
}

// renderResponseAuditStruct рендерит `type <AuditName> struct { Payload any }`.
func (g *Generator) renderResponseAuditStruct(
	w *codegen.BufferWriter,
	auditName string,
) {
	w.Print("type ", auditName, " struct {\n")
	w.Print("\tPayload any\n")
	w.Print("}\n\n")
}

// renderResponseAuditMethod рендерит метод, возвращающий audit-данные
// конкретного response-кода. Для headers-обёртки обращается через .Payload.
func (g *Generator) renderResponseAuditMethod(
	w *codegen.BufferWriter,
	opName, auditName, methodName, fieldName string,
	hasHeaders bool,
) {
	w.Print("func (resp *", opName, "Response) ", methodName, "() ", auditName, " {\n") //nolint:lll // method signature
	w.Print("\tam := ", auditName, "{}\n")
	w.Print("\tif resp.", fieldName, " != nil {\n")

	if hasHeaders {
		w.Print("\t\tif resp.", fieldName, ".Payload != nil {\n")
		w.Print("\t\t\tam.Payload = resp.", fieldName, ".Payload.GetAuditData()\n")
		w.Print("\t\t}\n")
	} else {
		w.Print("\t\tam.Payload = resp.", fieldName, ".GetAuditData()\n")
	}

	w.Print("\t}\n")
	w.Print("\treturn am\n")
	w.Print("}\n\n")
}

// filterParamsByIn возвращает параметры с указанным In (path/query/header/cookie).
func filterParamsByIn(params []*parser.Parameter, in string) []*parser.Parameter {
	out := make([]*parser.Parameter, 0, len(params))

	for _, p := range params {
		if p.In == in {
			out = append(out, p)
		}
	}

	return out
}
