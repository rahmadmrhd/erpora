package resolve

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"testing"
	"testing/quick"
)

// graph adalah graf penyertaan acak: abstrak m.a0..m.a(n-1), tiap abstrak punya field unik fK dan menyertakan
// subset abstrak lain (mungkin menimbulkan siklus atau berlian). Resource m.r menyertakan subset.
type graph struct {
	n    int
	adj  [][]int
	root []int
}

func genGraph(r *rand.Rand) graph {
	n := 2 + r.Intn(5)
	g := graph{n: n, adj: make([][]int, n)}
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			// kebanyakan DAG (j > i); sesekali sisi balik atau sisi diri untuk siklus.
			if j > i && r.Intn(3) == 0 || j <= i && r.Intn(12) == 0 {
				g.adj[i] = append(g.adj[i], j)
			}
		}
	}
	for j := 0; j < n; j++ {
		if r.Intn(2) == 0 {
			g.root = append(g.root, j)
		}
	}
	return g
}

func (g graph) catalog(t *testing.T) *Catalog {
	c := NewCatalog()
	var files []file
	for i := 0; i < g.n; i++ {
		var inc strings.Builder
		if len(g.adj[i]) > 0 {
			inc.WriteString("include:\n")
			for _, j := range g.adj[i] {
				fmt.Fprintf(&inc, "  - source: m.a%d\n", j)
			}
		}
		files = append(files, f(fmt.Sprintf("m/a%d.abstract.yaml", i),
			abstract(fmt.Sprintf("m.a%d", i), inc.String()+fmt.Sprintf("fields:\n  f%s: {type: text}\n", letters(i)))))
	}
	var inc strings.Builder
	if len(g.root) > 0 {
		inc.WriteString("include:\n")
		for _, j := range g.root {
			fmt.Fprintf(&inc, "  - source: m.a%d\n", j)
		}
	}
	files = append(files, f("m/r.resource.yaml", resource("m.r", inc.String()+"fields:\n  own: {type: text}\n")))
	addModule(t, c, Module{Slug: "m"}, files...)
	return c
}

// expect menghitung hasil yang diharapkan secara independen: ada siklus terjangkau, ada berlian (simpul
// dicapai lewat dua jalur), atau gabungan field.
func (g graph) expect() (cycle, diamond bool, fields []string) {
	state := make([]int, g.n) // 0 belum, 1 di tumpukan, 2 selesai
	visits := make([]int, g.n)
	var dfs func(i int)
	dfs = func(i int) {
		visits[i]++
		if state[i] == 1 {
			cycle = true
			return
		}
		if state[i] == 2 {
			return
		}
		state[i] = 1
		for _, j := range g.adj[i] {
			dfs(j)
		}
		state[i] = 2
	}
	for _, j := range g.root {
		dfs(j)
	}
	for i, v := range visits {
		if v > 1 && !cycle {
			diamond = true
		}
		if state[i] == 2 {
			fields = append(fields, "f"+letters(i))
		}
	}
	sort.Strings(fields)
	return
}

func TestPropertyIncludeGraphs(t *testing.T) {
	r := rand.New(rand.NewSource(20261101))
	for i := 0; i < 400; i++ {
		g := genGraph(r)
		c := g.catalog(t)
		res, ps := c.Resolve("m.r") // harus selalu berhenti
		cycle, diamond, fields := g.expect()
		switch {
		case cycle:
			if !hasCode(ps, CodeIncludeCycle) {
				t.Fatalf("graf %+v: siklus tidak terdeteksi: %v", g, ps)
			}
		case diamond:
			if !hasCode(ps, CodeDuplicateField) {
				t.Fatalf("graf %+v: berlian harus menghasilkan field ganda: %v", g, ps)
			}
		default:
			if len(ps) != 0 || res == nil {
				t.Fatalf("graf %+v: seharusnya lolos: %v", g, ps)
			}
			got := fieldNames(res)
			sort.Strings(got)
			want := append([]string{"own"}, fields...)
			sort.Strings(want)
			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Fatalf("graf %+v: field %v, ingin %v", g, got, want)
			}
			// deterministik
			res2, _ := c.Resolve("m.r")
			if joined(res) != joined(res2) {
				t.Fatal("tidak deterministik")
			}
		}
	}
}

// Tes properti: nama field hasil prefix selalu sama dengan Prefixed(prefix, nama) dan tidak bertabrakan.
func TestPropertyPrefixDeterministic(t *testing.T) {
	prefixes := []string{"billing", "shipping", "home", "office", "alt"}
	fn := func(a, b uint8) bool {
		p1, p2 := prefixes[int(a)%len(prefixes)], prefixes[int(b)%len(prefixes)]
		if p1 == p2 {
			return true
		}
		c := NewCatalog()
		addModule(t, c, Module{Slug: "m"},
			f("m/blk.abstract.yaml", abstract("m.blk", "fields:\n  street: {type: text}\n  city: {type: text}\n")),
			f("m/r.resource.yaml", resource("m.r", fmt.Sprintf("include:\n  - {source: m.blk, prefix: %s}\n  - {source: m.blk, prefix: %s}\n", p1, p2))))
		r, ps := c.Resolve("m.r")
		if len(ps) != 0 {
			return false
		}
		want := fmt.Sprintf("%sStreet,%sCity,%sStreet,%sCity", p1, p1, p2, p2)
		return joined(r) == want
	}
	if err := quick.Check(fn, &quick.Config{MaxCount: 100, Rand: rand.New(rand.NewSource(1))}); err != nil {
		t.Fatal(err)
	}
}
