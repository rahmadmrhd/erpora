package check

import (
	"fmt"
	"slices"
	"sort"

	"erpora/internal/devtools/metadatatools/naming"
)

// manifestFile adalah lokasi module.json modul untuk temuan yang menunjuk manifest.
func (x *rctx) manifestFile() string { return x.mod.in.Dir + "/module.json" }

func (x *rctx) atManifest(sev Severity, ptr, code, format string, args ...any) {
	x.emit(sev, x.manifestFile(), 0, 0, ptr, code, format, args...)
}

// checkManifest memeriksa integrasi dengan manifest (Requirement 7.1, 7.2, 7.4): kunci resource, grup field,
// kesamaan daftar fields, dan permission operasi yang dibangkitkan.
func (x *rctx) checkManifest() {
	mk, err := naming.ManifestResourceKey(x.r.Key)
	if err != nil {
		return
	}
	man := x.mod.in.Manifest
	if man.HasResources {
		if x.mres == nil {
			x.atDoc(SeverityError, "/key", CodeManifestResourceMissing,
				"resource %s tidak dideklarasikan di resources manifest (diharapkan kunci %q; PRD-001 FR-110)", x.r.Key, mk)
		} else {
			x.checkGroups()
		}
	} else {
		has := false
		for _, c := range man.Capabilities {
			if len(c) > len(mk) && c[:len(mk)+1] == mk+"." {
				has = true
				break
			}
		}
		if !has {
			x.atDoc(SeverityError, "/key", CodeManifestCapabilityPrefix,
				"resource %s tidak punya capability berawalan %q di manifest (sebelum PRD-001 kunci resource harus cocok dengan prefiks capability)", x.r.Key, mk+".")
		}
	}

	if crud, _ := x.r.Generate["crud"].(bool); crud {
		need := []string{"read", "create", "update", "delete"}
		if x.isDoc {
			need = append(need, "submit", "cancel", "amend")
		}
		var missing []string
		for _, a := range need {
			capOK := slices.Contains(man.Capabilities, mk+"."+a)
			actOK := true
			if x.mres != nil {
				actOK = slices.Contains(x.mres.Actions, a) || slices.Contains(x.mres.CustomActions, a)
			}
			if !capOK || !actOK {
				missing = append(missing, a)
			}
		}
		if len(missing) > 0 {
			x.atDoc(SeverityError, "/generate", CodeManifestActionMissing,
				"generate.crud memerlukan aksi %s tetapi belum terdaftar sebagai capability %s.<aksi> dan di resources[].actions manifest: %s", joinNames(need), mk, joinNames(missing))
		}
	}
}

// checkGroups memeriksa keanggotaan grup field terhadap definisi manifest (grup default = grup bertanda
// default; tanpa definisi hanya default implisit) dan kesamaan daftar fields bila manifest memuatnya (P-1).
func (x *rctx) checkGroups() {
	g := x.eff.groups
	for _, f := range x.r.Fields {
		raw := f.Str("fieldGroup")
		if raw == "" {
			continue
		}
		key := snake(raw)
		_, known := g.known[key]
		if !known && !(g.implicit && key == g.def) {
			x.err(f, "fieldGroup", CodeManifestGroupUnknown,
				"field %s menyebut grup %q yang tidak didefinisikan di field_groups manifest resource %s (grup default adalah %q)", f.Name, key, x.mres.Key, g.def)
		}
	}
	for _, mg := range x.mres.FieldGroups {
		if mg.Fields == nil {
			continue
		}
		want := map[string]bool{}
		for _, f := range x.r.Fields {
			if x.eff.group[f.Name] == mg.Key {
				want[snake(f.Name)] = true
			}
		}
		listed := map[string]bool{}
		for _, n := range mg.Fields {
			listed[n] = true
		}
		for _, n := range sortedKeys(want) {
			if !listed[n] {
				f := x.fields[camel(n)]
				if f != nil {
					x.err(f, "fieldGroup", CodeManifestFieldsMismatch,
						"field %s berada di grup %s menurut metadata tetapi tidak tercantum di fields grup itu pada manifest (metadata adalah sumber keanggotaan)", f.Name, mg.Key)
				}
			}
		}
		for _, n := range sortedKeysBool(listed) {
			if want[n] {
				continue
			}
			if f := x.fields[camel(n)]; f != nil {
				x.err(f, "fieldGroup", CodeManifestFieldsMismatch,
					"manifest mencantumkan field %s di grup %s tetapi metadata menempatkannya di grup %s", n, mg.Key, x.eff.group[f.Name])
				continue
			}
			x.atManifest(SeverityError, fmt.Sprintf("/resources/%s/field_groups/%s/fields", x.mres.Key, mg.Key), CodeManifestFieldsMismatch,
				"manifest mencantumkan field %q di grup %s yang tidak ada di metadata resource %s", n, mg.Key, x.r.Key)
		}
	}
}

func sortedKeysBool(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// checkOwnerScope memeriksa owner_field dan scope_fields (Requirement 7.5): merujuk field yang ada, bertipe uuid
// (id pengguna atau sumber dimensi scope), dan konsisten dengan manifest bila keduanya menyatakannya.
func (x *rctx) checkOwnerScope() {
	manOwner, manScope := "", map[string]string(nil)
	if x.mres != nil {
		manOwner, manScope = x.mres.OwnerField, x.mres.ScopeFields
	}
	owner := x.r.OwnerField
	if owner != "" && manOwner != "" && snake(owner) != manOwner {
		x.atDoc(SeverityError, "/ownerField", CodeOwnerManifestMismatch,
			"ownerField %s tidak sama dengan owner_field manifest %s", owner, manOwner)
	}
	check := func(name, code, codeType, ptr, what string, fromManifest bool) {
		f := x.fields[name]
		report := func(c, format string, args ...any) {
			if fromManifest {
				x.atManifest(SeverityError, fmt.Sprintf("/resources/%s/%s", x.mres.Key, what), c, format, args...)
			} else {
				x.atDoc(SeverityError, ptr, c, format, args...)
			}
		}
		if f == nil {
			report(code, "%s merujuk field %q yang tidak ada di resource %s", what, name, x.r.Key)
			return
		}
		if f.Type() != "uuid" {
			report(codeType, "%s harus merujuk field bertipe uuid, bukan %s (field %s)", what, f.Type(), name)
		}
	}
	switch {
	case owner != "":
		check(owner, CodeOwnerFieldUnknown, CodeOwnerFieldType, "/ownerField", "owner_field", false)
	case manOwner != "":
		check(camel(manOwner), CodeOwnerFieldUnknown, CodeOwnerFieldType, "", "owner_field", true)
	}
	dims := map[string]bool{}
	for d := range x.r.ScopeFields {
		dims[d] = true
	}
	for d := range manScope {
		dims[d] = true
	}
	for _, d := range sortedKeysBool(dims) {
		meta, hasMeta := x.r.ScopeFields[d]
		man, hasMan := manScope[d]
		if hasMeta && hasMan && snake(meta) != man {
			x.atDoc(SeverityError, "/scopeFields", CodeOwnerManifestMismatch,
				"scopeFields[%s] = %s tidak sama dengan scope_fields manifest (%s)", d, meta, man)
		}
		if hasMeta {
			check(meta, CodeScopeFieldUnknown, CodeScopeFieldType, "/scopeFields", "scope_fields["+d+"]", false)
		} else if hasMan {
			check(camel(man), CodeScopeFieldUnknown, CodeScopeFieldType, "", "scope_fields["+d+"]", true)
		}
	}
}
