package resolve

import (
	"erpora/internal/devtools/metadatatools/load"
	"erpora/sdk/metadata/builtin"
)

// builtinDir adalah awalan jalur berkas bawaan pada pesan galat.
const builtinDir = "sdk/metadata/builtin/"

// BuiltinDocs memuat metadata bawaan SDK yang dibenamkan (sdk.audit, sdk.document, sdk.email, sdk.phone,
// sdk.percentage, sdk.money).
func BuiltinDocs() ([]*load.Document, []load.Problem) {
	docs, probs := load.LoadFS(builtin.FS, ".")
	for _, d := range docs {
		d.File = builtinDir + d.File
	}
	for i := range probs {
		probs[i].File = builtinDir + probs[i].File
	}
	return docs, probs
}

// AddBuiltin mendaftarkan metadata bawaan SDK sebagai modul semu "sdk" (dapat dirujuk semua modul tanpa
// dependensi manifest).
func (c *Catalog) AddBuiltin() []load.Problem {
	docs, probs := BuiltinDocs()
	return append(probs, c.addModule(Module{Slug: SDKSlug}, docs...)...)
}

// NewCatalogWithBuiltin membuat katalog yang sudah memuat metadata bawaan SDK.
func NewCatalogWithBuiltin() (*Catalog, []load.Problem) {
	c := NewCatalog()
	return c, c.AddBuiltin()
}
