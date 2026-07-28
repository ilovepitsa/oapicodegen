# Design: Server-side validation, strict-body binding, and strict-schema generation

**Date:** 2026-07-28
**Status:** Approved (brainstorming)
**Scope:** Tasks A, B, C. Task D (unified `pkg/apierror`) dropped — no `pkg/auth`/`APIError` exists in this repo.

---

## Context

`oapigen` generates an Echo HTTP server per OpenAPI project. Today:

- `bindBody` (generated into every `impl/echoserver/server.gen.go`) decodes the request body with `json.Unmarshal`, which silently ignores unknown JSON fields.
- Generated handlers only bind params; validation is entirely on the developer. `pkg/validator` already exists and is stable (`Validatable{ValidateOwn(reg)}`, reflection walker `Validate(obj, reg)`, `Registry.AssertExact`), and generated models already carry `ValidateOwn`.
- The type mapper falls back to Go `any` (`goTypeAny`) in several situations — nil response schema, empty `schema: {}`, unresolved cross-service `$ref`, union without a name, unknown primitive — producing blind `*any` / `map[string]any` spots with no signal at generation time.
- The generated server has no error envelope type; it returns `echo.NewHTTPError(...)`.

Three generator-side changes are requested. All touch `internal/generator/render/operations/` (and the type-mapper audit surface); Task B also adds a generated dependency on `pkg/validator`. Every generated `server.gen.go` carries its own copy of the helpers (this is the existing pattern — not a shared package), so **all golden files must be regenerated** (minimal, petstore, integration_out/{store,pets,users,orders}).

Reference memory: `project_validation_design.md` — "Variant A (registry param) + auto-pull reflection walker; ValidateOwn per struct; server-only; fail-fast". This design follows it.

---

## Task A — `DisallowUnknownFields` in `bindBody`

### Trigger / change point
`renderBindBody` in `internal/generator/render/operations/impl_server.go` (the JSON branch only; the `application/x-www-form-urlencoded` branch and its `UnmarshalURLForm` path stay unchanged).

### Generated `bindBody` (JSON branch)
```go
func bindBody(c echo.Context, dst any) error {
    // url-form branch unchanged ...

    body, err := io.ReadAll(c.Request().Body)
    if err != nil { return err }
    c.Request().Body = io.NopCloser(bytes.NewReader(body))
    if len(body) == 0 { return nil }

    dec := json.NewDecoder(bytes.NewReader(body))
    dec.DisallowUnknownFields()
    if err := dec.Decode(dst); err != nil {
        field := extractUnknownField(err)            // parses 'json: unknown field "foo"'
        if field != "" {
            return echo.NewHTTPError(http.StatusBadRequest,
                fmt.Sprintf("unknown field %q", field))
        }
        return echo.NewHTTPError(http.StatusBadRequest, err.Error())
    }
    return nil
}
```

### Helper: `extractUnknownField`
Small generated helper next to `bindBody`. Parses `err.Error()` for the `json: unknown field "<name>"` shape and returns the field name; returns `""` when the error is a different JSON error (e.g. type mismatch), in which case the message degrades to `err.Error()` and the status stays 400.

### Error body (A)
`echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("unknown field %q", field))`. Echo serializes `*echo.HTTPError` to `{"message":"unknown field \"foo\""}`. **HTTP 400.**

### Imports
When a body is present the renderer already adds `bytes`, `encoding/json`, `io`, `net/http`. Add `fmt` (used by `extractUnknownField` and the message). Emit `fmt` only when `needBody` is true.

### Blast radius
All generated `server.gen.go` change; behavior change for clients sending extra fields (now rejected with 400 instead of silently ignored). Golden files regenerated.

---

## Task B — Auto `ValidateOwn` in handlers

### Change points
`renderImplServerStruct` (struct + constructor), `renderImplServerMethod` (handler body), imports in `ImplServerRenderer.Render`.

### Struct + constructor
```go
type ServerHTTP struct {
    impl apiserver.Server
    reg  *validator.Registry
}

func NewServerHTTP(impl apiserver.Server, reg *validator.Registry) *ServerHTTP {
    return &ServerHTTP{impl: impl, reg: reg}
}
```
New import: `github.com/ilovepitsa/oapicodegen/pkg/validator` (alias `validator`). The exact import path is derived from the project's module path (same module that hosts `pkg/validator` today); resolved at render time via `ctx.Project.Paths`/module config, consistent with how `apiclient`/`apiserver` imports are built.

### Handler body
Insert the validation call after `c.Bind(req)` and after `SetDefaults` (when present), before `s.impl`:
```go
    if err := c.Bind(req); err != nil { return err }
    // SetDefaults (when needed) stays where it is
    if err := validator.Validate(req, s.reg); err != nil {
        return writeValidationError(c, err)
    }
    resp, err := s.impl.X(c.Request().Context(), req)
    ...
```

