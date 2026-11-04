//go:build integration

package mfa_test

import (
	"bytes"
	"context"
	"encoding/base32"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	"erpora/internal/platform/config"
	"erpora/internal/platform/crypto"
	"erpora/internal/platform/database"
	"erpora/internal/platform/identity/mfa"
	"erpora/internal/platform/identity/store"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/tenancy"
	"erpora/internal/platform/testsupport/pgtest"
)

var (
	tenantA = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"))
	tenantB = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c"))
)

type bothTenants struct{ db database.TenantDB }

func (c bothTenants) For(_ context.Context, id tenancy.TenantID) (database.TenantDB, error) {
	if id != tenantA && id != tenantB {
		return database.TenantDB{}, database.ErrUnknownTenant
	}
	return c.db, nil
}

// noLockout adalah Lockout tiruan; lockout sungguhan (guard) diuji di paketnya sendiri.
type noLockout struct{}

func (noLockout) RegisterFailure(context.Context, string, string) (mfa.LockState, error) {
	return mfa.LockState{}, nil
}
func (noLockout) RegisterSuccess(context.Context, string, string) error { return nil }

func setup(t *testing.T) (*gorm.DB, *store.Store, *mfa.Service, *crypto.Sealer) {
	t.Helper()
	dsn := pgtest.Start(t)
	db, err := database.Open(context.Background(),
		config.Config{DatabaseURL: dsn, DBSlowQuery: 200 * time.Millisecond}, noop.NewTracerProvider(), slog.Default())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		if s, err := db.DB(); err == nil {
			_ = s.Close()
		}
	})
	if err := migrate.Up(context.Background(), db, config.EnvProduction); err != nil {
		t.Fatal(err)
	}
	st := store.New(database.ControlDB{DB: db}, bothTenants{db: database.TenantDB{DB: db}})
	seal, err := crypto.NewSealerFromMaterial(bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := mfa.New(mfa.Deps{Store: st, Sealer: seal, Lockout: noLockout{}, Issuer: "ERPora"})
	if err != nil {
		t.Fatal(err)
	}
	return db, st, svc, seal
}

func ctxFor(tn tenancy.TenantID) context.Context {
	return mfa.WithClientIP(tenancy.WithTenant(context.Background(), tn), "203.0.113.7")
}

