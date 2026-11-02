//go:build integration

package migrate

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"testing"
	"testing/fstest"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	tnoop "go.opentelemetry.io/otel/trace/noop"

	"erpora/internal/devtools/metadatatools/dbmodel"
	"erpora/internal/devtools/metadatatools/diff"
	"erpora/internal/devtools/metadatatools/sqlgen"
	"erpora/internal/platform/config"
	"erpora/internal/platform/database"
	"erpora/internal/platform/testsupport/pgtest"
)

func gooseFile(stmts []sqlgen.Statement) string {
	var sb strings.Builder
	sb.WriteString("-- +goose Up\n")
	for _, s := range stmts {
		if s.Kind == sqlgen.KindCreateFunction {
			sb.WriteString("-- +goose StatementBegin\n" + s.SQL + ";\n-- +goose StatementEnd\n")
			continue
		}
		sb.WriteString(s.SQL + ";\n")
	}
	sb.WriteString("-- +goose Down\nSELECT 1;\n")
	return sb.String()
}

func baseFS(t *testing.T, model *dbmodel.Schema, extra map[string]string) fstest.MapFS {
	t.Helper()
	stmts, err := sqlgen.CreateSchema(model, sqlgen.Options{})
	if err != nil {
		t.Fatal(err)
	}
	fs := fstest.MapFS{"0001_init.sql": {Data: []byte(gooseFile(stmts))}}
	for n, c := range extra {
		fs[n] = &fstest.MapFile{Data: []byte(c)}
	}
	return fs
}

const v1Thing = `
fields:
  title: {type: string, minLength: 1, maxLength: 120, label: m.thing.title}
  kind:  {type: enum, values: [a, b], extensible: true, default: a, label: m.thing.kind}
  note:  {type: text, label: m.thing.note}
`

// Alur penuh: selisih -> Generate -> berkas 0002 -> selisih kosong setelah diterapkan.
func TestGenerate_AutoMigrationClosesTheDiff(t *testing.T) {
	v1 := modelOf(t, f("m/thing.resource.yaml", resourceDoc("m.thing", v1Thing)))
	v2 := modelOf(t,
		f("m/thing.resource.yaml", resourceDoc("m.thing", `
fields:
  title: {type: string, minLength: 1, maxLength: 200, index: true, label: m.thing.title}
  kind:  {type: enum, values: [a, b, c], extensible: true, default: a, label: m.thing.kind}
  note:  {type: text, nullable: true, label: m.thing.note}
  extra: {type: string, maxLength: 30, nullable: true, label: m.thing.extra}
  flag:  {type: boolean, default: true, label: m.thing.flag}
`)),
		f("m/other.resource.yaml", resourceDoc("m.other", "fields:\n  n: {type: integer, index: true, label: m.other.n}\nrelations:\n  thing: {type: belongsTo, to: m.thing, required: true, onDelete: cascade}\n")),
	)
	src := func(fs fstest.MapFS) []diff.Source { return []diff.Source{{Slug: "m", Migrations: fs}} }
	ctx := context.Background()

	rep, err := diff.Run(ctx, diff.Options{DSN: pgtest.Start(t), Sources: src(baseFS(t, v1, nil)), Model: v2})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Empty() {
		t.Fatal("v2 harus berselisih dengan v1")
	}
	res, err := Generate(rep, v2, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Todos) != 0 || HasTodo(res.SQL) {
		t.Fatalf("semua perubahan v2 aman, dapat TODO: %+v\n%s", res.Todos, res.SQL)
	}
	if !res.Irreversible {
		t.Error("pelonggaran NOT NULL dan CHECK membuat Down tidak dapat dibalik")
	}
	t.Logf("migrasi bangkitan:\n%s", res.SQL)

	fs := baseFS(t, v1, map[string]string{"0002_sync.sql": res.SQL})
	rep2, err := diff.Run(ctx, diff.Options{DSN: pgtest.Start(t), Sources: src(fs), Model: v2})
	if err != nil {
		t.Fatalf("menerapkan migrasi bangkitan: %v\n%s", err, res.SQL)
	}
	if !rep2.Empty() {
		t.Fatalf("selisih harus kosong setelah migrasi bangkitan:\n%s\n%s", rep2, res.SQL)
	}
}

func openGorm(t *testing.T, dsn string) *database.DB {
	t.Helper()
	db, err := database.Open(context.Background(), config.Config{DatabaseURL: dsn}, tnoop.NewTracerProvider(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if s, err := db.DB(); err == nil {
			_ = s.Close()
		}
	})
	return db
}

func newProvider(t *testing.T, dsn string, fs fstest.MapFS) (*goose.Provider, *database.DB) {
	t.Helper()
	g := openGorm(t, dsn)
	sqlDB, err := g.DB()
	if err != nil {
		t.Fatal(err)
	}
	p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, fs)
	if err != nil {
		t.Fatal(err)
	}
	return p, g
}

