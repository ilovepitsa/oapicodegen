# T27 Phase 3 — Operations-side Singleton Renderers Design

**Date:** 2026-07-22
**Status:** approved
**Goal:** Migrate `clientFile()`, `clientSugarFile()`, `auditClientFile()`, `serverFile()` from `*Generator` methods into `SingletonRenderer` implementations in a new `render/operations/` package.

## Architecture

```
internal/generator/render/operations/
  client_interface.go          — ClientInterfaceRenderer
  client_interface_test.go     — unit tests
  client_sugar.go              — ClientSugarRenderer
  client_sugar_test.go         — unit tests
  audit_client.go              — AuditClientRenderer
  audit_client_test.go         — unit tests
  server_interface.go          — ServerInterfaceRenderer
  server_interface_test.go     — unit tests
  helpers.go                   — shared helpers (sortedResponseCodes, goName, etc.)
```

Four new `SingletonRenderer` types, each producing one file:

| Renderer | File | Legacy source |
|---|---|---|
| `ClientInterfaceRenderer` | `interfaces/client/client.gen.go` | `client.go` |
| `ClientSugarRenderer` | `interfaces/client/client_sugar.gen.go` | `client_sugar.go` |
| `AuditClientRenderer` | `interfaces/client/audit.gen.go` | `audit_server.go` |
| `ServerInterfaceRenderer` | `interfaces/server/server.gen.go` | `server.go` |

**Not in scope:** `impl_client.go`, `impl_server.go`, `mocks.go`, `sdk.go` — these are implementations/mocks, not interfaces. Future phases.

## API and Contract

All four implement `render.SingletonRenderer`:

```go
Render(ctx *RenderContext) (body []byte, imports *ImportTracker, err error)
FilePath() string
```

### Operations access

Renderers read operations from `ctx.Project.Paths.Services` directly — each renderer calls a shared `allOperations(project)` helper that flattens all methods across all services.

### Type mapper

Renderers use `ctx.TypeMapper` (interface: `GoType`, `SetMode`). No new methods needed on the interface.

### Import tracking

Each `Render` call:
1. Creates a local `ImportTracker`
2. Sets `ctx.Imports = localTracker` (temporarily, so the typeMapperAdapter drains into it)
3. Builds body using `BufferWriter`
4. Returns body + localTracker

This is the same pattern `Base.Init` uses for schema renderers. The caller (`ComposeSingletonFile` or direct) must not assume `ctx.Imports` is preserved across `Render` calls.

### writeOperationFiles rewrite

```go
func (g *Generator) writeOperationFiles(fw codegen.FileWriter) error {
    singletonRenderers := []struct {
        path string
        r    render.SingletonRenderer
    }{
        {"interfaces/client/client.gen.go", opsrender.NewClientInterfaceRenderer()},
        {"interfaces/client/client_sugar.gen.go", opsrender.NewClientSugarRenderer()},
        {"interfaces/client/audit.gen.go", opsrender.NewAuditClientRenderer()},
        {"interfaces/server/server.gen.go", opsrender.NewServerInterfaceRenderer()},
    }
    ctx := g.newOperationsRenderContext()
    for _, sr := range singletonRenderers {
        file, err := g.composer.ComposeSingletonFile(sr.r, ctx)
        if err != nil {
            return fmt.Errorf("compose %s: %w", sr.path, err)
        }
        if err := fw.WriteFile(sr.path, file); err != nil {
            return fmt.Errorf("write %s: %w", sr.path, err)
        }
    }
    // impl/mocks/sdk remain as legacy methods for now
    // ...
}
```

## Shared Helpers

Extracted to `render/operations/helpers.go` as package-level (unexported) functions:

- `allOperations(project)` — flatten all services → []*parser.Method
- `operationMethodName(op)` — Go method name for an operation
- `sortedResponseCodes(responses)` — sorted status codes, default last
- `responseByCode(responses, code)` — lookup response by status code
- `responseFieldName(code)` — "Response200", "ResponseDefault", etc.
- `firstContentSchema(content)` — first content-type schema
- `responseSchema(resp)` — first response content schema
- `bodySchema(rb)` — first request body schema
- `hasResponseHeaders(resp)` — check for headers on response
- `payloadWithHeadersTypeName(op, code)` — PayloadWithHeaders type name
- `firstSuccessResponse(responses)` — first 2xx response code + schema
- `sugarReturnType(op, code, schema, mapper)` — return type for sugar method
- `isSuccessCode(code)` — 2xx check
- `goName(s)` — pascal case
- `isInherentlyNilable(t)` — type is nilable without pointer
- `echoTag(in, name)` — struct tag for path/query/header/cookie
- `filterParamsByIn(params, in)` — filter parameters by location
- `qualifyClient(name, suffix, mapper)` — "client." prefix if needed

All ported verbatim from `client.go`, `client_sugar.go`, `audit_server.go`, `server.go`, `naming.go`.

## Testing Strategy

### Unit tests (render/operations/)

Per renderer — tests for: FilePath, empty project, single operation, multiple operations, deprecated operations, edge cases.

### Wire tests (generator_test.go)

4 tests — one per file — verify file appears in `Generate()` output with expected content.

### Golden tests

`TestE2E_Minimal` + `TestE2E_GoldenCompiles` must stay green. `testdata/` working tree must be clean.

### Byte-for-byte fidelity

Bodies ported verbatim using `BufferWriter` — output must byte-match current golden files exactly.

## Tasks

1. **ClientInterfaceRenderer** — port body + helpers + FilePath
2. **Wire ClientInterfaceRenderer** + delete legacy `client.go`
3. **ServerInterfaceRenderer** — port body + FilePath
4. **Wire ServerInterfaceRenderer** + delete legacy `server.go`
5. **ClientSugarRenderer** — port body + FilePath
6. **Wire ClientSugarRenderer** + delete legacy `client_sugar.go`
7. **AuditClientRenderer** — port body + FilePath
8. **Wire AuditClientRenderer** + delete legacy `audit_server.go`
9. **Verify Phase 3 acceptance** — build, test, vet, golden

## Acceptance Criteria

- [ ] `client.go`, `server.go`, `client_sugar.go`, `audit_server.go` deleted from `internal/generator/`
- [ ] Four new `SingletonRenderer` implementations in `internal/generator/render/operations/`
- [ ] Each has unit tests
- [ ] `writeOperationFiles` dispatches via `ComposeSingletonFile` for these four
- [ ] `go build ./...`, `go test ./...`, `go vet ./...` green
- [ ] `TestE2E_Minimal`, `TestE2E_GoldenCompiles` pass, `testdata/` clean
