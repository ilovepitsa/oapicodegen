package operations

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
	"strings"
)

// AuditClientRenderer рендерит interfaces/client/audit.gen.go с audit-версиями
// request/response-структур операций и методами GetAuditData.
// Заменяет Generator.auditClientFile (internal/generator/audit_server.go).
type AuditClientRenderer struct{}

func NewAuditClientRenderer() *AuditClientRenderer { return &AuditClientRenderer{} }

func (AuditClientRenderer) FilePath() string { return "interfaces/client/audit.gen.go" }

func (r *AuditClientRenderer) Render(ctx *render.RenderContext) ([]byte, *render.ImportTracker, error) {
	imps := render.NewImportTracker()
	ctx.Imports = imps

	ops := allOperations(ctx.Project)
	m := ctx.TypeMapper

	w := codegen.NewBufferWriter()

	for _, op := range ops {
		renderOpRequestAudit(w, op, m, ctx.Project)
	}

	for _, op := range ops {
		renderOpResponseAudit(w, op)
	}

	return w.Content(), imps, nil
}

func renderOpRequestAudit(w *codegen.BufferWriter, op *parser.Method, m render.TypeMapper, project *parser.Project) {
	name := operationMethodName(op)
	auditName := name + "RequestAuditData"

	pathParams := filterParamsByIn(op.Parameters, "path")
	queryParams := filterParamsByIn(op.Parameters, "query")
	resolvedSchema := resolveBodySchema(op.RequestBody, project)
	hasBody := resolvedSchema != nil && resolvedSchema.Name != ""

	if len(pathParams) == 0 && len(queryParams) == 0 && !hasBody {
		return
	}

	renderRequestAuditStruct(w, auditName, pathParams, queryParams, hasBody, m)
	renderRequestAuditMethod(w, name, auditName, pathParams, queryParams, op.RequestBody, hasBody)
}

func renderRequestAuditStruct(w *codegen.BufferWriter, auditName string, pathParams, queryParams []*parser.Parameter, hasBody bool, m render.TypeMapper) {
	w.Print("type ", auditName, " struct {\n")

	for _, p := range pathParams {
		renderAuditParamField(w, p, m)
	}

	for _, p := range queryParams {
		renderAuditParamField(w, p, m)
	}

	if hasBody {
		w.Print("\tBody any\n")
	}

	w.Print("}\n\n")
}

func renderAuditParamField(w *codegen.BufferWriter, p *parser.Parameter, m render.TypeMapper) {
	fieldName := goName(p.Name)
	fieldType := m.GoType(p.Schema)

	required := p.Required || p.In == "path"
	if !required && !strings.HasPrefix(fieldType, "*") && !isInherentlyNilable(fieldType) {
		fieldType = "*" + fieldType
	}

	w.Print("\t", fieldName, " ", fieldType, "\n")
}

func renderRequestAuditMethod(w *codegen.BufferWriter, opName, auditName string, pathParams, queryParams []*parser.Parameter, rb *parser.RequestBody, hasBody bool) {
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

func renderOpResponseAudit(w *codegen.BufferWriter, op *parser.Method) {
	opName := operationMethodName(op)

	for _, r := range op.Responses {
		if r.StatusCode == "default" {
			continue
		}

		schema := responseSchema(r)
		if schema == nil {
			continue
		}

		if schema.Ref == "" {
			continue
		}

		codeName := goName(r.StatusCode)
		auditName := opName + "Response" + codeName + "AuditData"
		methodName := "Response" + codeName + "AuditData"
		fieldName := responseFieldName(r.StatusCode)

		renderResponseAuditStruct(w, auditName)
		renderResponseAuditMethod(w, opName, auditName, methodName, fieldName, hasResponseHeaders(r))
	}
}

func renderResponseAuditStruct(w *codegen.BufferWriter, auditName string) {
	w.Print("type ", auditName, " struct {\n")
	w.Print("\tPayload any\n")
	w.Print("}\n\n")
}

func renderResponseAuditMethod(w *codegen.BufferWriter, opName, auditName, methodName, fieldName string, hasHeaders bool) {
	w.Print("func (resp *", opName, "Response) ", methodName, "() ", auditName, " {\n")
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

var _ render.SingletonRenderer = (*AuditClientRenderer)(nil)
