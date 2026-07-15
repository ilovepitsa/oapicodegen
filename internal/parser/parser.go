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

	nfs "nschugorev/oapigenerator/internal/fs"
)

// parseBytes парсит OpenAPI 3.x документ из байтов. location — путь для
// разрешения относительных $ref (пустая строка = без BasePath). fsys —
// filesystem для чтения spec-файла и cross-file $ref.
//
// Для real OS filesystem (nfs.RealFS) LocalFS не настраивается — libopenapi
// использует default rolodex, что позволяет cross-service $ref выходить за
// каталог текущей спеки. Для mock FS (тесты) fsys оборачивается в
// index.LocalFS, чтобы rolodex мог читать из mock-файлов.
func parseBytes(data []byte, location string, fsys fs.FS) (*Document, error) {
	cfg := &datamodel.DocumentConfiguration{
		AllowFileReferences:     true,
		ExtractRefsSequentially: true,
	}
	if location != "" {
		cfg.BasePath = path.Dir(location)
	}

	if fsys != nil {
		if err := configureLocalFS(cfg, fsys); err != nil {
			return nil, err
		}
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

// configureLocalFS настраивает cfg.LocalFS для mock-filesystem (тесты).
// Для real OS filesystem (nfs.RealFS) пропускается — libopenapi использует
// default rolodex, что позволяет cross-service $ref выходить за каталог
// текущей спеки.
func configureLocalFS(cfg *datamodel.DocumentConfiguration, fsys fs.FS) error {
	if _, isReal := fsys.(*nfs.RealFS); isReal {
		return nil
	}

	localFS, err := index.NewLocalFSWithConfig(&index.LocalFSConfig{
		BaseDirectory: cfg.BasePath,
		DirFS:         fsys,
	})
	if err != nil {
		return fmt.Errorf("parser: wrap local fs: %w", err)
	}

	cfg.LocalFS = localFS

	return nil
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