**Always emitted, no flag.** `validator.Validate` is a no-op on `req` whose structs lack `ValidateOwn` (the walker finds no `Validatable` and returns `nil`), so endpoints without `x-validations` pay only a cheap reflection walk. `pkg/validator` becomes a mandatory import for every generated server.

### Scope of validation
`validator.Validate(req, s.reg)` over the **whole `req` struct** — body + path + query bound into `req`. The walker recurses the entire tree, so nested body structs and path/query param structs are covered by a single call.

### Error body (B) — structured JSON, HTTP 400
Generated helper `writeValidationError`:
```go
func writeValidationError(c echo.Context, err error) error {
    field, msg := splitValidationErr(err.Error())  // "Owner.Pets[2].Name: must be >= 1"
    return c.JSON(http.StatusBadRequest, map[string]any{
        "error":   "validation_error",
        "field":   field,
        "message": msg,
    })
}
```
`splitValidationErr` splits the walker's error string on the first `": "`: left = field path, right = message. When no separator is found, `field = ""` and `msg = err.Error()`.

**HTTP 400** (per "давай оба 400"), body `{"error":"validation_error","field":"...","message":"..."}`. A and B are both 400 but different bodies — A: Echo `{"message":...}`, B: structured `{error,field,message}`.

### Helpers placement
`writeValidationError` + `splitValidationErr` live in `server.gen.go` next to `bindBody` (one copy per project, matching the existing pattern). Emitted unconditionally (since `Validate` is always called).

### Blast radius
`NewServerHTTP` signature changes (`impl` → `impl, reg`). All consumers of generated servers (examples, integration tests, golden) must construct `validator.New()`, `Register(...)` validators, `AssertExact(ExpectedValidatorNames())`, and pass `reg` into `NewServerHTTP`. Golden files regenerated; examples updated.

---

## Task C — `GOLANG_SCHEMA_ANY` strict-schema flag

### Trigger
Any generated field/element/payload whose Go type resolves to a bare `any` — including `any`, `*any`, `[]any`, and `map[string]any` produced by an unresolved/empty schema.

### Exemptions (do NOT trigger)
- Explicit `additionalProperties: true` → `map[string]any` (deliberate free-form map).
- Explicit `additionalProperties: {schema}` → `map[string]<T>` (concrete value type).
- Endpoints with no body (no payload type emitted — absence alone is not a failure).

### Triggers (when the flag is active)
- `schema: {}` (empty object: no `type`, no `properties`, no `additionalProperties`) → today `map[string]any`.
- Nil response schema / payload without schema → `*any`.
- Unresolved external `$ref` (cross-service schema absent from `SchemaIndex`) → fallback `any`.
- Union without a name / unknown primitive → `any`.

### Severity model — tri-state flag, default `warn`
Flag `GOLANG_SCHEMA_ANY`, values `silent | warn | error`:
- `silent` — off; today's behavior, no diagnostics.
- `warn` — log every `any` site via the zap sugar logger and continue generation. **Default.**
- `error` — accumulate diagnostics and abort the `oapigen` run (non-zero exit); existing specs with `any` fail until the schema is fixed or the flag is lowered.

### Flag infrastructure — new `EnumFlag`
The current `internal/genflags` is strictly boolean (`FlagConfig.DefaultValue bool`, `Registry.Resolve(...) (bool, error)`, `ProjectFeature{Value bool}`). A tri-state flag does not fit. Per the package's own TODO (`genflags.go:12`: "Будущие типы флагов (enum, string) реализуют Flag и регистрируются самостоятельно"), add:

1. **`EnumFlag`** in `internal/genflags/genflags.go` — implements `Flag` with a string value drawn from an allowed set. Requires extending the `Flag` contract and `FlagConfig`/`Registry.Resolve` to carry non-bool values. Concretely:
   - `FlagConfig` gains an optional `DefaultEnum string` (and the loader maps it).
   - `Flag` interface gains (or is generalized so) `ValidateOverride` returns `(any, error)` rather than `(bool, error)`; `Registry.Resolve` returns `(any, error)`.
   - `EnumFlag.ValidateOverride` checks membership in the allowed set and the `DependsOn` rules (adapted for non-bool where relevant).
2. **`parser/generation_flags.go`** — register `FlagSchemaAny = "GOLANG_SCHEMA_ANY"` with allowed values `["silent","warn","error"]`, default `"warn"`. Add `SchemaAny ProjectFeature` to `ProjectFeatures`; `ProjectFeature` gains a string mode (or a second field) to carry enum values.
3. **`parser/generation_flags_loader.go`** — route `GOLANG_SCHEMA_ANY` to the enum setter.
4. **`generation_flags.yaml`** (config) — add the `GOLANG_SCHEMA_ANY` entry (camelCase keys, `affects: ["golang"]`, default `"warn"`).

