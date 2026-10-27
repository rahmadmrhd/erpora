package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/pressly/goose/v3"

	"erpora/internal/platform/database"
)

// ModuleTable adalah nama tabel versi goose per modul di tenant DB.
func ModuleTable(slug string) string {
	return "goose_module_" + strings.ReplaceAll(slug, "-", "_")
}

// AppliedMigration menggambarkan satu file migrasi modul yang sukses diterapkan.
type AppliedMigration struct {
	Version  int64
	Name     string
	Checksum string // sha256 hex isi file
	Duration time.Duration
}

// MigrationError dikembalikan ApplyModule untuk file migrasi yang gagal.
type MigrationError struct {
	Version  int64
	Name     string
	Checksum string
	Err      error
}

func (e *MigrationError) Error() string {
	return fmt.Sprintf("migrasi modul %s (versi %d) gagal: %v", e.Name, e.Version, e.Err)
}

func (e *MigrationError) Unwrap() error { return e.Err }

func moduleProvider(db *database.DB, slug string, fsys fs.FS) (*goose.Provider, error) {
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	// Sengaja tanpa WithSessionLocker: operasi lifecycle sudah diserialisasi advisory lock
	// per tenant (modules/store.WithTenantLock), sehingga lock sesi goose tidak diperlukan.
	return goose.NewProvider(goose.DialectPostgres, sqlDB, fsys,
		goose.WithTableName(ModuleTable(slug)),
	)
}

// ApplyModule menerapkan migrasi modul yang belum diterapkan, satu file per transaksi (perilaku goose),
// memanggil after setelah setiap file sukses; berhenti di file pertama yang gagal (*MigrationError).
func ApplyModule(ctx context.Context, db *database.DB, slug string, fsys fs.FS, after func(AppliedMigration) error) error {
	p, err := moduleProvider(db, slug, fsys)
	if err != nil {
		return fmt.Errorf("migrasi modul %s: %w", slug, err)
	}
	// Jangan p.Close(): itu menutup *sql.DB milik pemanggil.
	for {
		res, err := p.UpByOne(ctx)
		if errors.Is(err, goose.ErrNoNextVersion) {
			return nil
		}
		if err != nil {
			var pe *goose.PartialError
			if errors.As(err, &pe) && pe.Failed != nil && pe.Failed.Source != nil {
				src := pe.Failed.Source
				sum, _ := fileChecksum(fsys, src.Path)
				return &MigrationError{Version: src.Version, Name: path.Base(src.Path), Checksum: sum, Err: pe.Err}
			}
			return fmt.Errorf("migrasi modul %s: %w", slug, err)
		}
		sum, err := fileChecksum(fsys, res.Source.Path)
		if err != nil {
			return fmt.Errorf("migrasi modul %s: %w", slug, err)
		}
		if after == nil {
			continue
		}
		if err := after(AppliedMigration{
			Version:  res.Source.Version,
			Name:     path.Base(res.Source.Path),
			Checksum: sum,
			Duration: res.Duration,
		}); err != nil {
			return fmt.Errorf("migrasi modul %s: %w", slug, err)
		}
	}
}

func fileChecksum(fsys fs.FS, p string) (string, error) {
	b, err := fs.ReadFile(fsys, p)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// ModuleStatus melaporkan keadaan migrasi modul secara read-only (tidak membuat tabel goose).
func ModuleStatus(ctx context.Context, db *database.DB, slug string, fsys fs.FS) (TargetStatus, error) {
	embedded, err := embeddedVersions(fsys)
	if err != nil {
		return TargetStatus{}, fmt.Errorf("migrasi modul %s: %w", slug, err)
	}
	applied, err := appliedVersions(ctx, db, ModuleTable(slug))
	if err != nil {
		return TargetStatus{}, err
	}
	st := TargetStatus{Target: Target("module:" + slug)}
	for _, v := range embedded {
		if v > st.Latest {
			st.Latest = v
		}
		if !applied[v] {
			st.Pending++
		}
	}
	for v := range applied {
		if v > st.Current {
			st.Current = v
		}
	}
	return st, nil
}

// PendingModuleMigrations mengembalikan nama file migrasi yang belum diterapkan, terurut.
func PendingModuleMigrations(ctx context.Context, db *database.DB, slug string, fsys fs.FS) ([]string, error) {
	applied, err := appliedVersions(ctx, db, ModuleTable(slug))
	if err != nil {
		return nil, err
	}
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("migrasi modul %s: %w", slug, err)
	}
	type item struct {
		v    int64
		name string
	}
	var items []item
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".sql") {
			continue
		}
		v, err := versionOf(name)
		if err != nil {
			return nil, err
		}
		if !applied[v] {
			items = append(items, item{v, name})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].v < items[j].v })
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.name)
	}
	return out, nil
}
