package check

import (
	"strings"
	"testing"
)

func TestValidFieldTypesPass(t *testing.T) {
	rep := single(t, f("m/r.resource.yaml", resource("m.r", `fields:
  s: {type: string, minLength: 1, maxLength: 20, pattern: "^[A-Z]+$", default: ABC}
  em: {type: string, maxLength: 254, format: email, nullable: true}
  t: {type: text, nullable: true}
  i: {type: integer, minimum: 0, maximum: 100, default: 5, aggregatable: true}
  d: {type: decimal, precision: 18, scale: 2, currency: IDR, minimum: "0.00", maximum: "999.99", default: "1.5", aggregatable: true}
  b: {type: boolean, default: false}
  dt: {type: date, default: 2026-11-01}
  ts: {type: datetime, default: "2026-11-01T10:00:00Z", nullable: true}
  e: {type: enum, values: [open, closed], extensible: true, default: open}
  u: {type: uuid, nullable: true, default: null}
`)))
	noErrors(t, rep)
}

func TestFieldTypeConstraintViolations(t *testing.T) {
	tests := []struct {
		name  string
		attrs string
		code  string
	}{
		{"maxLength pada integer", "type: integer, maxLength: 5", CodeFieldAttrNotForType},
		{"precision pada string", "type: string, precision: 5", CodeFieldAttrNotForType},
		{"currency pada integer", "type: integer, currency: IDR", CodeFieldAttrNotForType},
		{"values pada string", "type: string, values: [a]", CodeFieldAttrNotForType},
		{"extensible pada string", "type: string, extensible: true", CodeFieldAttrNotForType},
		{"format pada integer", "type: integer, format: email", CodeFieldAttrNotForType},
		{"format pada text", "type: text, format: url", CodeFieldAttrNotForType},
		{"pattern pada integer", "type: integer, pattern: \"^a$\"", CodeFieldAttrNotForType},
		{"minimum pada string", "type: string, minimum: 1", CodeFieldAttrNotForType},
		{"pattern pada text", "type: text, pattern: \"^a$\"", CodeFieldAttrNotForType},
		{"decimal tanpa precision", "type: decimal, scale: 2", CodeFieldAttrRequired},
		{"decimal tanpa scale", "type: decimal, precision: 10", CodeFieldAttrRequired},
		{"scale melebihi precision", "type: decimal, precision: 5, scale: 6", CodeFieldScalePrecision},
		{"enum tanpa values", "type: enum", CodeFieldAttrRequired},
		{"minLength melebihi maxLength", "type: string, minLength: 10, maxLength: 5", CodeFieldRange},
		{"minimum melebihi maximum integer", "type: integer, minimum: 10, maximum: 5", CodeFieldRange},
		{"minimum melebihi maximum desimal", "type: decimal, precision: 10, scale: 2, minimum: \"5.5\", maximum: \"5.4\"", CodeFieldRange},
		{"pola di luar subset (lookahead)", "type: string, pattern: \"^(?=a)a$\"", CodeFieldPatternInvalid},
		{"pola kuantor bersarang", "type: string, pattern: \"^(a+)+$\"", CodeFieldPatternInvalid},
		{"aggregatable pada string", "type: string, aggregatable: true", CodeFieldAggregatableType},
		{"aggregatable pada boolean", "type: boolean, aggregatable: true", CodeFieldAggregatableType},
		{"default integer bukan bilangan", "type: integer, default: abc", CodeFieldDefaultInvalid},
		{"default integer pecahan", "type: integer, default: 1.5", CodeFieldDefaultInvalid},
		{"default integer di luar rentang", "type: integer, minimum: 0, maximum: 10, default: 11", CodeFieldDefaultInvalid},
		{"default desimal berupa bilangan", "type: decimal, precision: 10, scale: 2, default: 1.5", CodeFieldDefaultInvalid},
		{"default desimal skala berlebih", "type: decimal, precision: 10, scale: 2, default: \"1.234\"", CodeFieldDefaultInvalid},
		{"default desimal melebihi precision", "type: decimal, precision: 4, scale: 2, default: \"123.45\"", CodeFieldDefaultInvalid},
		{"default desimal di luar rentang", "type: decimal, precision: 10, scale: 2, maximum: \"5\", default: \"6\"", CodeFieldDefaultInvalid},
		{"default boolean berupa teks", "type: boolean, default: \"true\"", CodeFieldDefaultInvalid},
		{"default tanggal tidak sah", "type: date, default: \"2026-13-01\"", CodeFieldDefaultInvalid},
		{"default tanggal bukan ISO", "type: date, default: \"01/11/2026\"", CodeFieldDefaultInvalid},
		{"default datetime tanpa Z", "type: datetime, default: \"2026-11-01T10:00:00+07:00\"", CodeFieldDefaultInvalid},
		{"default uuid huruf besar", "type: uuid, default: \"6F9619FF-8B86-D011-B42D-00C04FC964FF\"", CodeFieldDefaultInvalid},
		{"default enum di luar values", "type: enum, values: [a, b], default: c", CodeFieldDefaultInvalid},
		{"default string melebihi maxLength", "type: string, maxLength: 3, default: abcd", CodeFieldDefaultInvalid},
		{"default tidak cocok pola", "type: string, pattern: \"^[0-9]+$\", default: abc", CodeFieldDefaultInvalid},
		{"default null pada non-nullable", "type: string, default: null", CodeFieldDefaultInvalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rep := single(t, oneField(tc.attrs))
			wantErr(t, rep, tc.code, "/fields/x")
		})
	}
}

