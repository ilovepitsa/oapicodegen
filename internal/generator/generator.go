// Package generator генерирует Go-код из parser.Project. Первая итерация:
// только стандартный OpenAPI 3.x (без x-* расширений, audit-data, split
// Request/Response, update-схем, URL-form-encoding — всё это в бэклоге).
//
// Layout (multi-package):
//
//	<import-prefix>/model/              — schemas + JSON-методы
//	<import-prefix>/interfaces/client/  — Client interface + Request/Response + sugar
//	<import-prefix>/interfaces/server/  — Server interface
//
// Для каждой схемы из components.schemas генерируется <name>.gen.go с
// определением Go-типа (struct / alias). Для oneOf/anyOf дополнительно
// генерируется <name>_json.gen.go с MarshalJSON/UnmarshalJSON.
package generator

import (
	"fmt"
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/parser"
)

// Generator конфигурируется через Option-ы и хранит общее состояние генерации.
type Generator struct {
	project     *parser.Project
	schemaIndex *parser.SchemaIndex
	factory     *gogen.FileFactory

	// splittable — имена object-схем, которые при включённом
	// GOLANG_SPLIT_REQUEST_RESPONSE рендерятся как <Name>Request + <Name>Response.
	// nil, если флаг выключен.
	splittable map[string]bool
}

// Option настраивает Generator.
type Option func(*Generator)

// Generate обходит все схемы и операции, пишет Go-файлы через fw.
func Generate(
	fw codegen.FileWriter,
	project *parser.Project,
	si *parser.SchemaIndex,
	opts ...Option,
) error {
	g := &Generator{
		project:     project,
		schemaIndex: si,
		factory:     gogen.NewFileFactory("oapigen"),
	}
	for _, opt := range opts {
		opt(g)
	}

	schemas := project.Model.Schemas()

	if g.project.Features.SplitRequestResponse.Value {
		g.splittable = computeSplittable(schemas)
	}

	for _, sh := range schemas {
		if sh.Name == "" {
			continue
		}

		if err := g.writeSchemaFiles(fw, sh); err != nil {
			return err
		}
	}

	if err := g.writeUTCTimeFile(fw); err != nil {
		return err
	}

	if err := g.writeExpectedValidatorsFile(fw); err != nil {
		return err
	}

	if hasOperations(project) {
		if err := g.writeOperationFiles(fw); err != nil {
			return err
		}
	}

	return nil
}

// hasOperations сообщает, есть ли в проекте хотя бы один метод.
func hasOperations(project *parser.Project) bool {
	for _, svc := range project.Paths.Services {
		if len(svc.Methods) > 0 {
			return true
		}
	}

	return false
}

// operations возвращает плоский срез всех методов проекта (по всем сервисам).
func (g *Generator) operations() []*parser.Method {
	var out []*parser.Method
	for _, svc := range g.project.Paths.Services {
		out = append(out, svc.Methods...)
	}

	return out
}

func (g *Generator) writeSchemaFiles(fw codegen.FileWriter, sh *parser.Schema) error {
	sf := g.schemaFile(sh)
	fname := "model/" + fileName(sh.Name) + ".gen.go"

	if err := fw.WriteFile(fname, sf); err != nil {
		return fmt.Errorf("write %s: %w", fname, err)
	}

	if needsJSONMethods(sh) {
		jf := g.jsonMethodsFile(sh)
		jname := "model/" + fileName(sh.Name) + "_json.gen.go"

		if err := fw.WriteFile(jname, jf); err != nil {
			return fmt.Errorf("write %s: %w", jname, err)
		}
	}

	if schemeHasURLFormat(sh, g.operations()) {
		uf := g.urlFormMethodsFile(sh)
		uname := "model/" + fileName(sh.Name) + "_url_form.gen.go"

		if err := fw.WriteFile(uname, uf); err != nil {
			return fmt.Errorf("write %s: %w", uname, err)
		}
	}

	if g.shouldGenerateConverters(sh) {
		cf := g.converterMethodsFile(sh)
		cname := "model/" + fileName(sh.Name) + "_converters.gen.go"

		if err := fw.WriteFile(cname, cf); err != nil {
			return fmt.Errorf("write %s: %w", cname, err)
		}
	}

	if schemaReferencedByOperation(sh, g.operations()) {
		af := g.auditModelFile(sh)
		aname := "model/" + fileName(sh.Name) + "_audit_data.gen.go"

		if err := fw.WriteFile(aname, af); err != nil {
			return fmt.Errorf("write %s: %w", aname, err)
		}
	}

	return nil
}

