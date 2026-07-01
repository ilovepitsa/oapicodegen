package parser

import (
	"errors"
	"fmt"
	"path"

	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/datamodel"
	highv3 "github.com/pb33f/libopenapi/datamodel/high/v3"
)

// parseBytes парсит OpenAPI 3.x документ из байтов. location — путь для
// разрешения относительных $ref (пустая строка = без BasePath).
func parseBytes(data []byte, location string) (*Document, error) {
	cfg := &datamodel.DocumentConfiguration{
		AllowFileReferences:     true,
		ExtractRefsSequentially: true,
	}
	if location != "" {
		cfg.BasePath = path.Dir(location)
	}

	doc, err := libopenapi.NewDocumentWithConfiguration(data, cfg)
	if err != nil {
		return nil, fmt.Errorf("parser: new document: %w", err)
	}

	model, errs := doc.BuildV3Model()
	if len(errs) > 0 {
		return nil, fmt.Errorf("parser: build v3 model: %w", errors.Join(errs...))
	}

	return convertDocument(&model.Model), nil
}

func convertDocument(m *highv3.Document) *Document {
	out := &Document{OpenAPI: m.Version}

	if m.Info != nil {
		out.Info = Info{
			Title:       m.Info.Title,
			Description: m.Info.Description,
			Version:     m.Info.Version,
		}
	}

	for _, s := range m.Servers {
		out.Servers = append(out.Servers, Server{
			URL:         s.URL,
			Description: s.Description,
		})
	}

	if m.Components != nil {
		extractComponentsSchemas(out, m.Components.Schemas)
	}

	extractPaths(out, m.Paths)

	return out
}