func TestEnumValueDefaultMustBeCanonical(t *testing.T) {
	// nilai enum sah menurut skema (snake_case); default dikanonisasi sesuai tipenya.
	rep := single(t, oneField("type: enum, values: [pending_approval, done], default: pending_approval"))
	noErrors(t, rep)
}

func TestHighlyConfidentialRejected(t *testing.T) {
	t.Run("langsung", func(t *testing.T) {
		rep := single(t, oneField("type: text, classification: highly_confidential"))
		p := wantErr(t, rep, CodeFieldHighlyConfidential, "/fields/x")
		if !strings.Contains(p.Message, "pengembangan berikutnya") {
			t.Errorf("pesan harus merujuk pengembangan berikutnya: %q", p.Message)
		}
	})
	t.Run("dari tipe domain", func(t *testing.T) {
		rep := single(t,
			f("m/secret.type.yaml", fieldType("m.secretText", "base: {type: text}\nclassification: highly_confidential\n")),
			oneField("type: m.secretText"))
		wantErr(t, rep, CodeFieldHighlyConfidential, "/fields/x")
	})
	t.Run("dari blok", func(t *testing.T) {
		rep := single(t,
			f("m/blk.abstract.yaml", abstract("m.blk", "fields:\n  hidden: {type: text, classification: highly_confidential}\n")),
			f("m/r.resource.yaml", resource("m.r", "include:\n  - {source: m.blk, prefix: pre}\n")))
		wantErr(t, rep, CodeFieldHighlyConfidential, "/fields/preHidden")
	})
	t.Run("accessor", func(t *testing.T) {
		rep := single(t, f("m/r.resource.yaml", resource("m.r", `fields:
  a: {type: string, maxLength: 10, label: m.r.a}
  x: {type: string, maxLength: 10, classification: highly_confidential, accessor: {kind: func, func: calc}, fieldGroup: general}
`)))
		wantErr(t, rep, CodeFieldHighlyConfidential, "/fields/x")
	})
	t.Run("nama sensitif diperlakukan highly_confidential", func(t *testing.T) {
		for _, name := range []string{"salary", "npwp", "nik", "userPassword", "resetToken", "apiKey", "bankAccountNumber", "accessToken", "privateKey", "clientSecret"} {
			rep := single(t, f("m/r.resource.yaml", resource("m.r", "fields:\n  "+name+": {type: string, maxLength: 20}\n")))
			p := wantErr(t, rep, CodeFieldSensitiveName, "/fields/"+name)
			if !strings.Contains(p.Message, "unmanagedColumns") {
				t.Errorf("%s: pesan harus menunjuk unmanagedColumns: %q", name, p.Message)
			}
		}
	})
	t.Run("nama biasa lolos", func(t *testing.T) {
		rep := single(t, f("m/r.resource.yaml", resource("m.r", "fields:\n  tokenless: {type: string, maxLength: 20}\n  salaryGrade: {type: string, maxLength: 3}\n")))
		// salaryGrade memuat token salary: tetap sensitif; tokenless bukan token.
		wantErr(t, rep, CodeFieldSensitiveName, "/fields/salaryGrade")
		for _, p := range rep.Problems {
			if p.Code == CodeFieldSensitiveName && strings.Contains(p.Pointer, "tokenless") {
				t.Errorf("tokenless bukan token sensitif: %+v", p)
			}
		}
	})
}

