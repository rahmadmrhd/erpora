package registry

import (
	"fmt"
	"io/fs"
	"strconv"

	"erpora/sdk/module"
)

// CodeAuthzLabelUnknown: kunci label resource, aksi kustom, grup field, dimensi, atau role bawaan tidak ada di
// katalog bahasa dasar modul (PRD-001 FR-110, K-C2.5).
const CodeAuthzLabelUnknown = "authz_label_unknown"

// CheckAuthorizationLabels memastikan setiap label deklarasi otorisasi manifest (resources, custom_actions,
// field_groups, scope_dimensions, default_roles) ada di katalog bahasa dasar modul, seperti label Desk (label
// berupa kunci i18n, bukan teks literal). catalogs adalah katalog modul (module.CatalogSource). Modul tanpa bagian
// "i18n" dilewati karena katalognya tidak dapat diperiksa; katalog yang rusak juga dilewati (dilaporkan i18n).
// Dipakai Build dan `module:check`; Field berupa JSON pointer ke module.json.
func CheckAuthorizationLabels(mf module.Manifest, catalogs fs.FS) []Problem {
	if mf.I18n == nil || (len(mf.Resources) == 0 && len(mf.ScopeDimensions) == 0 && len(mf.DefaultRoles) == 0) {
		return nil
	}
	has := labelLookup(mf, catalogs)
	if has == nil {
		return nil
	}
	var out []Problem
	check := func(field, key string) {
		if !has(key) {
			out = append(out, Problem{Module: mf.Key, Field: field, Code: CodeAuthzLabelUnknown,
				Message: fmt.Sprintf("kunci label %q tidak ada di katalog bahasa dasar modul", key)})
		}
	}
	for i, r := range mf.Resources {
		rb := "/resources/" + strconv.Itoa(i)
		check(rb+"/label", r.Label)
		for j, c := range r.CustomActions {
			check(rb+"/custom_actions/"+strconv.Itoa(j)+"/label", c.Label)
		}
		for j, g := range r.FieldGroups {
			check(rb+"/field_groups/"+strconv.Itoa(j)+"/label", g.Label)
		}
	}
	for i, d := range mf.ScopeDimensions {
		check("/scope_dimensions/"+strconv.Itoa(i)+"/label", d.Label)
	}
	for i, r := range mf.DefaultRoles {
		check("/default_roles/"+strconv.Itoa(i)+"/label", r.Label)
	}
	return out
}
