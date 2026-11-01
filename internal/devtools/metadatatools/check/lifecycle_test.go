package check

import "testing"

func TestMigrateFromPairs(t *testing.T) {
	mk := func(phone, e164 string) *Report {
		return single(t, f("m/r.resource.yaml", resource("m.r", "fields:\n  phone: {"+phone+"}\n  phoneE164: {"+e164+"}\n")))
	}
	base := "type: string, maxLength: 32, deprecated: {since: \"1.4.0\", replacedBy: phoneE164}"
	ok := mk(base+", fieldGroup: contact, classification: confidential", "type: string, maxLength: 20, migrateFrom: phone, nullable: true, fieldGroup: contact, classification: confidential")
	noErrors(t, ok)
	noErrors(t, mk(base, "type: string, maxLength: 20, migrateFrom: phone, nullable: true"))
	wantErr(t, mk(base+", fieldGroup: contact", "type: string, maxLength: 20, migrateFrom: phone, fieldGroup: other"), CodeMigrateFromGroup, "/fields/phoneE164")
	wantErr(t, mk(base+", classification: confidential", "type: string, maxLength: 20, migrateFrom: phone"), CodeMigrateFromClassification, "/fields/phoneE164")
	wantErr(t, mk(base, "type: string, maxLength: 20, migrateFrom: phone, classification: confidential"), CodeMigrateFromClassification, "/fields/phoneE164")
	wantErr(t, mk(base, "type: string, maxLength: 20, migrateFrom: ghost"), CodeMigrateFromUnknown, "/fields/phoneE164")
	wantErr(t, mk(base, "type: string, maxLength: 20, migrateFrom: phoneE164"), CodeMigrateFromUnknown, "/fields/phoneE164")
}

const sourceCustomer = `fields:
  code: {type: string, maxLength: 20, snapshotSource: true, fieldGroup: general, classification: internal}
  name: {type: string, maxLength: 100, snapshotSource: true, fieldGroup: general, classification: internal}
  creditLimit: {type: decimal, precision: 18, scale: 2, snapshotSource: true, fieldGroup: finance, classification: confidential}
  notes: {type: text, nullable: true}
`

func crmSrc(t *testing.T) Module {
	return mod(t, "crm", nil, f("crm/customer.resource.yaml", resource("crm.customer", "export: true\n"+sourceCustomer)))
}

func salesInvoice(snapshot, fields string) file {
	return f("sales/invoice.resource.yaml", resource("sales.invoice", "fields:\n"+fields+"relations:\n  customer:\n    type: belongsTo\n    to: crm.customer\n    snapshot: "+snapshot+"\n"))
}

func TestSnapshotDeclarations(t *testing.T) {
	cust := "  customerName: {type: string, maxLength: 100, readOnly: true, fieldGroup: general, classification: internal}\n"
	rep := run(t, crmSrc(t), mod(t, "sales", []string{"acme.crm"}, salesInvoice("[{from: name, to: customerName}]", cust)))
	noErrors(t, rep)

	tests := []struct {
		name, snapshot, fields, code, ptr string
	}{
		{"field tujuan tidak ada", "[{from: name, to: ghost}]", cust, CodeSnapshotToUnknown, "/relations/customer/snapshot/0"},
		{"field tujuan tidak readOnly", "[{from: name, to: customerName}]", "  customerName: {type: string, maxLength: 100, fieldGroup: general, classification: internal}\n", CodeSnapshotToNotReadOnly, "/fields/customerName"},
		{"tanpa fieldGroup eksplisit", "[{from: name, to: customerName}]", "  customerName: {type: string, maxLength: 100, readOnly: true, classification: internal}\n", CodeSnapshotToExplicit, "/fields/customerName"},
		{"tanpa classification eksplisit", "[{from: name, to: customerName}]", "  customerName: {type: string, maxLength: 100, readOnly: true, fieldGroup: general}\n", CodeSnapshotToExplicit, "/fields/customerName"},
		{"field sumber tidak ada", "[{from: ghost, to: customerName}]", cust, CodeSnapshotFromUnknown, "/relations/customer/snapshot/0"},
		{"sumber lintas modul bukan snapshotSource", "[{from: notes, to: customerName}]", cust, CodeSnapshotSourceFlag, "/relations/customer/snapshot/0"},
		{"klasifikasi tujuan lebih longgar", "[{from: creditLimit, to: limitCopy}]", "  limitCopy: {type: decimal, precision: 18, scale: 2, readOnly: true, fieldGroup: finance, classification: internal}\n", CodeSnapshotLooser, "/fields/limitCopy"},
		{"grup non-default sumber ke grup default tujuan", "[{from: creditLimit, to: limitCopy}]", "  limitCopy: {type: decimal, precision: 18, scale: 2, readOnly: true, fieldGroup: default, classification: confidential}\n", CodeSnapshotGroupDowngrade, "/fields/limitCopy"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rep := run(t, crmSrc(t), mod(t, "sales", []string{"acme.crm"}, salesInvoice(tc.snapshot, tc.fields)))
			wantErr(t, rep, tc.code, tc.ptr)
		})
	}
	t.Run("hanya belongsTo", func(t *testing.T) {
		rep := run(t, crmSrc(t), mod(t, "sales", []string{"acme.crm"}, f("sales/invoice.resource.yaml", resource("sales.invoice", `fields:
  customerName: {type: string, maxLength: 100, readOnly: true, fieldGroup: general, classification: internal}
relations:
  customers: {type: belongsToMany, to: crm.customer, snapshot: [{from: name, to: customerName}]}
`))))
		wantErr(t, rep, CodeSnapshotRelationType, "/relations/customers/snapshot")
	})
	t.Run("sumber satu modul tidak wajib snapshotSource", func(t *testing.T) {
		rep := single(t,
			f("m/c.resource.yaml", resource("m.customer", "fields:\n  name: {type: string, maxLength: 100}\n")),
			f("m/r.resource.yaml", resource("m.r", `fields:
  customerName: {type: string, maxLength: 100, readOnly: true, fieldGroup: general, classification: internal}
relations:
  customer: {type: belongsTo, to: m.customer, snapshot: [{from: name, to: customerName}]}
`)))
		noErrors(t, rep)
	})
	t.Run("sumber lintas modul cukup snapshotSource; grup dan klasifikasi efektif yang dipakai", func(t *testing.T) {
		crm := mod(t, "crm", nil, f("crm/customer.resource.yaml", resource("crm.customer", "export: true\nfields:\n  name: {type: string, maxLength: 100, snapshotSource: true}\n")))
		rep := run(t, crm, mod(t, "sales", []string{"acme.crm"}, salesInvoice("[{from: name, to: customerName}]", cust)))
		noErrors(t, rep)
	})
}

