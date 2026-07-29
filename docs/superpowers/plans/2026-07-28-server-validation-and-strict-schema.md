# Server-side validation, strict-body binding, and strict-schema generation — Implementation Plan

> **Status:** IMPLEMENTED on branch `feat/server-validation-strict-schema` (13 commits, 2dbba70..0377321). All 22 packages green, `go vet` clean.

**Goal:** Generate `DisallowUnknownFields` body binding (Task A), auto `validator.Validate` calls in handlers (Task B), and a tri-state `GOLANG_SCHEMA_ANY` flag that warns/errors on any generated `any` type (Task C).

**Architecture:** All three are generator-side changes in `internal/generator/`. Task C infra introduces a new `EnumFlag` in `internal/genflags` (additive) plus a `Diagnostic` collector on `render.RenderContext` drained by `cmd/oapigen`. Task A rewrites `renderBindBody`; Task B adds a `*validator.Registry` to `ServerHTTP` and a `validator.Validate` call + structured-400 helper per handler. Helpers live in the per-project generated `server.gen.go` (existing pattern); golden files regenerated.

**Spec:** `docs/superpowers/specs/2026-07-28-server-validation-and-strict-schema-design.md`

## Task → commit mapping (executed)

| # | Task | Commit |
|---|------|--------|
| 1 | Generalize `genflags` to carry `any` | `2eec709` |
| 2 | Add `EnumFlag` (tri-state string flag) | `c4b90b6` |
| 3 | Register `GOLANG_SCHEMA_ANY` flag (default warn) | `360d1c3` + fix `dae5458` |
| 4 | `Diagnostic` collector on `RenderContext` | `e64f0bb` |
| 5 | Task A — `bindBody` DisallowUnknownFields + named-field 400 | `f66bca7` |
| 6 | Task B — `ServerHTTP` holds `*validator.Registry` | `3667e07` |
| 7 | Task B — handlers auto-call `validator.Validate`, structured 400 | `483facd` |
| 8 | Task B — examples (no-op: example is validator-only) | (no commit) |
| 9 | Task C — audit helpers `isAnyType`/`reportIfAny` | `4081b59` |
| 10 | Task C — wire audit at field/payload render sites | `d378c84` |
| 11 | Task C — drain diagnostics in `cmd/oapigen` (warn logs, error aborts) | `9e47fa6` |
| 12 | Regenerate goldens + smoke-test warn/error modes | `618b863` |
| — | Final fixes: audit request-body path; validate enum DefaultEnum | `0377321` |

## What landed

**Task A — `DisallowUnknownFields` in `bindBody`:** `renderBindBody` now emits `json.NewDecoder` + `dec.DisallowUnknownFields()` + `dec.Decode(dst)`. On error, `extractUnknownField` parses the `json: unknown field "<name>"` message and returns HTTP 400 (`echo.NewHTTPError`) naming the field; other JSON errors fall back to `err.Error()`. Imports `fmt`+`strings` added to the `needBody` block. URL-form branch unchanged.

**Task B — Auto `ValidateOwn`:** `ServerHTTP` struct holds `reg *validator.Registry`; `NewServerHTTP(impl apiserver.Server, reg *validator.Registry)`. Every handler calls `validator.Validate(req, s.reg)` after bind/SetDefaults and before `s.impl` — always emitted, no flag (walker no-ops on structs without `ValidateOwn`). On error: HTTP **400** with structured JSON `{"error":"validation_error","field":...,"message":...}` via `writeValidationError`/`splitValidationErr`. A and B are both 400, different bodies. `pkg/validator` imported (hardcoded absolute path, matching the repo's `pkg/httpclient`/`pkg/optional` convention).

**Task C — `GOLANG_SCHEMA_ANY`:** tri-state flag (silent/warn/**error**-default `warn`) via new `genflags.EnumFlag` (the `Flag` interface was generalized to return `any` — bool flags unchanged). Audit at model-field, response-payload, and request-body render sites emits `render.Diagnostic` when a generated type resolves to `any` (exempting explicit `additionalProperties`). `cmd/oapigen` drains per-project: `silent` no-op, `warn` logs + continues, `error` aborts. `ValidateConfig` now checks `DefaultEnum ∈ Allowed`. Smoke-tested on `testdata/integration`: warn logs ~15 sites; error aborts with a count + remediation hint.

**Task D — dropped:** no `pkg/auth`/`APIError` exists in this repo.

## Notes
- `testdata/integration_out/**` is gitignored (local-only artifacts) — intentionally not committed/regenerated.
- `examples/validation/main.go` is a validator-only demo (no `ServerHTTP`), so Task 8 was a no-op.
- Behavioral change: `DisallowUnknownFields` is wire-protocol-breaking for clients relying on lenient body decoding (now 400 instead of silent acceptance). Worth a CHANGELOG note.
- Downstream consumers of the generated server must now pass a `*validator.Registry` to `NewServerHTTP`.
