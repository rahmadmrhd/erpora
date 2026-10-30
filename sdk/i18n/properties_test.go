package i18n

import (
	"context"
	"math/rand"
	"sort"
	"strings"
	"testing"
	"testing/quick"
)

// tree adalah pohon katalog acak: setiap simpul punya anak berkunci unik (peta) sehingga tidak ada
// kunci ganda pada tingkat yang sama; tabrakan hanya mungkin lewat segmen bertitik.
type tree struct {
	leaf     string
	children map[string]*tree
}

var segAlphabet = []string{"a", "b", "c", "a.b", "b.c", "x.y.z", "k-1", "k_2", "n1", "a.a"}

func genTree(r *rand.Rand, depth int) *tree {
	t := &tree{children: map[string]*tree{}}
	n := 1 + r.Intn(4)
	for i := 0; i < n; i++ {
		seg := segAlphabet[r.Intn(len(segAlphabet))]
		if _, dup := t.children[seg]; dup {
			continue
		}
		if depth >= 3 || r.Intn(3) == 0 {
			t.children[seg] = &tree{leaf: "pesan " + seg}
		} else {
			t.children[seg] = genTree(r, depth+1)
		}
	}
	return t
}

// naiveFlatten menghitung perataan acuan: kunci lengkap -> daftar jalur penghasilnya.
func naiveFlatten(t *tree, segs []string, out map[string][]string) {
	keys := make([]string, 0, len(t.children))
	for k := range t.children {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		c := t.children[k]
		next := append(append([]string(nil), segs...), k)
		if len(c.children) == 0 && c.leaf != "" {
			out[strings.Join(next, ".")] = append(out[strings.Join(next, ".")], pathString(next))
			continue
		}
		naiveFlatten(c, next, out)
	}
}

func (t *tree) writeJSON(sb *strings.Builder) {
	if len(t.children) == 0 {
		sb.WriteString(`"` + t.leaf + `"`)
		return
	}
	keys := make([]string, 0, len(t.children))
	for k := range t.children {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	sb.WriteString("{")
	for i, k := range keys {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`"` + k + `":`)
		t.children[k].writeJSON(sb)
	}
	sb.WriteString("}")
}

// P1 Perataan deterministik: pohon tanpa tabrakan menghasilkan tepat satu entri per daun dan setiap
// daun dapat dicari lewat kunci lengkap; dua jalur yang menghasilkan kunci sama selalu dilaporkan
// sebagai flatten_collision (tidak pernah hilang diam-diam).
func TestPropertyFlattenDeterministic(t *testing.T) {
	collisions, clean := 0, 0
	prop := func(seed int64) bool {
		ok, collided := checkFlattenSeed(t, seed)
		if collided {
			collisions++
		} else {
			clean++
		}
		return ok
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 3000}); err != nil {
		t.Fatal(err)
	}
	if collisions == 0 || clean == 0 {
		t.Errorf("generator harus menghasilkan kedua kelas (tabrakan=%d, bersih=%d)", collisions, clean)
	}
}

// Regresi (G23): seed ini menghasilkan tabrakan TIGA jalur pada kunci b.c.b.c; satu Issue wajib memuat semua jalur.
func TestPropertyFlattenRegressionSeeds(t *testing.T) {
	for _, seed := range []int64{4095330077871122206} {
		if ok, collided := checkFlattenSeed(t, seed); !ok || !collided {
			t.Errorf("seed %d: ok=%v collided=%v, want ok dan tabrakan", seed, ok, collided)
		}
	}
}

