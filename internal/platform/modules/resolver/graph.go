// Package resolver membangun graf dependensi modul: validasi, deteksi siklus,
// dan urutan topologis deterministik (PRD-000 FR-18).
package resolver

import (
	"slices"

	"erpora/internal/platform/modules/compatibility"
)

// Dep adalah dependensi satu modul ke modul lain.
type Dep struct {
	Key        string
	Constraint string
	Optional   bool
}

// Node adalah satu modul dalam katalog.
type Node struct {
	Key                string
	Version            string
	PlatformConstraint string
	SDKConstraint      string
	Deps               []Dep
}

// Versions adalah versi platform dan SDK yang berjalan.
type Versions struct{ Platform, SDK string }

// Graph adalah hasil resolusi yang valid.
type Graph struct {
	nodes        map[string]Node
	order        []string
	requiredDeps map[string][]string // dependensi wajib langsung
	requiredRev  map[string][]string // dependen wajib langsung
	incompatible map[string][]Issue
}

// Resolve memvalidasi katalog. Masalah struktural (dependensi hilang, versi
// tidak cocok, siklus, constraint rusak) dikembalikan sekaligus sebagai
// *ResolutionError; ketidakcocokan platform/SDK dicatat di Incompatible().
func Resolve(nodes []Node, v Versions) (*Graph, error) {
	g := &Graph{
		nodes:        make(map[string]Node, len(nodes)),
		requiredDeps: map[string][]string{},
		requiredRev:  map[string][]string{},
		incompatible: map[string][]Issue{},
	}
	for _, n := range nodes {
		g.nodes[n.Key] = n
	}
	keys := make([]string, 0, len(g.nodes))
	for k := range g.nodes {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	var issues []Issue
	adj := map[string][]string{} // semua edge ke dependensi yang ada (wajib + opsional)
	for _, k := range keys {
		n := g.nodes[k]
		issues = g.checkRuntime(n, v, issues)
		for _, d := range n.Deps {
			target, ok := g.nodes[d.Key]
			if !ok {
				if !d.Optional {
					issues = append(issues, Issue{Code: CodeMissingDependency, Module: k, Dependency: d.Key, Constraint: d.Constraint})
				}
				continue
			}
			adj[k] = append(adj[k], d.Key)
			if !d.Optional {
				g.requiredDeps[k] = append(g.requiredDeps[k], d.Key)
				g.requiredRev[d.Key] = append(g.requiredRev[d.Key], k)
			}
			satisfied, err := compatibility.Satisfies(target.Version, d.Constraint)
			base := Issue{Module: k, Dependency: d.Key, Constraint: d.Constraint, Found: target.Version}
			switch {
			case err != nil:
				base.Code = CodeInvalidConstraint
				issues = append(issues, base)
			case !satisfied && d.Optional:
				base.Code = CodeOptionalVersionMismatch
				issues = append(issues, base)
			case !satisfied:
				base.Code = CodeVersionMismatch
				issues = append(issues, base)
			}
		}
		slices.Sort(adj[k])
		adj[k] = slices.Compact(adj[k])
	}

	issues = append(issues, findCycles(keys, adj)...)
	if len(issues) > 0 {
		return nil, &ResolutionError{Issues: issues}
	}
	g.order = topoOrder(keys, adj)
	return g, nil
}

// checkRuntime memeriksa constraint platform/SDK; ketidakcocokan masuk
// incompatible, constraint rusak masuk issues.
func (g *Graph) checkRuntime(n Node, v Versions, issues []Issue) []Issue {
	check := func(constraint, running, code string) {
		if constraint == "" {
			return
		}
		ok, err := compatibility.Satisfies(running, constraint)
		switch {
		case err != nil:
			issues = append(issues, Issue{Code: CodeInvalidConstraint, Module: n.Key, Constraint: constraint, Found: running})
		case !ok:
			g.incompatible[n.Key] = append(g.incompatible[n.Key], Issue{Code: code, Module: n.Key, Constraint: constraint, Found: running})
		}
	}
	check(n.PlatformConstraint, v.Platform, CodePlatformIncompatible)
	check(n.SDKConstraint, v.SDK, CodeSDKIncompatible)
	return issues
}

// findCycles memakai DFS tiga warna; jalur direkonstruksi dari stack.
func findCycles(keys []string, adj map[string][]string) []Issue {
	const (
		white = iota
		grey
		black
	)
	color := map[string]int{}
	var stack []string
	var out []Issue
	var visit func(k string)
	visit = func(k string) {
		color[k] = grey
		stack = append(stack, k)
		for _, d := range adj[k] {
			switch color[d] {
			case white:
				visit(d)
			case grey:
				start := slices.Index(stack, d)
				path := append(slices.Clone(stack[start:]), d)
				out = append(out, Issue{Code: CodeCycle, Module: d, Path: path})
			}
		}
		stack = stack[:len(stack)-1]
		color[k] = black
	}
	for _, k := range keys {
		if color[k] == white {
			visit(k)
		}
	}
	return out
}

// topoOrder menjalankan Kahn; himpunan siap diurutkan alfabet di tiap langkah.
func topoOrder(keys []string, adj map[string][]string) []string {
	indeg := make(map[string]int, len(keys))
	rev := map[string][]string{}
	for _, k := range keys {
		indeg[k] = len(adj[k])
		for _, d := range adj[k] {
			rev[d] = append(rev[d], k)
		}
	}
	var ready []string
	for _, k := range keys {
		if indeg[k] == 0 {
			ready = append(ready, k)
		}
	}
	order := make([]string, 0, len(keys))
	for len(ready) > 0 {
		slices.Sort(ready)
		k := ready[0]
		ready = ready[1:]
		order = append(order, k)
		for _, dep := range rev[k] {
			indeg[dep]--
			if indeg[dep] == 0 {
				ready = append(ready, dep)
			}
		}
	}
	return order
}

// Order mengembalikan urutan topologis deterministik (dependensi lebih dulu).
func (g *Graph) Order() []string { return slices.Clone(g.order) }

// Incompatible mengembalikan modul yang tidak cocok dengan platform/SDK.
func (g *Graph) Incompatible() map[string][]Issue {
	out := make(map[string][]Issue, len(g.incompatible))
	for k, v := range g.incompatible {
		out[k] = slices.Clone(v)
	}
	return out
}

// Node mengembalikan node katalog berdasarkan key.
func (g *Graph) Node(key string) (Node, bool) {
	n, ok := g.nodes[key]
	return n, ok
}

// RequiredDependencies mengembalikan dependensi wajib transitif, urutan topologis.
func (g *Graph) RequiredDependencies(key string) []string {
	return g.closure(key, g.requiredDeps, false)
}

// RequiredDependents mengembalikan dependen wajib transitif, urutan topologis terbalik.
func (g *Graph) RequiredDependents(key string) []string {
	return g.closure(key, g.requiredRev, true)
}

func (g *Graph) closure(key string, edges map[string][]string, reverse bool) []string {
	seen := map[string]bool{}
	var walk func(k string)
	walk = func(k string) {
		for _, n := range edges[k] {
			if !seen[n] {
				seen[n] = true
				walk(n)
			}
		}
	}
	walk(key)
	out := make([]string, 0, len(seen))
	for _, k := range g.order {
		if seen[k] {
			out = append(out, k)
		}
	}
	if reverse {
		slices.Reverse(out)
	}
	return out
}