The bool flag path is preserved unchanged; `EnumFlag` is additive. This is the largest piece of the spec and the riskiest (touches the shared flag contract) — implement and test `EnumFlag` in isolation before wiring the schema audit.

### Audit point — not the type mapper
The mapper (`internal/generator/type.go`, `render/schema/*`) returns `any` as a fallback in many branches; embedding the check there duplicates logic across branches and loses location context. Instead, audit at the **renderer call sites that consume `m.GoType(schema)`** and know the context:

- `client_interface.go` (`renderResponseStruct`, the `Payload *<GoType>` line).
- `helpers.go` `responsePayloadType`.
- Request-body render path.
- Model-field render path.

At each call: if the resulting type is a bare `any` / `*any` / `[]any`, or a `map[string]any` **not** sourced from explicit `additionalProperties`, append a `Diagnostic`.

### Diagnostics collection
Add a collector to `RenderContext` (`Diagnostics *[]Diagnostic`, or a small collector type) that renderers `Append` to. After generating a project, `cmd/oapigen` drains it:
- `warn` mode → `sugar.Warnf` per diagnostic (location + reason).
- `error` mode → collect into an error returned from the project-generation step; `main` exits non-zero.
- `silent` mode → ignore.

```go
type Diagnostic struct {
    Severity string // "warn" | "error" (resolved from the flag mode)
    Location string // e.g. "components.schemas.Pet.properties.tag" or "paths./pets.post.responses.200"
    Reason   string // e.g. "schema resolves to Go `any` (empty schema {} / unresolved $ref)"
}
```

### Blast radius
`silent` = zero behavioral difference. `warn` (default) = new log lines, golden tests (which compare generated code, not logs) unaffected. `error` = existing specs with `any` fail until fixed or the flag is set to `silent`/`warn`. Existing specs likely emit several `warn` lines on first run with the new default — expected, non-blocking.

---

## Cross-cutting

### Golden regeneration
Every generated `server.gen.go` changes (Task A helper, Task B struct/constructor/handler/helpers/imports). Regenerate: `testdata/project/golden/minimal`, `internal/generator/testdata/golden/petstore`, `testdata/integration_out/{store,pets,users,orders}`. Update examples (`examples/validation`) to pass `reg` into `NewServerHTTP`.

### Task dependencies / order
1. **Task C infra first** (`EnumFlag` + flag registration + diagnostics collector) — isolated, testable, no generated-code change. Validates the tri-state contract before anything depends on it.
2. **Task A** — `bindBody` rewrite + `extractUnknownField` + `fmt` import. Regenerate golden.
3. **Task B** — struct/constructor + handler validation call + `writeValidationError`/`splitValidationErr` + `pkg/validator` import. Update examples. Regenerate golden.
4. **Task C audit** — wire diagnostics at the renderer call sites; drain in `cmd/oapigen`. Verify `warn` default produces expected log lines for existing specs.

### Testing
- `internal/genflags`: `EnumFlag` unit tests (allowed-set enforcement, override vs default, `DependsOn`, non-string rejection).
- `internal/parser`: `GOLANG_SCHEMA_ANY` loader/resolve tests (default `warn`, override to `silent`/`error`, invalid value rejected).
- `internal/generator/render/operations`: golden tests for `bindBody` (decoder + named-field 400) and handler (validation call + structured 400); constructor signature test.
- Audit tests: feed a spec with `schema: {}` / unresolved `$ref` / explicit `additionalProperties` and assert the right diagnostics fire (and that `additionalProperties` and no-body endpoints do not fire).
- Integration: existing `examples/validation` end-to-end — register validators, `AssertExact`, `NewServerHTTP(impl, reg)`, send a bad body → expect 400 with structured `{error,field,message}`; send unknown field → expect 400 with Echo `{"message":...}`.

### Non-goals
- Unified `pkg/apierror` type (Task D) — dropped; no `pkg/auth`/`APIError` exists here.
- Client-side validation (server-only, per `project_validation_design.md`).
- Changing the per-project helper placement to a shared package (out of scope; keep current pattern).

---

## Open items resolved during brainstorming
- Task D: dropped (no existing auth/APIError in repo).
- Task C trigger: "any anywhere"; exemptions = explicit `additionalProperties` + no-body endpoints; `schema:{}` and unresolved `$ref` still fail when active.
- Task C severity: tri-state flag, default `warn`.
- Task A error: `echo.NewHTTPError(400)` with named field (Echo `{"message":...}`).
- Task B validation call: `validator.Validate(req, reg)` walker over the whole `req`.
- Task B opt-out: always emit, no flag.
- Task B registry: `NewServerHTTP(impl, reg)`.
- Task B error: structured JSON `{error,field,message}`, **HTTP 400** ("давай оба 400"); A and B same status, different bodies.
- Task C flag infra: new `EnumFlag` (additive, anticipated by package TODO).
