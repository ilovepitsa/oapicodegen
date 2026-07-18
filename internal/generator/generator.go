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
	"nschugorev/oapigenerator/internal/generator/compose"
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/generator/walk"
	"nschugorev/oapigenerator/internal/parser"

	schemarender "nschugorev/oapigenerator/internal/generator/render/schema"
)

// Generator конфигурируется через Option-ы и хранит общее состояние генерации.
type Generator struct {
	project     *parser.Project
	schemaIndex *parser.SchemaIndex
	factory     *gogen.FileFactory
	composer    *compose.FileComposer

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
	g.composer = compose.NewFileComposer(g.factory)

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
	sf, err := g.renderSchemaFile(sh)
	if err != nil {
		return err
	}

	fname := "model/" + fileName(sh.Name) + ".gen.go"
	if err := fw.WriteFile(fname, sf); err != nil {
		return fmt.Errorf("write %s: %w", fname, err)
	}

	return g.writeSchemaAuxFiles(fw, sh)
}

// writeSchemaAuxFiles пишет вспомогательные файлы схемы (_json, _url_form,
// _converters, _audit_data) при выполнении условий. Вынесено из writeSchemaFiles
// для снижения cyclomatic complexity (каждый aux-файл — отдельная ветка).
func (g *Generator) writeSchemaAuxFiles(fw codegen.FileWriter, sh *parser.Schema) error {
	ops := g.operations()
	aux := []struct {
		cond   bool
		suffix string
		render schemaAuxRenderer
	}{
		{needsJSONMethods(sh), "json", g.jsonMethodsFile},
		{schemeHasURLFormat(sh, ops), "url_form", g.urlFormMethodsFile},
		{g.shouldGenerateConverters(sh), "converters", g.converterMethodsFile},
		{schemaReferencedByOperation(sh, ops), "audit_data", g.auditModelFile},
	}

	for _, a := range aux {
		if err := g.writeSchemaAuxFile(fw, sh, a.cond, a.suffix, a.render); err != nil {
			return err
		}
	}

	return nil
}

// schemaAuxRenderer генерирует body для aux-файла схемы.
type schemaAuxRenderer func(sh *parser.Schema) codegen.File

// writeSchemaAuxFile пишет один aux-файл, если cond истинно. suffix добавляется
// к базовому имени файла: "<name>_<suffix>.gen.go".
func (g *Generator) writeSchemaAuxFile(
	fw codegen.FileWriter,
	sh *parser.Schema,
	cond bool,
	suffix string,
	render schemaAuxRenderer,
) error {
	if !cond {
		return nil
	}

	body := render(sh)
	fname := "model/" + fileName(sh.Name) + "_" + suffix + ".gen.go"

	if err := fw.WriteFile(fname, body); err != nil {
		return fmt.Errorf("write %s: %w", fname, err)
	}

	return nil
}

// renderSchemaFile рендерит основной schema-файл. Alias/enum/map-alias схемы
// идут через composer + render/schema (Task 7); прочие (struct/array/union/
// allof) — через legacy schemaFile-путь с typeMapper напрямую.
func (g *Generator) renderSchemaFile(sh *parser.Schema) (codegen.File, error) {
	if isAliasLike(sh) || isEnumLike(sh) {
		return g.writeSchemaFilesViaComposer(sh)
	}

	return g.schemaFile(sh), nil
}

// isAliasLike сообщает, рендерится ли схема как alias (`type X string`) или
// map-alias (`type X map[string]Y`, `type X struct{}`). True для примитивных
// типов без enum и для object-схем без properties (map-alias). Такие схемы
// идут через AliasRenderer + compose.FileComposer (Task 7).
func isAliasLike(sh *parser.Schema) bool {
	if sh.Ref != "" || len(sh.Enum) > 0 {
		return false
	}

	if len(sh.OneOf) > 0 || len(sh.AnyOf) > 0 || len(sh.AllOf) > 0 {
		return false
	}

	if sh.Type == oapiTypeObject && len(sh.Properties) == 0 {
		return true
	}

	switch sh.Type {
	case oapiTypeString, oapiTypeInteger, oapiTypeNumber, oapiTypeBoolean:
		return true
	default:
		return false
	}
}

// isEnumLike сообщает, рендерится ли схема как enum (type + const-блок).
// True, если задан sh.Enum. Enum-схемы идут через EnumRenderer +
// compose.FileComposer (Task 7).
func isEnumLike(sh *parser.Schema) bool {
	return len(sh.Enum) > 0
}

// writeSchemaFilesViaComposer собирает schema-файл через FileComposer с
// новым renderer'ом для alias/enum/map-alias. RenderContext строится с
// TypeMapper-adapter'ом, который дренажит импорты typeMapper'а в общий
// ImportTracker renderer'а. AliasRenderer и EnumRenderer оба передаются в
// walker — для конкретной схемы сработает только один (OnAlias/OnMap или
// OnEnum), второй останется noop'ом.
func (g *Generator) writeSchemaFilesViaComposer(sh *parser.Schema) (codegen.File, error) {
	imports := render.NewImportTracker()
	tm := g.newRenderTypeMapper("model", "", imports)

	ctx := &render.RenderContext{
		Project:      g.project,
		SchemaIndex:  g.schemaIndex,
		Features:     g.project.Features,
		Splittable:   g.splittable,
		ModulePath:   g.project.ImportPrefix,
		ImportPrefix: g.project.ImportPrefix,
		TypeMapper:   tm,
	}

	renderers := []walk.SchemaRenderer{schemarender.NewAliasRenderer(), schemarender.NewEnumRenderer()}

	cf, err := g.composer.ComposeSchemaFile(sh, renderers, ctx)
	if err != nil {
		return nil, fmt.Errorf("compose schema %q: %w", sh.Name, err)
	}

	return cf, nil
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

	for _, sh := range schemas {
		sh.IsSplit = out[sh.Name]
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
