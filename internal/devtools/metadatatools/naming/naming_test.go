package naming

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestWords(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"id", []string{"id"}},
		{"a", []string{"a"}},
		{"customerName", []string{"customer", "name"}},
		{"address2Line", []string{"address2", "line"}},
		{"taxId", []string{"tax", "id"}},
		{"pointX", []string{"point", "x"}},
		{"billingStreet", []string{"billing", "street"}},
		{"a1B2", []string{"a1", "b2"}},
	}
	for _, tc := range tests {
		got, err := Words(tc.in)
		if err != nil {
			t.Fatalf("Words(%q): %v", tc.in, err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("Words(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestToSnake(t *testing.T) {
	tests := map[string]string{
		"id":            "id",
		"customerName":  "customer_name",
		"customerId":    "customer_id",
		"address2Line":  "address2_line",
		"billingStreet": "billing_street",
		"pointX":        "point_x",
		"a1B2":          "a1_b2",
	}
	for in, want := range tests {
		got, err := ToSnake(in)
		if err != nil || got != want {
			t.Errorf("ToSnake(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestToPascalInitialisms(t *testing.T) {
	tests := map[string]string{
		"id":            "ID",
		"customerId":    "CustomerID",
		"customerUrl":   "CustomerURL",
		"customerUuid":  "CustomerUUID",
		"customerName":  "CustomerName",
		"address2Line":  "Address2Line",
		"invoiceLine":   "InvoiceLine",
		"httpServer":    "HTTPServer",
		"htmlUrl":       "HTMLURL",
		"idNumber":      "IDNumber",
		"userIds":       "UserIds",
		"pointX":        "PointX",
		"urlId":         "URLID",
		"externalApiId": "ExternalAPIID",
	}
	for in, want := range tests {
		got, err := ToPascal(in)
		if err != nil || got != want {
			t.Errorf("ToPascal(%q) = %q, %v; want %q", in, got, err, want)
		}
		back, err := FromPascal(got)
		if err != nil || back != in {
			t.Errorf("FromPascal(%q) = %q, %v; want %q", got, back, err, in)
		}
	}
}

func TestToKebab(t *testing.T) {
	tests := map[string]string{
		"invoice":      "invoice",
		"invoiceLine":  "invoice-line",
		"address2Line": "address2-line",
	}
	for in, want := range tests {
		got, err := ToKebab(in)
		if err != nil || got != want {
			t.Errorf("ToKebab(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestPlural(t *testing.T) {
	tests := map[string]string{
		"line":      "lines",
		"note":      "notes",
		"address":   "addresses",
		"category":  "categories",
		"company":   "companies",
		"currency":  "currencies",
		"key":       "keys",
		"day":       "days",
		"box":       "boxes",
		"tax":       "taxes",
		"branch":    "branches",
		"wish":      "wishes",
		"bus":       "buses",
		"status":    "statuses",
		"person":    "people",
		"child":     "children",
		"matrix":    "matrices",
		"data":      "data",
		"metadata":  "metadata",
		"series":    "series",
		"money":     "money",
		"address2":  "address2s",
		"inventory": "inventories",
	}
	for in, want := range tests {
		if got := Plural(in); got != want {
			t.Errorf("Plural(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPluralSnake(t *testing.T) {
	tests := map[string]string{
		"invoiceLine":  "invoice_lines",
		"salesPerson":  "sales_people",
		"note":         "notes",
		"lineCategory": "line_categories",
	}
	for in, want := range tests {
		got, err := PluralSnake(in)
		if err != nil || got != want {
			t.Errorf("PluralSnake(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestTableAndURLNames(t *testing.T) {
	tbl, err := TableName("sales", "invoiceLine")
	if err != nil || tbl != "sales_invoice_lines" {
		t.Fatalf("TableName = %q, %v", tbl, err)
	}
	tbl, err = TableName("example", "note")
	if err != nil || tbl != "example_notes" {
		t.Fatalf("TableName(example, note) = %q, %v", tbl, err)
	}
	tbl, err = TableName("my-mod", "note")
	if err != nil || tbl != "my_mod_notes" {
		t.Fatalf("TableName(my-mod, note) = %q, %v", tbl, err)
	}
	seg, err := URLSegment("invoiceLine")
	if err != nil || seg != "invoice-lines" {
		t.Fatalf("URLSegment = %q, %v", seg, err)
	}
	if _, err := TableName("Sales", "invoice"); err == nil {
		t.Fatal("slug berhuruf kapital harus ditolak")
	}
}

func TestIndexConstraintAndTriggerNames(t *testing.T) {
	tests := []struct {
		got  func() (string, error)
		want string
	}{
		{func() (string, error) { return IndexName("sales_invoice_lines", "customerName") }, "sales_invoice_lines_customer_name_idx"},
		{func() (string, error) { return IndexName("sales_invoice_lines", "customerId", "issuedOn") }, "sales_invoice_lines_customer_id_issued_on_idx"},
		{func() (string, error) { return UniqueName("sales_invoices", "number") }, "sales_invoices_number_key"},
		{func() (string, error) { return ForeignKeyName("sales_invoice_lines", "invoiceId") }, "sales_invoice_lines_invoice_id_fkey"},
		{func() (string, error) { return TriggerName("sales_invoices", "immutable") }, "sales_invoices_immutable_trg"},
	}
	for i, tc := range tests {
		got, err := tc.got()
		if err != nil || got != tc.want {
			t.Errorf("kasus %d: %q, %v; want %q", i, got, err, tc.want)
		}
	}
}

func TestComposedNamesOver63Bytes(t *testing.T) {
	long := strings.Repeat("a", 60) // 60 byte, sah sebagai identifier
	if err := ValidateIdentifier(long); err != nil {
		t.Fatalf("60 byte seharusnya sah: %v", err)
	}
	if _, err := TableName("sales", long); !hasCode(err, CodeTooLong) {
		t.Errorf("TableName berlebih: %v", err)
	}
	if _, err := IndexName("sales_"+long, "customerName"); !hasCode(err, CodeTooLong) {
		t.Errorf("IndexName berlebih: %v", err)
	}
	if _, err := UniqueName("t_"+long, "x"); !hasCode(err, CodeTooLong) {
		t.Errorf("UniqueName berlebih: %v", err)
	}
	if _, err := ForeignKeyName("t_"+long, "x"); !hasCode(err, CodeTooLong) {
		t.Errorf("ForeignKeyName berlebih: %v", err)
	}
	if _, err := TriggerName("t_"+long, "x"); !hasCode(err, CodeTooLong) {
		t.Errorf("TriggerName berlebih: %v", err)
	}
	if _, err := Prefixed(strings.Repeat("a", 40), strings.Repeat("b", 30)); !hasCode(err, CodeTooLong) {
		t.Errorf("Prefixed berlebih: %v", err)
	}
	// kolom berawalan blok: panjang dihitung pada identifier camelCase maupun snake_case (garis bawah menambah byte).
	camelFits := strings.Repeat("a", 30) + "B" + strings.Repeat("c", 30) // 61 byte camel
	if _, err := ToSnake(camelFits); err != nil {
		t.Fatalf("snake 62 byte seharusnya sah: %v", err)
	}
	snakeOver := strings.Repeat("a", 21) + "B" + strings.Repeat("a", 20) + "C" + strings.Repeat("a", 19) // 62 byte camel -> 64 byte snake
	if _, err := ToSnake(snakeOver); !hasCode(err, CodeTooLong) {
		t.Errorf("snake > 63 byte harus ditolak: %v", err)
	}
	// tepat 63 byte lolos.
	exact := strings.Repeat("x", 63)
	if err := ValidateIdentifier(exact); err != nil {
		t.Errorf("63 byte lolos: %v", err)
	}
	if err := ValidateIdentifier(exact + "x"); !hasCode(err, CodeTooLong) {
		t.Errorf("64 byte ditolak: %v", err)
	}
}

func TestPrefixed(t *testing.T) {
	tests := []struct{ prefix, name, want string }{
		{"billing", "street", "billingStreet"},
		{"shipping", "postalCode", "shippingPostalCode"},
		{"billingAddr", "city", "billingAddrCity"},
		{"a", "b", "aB"},
	}
	for _, tc := range tests {
		got, err := Prefixed(tc.prefix, tc.name)
		if err != nil || got != tc.want {
			t.Errorf("Prefixed(%q,%q) = %q, %v; want %q", tc.prefix, tc.name, got, err, tc.want)
		}
	}
	if _, err := Prefixed("Billing", "street"); err == nil {
		t.Error("prefix huruf kapital harus ditolak")
	}
	col, err := ToSnake("billingStreet")
	if err != nil || col != "billing_street" {
		t.Errorf("kolom berawalan = %q, %v", col, err)
	}
}

func TestValidateIdentifierViolations(t *testing.T) {
	tests := []struct {
		name string
		in   string
		code Code
	}{
		{"kosong", "", CodeNotCamelCase},
		{"awalan kapital", "Customer", CodeNotCamelCase},
		{"snake", "customer_name", CodeNotCamelCase},
		{"kebab", "customer-name", CodeNotCamelCase},
		{"spasi", "customer name", CodeNotCamelCase},
		{"awalan angka", "1abc", CodeNotCamelCase},
		{"non ascii", "customérName", CodeNotCamelCase},
		{"akronim taxID", "taxID", CodeConsecutiveCapitals},
		{"kapital berurutan aBC", "aBC", CodeConsecutiveCapitals},
		{"kapital berurutan di tengah", "orderIDNumber", CodeConsecutiveCapitals},
		{"terlalu panjang", strings.Repeat("a", 64), CodeTooLong},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateIdentifier(tc.in)
			if !hasCode(err, tc.code) {
				t.Fatalf("ValidateIdentifier(%q) = %v; ingin kode %q", tc.in, err, tc.code)
			}
		})
	}
	for _, ok := range []string{"a", "id", "customerName", "address2Line", "pointX", "taxId", "a1B2", strings.Repeat("a", 63)} {
		if err := ValidateIdentifier(ok); err != nil {
			t.Errorf("ValidateIdentifier(%q) = %v; seharusnya sah", ok, err)
		}
	}
}

func TestIdentifierPatternMatchesValidator(t *testing.T) {
	for _, s := range []string{"taxID", "aBC", "customerName", "pointX", "Customer", "a_b", "x", "orderIDNumber"} {
		pattern := IdentifierRegexp().MatchString(s)
		validator := ValidateIdentifier(s) == nil
		if pattern != validator {
			t.Errorf("pola dan validator berbeda untuk %q: pola=%v validator=%v", s, pattern, validator)
		}
	}
}

// TestKeywordsAreValidNames: kata kunci SQL, Go, dan TypeScript sah sebagai nama field (keputusan pemilik produk,
// 2 Nov 2026); hanya bentrokan nyata per target yang ditolak.
func TestKeywordsAreValidNames(t *testing.T) {
	for _, name := range []string{"type", "order", "user", "group", "limit", "default", "left", "class", "let",
		"select", "func", "range", "enum", "delete", "currentDate", "customerName", "userName", "groupName"} {
		if got := ReservedTargets(name); len(got) != 0 {
			t.Errorf("ReservedTargets(%q) = %v; kata kunci harus lolos", name, got)
		}
		if vs := Validate(name); len(vs) != 0 {
			t.Errorf("Validate(%q) = %v", name, vs)
		}
		if err := CheckReserved(name); err != nil {
			t.Errorf("CheckReserved(%q) = %v", name, err)
		}
	}
}

// TestRealConflictsPerTarget: bentrokan nyata tetap ditolak per target.
func TestRealConflictsPerTarget(t *testing.T) {
	tests := []struct {
		in   string
		want []Target
		code Code
	}{
		{"xmin", []Target{TargetSQL}, CodeReservedSQL},
		{"ctid", []Target{TargetSQL}, CodeReservedSQL},
		{"tableoid", []Target{TargetSQL}, CodeReservedSQL},
	}
	for _, tc := range tests {
		got := ReservedTargets(tc.in)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ReservedTargets(%q) = %v, want %v", tc.in, got, tc.want)
		}
		if err := CheckReserved(tc.in); !hasCode(err, tc.code) {
			t.Errorf("CheckReserved(%q) = %v; ingin kode %q", tc.in, err, tc.code)
		}
		if vs := Validate(tc.in); len(vs) == 0 || vs[0].Code != tc.code {
			t.Errorf("Validate(%q) = %v", tc.in, vs)
		}
	}
}

func TestRoundTripDetection(t *testing.T) {
	// Aman: bolak-balik identik di semua target.
	for _, ok := range []string{"customerName", "customerId", "address2Line", "invoiceLine", "pointX", "idNumber", "httpServer", "urlId"} {
		if err := CheckRoundTrip(ok); err != nil {
			t.Errorf("CheckRoundTrip(%q) = %v; seharusnya lolos", ok, err)
		}
	}
	// Tidak identik: pemisahan kata ke PascalCase dengan inisialisme tidak dapat dibalik.
	for _, bad := range []string{"iD", "uI", "iP"} {
		if err := CheckRoundTrip(bad); !hasCode(err, CodeRoundTrip) {
			t.Errorf("CheckRoundTrip(%q) = %v; seharusnya CodeRoundTrip", bad, err)
		}
	}
}

func TestValidateAggregatesAllViolations(t *testing.T) {
	if vs := Validate("customerName"); len(vs) != 0 {
		t.Errorf("Validate(customerName) = %v", vs)
	}
	vs := Validate("taxID")
	if len(vs) == 0 || vs[0].Code != CodeConsecutiveCapitals {
		t.Fatalf("Validate(taxID) = %v", vs)
	}
	// Req 1.4: akronim ditulis taxId; pesan galat menyarankannya.
	if !strings.Contains(vs[0].Error(), "taxId") {
		t.Errorf("pesan harus menyarankan taxId: %q", vs[0].Error())
	}
	if err := ValidateIdentifier("taxId"); err != nil {
		t.Errorf("taxId harus sah: %v", err)
	}
}

func TestCollisions(t *testing.T) {
	// Tabrakan akibat jamak: "category" dan "categorie" sama-sama menjadi "..categories".
	got := FindCollisions([]string{"category", "categorie", "note"}, PluralSnake)
	if len(got) != 1 || got[0].Target != "categories" || !reflect.DeepEqual(got[0].Sources, []string{"categorie", "category"}) {
		t.Fatalf("FindCollisions = %+v", got)
	}
	// Tidak ada tabrakan.
	if got := FindCollisions([]string{"customerName", "customerId", "invoiceLine"}, ToSnake); len(got) != 0 {
		t.Fatalf("seharusnya tanpa tabrakan: %+v", got)
	}
	// Tabrakan PascalCase: "iD" tidak mungkin, tetapi dua sumber berbeda bertemu pada hasil yang sama.
	fake := func(s string) (string, error) { return strings.ToLower(s), nil }
	got = FindCollisions([]string{"a", "A"}, fake)
	if len(got) != 1 {
		t.Fatalf("FindCollisions fake = %+v", got)
	}
}

func TestNamespaceUniqueness(t *testing.T) {
	ns := NewNamespace("public")
	if err := ns.Claim("sales_invoices", "tabel sales.invoice"); err != nil {
		t.Fatal(err)
	}
	if err := ns.Claim("sales_invoices_number_key", "unique number"); err != nil {
		t.Fatal(err)
	}
	err := ns.Claim("sales_invoices", "tabel lain")
	if !hasCode(err, CodeCollision) {
		t.Fatalf("klaim ganda = %v", err)
	}
	if !strings.Contains(err.Error(), "tabel sales.invoice") || !strings.Contains(err.Error(), "tabel lain") {
		t.Errorf("pesan harus menyebut kedua asal: %v", err)
	}
	if err := ns.Claim(strings.Repeat("a", 64), "terlalu panjang"); !hasCode(err, CodeTooLong) {
		t.Errorf("klaim > 63 byte = %v", err)
	}
	if err := ns.Claim("Bukan_Snake", "x"); err == nil {
		t.Error("nama bukan snake_case harus ditolak")
	}
}

func TestManifestResourceKey(t *testing.T) {
	tests := map[string]string{
		"sales.invoice":     "sales.invoice",
		"sales.invoiceLine": "sales.invoice_line",
		"my-mod.noteItem":   "my-mod.note_item",
	}
	for in, want := range tests {
		got, err := ManifestResourceKey(in)
		if err != nil || got != want {
			t.Errorf("ManifestResourceKey(%q) = %q, %v; want %q", in, got, err, want)
		}
		back, err := ResourceKeyFromManifest(got)
		if err != nil || back != in {
			t.Errorf("ResourceKeyFromManifest(%q) = %q, %v; want %q", got, back, err, in)
		}
	}
	for _, bad := range []string{"", "sales", "Sales.invoice", "sales.Invoice", "sales.invoice_line", "sales.taxID", "sales.invoice.line"} {
		if _, err := ManifestResourceKey(bad); err == nil {
			t.Errorf("ManifestResourceKey(%q) seharusnya galat", bad)
		}
	}
	perm, err := CapabilityKey("sales.invoiceLine", "read")
	if err != nil || perm != "sales.invoice_line.read" {
		t.Errorf("CapabilityKey = %q, %v", perm, err)
	}
	if _, err := CapabilityKey("sales.invoice", "Read"); err == nil {
		t.Error("aksi huruf kapital harus ditolak")
	}
	slug, name, err := SplitResourceKey("my-mod.noteItem")
	if err != nil || slug != "my-mod" || name != "noteItem" {
		t.Errorf("SplitResourceKey = %q %q %v", slug, name, err)
	}
}

func hasCode(err error, code Code) bool {
	var ne *Error
	if !errors.As(err, &ne) {
		return false
	}
	return ne.Code == code
}

// TestGoDTOMembersAreNotNameLevelReserved: bentrok anggota DTO bergantung pada kemampuan tulis field, jadi tidak
// ditolak pada tingkat nama (hanya CheckGoPackage, untuk field tulis).
func TestGoDTOMembersAreNotNameLevelReserved(t *testing.T) {
	for _, name := range []string{"present", "validate", "fillRenamePairs"} {
		if got := ReservedTargets(name); len(got) != 0 {
			t.Errorf("ReservedTargets(%q) = %v; ingin kosong", name, got)
		}
	}
}

func TestCheckGoPackage(t *testing.T) {
	res := func(key, name string, fields ...GoField) GoResource {
		return GoResource{Key: key, Name: name, Fields: fields}
	}
	f := func(name string, enum ...string) GoField { return GoField{Name: name, Enum: enum, Writable: true} }
	ro := func(name string) GoField { return GoField{Name: name} }
	codes := func(cs []GoConflict) string {
		var out []string
		for _, c := range cs {
			if c.Err.Code != CodeReservedGo {
				t.Errorf("kode = %q, ingin %q", c.Err.Code, CodeReservedGo)
			}
			out = append(out, c.Resource+"/"+c.Field)
		}
		return strings.Join(out, ",")
	}
	if got := CheckGoPackage([]GoResource{res("m.invoice", "invoice", f("total"), f("status", "draft", "done"))}); len(got) != 0 {
		t.Errorf("resource sah: %v", got)
	}
	// (a) field tulis bernama validate/present/fillRenamePairs
	if got := codes(CheckGoPackage([]GoResource{res("m.a", "a", f("validate"), f("present"), f("fillRenamePairs"))})); got != "m.a/validate,m.a/present,m.a/fillRenamePairs" {
		t.Errorf("anggota DTO: %q", got)
	}
	// field baca-saja tidak ada di DTO sehingga bebas memakai nama itu
	if got := CheckGoPackage([]GoResource{res("m.a", "a", ro("validate"), ro("present"), ro("fillRenamePairs"))}); len(got) != 0 {
		t.Errorf("field readOnly tidak boleh dilaporkan: %v", got)
	}
	// (b) resource field, rule, status, resource bentrok dengan tipe runtime (<P>Spec)
	for _, name := range []string{"field", "rule", "status", "resource"} {
		got := CheckGoPackage([]GoResource{res("m."+name, name)})
		if len(got) == 0 || !strings.Contains(got[0].Err.Detail, "runtime") {
			t.Errorf("resource %s harus bentrok dengan runtime: %v", name, got)
		}
	}
	// konstanta runtime
	if got := CheckGoPackage([]GoResource{res("m.code", "code", f("required", "x"))}); len(got) != 0 {
		t.Errorf("CodeRequired tidak dihasilkan resource code: %v", got)
	}
	// bentrok antar resource: invoiceStatus.Spec vs invoice.status enum "spec"
	got := CheckGoPackage([]GoResource{res("m.invoice", "invoice", f("status", "spec")), res("m.invoiceStatus", "invoiceStatus")})
	if len(got) == 0 || !strings.Contains(got[0].Err.Detail, "InvoiceStatusSpec") {
		t.Errorf("bentrok antar resource: %v", got)
	}
	// nama sah lain tetap lolos
	if got := CheckGoPackage([]GoResource{res("m.fieldGroup", "fieldGroup"), res("m.ruleSet", "ruleSet"), res("m.statusLog", "statusLog")}); len(got) != 0 {
		t.Errorf("nama sah lain: %v", got)
	}
	if PascalEnumValue("net_15") != "Net15" || PascalEnumValue("Bad") != "" {
		t.Error("PascalEnumValue")
	}
}

// TestResourceKeyMappingRule mengunci aturan pemetaan satu kunci resource ke semua bentuk turunannya (K35,
// 2 Nov 2026): kunci selalu tunggal; identitas (kunci manifest dan prefiks permission) tunggal snake_case,
// sedangkan tabel dan segmen URL jamak. Capability tulis-tangan jamak peninggalan modul example
// (example.notes.read, sebelum metadata) sudah diganti example.note.read di PRD-001 C2.1 (Q36, 4 Nov 2026) dan
// bukan bagian aturan ini.
func TestResourceKeyMappingRule(t *testing.T) {
	cases := []struct {
		key, manifest, table, url string
	}{
		{"example.note", "example.note", "example_notes", "notes"},
		{"sales.invoiceLine", "sales.invoice_line", "sales_invoice_lines", "invoice-lines"},
		{"inventory.category", "inventory.category", "inventory_categories", "categories"},
		{"core.status", "core.status", "core_statuses", "statuses"},
	}
	for _, c := range cases {
		t.Run(c.key, func(t *testing.T) {
			slug, name, err := SplitResourceKey(c.key)
			if err != nil {
				t.Fatal(err)
			}
			if got, _ := ManifestResourceKey(c.key); got != c.manifest {
				t.Errorf("kunci manifest = %q, want %q", got, c.manifest)
			}
			if got, _ := TableName(slug, name); got != c.table {
				t.Errorf("tabel = %q, want %q", got, c.table)
			}
			if got, _ := URLSegment(name); got != c.url {
				t.Errorf("url = %q, want %q", got, c.url)
			}
			if back, _ := ResourceKeyFromManifest(c.manifest); back != c.key {
				t.Errorf("balik dari manifest = %q, want %q", back, c.key)
			}
		})
	}
}
