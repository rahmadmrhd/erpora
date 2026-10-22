// Package registry adalah katalog modul terkompilasi: validasi manifest,
// resolusi dependensi, route table, command table, dan capability table
// (PRD-000 FR-05).
package registry

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"erpora/internal/platform/modules/manifest"
	"erpora/internal/platform/modules/resolver"
	"erpora/sdk/module"
)

const (
	// PlatformKey dan PlatformSlug mengidentifikasi modul bawaan platform.core.
	PlatformKey  = "platform.core"
	PlatformSlug = "platform"
)

// Options mengonfigurasi Build.
type Options struct {
	PlatformVersion, SDKVersion string
	// Platform mendaftarkan route/command milik platform.core (dipakai M2b untuk /api/platform/modules).
	Platform func(r module.Registrar) error
}

// RouteInfo adalah satu route terdaftar. Pattern adalah path penuh "/api/{slug}{path}".
type RouteInfo struct {
	Module, Method, Pattern, Permission string
	Public                              bool
	Handler                             http.Handler `json:"-"`
}

// CommandInfo adalah satu command terdaftar; Name berbentuk "<slug>:<name>".
type CommandInfo struct {
	Module, Name string
	Command      module.Command
}

// Entry adalah satu modul dalam katalog; Module nil untuk platform.core.
type Entry struct {
	Manifest module.Manifest
	Module   module.Module
}

// Registry adalah katalog modul yang sudah tervalidasi.
type Registry struct {
	entries  []Entry
	byKey    map[string]Entry
	routes   []RouteInfo
	commands []CommandInfo
	caps     []string
	graph    *resolver.Graph
}

// Build memvalidasi dan merakit katalog. Semua problem dikumpulkan; error
// berupa *Error. Ketidakcocokan platform/SDK bukan error Build (lihat
// Graph().Incompatible()).
func Build(opts Options, mods ...module.Module) (*Registry, error) {
	var problems []Problem

	platform := module.Manifest{Key: PlatformKey, Slug: PlatformSlug, Version: opts.PlatformVersion, Trust: "builtin"}
	byKey := map[string]Entry{PlatformKey: {Manifest: platform}}
	var unique []Entry // modul non-platform dengan key unik, urutan input
	for _, m := range mods {
		mf := m.Manifest()
		for _, p := range manifest.Validate(mf) {
			problems = append(problems, Problem{Module: p.Module, Field: p.Field, Code: p.Code, Message: p.Message})
		}
		if _, dup := byKey[mf.Key]; dup {
			problems = append(problems, Problem{
				Module: mf.Key, Code: CodeDuplicateKey,
				Message: fmt.Sprintf("key %q terdaftar lebih dari sekali", mf.Key),
			})
			continue
		}
		e := Entry{Manifest: mf, Module: m}
		byKey[mf.Key] = e
		unique = append(unique, e)
	}

	nodes := []resolver.Node{{Key: PlatformKey, Version: platform.Version}}
	for _, e := range unique {
		nodes = append(nodes, nodeOf(e.Manifest))
	}
	graph, err := resolver.Resolve(nodes, resolver.Versions{Platform: opts.PlatformVersion, SDK: opts.SDKVersion})
	var order []string
	if err != nil {
		var re *resolver.ResolutionError
		if errors.As(err, &re) {
			for _, is := range re.Issues {
				problems = append(problems, Problem{Module: is.Module, Code: is.Code, Message: is.String()})
			}
		} else {
			problems = append(problems, Problem{Code: "resolve_failed", Message: err.Error()})
		}
		order = sortedKeys(byKey)
	} else {
		order = graph.Order()
	}

	col := &collector{}
	if opts.Platform != nil {
		r := &recorder{key: PlatformKey, slug: PlatformSlug}
		if err := opts.Platform(r); err != nil {
			problems = append(problems, Problem{Module: PlatformKey, Code: CodeRegisterFailed, Message: fmt.Sprintf("Register gagal: %v", err)})
		}
		col.add(r)
	}
	var entries []Entry
	for _, key := range order {
		e := byKey[key]
		entries = append(entries, e)
		if e.Module == nil {
			continue
		}
		r := &recorder{key: key, slug: e.Manifest.Slug}
		if err := e.Module.Register(r); err != nil {
			problems = append(problems, Problem{Module: key, Code: CodeRegisterFailed, Message: fmt.Sprintf("Register gagal: %v", err)})
		}
		col.add(r)
	}

	if len(problems) > 0 {
		sortProblems(problems)
		return nil, &Error{Problems: problems}
	}

	reg := &Registry{entries: entries, byKey: byKey, graph: graph, routes: col.routes, commands: col.cmds}
	slices.SortFunc(reg.routes, func(a, b RouteInfo) int {
		return strings.Compare(a.Pattern+"\x00"+a.Method, b.Pattern+"\x00"+b.Method)
	})
	slices.SortFunc(reg.commands, func(a, b CommandInfo) int { return strings.Compare(a.Name, b.Name) })
	for _, e := range entries {
		reg.caps = append(reg.caps, e.Manifest.Capabilities...)
	}
	slices.Sort(reg.caps)
	return reg, nil
}

func nodeOf(m module.Manifest) resolver.Node {
	n := resolver.Node{Key: m.Key, Version: m.Version, PlatformConstraint: m.Compatibility.Platform, SDKConstraint: m.Compatibility.SDK}
	for _, d := range m.Dependencies {
		n.Deps = append(n.Deps, resolver.Dep{Key: d.Module, Constraint: d.Version, Optional: d.Optional})
	}
	return n
}

func sortedKeys(m map[string]Entry) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// collector menggabungkan rekaman semua recorder.
type collector struct {
	routes []RouteInfo
	cmds   []CommandInfo
}

func (c *collector) add(r *recorder) {
	c.routes = append(c.routes, r.routes...)
	c.cmds = append(c.cmds, r.cmds...)
}

// Routes mengembalikan salinan route table, terurut (Pattern, Method).
func (r *Registry) Routes() []RouteInfo { return slices.Clone(r.routes) }

// Commands mengembalikan salinan command table, terurut Name.
func (r *Registry) Commands() []CommandInfo { return slices.Clone(r.commands) }

// Entries mengembalikan modul dalam urutan topologis graph.
func (r *Registry) Entries() []Entry { return slices.Clone(r.entries) }

// Entry mencari modul berdasarkan key.
func (r *Registry) Entry(key string) (Entry, bool) {
	e, ok := r.byKey[key]
	return e, ok
}

// Graph mengembalikan graf dependensi hasil resolusi.
func (r *Registry) Graph() *resolver.Graph { return r.graph }

// Capabilities mengembalikan semua capability, terurut.
func (r *Registry) Capabilities() []string { return slices.Clone(r.caps) }
