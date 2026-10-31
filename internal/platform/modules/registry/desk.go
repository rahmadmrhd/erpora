package registry

import (
	"fmt"
	"io/fs"
	"path"
	"strconv"
	"strings"

	sdki18n "erpora/sdk/i18n"
	"erpora/sdk/module"
)

// dashboardID adalah id item Dashboard bawaan setiap Desk; dibangkitkan platform sehingga tidak boleh dipakai
// menu yang dideklarasikan modul (Q40 butir 13, design.md D-8).
const dashboardID = "dashboard"

// maxDeskToLen membatasi panjang path menu (skema juga membatasinya).
const maxDeskToLen = 200

// CheckDesk memeriksa bagian "desk" manifest di luar yang sudah dicakup JSON Schema (revisi M3a, design.md
// Komponen 5): id menu unik, permission termasuk capabilities modul, "to" path internal yang bersih, tidak ada
// menu yang menyamar sebagai Dashboard, dan setiap kunci label ada di katalog bahasa dasar modul. catalogs
// adalah katalog modul (module.CatalogSource; nil bila tidak ada, sehingga semua label dianggap tidak ada).
// Dipakai Build dan `module:check`; Field berupa JSON pointer ke module.json. nil bila modul tanpa Desk.
func CheckDesk(mf module.Manifest, catalogs fs.FS) []Problem {
	if mf.Desk == nil {
		return nil
	}
	return checkDesk(mf, labelLookup(mf, catalogs))
}

// labelLookup membangun pencarian kunci label pada katalog bahasa dasar modul. Katalog yang tidak terbaca
// (rusak) dikembalikan nil: pemeriksaan label dilewati karena kerusakannya dilaporkan i18n (startup,
// i18n:check), bukan Desk.
func labelLookup(mf module.Manifest, catalogs fs.FS) func(string) bool {
	none := func(string) bool { return false }
	if catalogs == nil || mf.I18n == nil {
		return none
	}
	f, err := catalogs.Open(mf.I18n.DefaultLocale + ".json")
	if err != nil {
		return none
	}
	defer func() { _ = f.Close() }()
	flat, issues := sdki18n.ParseCatalog(f, sdki18n.DefaultLimits())
	if len(issues) > 0 {
		return nil
	}
	return func(key string) bool { _, ok := flat[key]; return ok }
}

// checkDesk menjalankan pemeriksaan Desk; hasLabel nil melewati pemeriksaan label (dipakai Desk platform,
// yang katalognya diperiksa test, bukan registry).
func checkDesk(mf module.Manifest, hasLabel func(string) bool) []Problem {
	d := mf.Desk
	if d == nil {
		return nil
	}
	var out []Problem
	add := func(field, code, format string, a ...any) {
		out = append(out, Problem{Module: mf.Key, Field: "/desk" + field, Code: code, Message: fmt.Sprintf(format, a...)})
	}
	label := func(field, key string) {
		if hasLabel != nil && !hasLabel(key) {
			add(field, CodeDeskLabelUnknown, "kunci label %q tidak ada di katalog bahasa dasar modul", key)
		}
	}
	caps := capSet(mf)
	seen := map[string]bool{}
	id := func(field, v string) {
		switch {
		case v == dashboardID:
			add(field, CodeDeskDashboardLocked, "id %q dicadangkan untuk Dashboard yang dibangkitkan platform", v)
		case seen[v]:
			add(field, CodeDeskDuplicateID, "id menu %q dipakai lebih dari sekali dalam Desk", v)
		default:
			seen[v] = true
		}
	}
	item := func(base string, m module.DeskMenu) {
		id(base+"/id", m.ID)
		label(base+"/label", m.Label)
		switch {
		case m.To == "/":
			add(base+"/to", CodeDeskDashboardLocked, "path \"/\" adalah indeks modul (Dashboard) dan tidak boleh dideklarasikan sebagai menu")
		case !validDeskTo(m.To):
			add(base+"/to", CodeDeskInvalidTo, "path %q tidak valid: wajib path internal relatif base route (diawali \"/\", bersih, tanpa \"//\", skema, host, query, atau fragmen)", m.To)
		}
		if !caps[m.Permission] {
			add(base+"/permission", CodeDeskUnknownPermission, "permission %q bukan salah satu capabilities modul", m.Permission)
		}
	}

	label("/label", d.Label)
	if d.Dashboard != nil && d.Dashboard.Label != "" {
		label("/dashboard/label", d.Dashboard.Label)
	}
	for i, m := range d.Menus {
		base := "/menus/" + strconv.Itoa(i)
		if len(m.Children) == 0 {
			item(base, m)
			continue
		}
		id(base+"/id", m.ID)
		label(base+"/label", m.Label)
		for j, c := range m.Children {
			item(base+"/children/"+strconv.Itoa(j), c)
		}
	}
	return out
}

// validDeskTo memeriksa path menu relatif base route modul: diawali "/", bersih (path.Clean), tanpa trailing
// slash, "//", segmen "." atau "..", dan hanya karakter segmen URL aman (tanpa skema, host, query, fragmen,
// persen, atau spasi).
func validDeskTo(p string) bool {
	if len(p) < 2 || len(p) > maxDeskToLen || !strings.HasPrefix(p, "/") || path.Clean(p) != p {
		return false
	}
	for _, seg := range strings.Split(p[1:], "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
		for _, r := range seg {
			ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' || r == '~'
			if !ok {
				return false
			}
		}
	}
	return true
}
