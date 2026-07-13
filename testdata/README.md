# testdata

Golden files and reference OpenAPI specs for e2e generation tests.

## minimal/

Минимальная спека со стандартными конструкциями OpenAPI 3.x и расширениями
`x-validations` для проверки codegen-а валидаций.

Покрывает:
- object с required/optional/nullable свойствами
- array of objects (`$ref`)
- `$ref` на схемы
- string enum (`Kind`)
- oneOf (`Event` → `CreatedEvent` | `DeletedEvent`)
- additionalProperties (map)
- path/query/body параметры
- CRUD-операции: list, create, get, update (PUT), delete
- несколько response-кодов на операцию (200/400, 201/400, 200/404, 204)
- `x-validations`:
  - простые правила `Size >=N` / `Size <=N` на string-полях (required и nullable)
  - именованные property-валидаторы (`app.NonEmptyName`)
  - schema-level кросс-полевые валидаторы (`app.ItemConsistency`, `app.ItemCreateConsistency`)
  - `Immutable` маркер для update-marker'а (поле `ItemCreate.tag` пропускается в Update<Name>)
- `Update<Name>` структура для PUT request body (`UpdateItemCreate`) с `ValidateOwn`
  на property-level правилах (schema-level пропускается — см. renderValidateOwn)

E2e-тест: `cmd/oapigen/e2e_test.go` (`TestE2E_Minimal`) гоняет полный
пайплайн `cmd/oapigen` и сравнивает вывод с `golden/`. Обновление эталонов:
`go test ./cmd/oapigen/ -run TestE2E_Minimal -update`.
