// Package must предоставляет функции-«must»: возвращают значение и паникуют
// при ошибке. Используется в сгенерированном коде, где ошибка невозможна по
// построению (например, обёртка над конструктором, валидирующим ref на этапе
// парсинга). Замена git.mws-team.ru/mws/devp/platform-go/pkg/must.
package must

// Value возвращает v, если err == nil. Если err != nil — паникует с err.
func Value[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}

	return v
}
