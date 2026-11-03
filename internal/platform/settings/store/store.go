// Package store adalah persistensi minimal Settings platform (subset C1 dari FR-120; PRD-001 Q20): tabel settings di
// tenant DB. Satu-satunya pemakai GORM untuk tabel itu (D3b). Tenant selalu diambil dari context
// (tenancy.FromContext) dan database dipilih lewat TenantConnector (isolasi per database, ADR-0004; Q18), tanpa kolom
// tenant_id. Nilai berupa JSON; bila Encrypted, nilai disegel dengan crypto.Sealer (AAD settings|scope|module|key)
// dan disimpan sebagai string JSON base64, tidak pernah sebagai plaintext.
//
// Cakupan sengaja kecil: tanpa registri skema, validasi per tipe, audit, cache revisi, atau UI. Itu pekerjaan C3
// (FR-120), yang memperluas paket ini secara aditif.
package store

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"gorm.io/gorm"

	"erpora/internal/platform/crypto"
	"erpora/internal/platform/database"
	"erpora/internal/platform/tenancy"
)

// Scope setting.
const (
	ScopePlatform = "platform"
	ScopeModule   = "module"
)

// MaxValueBytes adalah batas ukuran nilai JSON (plaintext).
const MaxValueBytes = 64 << 10

var (
	// ErrNotFound berarti setting belum diisi pada tenant itu.
	ErrNotFound = errors.New("settings/store: tidak ditemukan")
	// ErrInvalidArgument berarti masukan melanggar aturan bentuk yang juga ditegakkan CHECK database.
	ErrInvalidArgument = errors.New("settings/store: argumen tidak valid")
	// ErrNoTenant berarti context tidak membawa tenant.
	ErrNoTenant = errors.New("settings/store: tenant tidak ada di context")
	// ErrSealerRequired berarti nilai terenkripsi diakses tanpa crypto.Sealer (gagal tertutup).
	ErrSealerRequired = errors.New("settings/store: nilai terenkripsi membutuhkan Sealer (ERP_SECRET_KEY)")
)

var (
	keyPattern    = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,127}$`)
	modulePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)
)

// Setting adalah satu nilai pengaturan. Value selalu plaintext JSON pada API ini (tersegel hanya di database).
type Setting struct {
	Scope     string
	ModuleKey string // kosong untuk scope platform
	Key       string
	Value     json.RawMessage
	Encrypted bool
	UpdatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Store menyimpan settings. Nilai nol tidak valid; gunakan New.
type Store struct {
	tenants database.TenantConnector
	sealer  *crypto.Sealer
}

// New membuat Store. sealer boleh nil bila tidak ada nilai terenkripsi; mengakses nilai terenkripsi lalu gagal
// dengan ErrSealerRequired.
func New(tenants database.TenantConnector, sealer *crypto.Sealer) *Store {
	return &Store{tenants: tenants, sealer: sealer}
}

type row struct {
	Scope     string    `gorm:"column:scope"`
	ModuleKey string    `gorm:"column:module_key"`
	Key       string    `gorm:"column:key"`
	Value     []byte    `gorm:"column:value"`
	Encrypted bool      `gorm:"column:encrypted"`
	UpdatedBy string    `gorm:"column:updated_by"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (row) TableName() string { return "settings" }

func validScope(scope, moduleKey string) bool {
	switch scope {
	case ScopePlatform:
		return moduleKey == ""
	case ScopeModule:
		return modulePattern.MatchString(moduleKey)
	}
	return false
}

func validate(s Setting, updatedBy string) error {
	if !validScope(s.Scope, s.ModuleKey) || !keyPattern.MatchString(s.Key) || len(updatedBy) > 128 {
		return ErrInvalidArgument
	}
	if len(s.Value) == 0 || len(s.Value) > MaxValueBytes || !json.Valid(s.Value) {
		return ErrInvalidArgument
	}
	return nil
}

func aad(scope, moduleKey, key string) []byte {
	return []byte("settings|" + scope + "|" + moduleKey + "|" + key)
}

// encodeValue menghasilkan nilai yang disimpan: plaintext JSON apa adanya, atau (Encrypted) string JSON base64
// berisi segel.
func encodeValue(sl *crypto.Sealer, s Setting) (json.RawMessage, error) {
	if !s.Encrypted {
		return s.Value, nil
	}
	if sl == nil {
		return nil, ErrSealerRequired
	}
	sealed, err := sl.Seal(s.Value, aad(s.Scope, s.ModuleKey, s.Key))
	if err != nil {
		return nil, fmt.Errorf("settings/store: segel nilai: %w", err)
	}
	b, err := json.Marshal(base64.StdEncoding.EncodeToString(sealed))
	if err != nil {
		return nil, err
	}
	return b, nil
}