func TestUnmanagedColumns(t *testing.T) {
	t.Run("sah", func(t *testing.T) {
		rep := single(t, f("m/r.resource.yaml", resource("m.r", `fields:
  a: {type: text}
unmanagedColumns:
  - {name: idNumberEnc, classification: highly_confidential, reason: dikelola kode tulis tangan}
`)))
		noErrors(t, rep)
	})
	t.Run("bentrok dengan field dikelola metadata", func(t *testing.T) {
		rep := single(t, f("m/r.resource.yaml", resource("m.r", `fields:
  a: {type: text}
unmanagedColumns:
  - {name: a, classification: highly_confidential, reason: salah}
`)))
		wantErr(t, rep, CodeUnmanagedColumnManaged, "/unmanagedColumns/0")
	})
	t.Run("nama tidak sah", func(t *testing.T) {
		rep := single(t, f("m/r.resource.yaml", resource("m.r", `fields:
  a: {type: text}
unmanagedColumns:
  - {name: xmin, classification: highly_confidential, reason: kolom sistem}
`)))
		wantErr(t, rep, CodeNameReservedSQL, "/unmanagedColumns/0")
	})
	t.Run("kata kunci SQL sah sebagai nama kolom unmanaged", func(t *testing.T) {
		noErrors(t, single(t, f("m/r.resource.yaml", resource("m.r", `fields:
  a: {type: text}
unmanagedColumns:
  - {name: order, classification: highly_confidential, reason: kolom tulis tangan}
`))))
	})
	t.Run("classification selain highly_confidential ditolak", func(t *testing.T) {
		for _, class := range []string{"confidential", "internal", "public", "secret"} {
			rep := single(t, f("m/r.resource.yaml", resource("m.r", "fields:\n  a: {type: text}\nunmanagedColumns:\n  - {name: secretCol, classification: "+class+", reason: tangan}\n")))
			wantErr(t, rep, CodeUnmanagedColumnClassification, "/unmanagedColumns/0/classification")
		}
	})
	t.Run("reason kosong setelah trim ditolak", func(t *testing.T) {
		rep := single(t, f("m/r.resource.yaml", resource("m.r", "fields:\n  a: {type: text}\nunmanagedColumns:\n  - {name: secretCol, classification: highly_confidential, reason: \"   \"}\n")))
		wantErr(t, rep, CodeUnmanagedColumnReason, "/unmanagedColumns/0/reason")
	})
	t.Run("ganda", func(t *testing.T) {
		rep := single(t, f("m/r.resource.yaml", resource("m.r", `fields:
  a: {type: text}
unmanagedColumns:
  - {name: secretCol, classification: highly_confidential, reason: satu}
  - {name: secretCol, classification: highly_confidential, reason: dua}
`)))
		wantErr(t, rep, CodeUnmanagedColumnDuplicate, "/unmanagedColumns/1")
	})
	t.Run("tidak bersama generate.crud", func(t *testing.T) {
		rep := single(t, f("m/r.resource.yaml", resource("m.r", `fields:
  a: {type: text}
unmanagedColumns:
  - {name: secretCol, classification: highly_confidential, reason: tangan}
generate: {crud: true}
`)))
		wantErr(t, rep, CodeCrudWithUnmanaged, "/generate")
	})
}

