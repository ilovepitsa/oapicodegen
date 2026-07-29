// Package version хранит версию генератора oapigen. Значение инжектируется
// при релизной сборке через ldflags:
//
//	go build -ldflags "-X github.com/ilovepitsa/oapicodegen/internal/version.Version=v2.0.0"
//
// Без ldflags Version = "dev" (локальные сборки). Тесты, которым нужна
// детерминированная версия (совпадение с golden-файлами), устанавливают её
// через Set в TestMain.
package version

// Version — текущая версия генератора. Default "dev"; перезаписывается
// ldflags при релизной сборке или Set в тестах.
var Version = "dev"

// Get возвращает текущую версию.
func Get() string { return Version }

// Set устанавливает версию. Используется тестами (TestMain) для детерминированного
// совпадения сгенерированных заголовков с golden-файлами.
func Set(v string) { Version = v }
