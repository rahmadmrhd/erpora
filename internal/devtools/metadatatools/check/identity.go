package check

import (
	"regexp"
	"strconv"
	"strings"

	"erpora/internal/platform/modules/registry"
)

// moduleSlugRe dan batas 50 karakter sama dengan contracts/module.schema.json (kebab-case, diawali huruf).
var moduleSlugRe = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// expectedModuleDir adalah direktori modul yang sah untuk sebuah slug: modules/<slug tanpa tanda hubung>
// (aturan yang sama dengan generator make:module dan pembangkit metadata).
func expectedModuleDir(slug string) string {
	return "modules/" + strings.ReplaceAll(slug, "-", "")
}

// checkModuleIdentities memeriksa identitas tiap modul sebelum metadatanya dipercaya (Requirement 3.5, 19.3,
// 19.4): slug sah dan tidak tercadang, direktori sama dengan slug tanpa tanda hubung, slug dan kunci modul unik.
// Modul yang melanggar dilaporkan dan dikeluarkan dari Input yang diperiksa sehingga tidak dapat menyamar
// sebagai modul lain atau sebagai metadata bawaan SDK, dan pembangkit tidak pernah menulis ke direktori yang
// bukan miliknya. Modul pertama yang sah mempertahankan slug dan kunci; kemunculan berikutnya ditolak.
func (c *checker) checkModuleIdentities(in Input) Input {
	out := Input{Git: in.Git}
	slugs := map[string]string{} // slug -> direktori pemilik pertama
	keys := map[string]string{}  // kunci modul -> direktori pemilik pertama
	for _, m := range in.Modules {
		slug := m.Manifest.Slug
		file := m.Dir + "/module.json"
		bad := false
		report2 := func(code, ptr, msg string) {
			bad = true
			c.add(Problem{Severity: SeverityError, File: file, Pointer: ptr, Code: code, Message: msg, Module: slug})
		}
		report := func(code, msg string) { report2(code, "/slug", msg) }
		switch {
		case len(slug) > 50 || !moduleSlugRe.MatchString(slug):
			report(CodeModuleSlugInvalid, "slug modul "+quote(slug)+" tidak sah: kebab-case huruf kecil (a-z, 0-9, tanda hubung), paling banyak 50 karakter, diawali huruf")
		case registry.IsReservedSlug(slug):
			report(CodeModuleSlugReserved, "slug modul "+quote(slug)+" dicadangkan untuk platform dan tidak boleh dipakai modul (Requirement 3.5)")
		}
		if bad {
			continue
		}
		if want := expectedModuleDir(slug); m.Dir != want {
			report(CodeModuleDirMismatch, "direktori modul "+quote(m.Dir)+" tidak sama dengan direktori yang diturunkan dari slug "+quote(slug)+" ("+want+"); metadata hanya boleh dibangkitkan ke direktori modulnya sendiri (Requirement 19.3)")
		}
		if prev, dup := slugs[slug]; dup {
			report(CodeModuleSlugDuplicate, "slug modul "+quote(slug)+" sudah dipakai oleh modul di "+prev)
		}
		if k := m.Manifest.Key; k != "" {
			if prev, dup := keys[k]; dup {
				report2(CodeModuleKeyDuplicate, "/key", "kunci modul "+quote(k)+" sudah dipakai oleh modul di "+prev)
			}
		}
		if bad {
			continue
		}
		if k := m.Manifest.Key; k != "" {
			keys[k] = m.Dir
		}
		slugs[slug] = m.Dir
		c.rep.ModuleDirs[slug] = m.Dir
		out.Modules = append(out.Modules, m)
	}
	return out
}

func quote(s string) string { return strconv.Quote(s) }
