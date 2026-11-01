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

func TestReservedWords(t *testing.T) {
	tests := []struct {
		in   string
		want []Target
	}{
		{"customerName", nil},
		{"select", []Target{TargetSQL, TargetGo}},
		{"user", []Target{TargetSQL}},
		{"order", []Target{TargetSQL}},
		{"group", []Target{TargetSQL}},
		{"limit", []Target{TargetSQL}},
		{"default", []Target{TargetSQL, TargetGo, TargetTS}},
		{"type", []Target{TargetGo}},
		{"func", []Target{TargetGo}},
		{"range", []Target{TargetGo}},
		{"class", []Target{TargetTS}},
		{"let", []Target{TargetTS}},
		{"enum", []Target{TargetTS}},
		{"delete", []Target{TargetTS}},
		{"currentDate", []Target{TargetSQL}},
		{"left", []Target{TargetSQL}},
		{"userName", nil},
		{"groupName", nil},
	}
	for _, tc := range tests {
		got := ReservedTargets(tc.in)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ReservedTargets(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
	if err := CheckReserved("user"); !hasCode(err, CodeReservedSQL) {
		t.Errorf("CheckReserved(user) = %v", err)
	}
	if err := CheckReserved("type"); !hasCode(err, CodeReservedGo) {
		t.Errorf("CheckReserved(type) = %v", err)
	}
	if err := CheckReserved("class"); !hasCode(err, CodeReservedTS) {
		t.Errorf("CheckReserved(class) = %v", err)
	}
	if err := CheckReserved("customerName"); err != nil {
		t.Errorf("CheckReserved(customerName) = %v", err)
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
	vs := Validate("default")
	codes := map[Code]bool{}
	for _, v := range vs {
		codes[v.Code] = true
	}
	for _, c := range []Code{CodeReservedSQL, CodeReservedGo, CodeReservedTS} {
		if !codes[c] {
			t.Errorf("Validate(default) tidak memuat %q: %v", c, vs)
		}
	}
	if vs := Validate("customerName"); len(vs) != 0 {
		t.Errorf("Validate(customerName) = %v", vs)
	}
	vs = Validate("taxID")
	if len(vs) == 0 || vs[0].Code != CodeConsecutiveCapitals {
		t.Errorf("Validate(taxID) = %v", vs)
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