// shouldGenerateConverters сообщает, нужно ли генерировать <Name>_converters.gen.go
// для схемы. True только при включённом split-режиме для splittable-схемы с
// хотя бы одним shared-полем (не readOnly && не writeOnly).
func (g *Generator) shouldGenerateConverters(sh *parser.Schema) bool {
	if !g.project.Features.SplitRequestResponse.Value {
		return false
	}

	if !g.splittable[sh.Name] {
		return false
	}

	return schemaHasSharedFields(sh)
}

// writeUTCTimeFile пишет model/utc_time.gen.go, если включён флаг
// USE_UTC_FOR_DATE_TIME. Вызывается один раз за генерацию.
func (g *Generator) writeUTCTimeFile(fw codegen.FileWriter) error {
	if !g.project.Features.UseUTCForDateTime.Value {
		return nil
	}

	const fname = "model/utc_time.gen.go"

	if err := fw.WriteFile(fname, g.utcTimeFile()); err != nil {
		return fmt.Errorf("write %s: %w", fname, err)
	}

	return nil
}

// computeSplittable возвращает map имён object-схем с properties —
// тех, которые при включённом GOLANG_SPLIT_REQUEST_RESPONSE рендерятся
// как <Name>Request + <Name>Response.
//
// Схема исключается из split, если на неё ссылается composite-контекст
// (oneOf/anyOf/allOf/items/additionalProperties) любой другой схемы:
// эти рендеры идут с mode=="", поэтому splittable-ссылка породила бы
// несуществующий идентификатор (есть только <Name>Request/<Name>Response).
// Ссылки из properties других splittable-схем безопасны — renderSplitStruct
// выставляет mode. Ссылки из operation body/response тоже безопасны —
// они рендерятся в renderRequestStruct/renderResponseStruct с mode.
func computeSplittable(schemas []*parser.Schema) map[string]bool {
	out := make(map[string]bool)

	for _, sh := range schemas {
		if sh.Name == "" {
			continue
		}

		if sh.Type == oapiTypeObject && len(sh.Properties) > 0 &&
			len(sh.OneOf) == 0 && len(sh.AnyOf) == 0 && len(sh.AllOf) == 0 {
			out[sh.Name] = true
		}
	}

	for _, sh := range schemas {
		excludeReferencedByComposite(sh, out)
	}

	return out
}

// excludeReferencedByComposite удаляет из out имена схем, на которые
// ссылается sh через oneOf/anyOf/allOf/items/additionalProperties.
// Эти контексты рендерятся с mode=="", поэтому splittable-ссылка
// породила бы несуществующий идентификатор.
func excludeReferencedByComposite(sh *parser.Schema, out map[string]bool) {
	for _, v := range sh.OneOf {
		delete(out, refToName(v.Ref))
	}

	for _, v := range sh.AnyOf {
		delete(out, refToName(v.Ref))
	}

	for _, v := range sh.AllOf {
		delete(out, refToName(v.Ref))
	}

	if sh.Items != nil {
		delete(out, refToName(sh.Items.Ref))
	}

	if sh.AdditionalProperties != nil {
		delete(out, refToName(sh.AdditionalProperties.Ref))
	}
}

func (g *Generator) writeOperationFiles(fw codegen.FileWriter) error {
	files := []struct {
		path string
		gen  func() codegen.File
	}{
		{"interfaces/client/client.gen.go", g.clientFile},
		{"interfaces/client/client_sugar.gen.go", g.clientSugarFile},
		{"interfaces/client/audit.gen.go", g.auditClientFile},
		{"interfaces/server/server.gen.go", g.serverFile},
		{"impl/httpclient/client.gen.go", g.implClientFile},
		{"impl/echoserver/server.gen.go", g.implServerFile},
		{"impl/mocks/client/mocks.gen.go", g.mockClientFile},
		{"impl/mocks/server/mocks.gen.go", g.mockServerFile},
		{"sdk/sdk.gen.go", g.sdkFile},
	}

	for _, f := range files {
		if err := fw.WriteFile(f.path, f.gen()); err != nil {
			return fmt.Errorf("write %s: %w", f.path, err)
		}
	}

	return nil
}
