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

	opsrender "nschugorev/oapigenerator/internal/generator/render/operations"
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
//
// _json и _url_form рендерятся через composer + render/schema.JSONRenderer /
// URLFormRenderer (Tasks 6, 8); остальные aux-файлы — через legacy
// schemaAuxRenderer.
func (g *Generator) writeSchemaAuxFiles(fw codegen.FileWriter, sh *parser.Schema) error {
	ops := g.operations()

	if needsJSONMethods(sh) {
		if err := g.writeJSONMethodsAuxFile(fw, sh); err != nil {
			return err
		}
	}

	if schemeHasURLFormat(sh, ops) {
		if err := g.writeURLFormAuxFile(fw, sh); err != nil {
			return err
		}
	}

	if g.shouldGenerateConverters(sh) {
		if err := g.writeConvertersAuxFile(fw, sh); err != nil {
			return err
		}
	}

	legacyAux := []struct {
		cond   bool
		suffix string
		render schemaAuxRenderer
	}{
		{schemaReferencedByOperation(sh, ops), "audit_data", g.auditModelFile},
	}

	for _, a := range legacyAux {
		if err := g.writeSchemaAuxFile(fw, sh, a.cond, a.suffix, a.render); err != nil {
			return err
		}
	}

	return nil
}

// newSchemaRenderContext строит RenderContext для schema-рендеринга через
// composer. TypeMapper-adapter дренажит импорты в ctx.Imports, проставляемый
// compose.FileComposer через Base.Init.
func (g *Generator) newSchemaRenderContext() *render.RenderContext {
	ctx := &render.RenderContext{
		Project:      g.project,
		SchemaIndex:  g.schemaIndex,
		Features:     g.project.Features,
		Splittable:   g.splittable,
		ModulePath:   g.project.ImportPrefix,
		ImportPrefix: g.project.ImportPrefix,
	}
	ctx.TypeMapper = g.newRenderTypeMapper("model", "", ctx)

	return ctx
}

// newOperationsRenderContext строит RenderContext для operations-рендеринга.
// pkg — имя целевого пакета ("client" или "server").
func (g *Generator) newOperationsRenderContext(pkg string) *render.RenderContext {
	ctx := &render.RenderContext{
		Project:      g.project,
		SchemaIndex:  g.schemaIndex,
		Features:     g.project.Features,
		Splittable:   g.splittable,
		ModulePath:   g.project.ImportPrefix,
		ImportPrefix: g.project.ImportPrefix,
	}
	ctx.TypeMapper = g.newRenderTypeMapper(pkg, "", ctx)
	return ctx
}

// needsJSONMethods сообщает, нужна ли схеме отдельная функция UnmarshalJSON.
// True для oneOf/anyOf — union-схемы рендерятся как struct с вариантами, и
// Marshal/Unmarshal нужен для диспетчеризации по вариантам.
func needsJSONMethods(sh *parser.Schema) bool {
	return len(sh.OneOf) > 0 || len(sh.AnyOf) > 0
}

// writeJSONMethodsAuxFile рендерит <name>_json.gen.go через composer +
// render/schema.JSONRenderer.
func (g *Generator) writeJSONMethodsAuxFile(fw codegen.FileWriter, sh *parser.Schema) error {
	ctx := g.newSchemaRenderContext()

	renderers := []walk.SchemaRenderer{schemarender.NewJSONRenderer()}

	cf, err := g.composer.ComposeSchemaFile(sh, renderers, ctx) //nolint:lll // renderer-list literal, splitting harms readability
	if err != nil {
		return fmt.Errorf("compose json methods %q: %w", sh.Name, err)
	}

	fname := "model/" + fileName(sh.Name) + "_json.gen.go"
	if err := fw.WriteFile(fname, cf); err != nil {
		return fmt.Errorf("write %s: %w", fname, err)
	}

	return nil
}

