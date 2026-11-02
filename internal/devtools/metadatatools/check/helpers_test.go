package check

import (
	"fmt"
	"strings"
	"testing"

	"erpora/internal/devtools/metadatatools/load"
	"erpora/internal/devtools/metadatatools/naming"
)

func header(kind, key string) string {
	return "schemaVersion: \"1\"\nkind: " + kind + "\nkey: " + key + "\n"
}

func resource(key, body string) string  { return header("resource", key) + body }
func abstract(key, body string) string  { return header("abstract", key) + body }
func fieldType(key, body string) string { return header("fieldType", key) + body }

type file struct{ name, src string }

func f(name, src string) file { return file{name, src} }

func mustDoc(t *testing.T, name, src string) *load.Document {
	t.Helper()
	d, ps := load.Parse(name, []byte(src))
	if len(ps) != 0 {
		t.Fatalf("berkas uji %s tidak lolos pemuatan: %v", name, ps)
	}
	return d
}

func docs(t *testing.T, files []file) []*load.Document {
	t.Helper()
	var out []*load.Document
	for _, fl := range files {
		out = append(out, mustDoc(t, fl.name, fl.src))
	}
	return out
}

var allActions = []string{"read", "create", "update", "delete", "submit", "cancel", "amend"}

// autoCaps membuat capability <slug>.<resource_snake>.<aksi> untuk semua dokumen resource.
func autoCaps(slug string, ds []*load.Document) []string {
	var caps []string
	for _, d := range ds {
		if d.Kind != "resource" {
			continue
		}
		mk, err := naming.ManifestResourceKey(d.Key)
		if err != nil {
			continue
		}
		for _, a := range allActions {
			caps = append(caps, mk+"."+a)
		}
	}
	return caps
}

// mod membangun modul uji tanpa manifest.resources (keadaan sebelum PRD-001) dengan capability otomatis.
func mod(t *testing.T, slug string, deps []string, files ...file) Module {
	t.Helper()
	ds := docs(t, files)
	return Module{Dir: "modules/" + slug, Manifest: Manifest{Key: "acme." + slug, Slug: slug, Dependencies: deps, Capabilities: autoCaps(slug, ds)}, Docs: ds}
}

func run(t *testing.T, mods ...Module) *Report {
	t.Helper()
	return Run(Input{Modules: mods})
}

// single menjalankan satu modul "m" tanpa dependensi.
func single(t *testing.T, files ...file) *Report {
	t.Helper()
	return run(t, mod(t, "m", nil, files...))
}

func hasCode(ps []Problem, code string) bool {
	for _, p := range ps {
		if p.Code == code {
			return true
		}
	}
	return false
}

func find(ps []Problem, code string) *Problem {
	for i := range ps {
		if ps[i].Code == code {
			return &ps[i]
		}
	}
	return nil
}

// wantErr menegaskan ada galat berkode code dan (bila ptr tidak kosong) pointer memuat ptr.
func wantErr(t *testing.T, rep *Report, code, ptr string) *Problem {
	t.Helper()
	for i := range rep.Problems {
		p := &rep.Problems[i]
		if p.Code == code && p.Severity == SeverityError && strings.Contains(p.Pointer, ptr) {
			if p.File == "" || p.Message == "" {
				t.Errorf("temuan tanpa berkas atau pesan: %+v", *p)
			}
			return p
		}
	}
	t.Fatalf("tidak ada galat %s di %q; temuan: %s", code, ptr, dump(rep.Problems))
	return nil
}

func wantWarning(t *testing.T, rep *Report, code, ptr string) {
	t.Helper()
	for _, p := range rep.Problems {
		if p.Code == code && p.Severity == SeverityWarning && strings.Contains(p.Pointer, ptr) {
			return
		}
	}
	t.Fatalf("tidak ada peringatan %s di %q; temuan: %s", code, ptr, dump(rep.Problems))
}

func noErrors(t *testing.T, rep *Report) {
	t.Helper()
	var errs []Problem
	for _, p := range rep.Problems {
		if p.Severity == SeverityError {
			errs = append(errs, p)
		}
	}
	if len(errs) != 0 {
		t.Fatalf("seharusnya tanpa galat: %s", dump(errs))
	}
}

func noCode(t *testing.T, rep *Report, code string) {
	t.Helper()
	if p := find(rep.Problems, code); p != nil {
		t.Fatalf("tidak boleh ada %s: %+v", code, *p)
	}
}

func dump(ps []Problem) string {
	var sb strings.Builder
	for _, p := range ps {
		fmt.Fprintf(&sb, "\n  %s", p)
	}
	return sb.String()
}

// withField menyusun resource m.r satu field "x" dengan atribut tambahan (YAML flow di dalam {...}).
func oneField(attrs string) file {
	return f("m/r.resource.yaml", resource("m.r", "fields:\n  x: {"+attrs+"}\n"))
}

// noErrorsExceptNonDocLock seperti noErrors tetapi mengabaikan lockwhen_not_document: dipakai tes kondisi yang
// sengaja memakai lockWhen pada resource non-dokumen sekadar untuk menguji model kondisinya.
func noErrorsExceptNonDocLock(t *testing.T, rep *Report) {
	t.Helper()
	var errs []Problem
	for _, p := range rep.Problems {
		if p.Severity == SeverityError && p.Code != CodeLockWhenNotDocument {
			errs = append(errs, p)
		}
	}
	if len(errs) != 0 {
		t.Fatalf("seharusnya tanpa galat: %s", dump(errs))
	}
}
