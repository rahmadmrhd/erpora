package naming

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"testing/quick"
)

// safeWords adalah kosakata kata yang pemisahannya tidak ambigu (panjang >= 2, termasuk inisialisme baku).
var safeWords = []string{
	"customer", "name", "invoice", "line", "address2", "street", "city", "postal", "code", "tax", "total",
	"id", "url", "uuid", "api", "http", "html", "json", "price", "unit", "ref", "number", "status", "amount",
	"x1", "note", "item", "created", "at", "by",
}

// genName membangkitkan identifier camelCase sah dari kosakata aman (1 sampai 5 kata).
type genName string

func (genName) Generate(r *rand.Rand, _ int) reflect.Value {
	n := 1 + r.Intn(5)
	var sb strings.Builder
	for i := 0; i < n; i++ {
		w := safeWords[r.Intn(len(safeWords))]
		if i > 0 {
			w = strings.ToUpper(w[:1]) + w[1:]
		}
		sb.WriteString(w)
	}
	return reflect.ValueOf(genName(sb.String()))
}

func quickCfg() *quick.Config {
	return &quick.Config{MaxCount: 2000, Rand: rand.New(rand.NewSource(20261101))}
}

func TestPropertySnakeKebabRoundTrip(t *testing.T) {
	f := func(g genName) bool {
		name := string(g)
		if len(name) > MaxIdentifierBytes || ValidateIdentifier(name) != nil {
			return true // di luar domain properti
		}
		snake, err := ToSnake(name)
		if err != nil {
			return true // snake > 63 byte: ditolak, bukan salah konversi
		}
		back, err := FromSnake(snake)
		if err != nil || back != name {
			t.Logf("snake bolak-balik gagal: %q -> %q -> %q (%v)", name, snake, back, err)
			return false
		}
		kebab, err := ToKebab(name)
		if err != nil {
			return true
		}
		back, err = FromKebab(kebab)
		return err == nil && back == name
	}
	if err := quick.Check(f, quickCfg()); err != nil {
		t.Fatal(err)
	}
}

func TestPropertyPascalRoundTripOnSafeWords(t *testing.T) {
	f := func(g genName) bool {
		name := string(g)
		if ValidateIdentifier(name) != nil {
			return true
		}
		p, err := ToPascal(name)
		if err != nil {
			return false
		}
		back, err := FromPascal(p)
		if err != nil || back != name {
			t.Logf("pascal bolak-balik gagal: %q -> %q -> %q (%v)", name, p, back, err)
			return false
		}
		return CheckRoundTrip(name) == nil
	}
	if err := quick.Check(f, quickCfg()); err != nil {
		t.Fatal(err)
	}
}

func TestPropertyConversionsAreInjectiveAcrossGeneratedNames(t *testing.T) {
	rnd := rand.New(rand.NewSource(7))
	seenSnake := map[string]string{}
	seenPascal := map[string]string{}
	for i := 0; i < 5000; i++ {
		name := string(genName("").Generate(rnd, 0).Interface().(genName))
		if ValidateIdentifier(name) != nil {
			continue
		}
		if s, err := ToSnake(name); err == nil {
			if prev, ok := seenSnake[s]; ok && prev != name {
				t.Fatalf("tabrakan snake_case: %q dan %q -> %q", prev, name, s)
			}
			seenSnake[s] = name
		}
		if p, err := ToPascal(name); err == nil {
			if prev, ok := seenPascal[p]; ok && prev != name {
				t.Fatalf("tabrakan PascalCase: %q dan %q -> %q", prev, name, p)
			}
			seenPascal[p] = name
		}
	}
}

func TestPropertyPluralNeverEmptyAndDeterministic(t *testing.T) {
	f := func(g genName) bool {
		w, err := Words(string(g))
		if err != nil {
			return true
		}
		last := w[len(w)-1]
		p1, p2 := Plural(last), Plural(last)
		return p1 != "" && p1 == p2
	}
	if err := quick.Check(f, quickCfg()); err != nil {
		t.Fatal(err)
	}
}

// FuzzConvert memastikan tidak ada panik dan bahwa setiap nama yang sah bolak-balik identik ke snake_case.
func FuzzConvert(f *testing.F) {
	for _, s := range []string{"", "a", "customerName", "taxID", "aBC", "address2Line", "iD", "Customer", "x_y", "a-b", "ünï", "\x00", strings.Repeat("a", 70)} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		_ = Validate(s)
		_ = ReservedTargets(s)
		if ValidateIdentifier(s) != nil {
			if _, err := ToSnake(s); err == nil {
				t.Fatalf("ToSnake(%q) menerima identifier tidak sah", s)
			}
			return
		}
		snake, err := ToSnake(s)
		if err != nil {
			return
		}
		back, err := FromSnake(snake)
		if err != nil || back != s {
			t.Fatalf("snake bolak-balik: %q -> %q -> %q (%v)", s, snake, back, err)
		}
		_, _ = ToPascal(s)
		_, _ = ToKebab(s)
	})
}
