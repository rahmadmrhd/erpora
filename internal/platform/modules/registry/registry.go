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
	// PlatformCapabilities adalah capability yang dideklarasikan platform.core;
	// route platform tunduk pada aturan permission_undeclared seperti modul lain.
	PlatformCapabilities []string
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

	platform := module.Manifest{
		Key: PlatformKey, Slug: PlatformSlug, Version: opts.PlatformVersion, Trust: "builtin",
		Capabilities: slices.Clone(opts.PlatformCapabilities),
	}
	byKey := map[string]Entry{PlatformKey: {Manifest: platform}}
	var unique []Entry // modul non-platform dengan key unik, urutan input
	for i, m := range mods {
		mf, perr := safeManifest(m)
		if perr != "" {
			problems = append(problems, Problem{Module: fmt.Sprintf("modules[%d]", i), Code: CodeRegisterFailed, Message: perr})
			continue
		}
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

	problems = append(problems, checkIdentity(unique, platform)...)

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
		r := &recorder{key: PlatformKey, slug: PlatformSlug, caps: capSet(platform)}
		if msg := safeRegister(opts.Platform, r); msg != "" {
			problems = append(problems, Problem{Module: PlatformKey, Code: CodeRegisterFailed, Message: msg})
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
		r := &recorder{key: key, slug: e.Manifest.Slug, caps: capSet(e.Manifest)}
		if msg := safeRegister(e.Module.Register, r); msg != "" {
			problems = append(problems, Problem{Module: key, Code: CodeRegisterFailed, Message: msg})
		}
		col.add(r)
	}

	problems = append(problems, col.problems...)
	problems = append(problems, checkRouteConflicts(col.routes)...)
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

// safeManifest memanggil m.Manifest() dengan recover; modul nil atau panic dilaporkan sebagai pesan.
func safeManifest(m module.Module) (mf module.Manifest, problem string) {
	if m == nil {
		return mf, "modul nil tidak dapat didaftarkan"
	}
	defer func() {
		if p := recover(); p != nil {
			problem = fmt.Sprintf("Manifest() panic: %v", p)
		}
	}()
	return m.Manifest(), ""
}

// safeRegister memanggil register dengan recover; mengembalikan pesan bila gagal atau panic.
func safeRegister(register func(module.Registrar) error, r module.Registrar) (problem string) {
	defer func() {
		if p := recover(); p != nil {
			problem = fmt.Sprintf("Register panic: %v", p)
		}
	}()
	if err := register(r); err != nil {
		return fmt.Sprintf("Register gagal: %v", err)
	}
	return ""
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
	routes   []RouteInfo
	cmds     []CommandInfo
	problems []Problem
}

func (c *collector) add(r *recorder) {
	c.problems = append(c.problems, r.problems...)
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

func capSet(m module.Manifest) map[string]bool {
	s := make(map[string]bool, len(m.Capabilities))
	for _, c := range m.Capabilities {
		s[c] = true
	}
	return s
}

// checkIdentity memeriksa keunikan slug, slug tercadang, serta keunikan dan
// prefix capability. Modul diproses sesuai urutan input.
func checkIdentity(mods []Entry, platform module.Manifest) []Problem {
	var out []Problem
	slugOwner := map[string]string{PlatformSlug: PlatformKey}
	capOwner := map[string]string{}
	// Capability platform.core: prefix "platform." dan keunikan lintas modul.
	for _, c := range platform.Capabilities {
		if !strings.HasPrefix(c, platform.Slug+".") {
			out = append(out, Problem{Module: platform.Key, Field: "/capabilities", Code: CodeCapabilityPrefix,
				Message: fmt.Sprintf("capability %q harus berawalan %q", c, platform.Slug+".")})
		}
		capOwner[c] = platform.Key
	}
	for _, e := range mods {
		m := e.Manifest
		if !dependsOnPlatform(m) {
			out = append(out, Problem{Module: m.Key, Field: "/dependencies", Code: resolver.CodeMissingDependency,
				Message: "modul wajib bergantung (non-opsional) pada " + PlatformKey})
		}
		if reservedSlugs[m.Slug] {
			out = append(out, Problem{Module: m.Key, Field: "/slug", Code: CodeReservedSlug,
				Message: fmt.Sprintf("slug %q dicadangkan untuk platform", m.Slug)})
		}
		if other, dup := slugOwner[m.Slug]; dup {
			out = append(out, Problem{Module: m.Key, Field: "/slug", Code: CodeDuplicateSlug,
				Message: fmt.Sprintf("slug %q dipakai oleh %s dan %s", m.Slug, other, m.Key)})
		} else {
			slugOwner[m.Slug] = m.Key
		}
		for _, c := range m.Capabilities {
			if !strings.HasPrefix(c, m.Slug+".") {
				out = append(out, Problem{Module: m.Key, Field: "/capabilities", Code: CodeCapabilityPrefix,
					Message: fmt.Sprintf("capability %q harus berawalan %q", c, m.Slug+".")})
			}
			if other, dup := capOwner[c]; dup {
				out = append(out, Problem{Module: m.Key, Field: "/capabilities", Code: CodeDuplicateCapability,
					Message: fmt.Sprintf("capability %q didefinisikan oleh %s dan %s", c, other, m.Key)})
			} else {
				capOwner[c] = m.Key
			}
		}
	}
	return out
}

func dependsOnPlatform(m module.Manifest) bool {
	for _, d := range m.Dependencies {
		if d.Module == PlatformKey && !d.Optional {
			return true
		}
	}
	return false
}

// muxRegister dapat diganti test agar klasifikasi konflik terbukti tidak bergantung pada teks panic.
var muxRegister = tryRegister

// checkRouteConflicts mendaftarkan semua pola ke ServeMux sementara; panic
// karena konflik atau pola rusak diubah menjadi Problem.
func checkRouteConflicts(routes []RouteInfo) []Problem {
	var out []Problem
	mux := http.NewServeMux()
	var seen []RouteInfo
	for _, rt := range routes {
		msg := muxRegister(mux, rt)
		if msg == "" {
			seen = append(seen, rt)
			continue
		}
		// Pola yang sendirian saja ditolak ServeMux = pola rusak, bukan konflik.
		if alone := muxRegister(http.NewServeMux(), rt); alone != "" {
			out = append(out, Problem{Module: rt.Module, Code: CodeInvalidRoute,
				Message: fmt.Sprintf("pola route %s %s ditolak: %s", rt.Method, rt.Pattern, alone)})
			continue
		}
		// Pola valid tetapi gagal didaftarkan: cari pendaftaran lain yang bentrok (tanpa membaca teks panic).
		if other := findConflict(seen, rt); other.Pattern != "" {
			out = append(out, Problem{Module: rt.Module, Code: CodeRouteConflict,
				Message: fmt.Sprintf("route %s %s (%s) bentrok dengan %s %s (%s)", rt.Method, rt.Pattern, rt.Module, other.Method, other.Pattern, other.Module)})
			continue
		}
		out = append(out, Problem{Module: rt.Module, Code: CodeInvalidRoute,
			Message: fmt.Sprintf("pola route %s %s ditolak: %s", rt.Method, rt.Pattern, msg)})
	}
	return out
}

var noopHandler = http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})

// tryRegister mengembalikan pesan panic ServeMux (kosong bila sukses).
func tryRegister(mux *http.ServeMux, rt RouteInfo) (msg string) {
	defer func() {
		if p := recover(); p != nil {
			msg = fmt.Sprint(p)
		}
	}()
	mux.Handle(rt.Method+" "+rt.Pattern, noopHandler)
	return ""
}

// findConflict mencari pendaftaran terdahulu yang bentrok dengan rt.
func findConflict(seen []RouteInfo, rt RouteInfo) RouteInfo {
	for _, prev := range seen {
		m := http.NewServeMux()
		if muxRegister(m, prev) == "" && muxRegister(m, rt) != "" {
			return prev
		}
	}
	return RouteInfo{}
}
