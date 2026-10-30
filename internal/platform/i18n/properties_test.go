package i18n

import (
	"context"
	"encoding/json"
	"maps"
	"math/rand"
	"reflect"
	"sort"
	"strings"
	"testing"
	"testing/quick"

	sdki18n "erpora/sdk/i18n"
)

// P3 Pencarian total: untuk namespace, kunci, dan bahasa sembarang (string acak), Resolve selalu
// mengembalikan string tanpa panik; bila tidak ada di rantai, hasilnya tepat namespace::kunci.
func TestPropertyResolveTotalRandomInputs(t *testing.T) {
	s := newTestService(t, Options{})
	prop := func(ns, key, loc string) bool {
		src, origin := s.Resolve(context.Background(), tenantA, ns, key, sdki18n.Locale(loc))
		if origin == OriginMissing {
			return src == ns+"::"+key
		}
		return src != ""
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 5000}); err != nil {
		t.Fatal(err)
	}
	// ns dan bahasa valid dengan kunci acak
	prop2 := func(key string) bool {
		src, origin := s.Resolve(context.Background(), tenantA, "example", key, "en-GB")
		return origin != OriginMissing || src == "example::"+key
	}
	if err := quick.Check(prop2, &quick.Config{MaxCount: 2000}); err != nil {
		t.Fatal(err)
	}
}

// P7 ETag mengikuti isi: untuk dua peta pesan acak, ETag sama tepat bila petanya sama. Diuji pada Service
// sebenarnya dengan katalog JSON yang dibangkitkan dari peta.
func TestPropertyETagFollowsContent(t *testing.T) {
	etagOf := func(m map[string]string) string {
		raw, _ := json.Marshal(normalize(m))
		files := map[string]string{"id.json": string(raw)}
		s, err := New(Options{Registry: testRegistry(t), Sources: []Source{
			platformSource(), uiSource(),
			{Namespace: "example", Module: true, Default: "id", Locales: []string{"id"}, FS: mapFS(files)},
		}})
		if err != nil {
			t.Fatalf("New: %v (json=%s)", err, raw)
		}
		v, err := s.Catalog(context.Background(), tenantA, "example", "id")
		if err != nil {
			t.Fatal(err)
		}
		return v.ETag
	}
	prop := func(a, b map[string]string) bool {
		same := maps.EqualFunc(normalize(a), normalize(b), func(x, y string) bool { return x == y })
		return (etagOf(a) == etagOf(b)) == same
	}
	small := func(r *rand.Rand) reflect.Value {
		m := map[string]string{}
		for _, k := range []string{"a", "b", "c"} {
			if r.Intn(3) > 0 {
				m[k] = []string{"x", "y", "z"}[r.Intn(3)]
			}
		}
		return reflect.ValueOf(m)
	}
	cfg := &quick.Config{MaxCount: 400, Values: func(args []reflect.Value, r *rand.Rand) {
		args[0], args[1] = small(r), small(r)
	}}
	if err := quick.Check(prop, cfg); err != nil {
		t.Fatal(err)
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 100}); err != nil { // peta acak liar
		t.Fatal(err)
	}
	// peta yang sama selalu ETag yang sama (deterministik walau urutan iterasi map acak)
	m := map[string]string{"a": "1", "b": "2", "c": "3", "d": "4"}
	first := etagOf(m)
	for i := 0; i < 20; i++ {
		if etagOf(m) != first {
			t.Fatal("ETag tidak deterministik")
		}
	}
}

func sanitizeKey(k string) string {
	var sb strings.Builder
	for _, r := range k {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// escapeICU membuang karakter yang bermakna di ICU/HTML agar pesan acak selalu lolos validasi katalog.
func escapeICU(v string) string {
	var sb strings.Builder
	for _, r := range v {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == ' ' {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// normalize memetakan peta acak ke peta berkunci/bernilai sah secara deterministik (urutan kunci terurut
// agar tabrakan hasil penyaringan selalu diselesaikan sama).
func normalize(m map[string]string) map[string]string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := map[string]string{}
	for _, k := range keys {
		out["k"+sanitizeKey(k)] = "v" + escapeICU(m[k])
	}
	return out
}
