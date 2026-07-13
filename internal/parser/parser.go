package parser

import (
	"errors"
	"fmt"
	"io/fs"
	"path"

	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/datamodel"
	highv3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/index"
)

// parseBytes парсит OpenAPI 3.x документ из байтов. location — путь для
// разрешения относительных $ref (пустая строка = без BasePath). fsys —
// filesystem для резолвинга cross-file $ref (nil = OS filesystem по умолчанию).
//
// fsys оборачивается в *index.LocalFS libopenpi, потому что rolodex требует
// реализации RolodexFS (plain fs.FS отвергается с "is not a RolodexFS").
func parseBytes(data []byte, location string, fsys fs.FS) (*Document, error) {
	cfg := &datamodel.DocumentConfiguration{
		AllowFileReferences:     true,
		ExtractRefsSequentially: true,
	}
	if location != "" {
		cfg.BasePath = path.Dir(location)
	}

	if fsys != nil {
		localFS, err := index.NewLocalFSWithConfig(&index.LocalFSConfig{
			BaseDirectory: cfg.BasePath,
			DirFS:         fsys,
		})
		if err != nil {
			return nil, fmt.Errorf("parser: wrap local fs: %w", err)
		}

		cfg.LocalFS = localFS
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

	markUpdateSchemas(out)

	return out
}
