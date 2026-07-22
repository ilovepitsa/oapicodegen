// Package schema: UTCTimeRenderer — SingletonRenderer, рендерящий
// model/utc_time.gen.go (кастомный тип UTCTime). Заменяет
// Generator.utcTimeFile (internal/generator/utc_time.go). Тело портировано
// байт-в-байт; генерация происходит только когда вызывающая сторона
// (Generator.writeUTCTimeFile) подтвердила, что флаг USE_UTC_FOR_DATE_TIME
// включён — renderer не проверяет флаг сам.
package schema

import (
	"nschugorev/oapigenerator/internal/codegen/gogen"
	"nschugorev/oapigenerator/internal/generator/render"
)

// utcTimeBody — дословное тело файла model/utc_time.gen.go. Хранится как
// единая константа, чтобы гарантировать байт-в-байт совпадение со старым
// выводом Generator.renderUTCTimeType.
const utcTimeBody = `// UTCTime — обёртка над time.Time, принудительно сериализующая
// значение в UTC. Используется для date-time полей, когда включён
// флаг USE_UTC_FOR_DATE_TIME.
type UTCTime time.Time

func (u UTCTime) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Time(u).UTC())
}

func (u *UTCTime) UnmarshalJSON(data []byte) error {
	var t time.Time
	if err := json.Unmarshal(data, &t); err != nil {
		return err
	}

	*u = UTCTime(t.UTC())

	return nil
}
`

// utcTimeImports — импорты для model/utc_time.gen.go. Порядок совпадает со
// старым utcTimeFile: encoding/json, time.
var utcTimeImports = []gogen.Import{
	{Path: "encoding/json"},
	{Path: "time"},
}

// UTCTimeRenderer рендерит model/utc_time.gen.go. Не зависит от RenderContext —
// тип фиксированный, без schema-специфичных данных.
type UTCTimeRenderer struct{}

// NewUTCTimeRenderer строит UTCTimeRenderer.
func NewUTCTimeRenderer() *UTCTimeRenderer { return &UTCTimeRenderer{} }

// Render возвращает тело файла и импорты. ctx не используется.
func (UTCTimeRenderer) Render(_ *render.RenderContext) ([]byte, *render.ImportTracker, error) {
	imps := render.NewImportTracker()
	for _, imp := range utcTimeImports {
		imps.Add(imp)
	}

	return []byte(utcTimeBody), imps, nil
}

// FilePath возвращает путь генерируемого файла.
func (UTCTimeRenderer) FilePath() string { return "model/utc_time.gen.go" }

// compile-time guard: UTCTimeRenderer удовлетворяет SingletonRenderer.
var _ render.SingletonRenderer = (*UTCTimeRenderer)(nil)
