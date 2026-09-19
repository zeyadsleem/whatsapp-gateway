package httpapi

import (
	_ "embed"
)

//go:embed openapi.yaml
var openAPISpecBytes []byte

func openAPISpec() []byte { return openAPISpecBytes }
