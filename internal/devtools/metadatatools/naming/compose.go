package naming

import (
	"fmt"
	"sort"
	"strings"
)

// snakeSlug mengonversi slug modul (kebab-case) ke bentuk snake_case untuk nama SQL.
func snakeSlug(slug string) (string, error) {
	if !slugRe.MatchString(slug) || len(slug) > MaxSlugBytes {
		return "", errf(CodeInvalidSlug, slug, "slug modul harus huruf kecil, angka, dan tanda hubung (paling banyak %d byte)", MaxSlugBytes)
	}
	return strings.ReplaceAll(slug, "-", "_"), nil
}

// TableName menyusun nama tabel <slug>_<nama jamak> dalam snake_case (design Komponen 13): slug "sales" dan
// resource "invoiceLine" menjadi "sales_invoice_lines". Panjang hasil dibatasi 63 byte.
func TableName(slug, resourceName string) (string, error) {
	s, err := snakeSlug(slug)
	if err != nil {
		return "", err
	}
	n, err := PluralSnake(resourceName)
	if err != nil {
		return "", err
	}
	full := s + "_" + n
	if len(full) > MaxIdentifierBytes {
		return "", errf(CodeTooLong, full, "nama tabel %d byte melebihi batas %d byte", len(full), MaxIdentifierBytes)
	}
	return full, nil
}

// JoinName menggabungkan bagian snake_case dengan garis bawah dan memastikan hasilnya <= 63 byte. Dipakai
// untuk nama gabungan yang aturannya belum dibakukan (mis. tabel pivot): pemanggil menentukan bagian-bagiannya.
func JoinName(parts ...string) (string, error) {
	if len(parts) == 0 {
		return "", errf(CodeNotSnakeCase, "", "tidak ada bagian nama")
	}
	for _, p := range parts {
		if !relationRe.MatchString(p) {
			return "", errf(CodeNotSnakeCase, p, "bagian nama harus snake_case huruf kecil, angka, dan garis bawah")
		}
	}
	full := strings.Join(parts, "_")
	if len(full) > MaxIdentifierBytes {
		return "", errf(CodeTooLong, full, "nama gabungan %d byte melebihi batas %d byte", len(full), MaxIdentifierBytes)
	}
	return full, nil
}

func columnsSnake(cols []string) ([]string, error) {
	if len(cols) == 0 {
		return nil, errf(CodeNotCamelCase, "", "minimal satu kolom")
	}
	out := make([]string, len(cols))
	for i, c := range cols {
		s, err := ToSnake(c)
		if err != nil {
			return nil, err
		}
		out[i] = s
	}
	return out, nil
}

func suffixedName(table string, cols []string, suffix string) (string, error) {
	snake, err := columnsSnake(cols)
	if err != nil {
		return "", err
	}
	return JoinName(append(append([]string{table}, snake...), suffix)...)
}

// IndexName menyusun nama indeks <tabel>_<kolom...>_idx (kolom camelCase dikonversi).
func IndexName(table string, cols ...string) (string, error) { return suffixedName(table, cols, "idx") }

// UniqueName menyusun nama constraint unique <tabel>_<kolom...>_key (gaya bawaan PostgreSQL).
func UniqueName(table string, cols ...string) (string, error) {
	return suffixedName(table, cols, "key")
}

// ForeignKeyName menyusun nama foreign key <tabel>_<kolom...>_fkey (gaya bawaan PostgreSQL).
func ForeignKeyName(table string, cols ...string) (string, error) {
	return suffixedName(table, cols, "fkey")
}

// CheckName menyusun nama constraint CHECK <tabel>_<kolom>_check.
func CheckName(table string, cols ...string) (string, error) {
	return suffixedName(table, cols, "check")
}

// TriggerName menyusun nama pemicu <tabel>_<tujuan>_trg; tujuan berupa identifier camelCase.
func TriggerName(table, purpose string) (string, error) {
	return suffixedName(table, []string{purpose}, "trg")
}

// Prefixed menggabungkan awalan blok dan nama field secara camelCase (billing + street menjadi
// billingStreet; Requirement 3.10). Hasil harus lolos aturan identifier (termasuk batas 63 byte).
func Prefixed(prefix, name string) (string, error) {
	if err := ValidateIdentifier(prefix); err != nil {
		return "", err
	}
	if err := ValidateIdentifier(name); err != nil {
		return "", err
	}
	out := prefix + capitalize(name)
	if err := ValidateIdentifier(out); err != nil {
		return "", err
	}
	return out, nil
}

// Collision adalah beberapa sumber berbeda yang menghasilkan nama target yang sama.
type Collision struct {
	Target  string
	Sources []string // terurut, tanpa duplikat
}

