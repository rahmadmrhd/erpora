package resolve

import (
	"reflect"
	"slices"
	"sort"

	"erpora/internal/devtools/metadatatools/load"
)

// Pelaporan dampak perubahan tipe domain dan blok bersama (Requirement 3.12). Perubahan pada dokumen bersama
// diperlakukan sebagai perubahan atas setiap resource pemakainya (langsung maupun lewat include, termasuk lintas
// modul untuk dokumen export: true). Laporan memuat per pemakai field yang berubah; keputusan SemVer (perubahan
// merusak pada ekspor lintas modul butuh MAJOR) diambil pemeriksa dari DocImpact.Exported, UserImpact.CrossModule,
// dan FieldChange.PotentiallyBreaking, dengan contracts:breaking dan mesin selisih sebagai pengaman kedua.

// Jenis perubahan field.
const (
	ChangeAdded   = "added"
	ChangeRemoved = "removed"
	ChangeChanged = "changed"
)

// FieldChange adalah perubahan satu field pada hasil resolusi sebuah resource.
type FieldChange struct {
	Field string
	Kind  string
	// Attrs adalah nama atribut yang berubah (terurut); hanya untuk ChangeChanged.
	Attrs []string
}

// breakingAttrs adalah atribut yang bila berubah berpotensi merusak klien atau data.
var breakingAttrs = map[string]bool{
	"type": true, "precision": true, "scale": true, "nullable": true, "values": true, "format": true,
	"classification": true, "fieldGroup": true, "readOnly": true, "pattern": true, "minLength": true, "maxLength": true,
	"minimum": true, "maximum": true, "unique": true, "currency": true,
}

// PotentiallyBreaking adalah petunjuk konservatif untuk pemeriksa: penghapusan field atau perubahan atribut
// yang memengaruhi bentuk atau batasan data. Penentu akhir perubahan merusak tetap contracts:breaking.
func (c FieldChange) PotentiallyBreaking() bool {
	if c.Kind == ChangeRemoved {
		return true
	}
	if c.Kind == ChangeChanged {
		for _, a := range c.Attrs {
			if breakingAttrs[a] {
				return true
			}
		}
	}
	return false
}

// UserImpact adalah dampak perubahan satu dokumen bersama pada satu resource pemakai.
type UserImpact struct {
	Resource    string
	Module      string
	CrossModule bool
	// Via adalah jalur dari resource ke dokumen yang berubah (dokumen itu di akhir).
	Via []string
	// Changes adalah field yang berubah pada hasil resolusi resource ini.
	Changes []FieldChange
	// Broken menandai resource yang tidak dapat diselesaikan lagi pada versi baru.
	Broken bool
}

// DocImpact adalah satu dokumen bersama yang berubah beserta semua pemakainya.
type DocImpact struct {
	Key      string
	Module   string
	Kind     string // resource, abstract, atau fieldType
	Exported bool
	Removed  bool
	Added    bool
	Users    []UserImpact
}

// ImpactReport adalah laporan dampak perubahan dokumen bersama antara dua katalog.
type ImpactReport struct {
	Docs []DocImpact
	// Problems adalah temuan resolusi pada katalog baru (mis. dokumen yang dihapus masih dipakai).
	Problems []load.Problem
}

// UserRef adalah resource pemakai sebuah dokumen.
type UserRef struct {
	Key    string
	Module string
	Via    []string
}

// Users mengembalikan resource yang memakai dokumen key (langsung atau transitif), terurut menurut kunci.
func (r *Result) Users(key string) []UserRef {
	var out []UserRef
	for _, res := range r.Resources {
		for _, u := range res.Uses {
			if u.Key == key {
				out = append(out, UserRef{Key: res.Key, Module: res.Module, Via: u.Via})
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func diffFields(old, nw *Resolved) []FieldChange {
	var out []FieldChange
	for _, nf := range nw.Fields {
		of := old.Field(nf.Name)
		if of == nil {
			out = append(out, FieldChange{Field: nf.Name, Kind: ChangeAdded})
			continue
		}
		keys := map[string]bool{}
		for k := range nf.Attrs {
			keys[k] = true
		}
		for k := range of.Attrs {
			keys[k] = true
		}
		var attrs []string
		for k := range keys {
			if !reflect.DeepEqual(nf.Attrs[k], of.Attrs[k]) {
				attrs = append(attrs, k)
			}
		}
		if of.DomainType != nf.DomainType {
			attrs = append(attrs, "domainType")
		}
		if len(attrs) > 0 {
			sort.Strings(attrs)
			out = append(out, FieldChange{Field: nf.Name, Kind: ChangeChanged, Attrs: attrs})
		}
	}
	for _, of := range old.Fields {
		if nw.Field(of.Name) == nil {
			out = append(out, FieldChange{Field: of.Name, Kind: ChangeRemoved})
		}
	}
	return out
}

// Impacts membandingkan dua katalog (mis. metadata pada tag rilis terakhir dan keadaan sekarang) dan
// melaporkan, untuk setiap dokumen yang berubah (ditambah, dihapus, atau isinya berbeda) dan masih atau pernah
// dipakai resource lain, semua resource pemakainya beserta field yang berubah. Dokumen tanpa pemakai tidak
// dilaporkan. Hasil terurut menurut kunci.
func Impacts(oldC, newC *Catalog) *ImpactReport {
	oldRes, newRes := oldC.ResolveAll(), newC.ResolveAll()
	rep := &ImpactReport{Problems: newRes.Problems}

	keys := map[string]bool{}
	for _, k := range oldC.Keys() {
		keys[k] = true
	}
	for _, k := range newC.Keys() {
		keys[k] = true
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)

	for _, key := range sorted {
		od, nd := oldC.Doc(key), newC.Doc(key)
		if od != nil && nd != nil && reflect.DeepEqual(od.Value, nd.Value) {
			continue
		}
		src := nd
		if src == nil {
			src = od
		}
		slug := ""
		if e := newC.docs[key]; e != nil {
			slug = e.mod.Slug
		} else if e := oldC.docs[key]; e != nil {
			slug = e.mod.Slug
		}
		exported, _ := src.Value["export"].(bool)
		di := DocImpact{Key: key, Module: slug, Kind: src.Kind, Exported: exported, Removed: nd == nil, Added: od == nil}

		users := map[string]UserRef{}
		for _, u := range oldRes.Users(key) {
			users[u.Key] = u
		}
		for _, u := range newRes.Users(key) {
			users[u.Key] = u
		}
		// Resource pemakai yang tidak dapat diselesaikan di katalog baru tidak punya Uses; cari lewat katalog lama.
		names := make([]string, 0, len(users))
		for k := range users {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, name := range names {
			if name == key {
				continue
			}
			u := users[name]
			ui := UserImpact{Resource: name, Module: u.Module, CrossModule: u.Module != slug, Via: slices.Clone(u.Via)}
			o, n := oldRes.Resource(name), newRes.Resource(name)
			switch {
			case n == nil:
				ui.Broken = true
			case o == nil:
				// resource baru: tidak ada pembanding
			default:
				ui.Changes = diffFields(o, n)
			}
			di.Users = append(di.Users, ui)
		}
		if len(di.Users) > 0 {
			rep.Docs = append(rep.Docs, di)
		}
	}
	return rep
}
