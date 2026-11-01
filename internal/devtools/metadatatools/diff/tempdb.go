package diff

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"erpora/internal/devtools/metadatatools/sqlgen"
)

// EnvAdminDSN adalah env berisi DSN admin server PostgreSQL pengembangan atau CI tempat database sementara
// dibuat (sama dengan infrastruktur pgtest). Mesin selisih TIDAK PERNAH membaca DSN aplikasi (ERP_DATABASE_URL).
const EnvAdminDSN = "ERP_TEST_DATABASE_URL"

// WithTempDatabase membuat database sementara bernama erpora_diff_<hex> di server adminDSN, memanggil fn
// dengan DSN admin ke database itu, lalu menghapusnya (DROP DATABASE ... WITH (FORCE)), juga saat fn gagal.
func WithTempDatabase(ctx context.Context, adminDSN string, fn func(dsn string) error) (err error) {
	adminGorm, err := openDSN(ctx, adminDSN, nil)
	if err != nil {
		return err
	}
	defer closeDB(adminGorm)
	admin, err := adminGorm.DB()
	if err != nil {
		return err
	}

	name := "erpora_diff_" + randHex(8)
	q, err := sqlgen.QuoteIdent(name)
	if err != nil {
		return err
	}
	u, err := url.Parse(adminDSN)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return errors.New("DSN admin harus berbentuk URL postgres://")
	}
	u.Path = "/" + name
	u.RawPath = ""

	created := false
	defer func() {
		if !created {
			return
		}
		if _, e := admin.ExecContext(context.WithoutCancel(ctx), `DROP DATABASE IF EXISTS `+q+` WITH (FORCE)`); e != nil && err == nil {
			err = fmt.Errorf("hapus database sementara %s: %w", name, e)
		}
	}()
	created = true
	if _, err := admin.ExecContext(ctx, `CREATE DATABASE `+q); err != nil {
		return fmt.Errorf("buat database sementara: %w", err)
	}
	return fn(u.String())
}
