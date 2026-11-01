package diff

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"

	tnoop "go.opentelemetry.io/otel/trace/noop"

	"erpora/internal/devtools/metadatatools/dbmodel"
	"erpora/internal/devtools/metadatatools/sqlgen"
	"erpora/internal/platform/config"
	"erpora/internal/platform/database"
	"erpora/internal/platform/migrate"
)

// expectedSchema adalah skema sementara tempat DDL bangkitan dari model diterapkan untuk dinormalkan.
const expectedSchema = "erpora_expected"

// Source adalah migrasi goose satu modul. Urutan Source pada Options adalah urutan penerapan: dependensi
// lebih dulu, modul yang diperiksa terakhir.
type Source struct {
	Slug string
	// Migrations adalah FS berisi berkas NNNN_nama.sql di akarnya (mis. os.DirFS("modules/x/migrations")); nil
	// berarti modul tanpa migrasi.
	Migrations fs.FS
}

// Options mengatur Run.
type Options struct {
	// DSN adalah DSN ADMIN ke database sementara yang kosong (sudah dibuat pemanggil: pgtest, atau
	// WithTempDatabase). Run membuat role non-superuser di server untuk menjalankan migrasi dan
	// menghapusnya kembali. JANGAN arahkan ke database produksi.
	DSN string
	// Sources adalah migrasi per modul, dependensi lebih dulu.
	Sources []Source
	// Model adalah skema yang diharapkan (hasil dbmodel.Derive, dibatasi modul bila perlu).
	Model *dbmodel.Schema
	// NotePrefixes: lihat CompareOptions.
	NotePrefixes []string
	// Log menerima pesan diagnostik (nil = dibuang).
	Log *slog.Logger
}

// ExpectedSchemaError dikembalikan bila DDL bangkitan dari model ditolak PostgreSQL (model atau templat
// bermasalah, bukan selisih).
type ExpectedSchemaError struct {
	Object string
	SQL    string
	Err    error
}

func (e *ExpectedSchemaError) Error() string {
	return fmt.Sprintf("skema diharapkan tidak dapat dibangun (%s): %v", e.Object, e.Err)
}

func (e *ExpectedSchemaError) Unwrap() error { return e.Err }

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func openDSN(ctx context.Context, dsn string, log *slog.Logger) (*database.DB, error) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return database.Open(ctx, config.Config{DatabaseURL: dsn}, tnoop.NewTracerProvider(), log)
}

// closeDB menutup pool koneksi.
func closeDB(db *database.DB) {
	if s, err := db.DB(); err == nil {
		_ = s.Close()
	}
}

func withUser(dsn, user, password string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", errors.New("DSN tidak dapat dibaca sebagai URL")
	}
	u.User = url.UserPassword(user, password)
	return u.String(), nil
}