func decodeValue(sl *crypto.Sealer, scope, moduleKey, key string, encrypted bool, stored json.RawMessage) (json.RawMessage, error) {
	if !encrypted {
		return stored, nil
	}
	if sl == nil {
		return nil, ErrSealerRequired
	}
	var b64 string
	if err := json.Unmarshal(stored, &b64); err != nil {
		return nil, errors.New("settings/store: nilai terenkripsi bukan string JSON")
	}
	sealed, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, errors.New("settings/store: nilai terenkripsi bukan base64 yang sah")
	}
	plain, err := sl.Open(sealed, aad(scope, moduleKey, key))
	if err != nil {
		return nil, fmt.Errorf("settings/store: buka nilai terenkripsi: %w", err)
	}
	return plain, nil
}

func (s *Store) db(ctx context.Context) (*gorm.DB, error) {
	t, ok := tenancy.FromContext(ctx)
	if !ok {
		return nil, ErrNoTenant
	}
	if s.tenants == nil {
		return nil, errors.New("settings/store: TenantConnector tidak dikonfigurasi")
	}
	db, err := s.tenants.For(ctx, t)
	if err != nil {
		return nil, fmt.Errorf("settings/store: pilih database tenant: %w", err)
	}
	return db.WithContext(ctx), nil
}

// Get membaca satu setting pada tenant di context; ErrNotFound bila belum diisi. Nilai terenkripsi dikembalikan
// sebagai plaintext JSON (Encrypted tetap true).
func (s *Store) Get(ctx context.Context, scope, moduleKey, key string) (Setting, error) {
	if !validScope(scope, moduleKey) || !keyPattern.MatchString(key) {
		return Setting{}, ErrInvalidArgument
	}
	db, err := s.db(ctx)
	if err != nil {
		return Setting{}, err
	}
	var r row
	res := db.Raw(`SELECT scope, module_key, key, value::text AS value, encrypted, updated_by, created_at, updated_at
		FROM settings WHERE scope = ? AND module_key = ? AND key = ?`, scope, moduleKey, key).Scan(&r)
	if res.Error != nil {
		return Setting{}, fmt.Errorf("settings/store: baca setting: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return Setting{}, ErrNotFound
	}
	val, err := decodeValue(s.sealer, r.Scope, r.ModuleKey, r.Key, r.Encrypted, json.RawMessage(r.Value))
	if err != nil {
		return Setting{}, err
	}
	return Setting{
		Scope: r.Scope, ModuleKey: r.ModuleKey, Key: r.Key, Value: val, Encrypted: r.Encrypted, UpdatedBy: r.UpdatedBy,
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}, nil
}

// Upsert menyimpan (atau menimpa) setting pada tenant di context. Validasi bentuk dilakukan lebih dulu dan
// sepenuhnya tanpa menyentuh DB. updated_by dicatat; created_at tetap pada penimpaan.
func (s *Store) Upsert(ctx context.Context, set Setting, updatedBy string) error {
	if err := validate(set, updatedBy); err != nil {
		return err
	}
	if _, ok := tenancy.FromContext(ctx); !ok {
		return ErrNoTenant
	}
	stored, err := encodeValue(s.sealer, set)
	if err != nil {
		return err
	}
	db, err := s.db(ctx)
	if err != nil {
		return err
	}
	at := time.Now().UTC().Truncate(time.Microsecond)
	if err := db.Exec(`INSERT INTO settings (scope, module_key, key, value, encrypted, updated_by, created_at, updated_at)
		VALUES (?, ?, ?, ?::jsonb, ?, ?, ?, ?)
		ON CONFLICT (scope, module_key, key) DO UPDATE
		SET value = EXCLUDED.value, encrypted = EXCLUDED.encrypted, updated_by = EXCLUDED.updated_by, updated_at = EXCLUDED.updated_at`,
		set.Scope, set.ModuleKey, set.Key, string(stored), set.Encrypted, updatedBy, at, at).Error; err != nil {
		return fmt.Errorf("settings/store: simpan setting: %w", err)
	}
	return nil
}
