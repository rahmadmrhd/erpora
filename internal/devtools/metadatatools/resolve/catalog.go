package resolve

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"erpora/internal/devtools/metadatatools/load"
)

// Module menjelaskan modul pemilik sekumpulan dokumen metadata.
type Module struct {
	// Slug adalah slug modul (awalan kunci dokumen).
	Slug string
	// Dependencies adalah slug modul yang tercantum di dependencies manifest (dan require go.mod; D1, D8). Hanya
	// modul ini yang boleh dirujuk lewat include atau tipe domain, dan hanya untuk dokumen export: true.
	Dependencies []string
	// DefaultGroup adalah kunci grup field yang bertanda default (PRD-001 FR-110: mis. "general"), dipakai untuk
	// field yang tidak menyebut fieldGroup. Kosong (manifest belum mendefinisikan grup, sebelum PRD-001) berarti
	// grup default implisit bernama DefaultGroupName.
	DefaultGroup string
}

// DefaultGroupName adalah nama grup default implisit bila manifest tidak mendefinisikan grup.
const DefaultGroupName = "default"

func (m Module) defaultGroup() string {
	if m.DefaultGroup != "" {
		return m.DefaultGroup
	}
	return DefaultGroupName
}

type entry struct {
	doc *load.Document
	mod Module
}

// Catalog adalah kumpulan dokumen metadata dari beberapa modul dan metadata bawaan SDK, diindeks menurut
// kunci. Kunci dokumen diselesaikan lewat katalog ini, bukan sebagai jalur berkas (Requirement 3.5).
type Catalog struct {
	mods map[string]Module
	docs map[string]*entry
}

// NewCatalog membuat katalog kosong. Metadata bawaan SDK ditambahkan lewat AddBuiltin (builtin.go).
func NewCatalog() *Catalog {
	return &Catalog{mods: map[string]Module{}, docs: map[string]*entry{}}
}

func problemAtDoc(d *load.Document, ptr, code, msg string) load.Problem {
	p := d.Pos(ptr)
	return load.Problem{File: d.File, Line: p.Line, Column: p.Column, Pointer: ptr, Code: code, Message: msg}
}

// AddModule mendaftarkan modul dan dokumennya. Slug pada kunci dokumen harus sama dengan slug modul dan kunci
// tidak boleh ganda; dokumen yang melanggar dilaporkan dan tidak didaftarkan.
func (c *Catalog) AddModule(m Module, docs ...*load.Document) []load.Problem {
	var probs []load.Problem
	if m.Slug == "" {
		return []load.Problem{{Code: CodeKeyModule, Message: "slug modul kosong"}}
	}
	if prev, ok := c.mods[m.Slug]; ok {
		// Pendaftaran ulang melengkapi dokumen; dependensi digabung.
		for _, d := range prev.Dependencies {
			if !slices.Contains(m.Dependencies, d) {
				m.Dependencies = append(m.Dependencies, d)
			}
		}
	}
	c.mods[m.Slug] = m
	for _, d := range docs {
		slug, _, _ := strings.Cut(d.Key, ".")
		if slug != m.Slug {
			probs = append(probs, problemAtDoc(d, "/key", CodeKeyModule,
				fmt.Sprintf("slug %q pada kunci %q tidak sama dengan slug modul %q", slug, d.Key, m.Slug)))
			continue
		}
		if prev, ok := c.docs[d.Key]; ok {
			probs = append(probs, problemAtDoc(d, "/key", CodeDuplicateKey,
				fmt.Sprintf("kunci %q sudah dipakai oleh %s", d.Key, prev.doc.File)))
			continue
		}
		c.docs[d.Key] = &entry{doc: d, mod: c.mods[m.Slug]}
	}
	// Dependensi yang diperbarui berlaku untuk semua dokumen modul.
	for _, e := range c.docs {
		if e.mod.Slug == m.Slug {
			e.mod = c.mods[m.Slug]
		}
	}
	return probs
}

// Doc mengembalikan dokumen menurut kunci, nil bila tidak ada.
func (c *Catalog) Doc(key string) *load.Document {
	if e := c.docs[key]; e != nil {
		return e.doc
	}
	return nil
}

// Keys mengembalikan semua kunci dokumen terurut.
func (c *Catalog) Keys() []string {
	keys := make([]string, 0, len(c.docs))
	for k := range c.docs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Module mengembalikan modul menurut slug.
func (c *Catalog) Module(slug string) (Module, bool) {
	m, ok := c.mods[slug]
	return m, ok
}

// sourceKey memetakan include.source atau rujukan tipe ke kunci katalog: sdk:<nama> menjadi sdk.<nama>.
func sourceKey(ref string) string {
	if rest, ok := strings.CutPrefix(ref, "sdk:"); ok {
		return SDKSlug + "." + rest
	}
	return ref
}

// accessible memeriksa apakah modul from boleh merujuk dokumen target: modul sendiri dan metadata bawaan SDK
// selalu boleh; modul lain hanya lewat dependensi manifest dan untuk dokumen export: true.
func accessible(from Module, target *entry) (bool, string) {
	slug := target.mod.Slug
	if slug == from.Slug || slug == SDKSlug {
		return true, ""
	}
	if !slices.Contains(from.Dependencies, slug) {
		return false, fmt.Sprintf("modul %q tidak mencantumkan %q di dependencies manifest (D1, D8)", from.Slug, slug)
	}
	if exp, _ := target.doc.Value["export"].(bool); !exp {
		return false, fmt.Sprintf("%q tidak diekspor oleh modul %q (export: true)", target.doc.Key, slug)
	}
	return true, ""
}
