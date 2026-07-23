# T28 — Subpackage Splitting Plan

**Goal:** Дробление `model/` по подпапкам на основе структуры spec-файла: `schemas/<subfolder>/` → `model/<subfolder>/`.

## Design

### Schema.SubPackage
Добавить `SubPackage string` на `parser.Schema`. Вычисляется в `ProjectLoader.Load()` после загрузки всех схем:
- Берём `Schema.SourceFile` (абсолютный путь к yaml-файлу)
- Отрезаем префикс `src/openapi/schemas/` (относительно корня сервиса)
- Берём оставшуюся директорию как subpackage
- Если схема в корневом `openapi.yaml` → `SubPackage = ""`
- Если схема в `schemas/users/user.yaml` → `SubPackage = "users"`

### writeSchemaFiles
Вместо `model/<Name>.gen.go` → `model/<SubPackage>/<Name>.gen.go`

### Cross-subpackage imports
Если схема из subpackage A ссылается на схему из subpackage B, нужно генерировать import:
```go
import users "xxx/model/users"
```
TypeMapper уже поддерживает cross-service imports через SchemaIndex. Cross-subpackage — аналогично, но в пределах одного сервиса.

### Tasks
1. Добавить `SubPackage` на Schema + вычисление в ProjectLoader
2. Обновить `writeSchemaFiles` для учёта subpackage
3. Обновить `typeMapper` для cross-subpackage imports
4. Обновить golden-файлы
5. Тесты

### Scope
Первая итерация: поддержка однофайловых спек (все схемы в `openapi.yaml` → `SubPackage = ""` → поведение не меняется). Многофайловые спеки — когда появятся реальные тестовые данные.
