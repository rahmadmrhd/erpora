package dbmodel

import (
	"strings"
	"testing"
)

// Pemetaan tipe tertulis dan teruji (Requirement 10.4; design Komponen 7).
func TestDerive_TypeMapping(t *testing.T) {
	s := deriveOne(t, f("m/thing.resource.yaml", resourceDoc("m.thing", `
fields:
  title:    {type: string, minLength: 1, maxLength: 200, label: m.thing.title}
  code:     {type: string, maxLength: 20, nullable: true, label: m.thing.code}
  free:     {type: string, label: m.thing.free}
  notes:    {type: text, nullable: true, label: m.thing.notes}
  qty:      {type: integer, label: m.thing.qty}
  rank:     {type: integer, minimum: 1, maximum: 100, label: m.thing.rank}
  big:      {type: integer, minimum: 0, label: m.thing.big}
  price:    {type: decimal, precision: 18, scale: 2, minimum: "0", label: m.thing.price}
  flag:     {type: boolean, default: true, label: m.thing.flag}
  born:     {type: date, nullable: true, label: m.thing.born}
  seenAt:   {type: datetime, label: m.thing.seenAt}
  ref:      {type: uuid, nullable: true, label: m.thing.ref}
  kind:     {type: enum, values: [alpha, beta_two], default: alpha, label: m.thing.kind}
`)))
	tb := table(t, s, "m_things")
	if tb.Resource != "m.thing" || tb.Module != "m" {
		t.Errorf("resource/module = %q/%q", tb.Resource, tb.Module)
	}
	type want struct {
		typ     string
		notNull bool
	}
	cases := map[string]want{
		"id":         {"uuid", true},
		"version":    {"bigint", true},
		"created_at": {"timestamp with time zone", true},
		"created_by": {"uuid", true},
		"title":      {"text", true},
		"code":       {"text", false},
		"free":       {"text", true},
		"notes":      {"text", false},
		"qty":        {"bigint", true},
		"rank":       {"integer", true},
		"big":        {"bigint", true},
		"price":      {"numeric(18,2)", true},
		"flag":       {"boolean", true},
		"born":       {"date", false},
		"seen_at":    {"timestamp with time zone", true},
		"ref":        {"uuid", false},
		"kind":       {"text", true},
	}
	for name, w := range cases {
		c := col(t, tb, name)
		if c.Type.String() != w.typ || c.NotNull != w.notNull {
			t.Errorf("%s = %s notNull=%v, want %s notNull=%v", name, c.Type, c.NotNull, w.typ, w.notNull)
		}
	}
	if col(t, tb, "title").Field != "title" || col(t, tb, "seen_at").Field != "seenAt" {
		t.Errorf("Field harus menyimpan nama metadata")
	}
	// Kolom id adalah kunci utama uuid; tanpa tenant_id (ADR-0004).
	if len(tb.PrimaryKey) != 1 || tb.PrimaryKey[0] != "id" {
		t.Errorf("PrimaryKey = %v", tb.PrimaryKey)
	}
	if tb.Column("tenant_id") != nil {
		t.Error("tabel tidak boleh punya tenant_id")
	}
}

func TestDerive_Checks(t *testing.T) {
	s := deriveOne(t, f("m/thing.resource.yaml", resourceDoc("m.thing", `
fields:
  title:    {type: string, minLength: 1, maxLength: 200, label: m.thing.title}
  free:     {type: string, label: m.thing.free}
  short:    {type: text, maxLength: 10, label: m.thing.short}
  rank:     {type: integer, minimum: 1, maximum: 100, label: m.thing.rank}
  price:    {type: decimal, precision: 18, scale: 2, minimum: "0.00", maximum: "99999.50", label: m.thing.price}
  plain:    {type: decimal, precision: 5, scale: 1, label: m.thing.plain}
  kind:     {type: enum, values: [alpha, beta_two], label: m.thing.kind}
`)))
	tb := table(t, s, "m_things")
	byCol := map[string]*Check{}
	for _, c := range tb.Checks {
		if _, dup := byCol[c.Column]; dup {
			t.Errorf("lebih dari satu CHECK untuk kolom %s", c.Column)
		}
		byCol[c.Column] = c
	}
	chk := func(column string, kind CheckKind) *Check {
		t.Helper()
		c := byCol[column]
		if c == nil || c.Kind != kind {
			t.Fatalf("CHECK %s = %+v, want kind %s", column, c, kind)
		}
		if c.Name != tb.Name+"_"+column+"_check" {
			t.Errorf("nama CHECK %s = %q", column, c.Name)
		}
		return c
	}
	if c := chk("title", CheckLength); c.Min != "1" || c.Max != "200" {
		t.Errorf("title = %+v", c)
	}
	if c := chk("short", CheckLength); c.Min != "" || c.Max != "10" {
		t.Errorf("short = %+v", c)
	}
	if c := chk("rank", CheckRange); c.Min != "1" || c.Max != "100" {
		t.Errorf("rank = %+v", c)
	}
	if c := chk("price", CheckRange); c.Min != "0.00" || c.Max != "99999.50" {
		t.Errorf("price = %+v", c)
	}
	if c := chk("kind", CheckEnum); strings.Join(c.Values, ",") != "alpha,beta_two" {
		t.Errorf("kind = %+v", c)
	}
	// version minimum 1 (sdk:audit) menghasilkan CHECK rentang.
	if c := chk("version", CheckRange); c.Min != "1" || c.Max != "" {
		t.Errorf("version = %+v", c)
	}
	for _, none := range []string{"free", "plain", "id", "created_at"} {
		if byCol[none] != nil {
			t.Errorf("kolom %s tidak boleh punya CHECK: %+v", none, byCol[none])
		}
	}
}