// FindCollisions mengonversi tiap sumber dengan convert dan melaporkan nama target yang dihasilkan lebih dari
// satu sumber berbeda. Sumber yang gagal dikonversi dilewati (galatnya dilaporkan pemeriksa lain). Hasil
// terurut menurut target.
func FindCollisions(sources []string, convert func(string) (string, error)) []Collision {
	byTarget := map[string]map[string]bool{}
	for _, s := range sources {
		t, err := convert(s)
		if err != nil {
			continue
		}
		if byTarget[t] == nil {
			byTarget[t] = map[string]bool{}
		}
		byTarget[t][s] = true
	}
	var out []Collision
	for t, set := range byTarget {
		if len(set) < 2 {
			continue
		}
		c := Collision{Target: t}
		for s := range set {
			c.Sources = append(c.Sources, s)
		}
		sort.Strings(c.Sources)
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Target < out[j].Target })
	return out
}

// Namespace melacak nama SQL yang harus unik dalam satu skema (atau satu tabel, tergantung jenis nama yang
// dimasukkan pemanggil: tabel, indeks, dan constraint unique berbagi ruang nama relasi PostgreSQL per skema;
// foreign key, CHECK, dan pemicu unik per tabel).
type Namespace struct {
	name  string
	owner map[string]string
}

// NewNamespace membuat ruang nama kosong; name hanya untuk pesan galat (mis. "public" atau nama tabel).
func NewNamespace(name string) *Namespace { return &Namespace{name: name, owner: map[string]string{}} }

// Claim mendaftarkan nama untuk origin (deskripsi sumber). Galat: bukan snake_case, > 63 byte, atau sudah
// dipakai (pesan menyebut kedua asal).
func (n *Namespace) Claim(name, origin string) error {
	if len(name) > MaxIdentifierBytes {
		return errf(CodeTooLong, name, "%d byte melebihi batas %d byte (%s)", len(name), MaxIdentifierBytes, origin)
	}
	if !relationRe.MatchString(name) {
		return errf(CodeNotSnakeCase, name, "nama SQL harus snake_case huruf kecil, angka, dan garis bawah (%s)", origin)
	}
	if prev, ok := n.owner[name]; ok {
		return errf(CodeCollision, name, "bentrok di %s: dipakai oleh %s dan %s", n.name, prev, origin)
	}
	n.owner[name] = origin
	return nil
}

// SplitResourceKey memecah kunci resource "<slug>.<nama>" menjadi slug dan nama (camelCase ketat).
func SplitResourceKey(key string) (slug, name string, err error) {
	m := keyRe.FindStringSubmatch(key)
	if m == nil {
		return "", "", errf(CodeInvalidKey, key, "kunci resource harus <slug>.<nama> (slug kebab-case, nama camelCase ketat)")
	}
	if len(m[1]) > MaxSlugBytes {
		return "", "", errf(CodeInvalidSlug, m[1], "slug melebihi %d byte", MaxSlugBytes)
	}
	if err := ValidateIdentifier(m[2]); err != nil {
		return "", "", err
	}
	return m[1], m[2], nil
}

// ManifestResourceKey memetakan kunci resource metadata ke kunci pada manifest dan permission: segmen nama
// dikonversi ke snake_case karena pola capability manifest hanya menerima huruf kecil, angka, dan garis bawah
// pada segmen setelah slug (module.schema.json): sales.invoiceLine menjadi sales.invoice_line.
func ManifestResourceKey(key string) (string, error) {
	slug, name, err := SplitResourceKey(key)
	if err != nil {
		return "", err
	}
	snake, err := ToSnake(name)
	if err != nil {
		return "", err
	}
	return slug + "." + snake, nil
}

// ResourceKeyFromManifest membalik ManifestResourceKey.
func ResourceKeyFromManifest(key string) (string, error) {
	slug, name, ok := strings.Cut(key, ".")
	if !ok || strings.Contains(name, ".") {
		return "", errf(CodeInvalidKey, key, "kunci manifest harus <slug>.<nama_snake>")
	}
	if _, err := snakeSlug(slug); err != nil {
		return "", err
	}
	camel, err := FromSnake(name)
	if err != nil {
		return "", err
	}
	return slug + "." + camel, nil
}

// CapabilityKey menyusun kunci permission <resource manifest>.<aksi> (PRD-001 FR-110); aksi snake_case.
func CapabilityKey(resourceKey, action string) (string, error) {
	mk, err := ManifestResourceKey(resourceKey)
	if err != nil {
		return "", err
	}
	if !relationRe.MatchString(action) || len(action) > MaxIdentifierBytes {
		return "", errf(CodeNotSnakeCase, action, "aksi harus snake_case huruf kecil, angka, dan garis bawah (paling banyak %d byte)", MaxIdentifierBytes)
	}
	return fmt.Sprintf("%s.%s", mk, action), nil
}
