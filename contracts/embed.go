// Package contracts menyematkan kontrak milik platform (mis. JSON Schema manifest modul).
package contracts

import _ "embed"

// ModuleSchema adalah JSON Schema (2020-12) untuk manifest module.json.
//
//go:embed module.schema.json
var ModuleSchema []byte

// CommonOpenAPI adalah komponen OpenAPI bersama (openapi/common.yaml); dirujuk kontrak lain lewat erp:common.
//
//go:embed openapi/common.yaml
var CommonOpenAPI []byte

// PlatformOpenAPI adalah kontrak OpenAPI platform.core (openapi/platform.yaml).
//
//go:embed openapi/platform.yaml
var PlatformOpenAPI []byte