func TestDeprecatedAndDescription(t *testing.T) {
	ok := single(t, f("m/r.resource.yaml", resource("m.r", `fields:
  phone: {type: string, maxLength: 32, description: Nomor telepon lama, deprecated: {since: "1.4.0", replacedBy: phoneE164}}
  phoneE164: {type: string, maxLength: 20, migrateFrom: phone, nullable: true}
`)))
	noErrors(t, ok)
	rep := single(t, f("m/r.resource.yaml", resource("m.r", `fields:
  phone: {type: string, maxLength: 32, deprecated: {since: "1.4.0", replacedBy: nothere}}
`)))
	wantErr(t, rep, CodeFieldDeprecatedTarget, "/fields/phone")
}

func TestFieldNamingRules(t *testing.T) {
	t.Run("kata kunci SQL, Go, dan TypeScript lolos (2 Nov 2026)", func(t *testing.T) {
		noErrors(t, single(t, f("m/r.resource.yaml", resource("m.r", `fields:
  order: {type: text}
  type: {type: text}
  class: {type: text}
  user: {type: text}
  group: {type: text}
  limit: {type: text}
  default: {type: text}
  left: {type: text}
  let: {type: text}
`))))
	})
	t.Run("bentrokan nyata per target ditolak", func(t *testing.T) {
		rep := single(t, f("m/r.resource.yaml", resource("m.r", `fields:
  xmin: {type: text}
  present: {type: text}
`)))
		wantErr(t, rep, CodeNameReservedSQL, "/fields/xmin")
		wantErr(t, rep, CodeNameReservedGo, "/fields/present")
	})
	t.Run("tidak bolak-balik", func(t *testing.T) {
		rep := single(t, f("m/r.resource.yaml", resource("m.r", "fields:\n  iD: {type: text}\n")))
		wantErr(t, rep, CodeNameRoundTrip, "/fields/iD")
	})
	t.Run("tabrakan kolom relasi dengan field", func(t *testing.T) {
		rep := single(t,
			f("m/c.resource.yaml", resource("m.customer", "fields:\n  name: {type: text}\n")),
			f("m/r.resource.yaml", resource("m.r", `fields:
  customerId: {type: uuid}
relations:
  customer: {type: belongsTo, to: m.customer}
`)))
		wantErr(t, rep, CodeRelationColumnCollision, "/relations/customer")
	})
	t.Run("nama hasil gabungan lebih dari 63 byte", func(t *testing.T) {
		long := strings.Repeat("a", 40)
		rep := single(t,
			f("m/blk.abstract.yaml", abstract("m.blk", "fields:\n  "+strings.Repeat("b", 30)+": {type: text}\n")),
			f("m/r.resource.yaml", resource("m.r", "include:\n  - {source: m.blk, prefix: "+long+"}\n")))
		wantErr(t, rep, CodePrefixInvalid, "")
	})
	t.Run("nama tabel melebihi 63 byte", func(t *testing.T) {
		name := "r" + strings.Repeat("a", 60) // m_ + nama + s > 63
		rep := single(t, f("m/x.resource.yaml", resource("m."+name, "fields:\n  a: {type: text}\n")))
		wantErr(t, rep, CodeNameTooLong, "")
	})
	t.Run("tabrakan nama tabel akibat jamak", func(t *testing.T) {
		rep := single(t,
			f("m/a.resource.yaml", resource("m.category", "fields:\n  a: {type: text}\n")),
			f("m/b.resource.yaml", resource("m.categorie", "fields:\n  a: {type: text}\n")))
		wantErr(t, rep, CodeNameCollision, "")
	})
}

func TestResolverProblemsAreForwarded(t *testing.T) {
	rep := single(t, f("m/r.resource.yaml", resource("m.r", "include:\n  - source: m.nothing\n")))
	wantErr(t, rep, "include_not_found", "/include/0/source")
}

func TestAbstractFieldsAreCheckedToo(t *testing.T) {
	rep := single(t, f("m/blk.abstract.yaml", abstract("m.blk", "export: true\nfields:\n  a: {type: integer, maxLength: 3}\n")))
	wantErr(t, rep, CodeFieldAttrNotForType, "/fields/a")
}

