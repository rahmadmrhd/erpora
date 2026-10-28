package gen

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"erpora/internal/devtools/templates"
)

// Render merender template ter-embed (name = path di templates/, mis. "migration/module.sql.tmpl") dengan
// missingkey=error, sehingga kunci data yang hilang menjadi error, bukan teks "<no value>".
//
// Delimiter: template .sql.tmpl memakai {{ }}; semua template lain (Go, TS, YAML, JSON, Markdown) memakai [[ ]]
// agar tidak bentrok dengan kode yang memuat "{{" (mis. []httpx.Detail{{...}}).
func Render(name string, data any) ([]byte, error) {
	src, err := templates.FS.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("template %s: %w", name, err)
	}
	t := template.New(name).Option("missingkey=error")
	if !strings.HasSuffix(name, ".sql.tmpl") {
		t = t.Delims("[[", "]]")
	}
	t, err = t.Parse(string(src))
	if err != nil {
		return nil, fmt.Errorf("template %s: %w", name, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("render %s: %w", name, err)
	}
	return buf.Bytes(), nil
}