// checkFlattenSeed membangkitkan pohon dari seed, memuatnya lewat ParseCatalog, dan memeriksa P1 terhadap
// perataan acuan. collided=true bila pohon punya tabrakan perataan. Pada tabrakan: Flat nil, dan untuk SETIAP kunci
// yang bertabrakan ada TEPAT SATU Issue flatten_collision yang menyebut SEMUA jalur penghasilnya (Requirement 1.6).
func checkFlattenSeed(t *testing.T, seed int64) (ok, collided bool) {
	t.Helper()
	r := rand.New(rand.NewSource(seed))
	root := genTree(r, 0)
	if len(root.children) == 0 {
		return true, false
	}
	ref := map[string][]string{}
	naiveFlatten(root, nil, ref)
	var sb strings.Builder
	root.writeJSON(&sb)

	got, issues := ParseCatalog(strings.NewReader(sb.String()), DefaultLimits())

	var collidedKeys []string
	for k, paths := range ref {
		if len(paths) > 1 {
			collidedKeys = append(collidedKeys, k)
		}
	}
	if len(collidedKeys) > 0 {
		if got != nil {
			t.Logf("seed %d: Flat harus nil saat ada tabrakan", seed)
			return false, true
		}
		total := 0
		for _, is := range issues {
			if is.Code == CodeFlattenCollision {
				total++
			}
		}
		if total != len(collidedKeys) {
			t.Logf("seed %d: %d Issue flatten_collision untuk %d kunci bertabrakan; json=%s issues=%v", seed, total, len(collidedKeys), sb.String(), issues)
			return false, true
		}
		for _, k := range collidedKeys {
			n := 0
			for _, is := range issues {
				if is.Code != CodeFlattenCollision || is.Key != k {
					continue
				}
				n++
				for _, p := range ref[k] {
					if !strings.Contains(is.Detail, p) {
						t.Logf("seed %d: Detail %q tidak menyebut jalur %s", seed, is.Detail, p)
						return false, true
					}
				}
			}
			if n != 1 {
				t.Logf("seed %d: kunci %q dilaporkan %d kali, want 1; json=%s issues=%v", seed, k, n, sb.String(), issues)
				return false, true
			}
		}
		return true, true
	}
	if len(issues) != 0 {
		t.Logf("seed %d: issues tak terduga %v; json=%s", seed, issues, sb.String())
		return false, false
	}
	if len(got) != len(ref) {
		t.Logf("seed %d: len(Flat)=%d, want %d", seed, len(got), len(ref))
		return false, false
	}
	for k := range ref {
		if _, present := got[k]; !present {
			t.Logf("seed %d: kunci %q hilang", seed, k)
			return false, false
		}
	}
	return true, false
}

// P12 Namespace tertutup: dengan scope modul m, T hanya pernah meminta namespace m, ui, atau platform
// ke Resolver, dan tidak pernah mengembalikan pesan dari namespace modul lain.
func TestPropertyNamespaceClosed(t *testing.T) {
	scopes := []string{"example", "stock", "hr"}
	others := []string{"example", "stock", "hr", "secret", "ui", "platform", "Example", "", "a::b"}
	rels := []string{"a.b", "note.title", "", "x::y", "a", "::a"}
	prop := func(si, oi, ri uint8, absolute bool) bool {
		scope := scopes[int(si)%len(scopes)]
		ns := others[int(oi)%len(others)]
		rel := rels[int(ri)%len(rels)]
		key := rel
		if absolute {
			key = ns + "::" + rel
		}
		spy := &allowAllResolver{}
		ctx := WithScope(WithResolver(context.Background(), spy), scope)
		out := T(ctx, key)
		for _, asked := range spy.asked {
			if asked != scope && asked != NamespaceUI && asked != NamespacePlatform {
				t.Logf("scope %q key %q meminta namespace %q", scope, key, asked)
				return false
			}
		}
		if strings.Contains(out, "ISI-") {
			// pesan hanya boleh berasal dari namespace yang diizinkan
			for _, ns := range []string{NamespaceUI, NamespacePlatform, scope} {
				if strings.HasPrefix(out, "ISI-"+ns+"|") {
					return true
				}
			}
			t.Logf("scope %q key %q mengembalikan pesan namespace lain: %q", scope, key, out)
			return false
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 2000}); err != nil {
		t.Fatal(err)
	}
}

// allowAllResolver menjawab setiap namespace; isinya memuat nama namespace asalnya.
type allowAllResolver struct{ asked []string }

func (a *allowAllResolver) Resolve(_ context.Context, ns, key string, _ Locale) (string, bool) {
	a.asked = append(a.asked, ns)
	return "ISI-" + ns + "|" + key, true
}