// writeURLFormAuxFile рендерит <name>_url_form.gen.go через composer +
// render/schema.URLFormRenderer. URLForm pack состоит из одного renderer'а —
// aux-файл не делит Buf с основным struct-файлом.
func (g *Generator) writeURLFormAuxFile(fw codegen.FileWriter, sh *parser.Schema) error {
	ctx := g.newSchemaRenderContext()
	renderers := []walk.SchemaRenderer{schemarender.NewURLFormRenderer()}

	cf, err := g.composer.ComposeSchemaFile(sh, renderers, ctx)
	if err != nil {
		return fmt.Errorf("compose url_form %q: %w", sh.Name, err)
	}

	fname := "model/" + fileName(sh.Name) + "_url_form.gen.go"
	if err := fw.WriteFile(fname, cf); err != nil {
		return fmt.Errorf("write %s: %w", fname, err)
	}

	return nil
}

// writeConvertersAuxFile рендерит <name>_converters.gen.go через composer +
// render/schema.ConvertersRenderer. Pack из одного renderer'а — aux-файл не
// делит Buf с основным struct-файлом.
func (g *Generator) writeConvertersAuxFile(fw codegen.FileWriter, sh *parser.Schema) error {
	ctx := g.newSchemaRenderContext()
	renderers := []walk.SchemaRenderer{schemarender.NewConvertersRenderer()}

	cf, err := g.composer.ComposeSchemaFile(sh, renderers, ctx)
	if err != nil {
		return fmt.Errorf("compose converters %q: %w", sh.Name, err)
	}

	fname := "model/" + fileName(sh.Name) + "_converters.gen.go"
	if err := fw.WriteFile(fname, cf); err != nil {
		return fmt.Errorf("write %s: %w", fname, err)
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

// renderSchemaFile рендерит основной schema-файл. Маршрутизация:
//   - alias/enum/mapAlias → composer + AliasRenderer/EnumRenderer (Task 7);
//   - object-struct и split → composer + StructRenderer (Task 8);
//   - array/union/allof → legacy schemaFile-путь (Task 9 мигрирует позже).
func (g *Generator) renderSchemaFile(sh *parser.Schema) (codegen.File, error) {
	if isAliasLike(sh) || isEnumLike(sh) {
		return g.writeSchemaFilesViaComposer(sh)
	}

	if isStructLike(sh) {
		return g.writeStructFileViaComposer(sh)
	}

	return g.schemaFile(sh), nil
}

// isStructLike сообщает, рендерится ли схема как object-struct (включая
// split-вариант). True для object-схем с properties и без composite-членов
// (oneOf/anyOf/allOf). Такие схемы идут через StructRenderer (Task 8),
// который пишет <Name> struct + SetDefaults + ValidateOwn + Update<Name>.
func isStructLike(sh *parser.Schema) bool {
	if len(sh.OneOf) > 0 || len(sh.AnyOf) > 0 || len(sh.AllOf) > 0 {
		return false
	}

	return sh.Type == oapiTypeObject && len(sh.Properties) > 0
}

// writeStructFileViaComposer собирает object-struct файл через FileComposer с
// pack'ом renderer'ов: StructRenderer, SetDefaultsRenderer, ValidateOwnRenderer,
// UpdateStructRenderer. TypeMapper-adapter дренажит импорты в ctx.Imports,
// проставляемый compose.FileComposer через Base.Init.
func (g *Generator) writeStructFileViaComposer(sh *parser.Schema) (codegen.File, error) {
	ctx := g.newSchemaRenderContext()

	renderers := []walk.SchemaRenderer{
		schemarender.NewStructRenderer(),
		schemarender.NewSetDefaultsRenderer(),
		schemarender.NewValidateOwnRenderer(),
		schemarender.NewUpdateStructRenderer(),
	}

	cf, err := g.composer.ComposeSchemaFile(sh, renderers, ctx) //nolint:lll // renderer-list literal, splitting harms readability
	if err != nil {
		return nil, fmt.Errorf("compose struct %q: %w", sh.Name, err)
	}

	return cf, nil
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
// новым renderer'ом для alias/enum/map-alias. RenderContext строится через
// newSchemaRenderContext: TypeMapper-adapter дренажит импорты typeMapper'а в
// общий ImportTracker renderer'а. AliasRenderer и EnumRenderer оба передаются в
// walker — для конкретной схемы сработает только один (OnAlias/OnMap или
// OnEnum), второй останется noop'ом.
func (g *Generator) writeSchemaFilesViaComposer(sh *parser.Schema) (codegen.File, error) {
	ctx := g.newSchemaRenderContext()

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
// USE_UTC_FOR_DATE_TIME. Рендер идёт через UTCTimeRenderer (SingletonRenderer)
// + compose.FileComposer — замена устаревшему Generator.utcTimeFile.
func (g *Generator) writeUTCTimeFile(fw codegen.FileWriter) error {
	if !g.project.Features.UseUTCForDateTime.Value {
		return nil
	}

	ctx := g.newSchemaRenderContext()

	file, err := g.composer.ComposeSingletonFile(schemarender.NewUTCTimeRenderer(), ctx)
	if err != nil {
		return fmt.Errorf("compose utc_time: %w", err)
	}

	const fname = "model/utc_time.gen.go"
	if err := fw.WriteFile(fname, file); err != nil {
		return fmt.Errorf("write %s: %w", fname, err)
	}

	return nil
}

// writeExpectedValidatorsFile пишет model/expected_validators.gen.go, если
// в документе есть хотя бы один named-валидатор. Рендер идёт через
// ExpectedValidatorsRenderer (SingletonRenderer) — замена устаревшему
// Generator.expectedValidatorsFile. Direct-Render (без ComposeSingletonFile)
// позволяет пропустить запись файла, когда именованных валидаторов нет.
func (g *Generator) writeExpectedValidatorsFile(fw codegen.FileWriter) error {
	ctx := g.newSchemaRenderContext()
	r := schemarender.NewExpectedValidatorsRenderer()

	body, imps, err := r.Render(ctx)
	if err != nil {
		return fmt.Errorf("render expected_validators: %w", err)
	}

	if len(body) == 0 {
		return nil
	}

	file := g.factory.Create(&gogen.File{
		Package: "model",
		Imports: imps.Imports(),
		Body:    body,
	})

	const fname = "model/expected_validators.gen.go"
	if err := fw.WriteFile(fname, file); err != nil {
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
	// Singleton-renderer'ы для интерфейсных файлов (Phase 3).
	singletonRenderers := []struct {
		path string
		r    render.SingletonRenderer
	}{
		{"interfaces/client/client.gen.go", opsrender.NewClientInterfaceRenderer()},
		// TODO(Task 7): раскомментировать audit renderer после создания.
		{"interfaces/client/client_sugar.gen.go", opsrender.NewClientSugarRenderer()},
		// {"interfaces/client/audit.gen.go", opsrender.NewAuditClientRenderer()},
		{"interfaces/server/server.gen.go", opsrender.NewServerInterfaceRenderer()},
	}

	for _, sr := range singletonRenderers {
		pkg := "client"
		if sr.path == "interfaces/server/server.gen.go" {
			pkg = "server"
		}
		ctx := g.newOperationsRenderContext(pkg)
		file, err := g.composer.ComposeSingletonFile(sr.r, ctx)
		if err != nil {
			return fmt.Errorf("compose %s: %w", sr.path, err)
		}
		if err := fw.WriteFile(sr.path, file); err != nil {
			return fmt.Errorf("write %s: %w", sr.path, err)
		}
	}

	// Legacy (impl, mocks, sdk + interfaces not yet migrated) — будут заменены в следующих фазах.
	legacyFiles := []struct {
		path string
		gen  func() codegen.File
	}{
		{"interfaces/client/audit.gen.go", g.auditClientFile},
		{"impl/httpclient/client.gen.go", g.implClientFile},
		{"impl/echoserver/server.gen.go", g.implServerFile},
		{"impl/mocks/client/mocks.gen.go", g.mockClientFile},
		{"impl/mocks/server/mocks.gen.go", g.mockServerFile},
		{"sdk/sdk.gen.go", g.sdkFile},
	}

	for _, f := range legacyFiles {
		if err := fw.WriteFile(f.path, f.gen()); err != nil {
			return fmt.Errorf("write %s: %w", f.path, err)
		}
	}

	return nil
}
