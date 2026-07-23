# T27 Phase 4 — Implementations-side Singleton Renderers

**Goal:** Migrate `implClientFile()`, `implServerFile()`, `mockClientFile()`, `mockServerFile()`, `sdkFile()` from `*Generator` methods into `SingletonRenderer` implementations.

**Files to delete:** `impl_client.go`, `impl_server.go`, `mocks.go`, `sdk.go`

## Tasks (by complexity, ascending)

| # | Task | Complexity | Lines |
|---|------|-----------|-------|
| 1 | SDKRenderer | Simple | ~50 |
| 2 | Wire SDKRenderer + delete sdk.go | — | — |
| 3 | MockClientRenderer + MockServerRenderer | Simple | ~100 |
| 4 | Wire mocks + delete mocks.go | — | — |
| 5 | ImplClientRenderer | Complex | ~800 |
| 6 | Wire ImplClientRenderer + delete impl_client.go | — | — |
| 7 | ImplServerRenderer | Complex | ~600 |
| 8 | Wire ImplServerRenderer + delete impl_server.go | — | — |
| 9 | Verify Phase 4 acceptance | — | — |

## Architecture

All 5 renderers follow the same SingletonRenderer pattern as Phase 3:
- Implement `Render(ctx) (body, imports, error)` + `FilePath()`
- Use `allOperations(ctx.Project)` for data access
- Use `ctx.TypeMapper` for type resolution
- Return `ImportTracker` for import management
- Wired through `writeOperationFiles` singletonRenderers list
