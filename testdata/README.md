# testdata

Golden files and reference OpenAPI specs for e2e generation tests.

## minimal/

Минимальная спека только со стандартными конструкциями OpenAPI 3.x (без `x-*`).

Покрывает:
- object с required/optional/nullable свойствами
- array of objects (`$ref`)
- `$ref` на схемы
- string enum (`Kind`)
- oneOf (`Event` → `CreatedEvent` | `DeletedEvent`)
- additionalProperties (map)
- path/query/body параметры
- CRUD-операции: list, create, get, delete
- несколько response-кодов на операцию (200/400, 201/400, 200/404, 204)

E2e-тест: `cmd/oapigen/e2e_test.go` (`TestE2E_Minimal`) гоняет полный
пайплайн `cmd/oapigen` и сравнивает вывод с `golden/`. Обновление эталонов:
`go test ./cmd/oapigen/ -run TestE2E_Minimal -update`.