func TestDerive_Defaults(t *testing.T) {
	s := deriveOne(t, f("m/thing.resource.yaml", resourceDoc("m.thing", `
fields:
  flag:   {type: boolean, default: true, label: m.thing.flag}
  kind:   {type: enum, values: [alpha, beta], default: beta, label: m.thing.kind}
  tax:    {type: decimal, precision: 5, scale: 2, default: "11", label: m.thing.tax}
  n:      {type: integer, default: 3, label: m.thing.n}
  name:   {type: string, maxLength: 20, default: "it's", label: m.thing.name}
  none:   {type: string, maxLength: 20, nullable: true, label: m.thing.none}
`)))
	tb := table(t, s, "m_things")
	d := func(name string) *Default {
		t.Helper()
		x := col(t, tb, name).Default
		if x == nil {
			t.Fatalf("default %s nil", name)
		}
		return x
	}
	if d("flag").Value != "true" || d("kind").Value != "beta" || d("tax").Value != "11" || d("n").Value != "3" || d("name").Value != "it's" {
		t.Errorf("defaults = %+v %+v %+v %+v %+v", d("flag"), d("kind"), d("tax"), d("n"), d("name"))
	}
	if col(t, tb, "none").Default != nil || col(t, tb, "id").Default != nil {
		t.Error("kolom tanpa default metadata tidak boleh punya default")
	}
}

func TestDerive_UniqueIndexDeclaredOnly(t *testing.T) {
	s := deriveOne(t, f("m/thing.resource.yaml", resourceDoc("m.thing", `
fields:
  code:   {type: string, maxLength: 20, unique: true, label: m.thing.code}
  slug:   {type: string, maxLength: 20, index: true, label: m.thing.slug}
  a:      {type: string, maxLength: 20, sortable: true, filterable: true, label: m.thing.a}
  b:      {type: string, maxLength: 20, label: m.thing.b}
  c:      {type: string, maxLength: 20, label: m.thing.c}
indexes:
  - fields: [a, b]
  - fields: [b, c]
    unique: true
`)))
	tb := table(t, s, "m_things")
	if len(tb.Uniques) != 2 || len(tb.Indexes) != 2 {
		t.Fatalf("uniques=%+v indexes=%+v", tb.Uniques, tb.Indexes)
	}
	uq := map[string][]string{}
	for _, u := range tb.Uniques {
		uq[u.Name] = u.Columns
	}
	if got := uq["m_things_code_key"]; strings.Join(got, ",") != "code" {
		t.Errorf("uniques = %v", uq)
	}
	if got := uq["m_things_b_c_key"]; strings.Join(got, ",") != "b,c" {
		t.Errorf("uniques = %v", uq)
	}
	ix := map[string][]string{}
	for _, i := range tb.Indexes {
		ix[i.Name] = i.Columns
	}
	if strings.Join(ix["m_things_slug_idx"], ",") != "slug" || strings.Join(ix["m_things_a_b_idx"], ",") != "a,b" {
		t.Errorf("indexes = %v", ix)
	}
	// sortable/filterable tidak menurunkan indeks (D-23).
	for _, i := range tb.Indexes {
		if len(i.Columns) == 1 && i.Columns[0] == "a" {
			t.Errorf("indeks tidak boleh diturunkan dari sortable/filterable: %+v", i)
		}
	}
}

