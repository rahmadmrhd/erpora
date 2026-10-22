// Package contracts menyematkan kontrak milik platform (mis. JSON Schema manifest modul).
package contracts

import _ "embed"

// ModuleSchema adalah JSON Schema (2020-12) untuk manifest module.json.
//
//go:embed module.schema.json
var ModuleSchema []byte