// Up lalu Down kembali ke skema semula bila semua langkah dapat dibalik.
func TestGenerate_ReversibleMigrationRoundTrips(t *testing.T) {
	v1 := modelOf(t, f("m/thing.resource.yaml", resourceDoc("m.thing", v1Thing)))
	v2 := modelOf(t,
		f("m/thing.resource.yaml", resourceDoc("m.thing", v1Thing+`
  extra: {type: string, maxLength: 30, nullable: true, label: m.thing.extra}
  flag:  {type: boolean, default: true, label: m.thing.flag}
`)),
		f("m/other.resource.yaml", resourceDoc("m.other", "fields:\n  n: {type: integer, index: true, label: m.other.n}\nrelations:\n  thing: {type: belongsTo, to: m.thing, required: true, onDelete: cascade}\n")),
	)
	ctx := context.Background()
	rep, err := diff.Run(ctx, diff.Options{DSN: pgtest.Start(t), Sources: []diff.Source{{Slug: "m", Migrations: baseFS(t, v1, nil)}}, Model: v2})
	if err != nil {
		t.Fatal(err)
	}
	res, err := Generate(rep, v2, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Irreversible || len(res.Todos) != 0 {
		t.Fatalf("v2 hanya penambahan: %+v\n%s", res, res.SQL)
	}
	fs := baseFS(t, v1, map[string]string{"0002_sync.sql": res.SQL})

	dsnBefore := pgtest.Start(t)
	pb, gb := newProvider(t, dsnBefore, baseFS(t, v1, nil))
	if _, err := pb.UpTo(ctx, 1); err != nil {
		t.Fatal(err)
	}
	before, err := diff.Introspect(ctx, gb, "public")
	if err != nil {
		t.Fatal(err)
	}

	dsn := pgtest.Start(t)
	p, g := newProvider(t, dsn, fs)
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("Up: %v\n%s", err, res.SQL)
	}
	if _, err := p.Down(ctx); err != nil {
		t.Fatalf("Down: %v\n%s", err, res.SQL)
	}
	after, err := diff.Introspect(ctx, g, "public")
	if err != nil {
		t.Fatal(err)
	}
	// goose_db_version milik masing-masing; yang dibandingkan hanya tabel bisnis.
	delete(before.Tables, "goose_db_version")
	delete(after.Tables, "goose_db_version")
	if r := diff.Compare(after, before, nil, diff.CompareOptions{}); !r.Empty() {
		t.Fatalf("setelah Down skema harus kembali ke semula:\n%s", r)
	}
	if len(after.Tables) != len(before.Tables) {
		t.Errorf("tabel setelah Down = %v, semula %v", after.Names(), before.Names())
	}
}

// Down tak dapat dibalik menggagalkan goose down dengan pesan jelas dan tidak mengubah apa pun.
func TestGenerate_IrreversibleDownRefuses(t *testing.T) {
	v1 := modelOf(t, f("m/thing.resource.yaml", resourceDoc("m.thing", v1Thing)))
	v2 := modelOf(t, f("m/thing.resource.yaml", resourceDoc("m.thing", strings.Replace(v1Thing, "note:  {type: text,", "note:  {type: text, nullable: true,", 1))))
	ctx := context.Background()
	rep, err := diff.Run(ctx, diff.Options{DSN: pgtest.Start(t), Sources: []diff.Source{{Slug: "m", Migrations: baseFS(t, v1, nil)}}, Model: v2})
	if err != nil {
		t.Fatal(err)
	}
	res, err := Generate(rep, v2, Options{})
	if err != nil || !res.Irreversible {
		t.Fatalf("res = %+v, %v", res, err)
	}
	p, _ := newProvider(t, pgtest.Start(t), baseFS(t, v1, map[string]string{"0002_sync.sql": res.SQL}))
	if _, err := p.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Down(ctx); err == nil || !strings.Contains(err.Error(), "tidak dapat dibalik") {
		t.Fatalf("Down harus ditolak dengan pesan jelas, err = %v", err)
	}
}

// Backfill per batch: berkas NO TRANSACTION dengan blok DO ber-COMMIT menyalin semua baris.
func TestBatchCopyBackfill_RunsWithNoTransaction(t *testing.T) {
	model := modelOf(t, f("m/thing.resource.yaml", resourceDoc("m.thing", "large: true\nfields:\n  phone: {type: string, maxLength: 20, nullable: true, label: m.thing.phone}\n  phoneE164: {type: string, maxLength: 20, nullable: true, label: m.thing.phoneE164}\n")))
	tb := model.Table("m_things")
	backfill, err := sqlgen.BatchCopyBackfill(tb, "phone_e164", "phone", 7, sqlgen.Options{})
	if err != nil {
		t.Fatal(err)
	}
	create, err := sqlgen.CreateTable(tb, sqlgen.Options{})
	if err != nil {
		t.Fatal(err)
	}
	fs := fstest.MapFS{
		"0001_init.sql": {Data: []byte("-- +goose NO TRANSACTION\n-- +goose Up\n" + create + ";\n" +
			"INSERT INTO m_things (id, phone) SELECT gen_random_uuid(), 'p' || g FROM generate_series(1, 100) g;\n-- +goose Down\nSELECT 1;\n")},
		"0002_backfill.sql": {Data: []byte("-- +goose NO TRANSACTION\n-- +goose Up\n-- +goose StatementBegin\n" + backfill + ";\n-- +goose StatementEnd\n-- +goose Down\nSELECT 1;\n")},
	}
	ctx := context.Background()
	dsn := pgtest.Start(t)
	p, _ := newProvider(t, dsn, fs)
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("Up: %v", err)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var copied, total int
	if err := db.QueryRow(`SELECT count(*) FILTER (WHERE phone_e164 = phone), count(*) FROM m_things`).Scan(&copied, &total); err != nil || copied != 100 || total != 100 {
		t.Fatalf("tersalin %d dari %d (%v)", copied, total, err)
	}
	// Diulang aman (idempoten): tidak ada baris yang tersisa.
	if _, err := db.Exec(backfill); err != nil {
		t.Fatalf("pengulangan: %v", err)
	}
}