func newUser(t *testing.T, s *store.Store, email string) store.User {
	t.Helper()
	u, err := s.CreateUser(context.Background(), tenantA, store.NewUser{Email: email, Name: "Nama", PasswordHash: "$argon2id$stub"})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func enroll(t *testing.T, svc *mfa.Service, ctx context.Context, userID string) ([]byte, []string) {
	t.Helper()
	info, err := svc.BeginEnrollment(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	sec, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(info.Secret)
	if err != nil {
		t.Fatal(err)
	}
	codes, err := svc.ConfirmEnrollment(ctx, userID, mfa.TOTP{}.Code(sec, time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	return sec, codes
}

func race(n int, f func() error) (wins int, invalid int, other []error) {
	var w, inv atomic.Int32
	var mu sync.Mutex
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			switch err := f(); {
			case err == nil:
				w.Add(1)
			case errors.Is(err, mfa.ErrInvalidCode):
				inv.Add(1)
			default:
				mu.Lock()
				other = append(other, err)
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()
	return int(w.Load()), int(inv.Load()), other
}

// Review Focus 5 dengan store sungguhan: AdvanceMFAStep dan UseRecoveryCode atomik di PostgreSQL.
func TestMFAWithRealStore(t *testing.T) {
	db, st, svc, seal := setup(t)
	ctx := ctxFor(tenantA)
	u := newUser(t, st, "ana@example.com")
	sec, codes := enroll(t, svc, ctx, u.ID)

	t.Run("secret di kolom tersegel dan terikat AAD", func(t *testing.T) {
		var row struct{ SecretSealed []byte }
		if err := db.Raw(`SELECT secret_sealed FROM user_mfa_factors WHERE user_id = ?`, u.ID).Scan(&row).Error; err != nil {
			t.Fatal(err)
		}
		raw := row.SecretSealed
		if bytes.Contains(raw, sec) {
			t.Error("kolom memuat secret mentah")
		}
		got, err := seal.Open(raw, []byte("user_mfa_factors|secret|"+u.ID))
		if err != nil || !bytes.Equal(got, sec) {
			t.Fatalf("Open: %v", err)
		}
		if _, err := seal.Open(raw, []byte("user_mfa_factors|secret|"+uuid.NewString())); err == nil {
			t.Error("AAD salah berhasil membuka")
		}
	})

	t.Run("verifikasi serentak kode sama: tepat satu sukses", func(t *testing.T) {
		// kode langkah berikutnya (drift +1) belum dipakai konfirmasi (yang memakai langkah saat ini)
		code := mfa.TOTP{}.Code(sec, time.Now().Add(mfa.Period*time.Second))
		wins, invalid, other := race(12, func() error { return svc.Verify(ctx, u.ID, code) })
		if wins != 1 || invalid != 11 || len(other) != 0 {
			t.Fatalf("wins=%d invalid=%d other=%v", wins, invalid, other)
		}
		if err := svc.Verify(ctx, u.ID, code); !errors.Is(err, mfa.ErrInvalidCode) {
			t.Errorf("replay = %v", err)
		}
	})

	t.Run("recovery code serentak: tepat satu sukses dan hash tersimpan", func(t *testing.T) {
		wins, invalid, other := race(12, func() error { return svc.UseRecoveryCode(ctx, u.ID, codes[0]) })
		if wins != 1 || invalid != 11 || len(other) != 0 {
			t.Fatalf("wins=%d invalid=%d other=%v", wins, invalid, other)
		}
		if n, _ := st.CountUnusedRecoveryCodes(context.Background(), tenantA, u.ID); n != 9 {
			t.Errorf("tersisa %d, want 9", n)
		}
		var plain int
		for _, c := range codes {
			if err := db.Raw(`SELECT count(*) FROM user_recovery_codes WHERE code_hash = convert_to(?, 'UTF8')`, c).Scan(&plain).Error; err != nil || plain != 0 {
				t.Errorf("recovery code tersimpan sebagai plaintext: %d, %v", plain, err)
			}
		}
	})

	t.Run("Required: pengguna berfaktor aktif dan administrator", func(t *testing.T) {
		if req, err := svc.Required(ctx, u.ID); err != nil || !req {
			t.Errorf("Required(berfaktor) = %v, %v", req, err)
		}
		v := newUser(t, st, "budi@example.com")
		if req, err := svc.Required(ctx, v.ID); err != nil || req {
			t.Errorf("Required(biasa) = %v, %v", req, err)
		}
		if _, err := st.GrantRole(context.Background(), tenantA, v.ID, mfa.AdministratorRole, time.Now()); err != nil {
			t.Fatal(err)
		}
		if req, err := svc.Required(ctx, v.ID); err != nil || !req {
			t.Errorf("Required(administrator) = %v, %v", req, err)
		}
	})

	t.Run("isolasi tenant", func(t *testing.T) {
		other := ctxFor(tenantB)
		if err := svc.Verify(other, u.ID, mfa.TOTP{}.Code(sec, time.Now().Add(2*mfa.Period*time.Second))); !errors.Is(err, mfa.ErrInvalidCode) {
			t.Errorf("Verify dari tenant B = %v", err)
		}
		if err := svc.Reset(other, "cli:core:reset-mfa", u.ID); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("Reset dari tenant B = %v", err)
		}
	})

	t.Run("Reset menghapus faktor dan recovery code", func(t *testing.T) {
		if err := svc.Reset(ctx, "cli:core:reset-mfa", u.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := st.GetMFAFactor(context.Background(), tenantA, u.ID); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("faktor setelah reset: %v", err)
		}
		if n, _ := st.CountUnusedRecoveryCodes(context.Background(), tenantA, u.ID); n != 0 {
			t.Errorf("recovery code tersisa: %d", n)
		}
		if err := svc.Verify(ctx, u.ID, mfa.TOTP{}.Code(sec, time.Now().Add(2*mfa.Period*time.Second))); !errors.Is(err, mfa.ErrInvalidCode) {
			t.Errorf("Verify setelah reset = %v", err)
		}
	})
}

// confirmHookStore menjalankan hook tepat sebelum ConfirmMFAFactor (setelah faktor dimuat dan kodenya diverifikasi).
type confirmHookStore struct {
	*store.Store
	before func()
}

func (h *confirmHookStore) ConfirmMFAFactor(ctx context.Context, t tenancy.TenantID, userID string, sealed []byte, step int64, at time.Time) (bool, error) {
	if h.before != nil {
		f := h.before
		h.before = nil
		f()
	}
	return h.Store.ConfirmMFAFactor(ctx, t, userID, sealed, step, at)
}

// M2: pendaftaran ulang (secret B) tepat di antara verifikasi kode (secret A) dan penetapan konfirmasi tidak boleh
// membuat faktor aktif dengan secret B yang tak pernah dibuktikan pemiliknya (CAS pada secret_sealed).
func TestIntegrationConfirmBoundToVerifiedSecretAgainstConcurrentBegin(t *testing.T) {
	_, st, _, seal := setup(t)
	hook := &confirmHookStore{Store: st}
	svc, err := mfa.New(mfa.Deps{Store: hook, Sealer: seal, Lockout: noLockout{}, Issuer: "ERPora"})
	if err != nil {
		t.Fatal(err)
	}
	u := newUser(t, st, "race@example.test")
	ctx := ctxFor(tenantA)

	infoA, err := svc.BeginEnrollment(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	secA, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(infoA.Secret)
	var infoB mfa.EnrollmentInfo
	hook.before = func() {
		var berr error
		if infoB, berr = svc.BeginEnrollment(ctx, u.ID); berr != nil {
			t.Error(berr)
		}
	}
	if _, err := svc.ConfirmEnrollment(ctx, u.ID, mfa.TOTP{}.Code(secA, time.Now())); !errors.Is(err, mfa.ErrInvalidCode) {
		t.Fatalf("konfirmasi atas secret yang sudah diganti = %v, want ErrInvalidCode", err)
	}
	f, err := st.GetMFAFactor(context.Background(), tenantA, u.ID)
	if err != nil || f.ConfirmedAt != nil {
		t.Fatalf("faktor tidak boleh aktif: %+v %v", f, err)
	}
	if n, _ := st.CountUnusedRecoveryCodes(context.Background(), tenantA, u.ID); n != 0 {
		t.Fatalf("recovery code terbit: %d", n)
	}
	// secret terbaru (B) tetap dapat dikonfirmasi dengan kodenya sendiri
	secB, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(infoB.Secret)
	if _, err := svc.ConfirmEnrollment(ctx, u.ID, mfa.TOTP{}.Code(secB, time.Now())); err != nil {
		t.Fatalf("konfirmasi secret terbaru: %v", err)
	}
}