// Seluruh metadata:check meloloskan dokumen yang memakai kata kunci SQL, Go, dan TypeScript sebagai nama field,
// termasuk pada kolom turunan, indeks, dan lockWhen (keputusan pemilik produk, 2 Nov 2026).
func TestKeywordNamedFieldsPassFullCheck(t *testing.T) {
	rep := single(t, f("m/thing.resource.yaml", resource("m.thing", `include:
  - source: sdk:document
fields:
  type: {type: string, maxLength: 20, default: t, index: true}
  order: {type: integer, minimum: 0, default: 0, sortable: true, index: true}
  user: {type: string, maxLength: 40, nullable: true, unique: true}
  group: {type: enum, values: [a, b], default: a}
  limit: {type: decimal, precision: 18, scale: 2, default: "0.00"}
  default: {type: boolean, default: false}
  left: {type: string, maxLength: 10, nullable: true}
  class: {type: string, maxLength: 10, default: x}
  let: {type: date, nullable: true}
  tag: {type: string, nullable: true, accessor: {kind: stored, expr: {concat: [{field: type}, {lit: "-"}, {field: class}]}}}
lockWhen: {field: class, op: eq, value: sealed}
indexes:
  - {fields: [type, order]}
relations:
  parentThing:
    type: belongsTo
    to: m.thing
`)))
	noErrors(t, rep)
}

// Identifier Go bangkitan yang pasti bentrok ditolak check dengan kode name_reserved_go (aturan sama dengan
// metadata:generate): anggota DTO (validate, present, fillRenamePairs), <Pascal>Spec terhadap tipe runtime
// (resource field, rule, status, resource), dan bentrok antar resource; nama sah lain tetap lolos.
func TestGeneratedGoIdentifierConflicts(t *testing.T) {
	t.Run("field validate, present, fillRenamePairs", func(t *testing.T) {
		rep := single(t, f("m/r.resource.yaml", resource("m.r", "fields:\n  validate: {type: text, nullable: true}\n  present: {type: text, nullable: true}\n  fillRenamePairs: {type: text, nullable: true}\n")))
		for _, n := range []string{"validate", "present", "fillRenamePairs"} {
			wantErr(t, rep, CodeNameReservedGo, "/fields/"+n)
		}
	})
	t.Run("field readOnly bernama validate, present, fillRenamePairs lolos", func(t *testing.T) {
		rep := single(t, f("m/r.resource.yaml", resource("m.r", readOnlyDTOFields)))
		noErrors(t, rep)
	})
	for _, name := range []string{"field", "rule", "status", "resource"} {
		t.Run("resource "+name, func(t *testing.T) {
			rep := single(t, f("m/"+name+".resource.yaml", resource("m."+name, "fields:\n  title: {type: text, nullable: true}\n")))
			p := wantErr(t, rep, CodeNameReservedGo, "/key")
			if !strings.Contains(p.Message, strings.ToUpper(name[:1])+name[1:]+"Spec") {
				t.Errorf("pesan harus menyebut identifier bentrok: %q", p.Message)
			}
		})
	}
	t.Run("bentrok antar resource", func(t *testing.T) {
		rep := run(t, mod(t, "m", nil,
			f("m/invoice.resource.yaml", resource("m.invoice", "fields:\n  status: {type: enum, values: [spec, ok], default: ok}\n")),
			f("m/invoiceStatus.resource.yaml", resource("m.invoiceStatus", "fields:\n  title: {type: text, nullable: true}\n"))))
		wantErr(t, rep, CodeNameReservedGo, "/key")
	})
	t.Run("nama sah lain lolos", func(t *testing.T) {
		rep := run(t, mod(t, "m", nil,
			f("m/fieldGroup.resource.yaml", resource("m.fieldGroup", "fields:\n  validated: {type: text, nullable: true}\n")),
			f("m/ruleSet.resource.yaml", resource("m.ruleSet", "fields:\n  presenter: {type: text, nullable: true}\n")),
			f("m/statusLog.resource.yaml", resource("m.statusLog", "fields:\n  fill: {type: text, nullable: true}\n"))))
		noErrors(t, rep)
	})
}

// readOnlyDTOFields: field baca-saja tidak ada di DTO sehingga boleh bernama seperti anggota DTO.
var readOnlyDTOFields = "fields:\n" +
	"  validate: {type: text, nullable: true, readOnly: true}\n" +
	"  present: {type: text, nullable: true, readOnly: true}\n" +
	"  fillRenamePairs: {type: text, nullable: true, readOnly: true}\n"