func TestDerive_IndexOnUnknownField(t *testing.T) {
	ps := deriveProblems(t, mod{slug: "m", docs: []file{
		f("m/thing.resource.yaml", resourceDoc("m.thing", `
fields:
  a: {type: string, maxLength: 20, label: m.thing.a}
indexes:
  - fields: [nope]
`))}})
	if !hasCode(ps, CodeIndexField) {
		t.Fatalf("temuan = %v", ps)
	}
}

func TestDerive_Accessors(t *testing.T) {
	s := deriveOne(t, f("m/line.resource.yaml", resourceDoc("m.line", `
fields:
  qty:   {type: decimal, precision: 18, scale: 3, label: m.line.qty}
  price: {type: decimal, precision: 18, scale: 2, label: m.line.price}
  total:
    type: decimal
    precision: 18
    scale: 2
    readOnly: true
    sortable: true
    label: m.line.total
    accessor: {kind: stored, expr: {mul: [{field: qty}, {field: price}], scale: 2, round: half_up}}
  shown:
    type: string
    maxLength: 50
    readOnly: true
    label: m.line.shown
    accessor: {kind: expr, expr: {concat: [{field: qty}, {lit: x}]}}
  counted:
    type: integer
    readOnly: true
    label: m.line.counted
    accessor: {kind: func, func: countThings}
`)))
	tb := table(t, s, "m_lines")
	total := col(t, tb, "total")
	if total.Generated == nil || total.Type.String() != "numeric(18,2)" {
		t.Fatalf("total = %+v", total)
	}
	if tb.Column("shown") != nil || tb.Column("counted") != nil {
		t.Error("accessor expr dan func tidak punya kolom")
	}
}

func TestDerive_AccessorExprInvalid(t *testing.T) {
	ps := deriveProblems(t, mod{slug: "m", docs: []file{
		f("m/line.resource.yaml", resourceDoc("m.line", `
fields:
  qty:   {type: decimal, precision: 18, scale: 3, label: m.line.qty}
  total:
    type: decimal
    precision: 18
    scale: 2
    readOnly: true
    label: m.line.total
    accessor: {kind: stored, expr: {frobnicate: [{field: qty}]}}
`))}})
	if !hasCode(ps, CodeExpr) {
		t.Fatalf("temuan = %v", ps)
	}
}