func TestSnapshotFieldsAreNotFilterableUnlessDeclared(t *testing.T) {
	// 14.6: kolom salinan hanya filterable/sortable bila dinyatakan eksplisit pada field lokalnya (tidak diwarisi).
	cust := "  customerName: {type: string, maxLength: 100, readOnly: true, fieldGroup: general, classification: internal}\n"
	rep := run(t, crmSrc(t), mod(t, "sales", []string{"acme.crm"}, salesInvoice("[{from: name, to: customerName}]", cust)))
	noErrors(t, rep)
}

func TestLockWhenMustTranslateToSQLForDocuments(t *testing.T) {
	doc := func(lock string) *Report {
		return single(t, f("m/d.resource.yaml", resource("m.d", `include:
  - source: sdk:document
fields:
  paid: {type: boolean, default: false}
  total: {type: decimal, precision: 18, scale: 2}
  qty: {type: integer}
  calc: {type: integer, accessor: {kind: expr, expr: {add: [{field: qty}, {lit: 1}], scale: 0, round: half_up}}}
  stored: {type: integer, accessor: {kind: stored, expr: {add: [{field: qty}, {lit: 1}], scale: 0, round: half_up}}}
lockWhen: `+lock+"\n")))
	}
	noErrors(t, doc("{field: paid, op: eq, value: true}"))
	noErrors(t, doc("{any: [{field: paid, op: ne, value: false}, {field: qty, op: in, value: [1, 2]}, {not: {field: qty, op: isnull, value: true}}]}"))
	noErrors(t, doc("{field: stored, op: eq, value: 3}"))
	wantErr(t, doc("{field: total, op: gt, value: \"10\"}"), CodeLockWhenNotSQL, "/lockWhen")
	wantErr(t, doc("{field: qty, op: contains, value: \"1\"}"), CodeLockWhenNotSQL, "/lockWhen")
	wantErr(t, doc("{field: calc, op: eq, value: 3}"), CodeLockWhenNotSQL, "/lockWhen")
	wantErr(t, doc("{field: qty, op: lt, ref: total}"), CodeLockWhenNotSQL, "/lockWhen")
	t.Run("resource non-dokumen tidak dibatasi ke SQL", func(t *testing.T) {
		rep := single(t, f("m/r.resource.yaml", resource("m.r", "fields:\n  total: {type: decimal, precision: 18, scale: 2}\nlockWhen: {field: total, op: gt, value: \"10\"}\n")))
		noErrors(t, rep)
	})
}

func TestGenerateCrudNotWithUnmanagedColumns(t *testing.T) {
	rep := single(t, f("m/r.resource.yaml", resource("m.r", "fields:\n  a: {type: text}\nunmanagedColumns:\n  - {name: secretCol, reason: tangan}\ngenerate: {crud: true}\n")))
	wantErr(t, rep, CodeCrudWithUnmanaged, "/generate")
}
