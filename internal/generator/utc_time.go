package generator

import (
	"nschugorev/oapigenerator/internal/codegen"
	"nschugorev/oapigenerator/internal/codegen/gogen"
)

// utcTimeFile генерирует model/utc_time.gen.go — кастомный тип UTCTime,
// принудительно приводящий time.Time к UTC при marshal/unmarshal.
// Генерируется только когда включён флаг USE_UTC_FOR_DATE_TIME.
func (g *Generator) utcTimeFile() codegen.File {
	w := codegen.NewBufferWriter()
	g.renderUTCTimeType(w)

	return g.factory.Create(&gogen.File{
		Package: "model",
		Imports: []gogen.Import{
			{Path: "encoding/json"},
			{Path: "time"},
		},
		Body: w.Content(),
	})
}

func (g *Generator) renderUTCTimeType(w *codegen.BufferWriter) {
	const body = `// UTCTime — обёртка над time.Time, принудительно сериализующая
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

	w.Print(body)
}