func TestDerive_Relations(t *testing.T) {
	rs := resolveMods(t,
		mod{slug: "core", docs: []file{
			f("core/customer.resource.yaml", header("resource", "core.customer")+"export: true\ninclude:\n  - source: sdk:audit\nfields:\n  name: {type: string, maxLength: 50, label: core.customer.name}\n"),
		}},
		mod{slug: "m", deps: []string{"core"}, docs: []file{
			f("m/product.resource.yaml", resourceDoc("m.product", `
fields:
  sku: {type: string, maxLength: 20, label: m.product.sku}
`)),
			f("m/tag.resource.yaml", resourceDoc("m.tag", `
fields:
  name: {type: string, maxLength: 20, label: m.tag.name}
`)),
			f("m/note.resource.yaml", resourceDoc("m.note", `
fields:
  title: {type: string, maxLength: 20, label: m.note.title}
relations:
  subject: {type: morphTo, types: [m.order, m.product], as: subject, required: true}
`)),
			f("m/orderLine.resource.yaml", resourceDoc("m.orderLine", `
fields:
  qty: {type: integer, label: m.orderLine.qty}
relations:
  order: {type: belongsTo, to: m.order, required: true, onDelete: cascade}
  product: {type: belongsTo, to: m.product, required: false, onDelete: setNull}
`)),
			f("m/order.resource.yaml", resourceDoc("m.order", `
fields:
  number: {type: string, maxLength: 20, label: m.order.number}
relations:
  customer: {type: belongsTo, to: core.customer, required: true}
  lines: {type: hasMany, of: m.orderLine, embedded: true}
  tags: {type: belongsToMany, to: m.tag, pivotFields: {remark: {type: string, maxLength: 100, nullable: true}}}
  notes: {type: morphMany, of: m.note, as: subject}
`)),
		}},
	)
	s, ps := Derive(rs)
	if len(ps) != 0 {
		t.Fatal(ps)
	}
	order := table(t, s, "m_orders")
	// Lintas modul: kolom ID saja, tanpa FK (Requirement 6.2).
	cc := col(t, order, "customer_id")
	if cc.Type.String() != "uuid" || !cc.NotNull || cc.Relation != "customer" {
		t.Errorf("customer_id = %+v", cc)
	}
	if len(order.ForeignKeys) != 0 {
		t.Errorf("relasi lintas modul tidak boleh FK: %+v", order.ForeignKeys)
	}
	line := table(t, s, "m_order_lines")
	fks := map[string]*ForeignKey{}
	for _, fk := range line.ForeignKeys {
		fks[strings.Join(fk.Columns, ",")] = fk
	}
	if fk := fks["order_id"]; fk == nil || fk.RefTable != "m_orders" || fk.RefColumns[0] != "id" || fk.OnDelete != OnDeleteCascade || fk.Name != "m_order_lines_order_id_fkey" {
		t.Errorf("fk order_id = %+v", fks["order_id"])
	}
	if fk := fks["product_id"]; fk == nil || fk.RefTable != "m_products" || fk.OnDelete != OnDeleteSetNull {
		t.Errorf("fk product_id = %+v", fks["product_id"])
	}
	if !col(t, line, "order_id").NotNull || col(t, line, "product_id").NotNull {
		t.Error("required menentukan NOT NULL")
	}
	// morphTo: kolom tipe + id, CHECK daftar putih, tanpa FK (Requirement 6.4).
	note := table(t, s, "m_notes")
	st, sid := col(t, note, "subject_type"), col(t, note, "subject_id")
	if st.Type.String() != "text" || !st.NotNull || sid.Type.String() != "uuid" || !sid.NotNull {
		t.Errorf("morph = %+v %+v", st, sid)
	}
	if len(note.ForeignKeys) != 0 {
		t.Errorf("morph tanpa FK: %+v", note.ForeignKeys)
	}
	var mc *Check
	for _, c := range note.Checks {
		if c.Column == "subject_type" {
			mc = c
		}
	}
	if mc == nil || mc.Kind != CheckEnum || strings.Join(mc.Values, ",") != "m.order,m.product" {
		t.Errorf("morph check = %+v", mc)
	}
	// belongsToMany: tabel pivot bangkitan, PK komposit, FK ke dua sisi, atribut pivot.
	pivot := table(t, s, "m_order_tags")
	if pivot.Kind != KindPivot || len(pivot.PrimaryKey) != 2 || pivot.PrimaryKey[0] != "order_id" || pivot.PrimaryKey[1] != "tag_id" {
		t.Errorf("pivot = %+v", pivot)
	}
	if len(pivot.ForeignKeys) != 2 {
		t.Errorf("pivot fks = %+v", pivot.ForeignKeys)
	}
	if c := col(t, pivot, "remark"); c.Type.String() != "text" || c.NotNull {
		t.Errorf("remark = %+v", c)
	}
	// hasMany dan morphMany tidak menambah kolom pada sisi pemilik.
	for _, c := range order.Columns {
		if c.Relation == "lines" || c.Relation == "notes" {
			t.Errorf("kolom tak terduga %+v", c)
		}
	}
}

func TestDerive_RelationProblems(t *testing.T) {
	cases := []struct {
		name string
		docs []file
		code string
	}{
		{"setNull pada required", []file{
			f("m/a.resource.yaml", resourceDoc("m.a", "fields:\n  x: {type: integer, label: m.a.x}\n")),
			f("m/b.resource.yaml", resourceDoc("m.b", "relations:\n  a: {type: belongsTo, to: m.a, required: true, onDelete: setNull}\n")),
		}, CodeRelation},
		{"target tidak ada di model", []file{
			f("m/b.resource.yaml", resourceDoc("m.b", "relations:\n  a: {type: belongsTo, to: m.a}\n")),
		}, CodeRelation},
		{"morphToMany belum didukung", []file{
			f("m/a.resource.yaml", resourceDoc("m.a", "fields:\n  x: {type: integer, label: m.a.x}\n")),
			f("m/b.resource.yaml", resourceDoc("m.b", "relations:\n  as: {type: morphToMany, to: m.a, as: owner}\n")),
		}, CodeUnsupported},
		{"hasOne tanpa belongsTo balik", []file{
			f("m/a.resource.yaml", resourceDoc("m.a", "fields:\n  x: {type: integer, label: m.a.x}\n")),
			f("m/b.resource.yaml", resourceDoc("m.b", "relations:\n  a: {type: hasOne, to: m.a}\n")),
		}, CodeRelation},
		{"kolom relasi bentrok dengan field", []file{
			f("m/a.resource.yaml", resourceDoc("m.a", "fields:\n  x: {type: integer, label: m.a.x}\n")),
			f("m/b.resource.yaml", resourceDoc("m.b", "fields:\n  aId: {type: uuid, label: m.b.aId}\nrelations:\n  a: {type: belongsTo, to: m.a}\n")),
		}, CodeCollision},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ps := deriveProblems(t, mod{slug: "m", docs: c.docs})
			if !hasCode(ps, c.code) {
				t.Fatalf("temuan = %v, want kode %s", ps, c.code)
			}
		})
	}
}

