package parser

import "nschugorev/oapigenerator/internal/codegen/gogen"

// PathImports — типизированные Go-импорты артефактов одного сервиса.
// Создаётся один раз при Project.CreatePaths(basePath) и переиспользуется
// генератором вместо строковой конкатенации modulePath+"/<artifact>".
type PathImports struct {
	ClientInterfaces gogen.Import // <base>/interfaces/client
	ServerInterfaces gogen.Import // <base>/interfaces/server
	ClientHTTP       gogen.Import // <base>/impl/httpclient
	ServerHTTP       gogen.Import // <base>/impl/echoserver
	ClientMocks      gogen.Import // <base>/impl/mocks/client
	ServerMocks      gogen.Import // <base>/impl/mocks/server
	Model            gogen.Import // <base>/model
	SDK              gogen.Import // <base>/sdk
}