// Run menjalankan seluruh alur mesin selisih pada database sementara dan mengembalikan laporan:
//  1. membuat role non-superuser (tanpa CREATEDB, CREATEROLE, REPLICATION, BYPASSRLS) dengan hak membuat
//     objek di database sementara;
//  2. menjalankan migrasi semua Source berurutan sebagai role itu (COPY ... PROGRAM, baca berkas server,
//     dan fasilitas superuser lain gagal, Requirement 10.8);
//  3. membaca katalog skema public (aktual);
//  4. menerapkan DDL bangkitan dari Model ke skema sementara dan membaca katalognya (diharapkan);
//  5. membandingkan, lalu menghapus role beserta objeknya (juga saat gagal).
//
// Run hanya untuk lingkungan pengembang dan CI.
func Run(ctx context.Context, o Options) (rep *Report, err error) {
	if o.Model == nil {
		return nil, errors.New("diff: Model wajib")
	}
	adminGorm, err := openDSN(ctx, o.DSN, o.Log)
	if err != nil {
		return nil, err
	}
	defer closeDB(adminGorm)
	admin, err := adminGorm.DB()
	if err != nil {
		return nil, err
	}

	role := "erpora_mig_" + randHex(6)
	password := randHex(16)
	qRole, err := sqlgen.QuoteIdent(role)
	if err != nil {
		return nil, err
	}
	pw, err := sqlgen.Literal(password)
	if err != nil {
		return nil, err
	}
	var dbName string
	if err := admin.QueryRowContext(ctx, `SELECT current_database()`).Scan(&dbName); err != nil {
		return nil, fmt.Errorf("nama database: %w", err)
	}
	qDB, err := sqlgen.QuoteIdent(dbName)
	if err != nil {
		return nil, err
	}

	// Pembersihan selalu dijalankan, dengan context sendiri agar tetap jalan bila ctx dibatalkan.
	created := false
	defer func() {
		if !created {
			return
		}
		cctx := context.WithoutCancel(ctx)
		var errs []error
		if _, e := admin.ExecContext(cctx, `DROP OWNED BY `+qRole); e != nil {
			errs = append(errs, e)
		}
		if _, e := admin.ExecContext(cctx, `DROP ROLE `+qRole); e != nil {
			errs = append(errs, e)
		}
		if ce := errors.Join(errs...); ce != nil && err == nil {
			err = fmt.Errorf("bersihkan role sementara %s: %w", role, ce)
		}
	}()

	// Role dibuat tanpa hak istimewa. Hak atas database: membuat skema (untuk skema diharapkan) dan atas
	// skema public: membuat objek (PostgreSQL 15 ke atas tidak lagi memberikannya kepada semua orang).
	created = true
	for _, stmt := range []string{
		`CREATE ROLE ` + qRole + ` LOGIN PASSWORD ` + pw + ` NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS`,
		`GRANT CONNECT, CREATE ON DATABASE ` + qDB + ` TO ` + qRole,
		`GRANT USAGE, CREATE ON SCHEMA public TO ` + qRole,
	} {
		if _, err := admin.ExecContext(ctx, stmt); err != nil {
			return nil, fmt.Errorf("siapkan role sementara: %w", err)
		}
	}

	roleDSN, err := withUser(o.DSN, role, password)
	if err != nil {
		return nil, err
	}
	roleGorm, err := openDSN(ctx, roleDSN, o.Log)
	if err != nil {
		return nil, fmt.Errorf("koneksi sebagai role sementara: %w", err)
	}
	defer closeDB(roleGorm)
	roleDB, err := roleGorm.DB()
	if err != nil {
		return nil, err
	}

	var super, privileged bool
	if err := roleDB.QueryRowContext(ctx, `SELECT rolsuper, rolsuper OR rolcreaterole OR rolcreatedb OR rolbypassrls OR rolreplication FROM pg_roles WHERE rolname = current_user`).Scan(&super, &privileged); err != nil {
		return nil, fmt.Errorf("periksa hak role: %w", err)
	}
	if super || privileged {
		return nil, errors.New("diff: role migrasi memiliki hak istimewa; dibatalkan")
	}

	for _, src := range o.Sources {
		if src.Migrations == nil {
			continue
		}
		if err := migrate.ApplyModule(ctx, roleGorm, src.Slug, src.Migrations, nil); err != nil {
			return nil, fmt.Errorf("modul %s: %w", src.Slug, err)
		}
	}

	actual, err := Introspect(ctx, roleGorm, "public")
	if err != nil {
		return nil, fmt.Errorf("introspeksi database: %w", err)
	}

	qSchema, err := sqlgen.QuoteIdent(expectedSchema)
	if err != nil {
		return nil, err
	}
	if _, err := roleDB.ExecContext(ctx, `CREATE SCHEMA `+qSchema); err != nil {
		return nil, fmt.Errorf("skema sementara: %w", err)
	}
	stmts, err := sqlgen.CreateSchema(o.Model, sqlgen.Options{Schema: expectedSchema})
	if err != nil {
		return nil, fmt.Errorf("DDL bangkitan: %w", err)
	}
	for _, st := range stmts {
		if _, err := roleDB.ExecContext(ctx, st.SQL); err != nil {
			return nil, &ExpectedSchemaError{Object: st.Object, SQL: st.SQL, Err: err}
		}
	}
	expected, err := Introspect(ctx, roleGorm, expectedSchema)
	if err != nil {
		return nil, fmt.Errorf("introspeksi skema diharapkan: %w", err)
	}
	rep = Compare(actual, expected, o.Model, CompareOptions{NotePrefixes: o.NotePrefixes})
	rep.Role = role
	return rep, nil
}