func TestDerive_HasOneUsesInverseBelongsTo(t *testing.T) {
	s := deriveOne(t,
		f("m/header.resource.yaml", resourceDoc("m.header", "relations:\n  invoice: {type: belongsTo, to: m.invoice, required: true, onDelete: cascade}\n")),
		f("m/invoice.resource.yaml", resourceDoc("m.invoice", "fields:\n  n: {type: integer, label: m.invoice.n}\nrelations:\n  header: {type: hasOne, to: m.header}\n")),
	)
	hd := table(t, s, "m_headers")
	found := false
	for _, u := range hd.Uniques {
		if len(u.Columns) == 1 && u.Columns[0] == "invoice_id" {
			found = true
		}
	}
	if !found {
		t.Errorf("hasOne harus membuat UNIQUE pada kolom FK seberang: %+v", hd.Uniques)
	}
}

func TestDerive_MissingIDField(t *testing.T) {
	ps := deriveProblems(t, mod{slug: "m", docs: []file{
		f("m/a.resource.yaml", resourceNoAudit("m.a", "fields:\n  x: {type: integer, label: m.a.x}\n")),
	}})
	if !hasCode(ps, CodeNoID) {
		t.Fatalf("temuan = %v", ps)
	}
}

func TestDerive_DecimalNeedsPrecision(t *testing.T) {
	ps := deriveProblems(t, mod{slug: "m", docs: []file{
		f("m/a.resource.yaml", resourceDoc("m.a", "fields:\n  x: {type: decimal, label: m.a.x}\n")),
	}})
	if !hasCode(ps, CodeType) {
		t.Fatalf("temuan = %v", ps)
	}
}

func TestDerive_UnmanagedColumns(t *testing.T) {
	// unmanagedColumns tidak menjadi kolom model; dicatat (snake_case) agar mesin selisih mengecualikannya.
	s := deriveOne(t, f("m/a.resource.yaml", resourceDoc("m.a", `
fields:
  x: {type: integer, label: m.a.x}
unmanagedColumns:
  - {name: secretToken, reason: dikelola kode tulis tangan}
`)))
	tb := table(t, s, "m_as")
	if len(tb.Unmanaged) != 1 || tb.Unmanaged[0] != "secret_token" {
		t.Errorf("Unmanaged = %v", tb.Unmanaged)
	}
	if tb.Column("secret_token") != nil {
		t.Error("kolom tak terkelola tidak boleh ada di model")
	}
}

func TestDerive_LargeAndLock(t *testing.T) {
	s := deriveOne(t, f("m/doc.resource.yaml", header("resource", "m.doc")+"large: true\ninclude:\n  - source: sdk:document\nfields:\n  n: {type: integer, label: m.doc.n}\n"))
	tb := table(t, s, "m_docs")
	if !tb.Large {
		t.Error("Large")
	}
	if tb.Lock == nil {
		t.Fatal("lockWhen sdk:document harus tersimpan sebagai AST kondisi")
	}
	if col(t, tb, "status").Default == nil || col(t, tb, "status").Default.Value != "draft" {
		t.Errorf("status default = %+v", col(t, tb, "status").Default)
	}
}

func TestDerive_EnumValueMustBeSafe(t *testing.T) {
	// Pemuat sudah menolak nilai enum di luar pola; Derive tetap memeriksa karena nilai berakhir di literal SQL.
	// Diuji lewat konstruksi langsung nilai yang melewati pemuat dengan menimpa atribut hasil resolusi.
	rs := resolveMods(t, mod{slug: "m", docs: []file{
		f("m/a.resource.yaml", resourceDoc("m.a", "fields:\n  k: {type: enum, values: [alpha, beta], label: m.a.k}\n")),
	}})
	rs[0].Field("k").Attrs["values"] = []any{"alpha", "x'); DROP TABLE y; --"}
	if _, ps := Derive(rs); !hasCode(ps, CodeEnumValue) {
		t.Fatalf("temuan = %v", ps)
	}
}
