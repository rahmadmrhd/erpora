//go:build integration

package store_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	"erpora/internal/platform/config"
	"erpora/internal/platform/database"
	"erpora/internal/platform/identity/store"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/tenancy"
	"erpora/internal/platform/testsupport/pgtest"
)

var (
	tenantA = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"))
	tenantB = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c"))
)

// bothTenants memetakan kedua tenant uji ke database yang sama (mode single-tenant fisik), agar peran di tenant
// DB dapat diuji bersama isolasi tenant pada tabel control.
type bothTenants struct{ db database.TenantDB }

func (c bothTenants) For(_ context.Context, id tenancy.TenantID) (database.TenantDB, error) {
	if id != tenantA && id != tenantB {
		return database.TenantDB{}, database.ErrUnknownTenant
	}
	return c.db, nil
}

func setup(t *testing.T) (*gorm.DB, *store.Store) {
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
	return db, store.New(database.ControlDB{DB: db}, bothTenants{db: database.TenantDB{DB: db}})
}

var ctx = context.Background()

func now() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

func hash(b byte) []byte { h := make([]byte, 32); h[0] = b; return h }

func newUser(t *testing.T, s *store.Store, tn tenancy.TenantID, email string) store.User {
	t.Helper()
	u, err := s.CreateUser(ctx, tn, store.NewUser{Email: email, Name: "Nama " + email, PasswordHash: "$argon2id$stub"})
	if err != nil {
		t.Fatalf("CreateUser(%s): %v", email, err)
	}
	return u
}

func newSession(t *testing.T, s *store.Store, tn tenancy.TenantID, userID string) (store.Session, store.RefreshToken) {
	t.Helper()
	c := now()
	sess := store.Session{
		ID: uuid.NewString(), UserID: userID, ClientKind: "web", AuthMethod: "password", AMR: []string{"pwd"},
		UserAgent: "uji", IP: "203.0.113.7", CreatedAt: c, LastSeenAt: c, AbsoluteExpiresAt: c.Add(90 * 24 * time.Hour),
	}
	rt := store.RefreshToken{
		ID: uuid.NewString(), SessionID: sess.ID, FamilyID: uuid.NewString(), TokenHash: hash(byte(time.Now().UnixNano())),
		CreatedAt: c, ExpiresAt: c.Add(14 * 24 * time.Hour),
	}
	// hash unik per pemanggilan
	copy(rt.TokenHash[1:], uuid.NewString())
	if err := s.CreateSessionWithRefresh(ctx, tn, sess, rt); err != nil {
		t.Fatalf("CreateSessionWithRefresh: %v", err)
	}
	return sess, rt
}

// race menjalankan n goroutine serentak (setelah palang) dan mengumpulkan hasil bool-nya.
func race(n int, f func(i int) (bool, error)) (wins int, errs []error) {
	var wg sync.WaitGroup
	start := make(chan struct{})
	var mu sync.Mutex
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			ok, err := f(i)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
			} else if ok {
				wins++
			}
		}(i)
	}
	close(start)
	wg.Wait()
	return wins, errs
}

func TestUsers(t *testing.T) {
	_, s := setup(t)

	t.Run("buat, ambil, dan normalisasi email", func(t *testing.T) {
		u, err := s.CreateUser(ctx, tenantA, store.NewUser{Email: "  Admin@Example.COM ", Name: "Admin", PasswordHash: "$h"})
		if err != nil {
			t.Fatal(err)
		}
		if u.Email != "admin@example.com" || u.Status != store.UserActive || u.PasswordHash != "$h" || u.ID == "" {
			t.Fatalf("user = %+v", u)
		}
		if u.CreatedAt.Location() != time.UTC {
			t.Errorf("CreatedAt tidak UTC: %v", u.CreatedAt.Location())
		}
		got, err := s.GetUser(ctx, tenantA, u.ID)
		if err != nil || got.ID != u.ID {
			t.Fatalf("GetUser = %+v, %v", got, err)
		}
		byEmail, err := s.FindUserByEmail(ctx, tenantA, "ADMIN@example.com ")
		if err != nil || byEmail.ID != u.ID {
			t.Fatalf("FindUserByEmail = %+v, %v", byEmail, err)
		}
		m, err := s.GetMembership(ctx, tenantA, u.ID)
		if err != nil || m.Status != store.MembershipActive {
			t.Fatalf("GetMembership = %+v, %v", m, err)
		}
	})

	t.Run("email unik ternormalisasi", func(t *testing.T) {
		newUser(t, s, tenantA, "uniq@example.com")
		for _, dup := range []string{"uniq@example.com", "UNIQ@Example.com", "  uniq@example.com\t"} {
			if _, err := s.CreateUser(ctx, tenantA, store.NewUser{Email: dup, Name: "X"}); !errors.Is(err, store.ErrEmailTaken) {
				t.Errorf("CreateUser(%q) err = %v, want ErrEmailTaken", dup, err)
			}
		}
		// email unik lintas tenant (identitas global): membuat di tenant B juga ditolak
		if _, err := s.CreateUser(ctx, tenantB, store.NewUser{Email: "uniq@example.com", Name: "X"}); !errors.Is(err, store.ErrEmailTaken) {
			t.Errorf("tenant lain: err = %v, want ErrEmailTaken", err)
		}
		// pembuatan serentak: tepat satu menang, tanpa baris yatim
		wins, errs := race(8, func(int) (bool, error) {
			_, err := s.CreateUser(ctx, tenantA, store.NewUser{Email: "race@example.com", Name: "R"})
			if errors.Is(err, store.ErrEmailTaken) {
				return false, nil
			}
			return err == nil, err
		})
		if wins != 1 || len(errs) != 0 {
			t.Errorf("pembuatan serentak: wins=%d errs=%v, want 1 menang", wins, errs)
		}
	})

	t.Run("input tidak valid ditolak", func(t *testing.T) {
		for _, in := range []store.NewUser{{Email: "", Name: "X"}, {Email: "tanpa-at", Name: "X"}, {Email: "a@b.co", Name: ""}} {
			if _, err := s.CreateUser(ctx, tenantA, in); !errors.Is(err, store.ErrInvalidArgument) {
				t.Errorf("CreateUser(%+v) err = %v, want ErrInvalidArgument", in, err)
			}
		}
	})

	t.Run("isolasi tenant (IDOR)", func(t *testing.T) {
		u := newUser(t, s, tenantA, "idor@example.com")
		if _, err := s.GetUser(ctx, tenantB, u.ID); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("GetUser dari tenant B: err = %v, want ErrNotFound", err)
		}
		if _, err := s.FindUserByEmail(ctx, tenantB, "idor@example.com"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("FindUserByEmail dari tenant B: err = %v, want ErrNotFound", err)
		}
		if _, err := s.GetMembership(ctx, tenantB, u.ID); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("GetMembership dari tenant B: err = %v, want ErrNotFound", err)
		}
		if err := s.SetPasswordHash(ctx, tenantB, u.ID, "$evil"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("SetPasswordHash dari tenant B: err = %v, want ErrNotFound", err)
		}
		got, _ := s.GetUser(ctx, tenantA, u.ID)
		if got.PasswordHash != "$argon2id$stub" {
			t.Errorf("hash sandi berubah lewat tenant lain: %q", got.PasswordHash)
		}
		// setelah membership ditambahkan di tenant B, user terlihat di sana
		if created, err := s.AddMembership(ctx, tenantB, u.ID, store.MembershipActive); err != nil || !created {
			t.Fatalf("AddMembership = %v, %v", created, err)
		}
		if created, err := s.AddMembership(ctx, tenantB, u.ID, store.MembershipActive); err != nil || created {
			t.Fatalf("AddMembership ulang = %v, %v, want created=false", created, err)
		}
		if _, err := s.GetUser(ctx, tenantB, u.ID); err != nil {
			t.Errorf("GetUser setelah membership: %v", err)
		}
	})

	t.Run("sandi, login terakhir, status, kunci", func(t *testing.T) {
		u := newUser(t, s, tenantA, "state@example.com")
		if err := s.SetPasswordHash(ctx, tenantA, u.ID, "$new"); err != nil {
			t.Fatal(err)
		}
		at := now()
		if err := s.TouchLastLogin(ctx, tenantA, u.ID, at); err != nil {
			t.Fatal(err)
		}
		got, _ := s.GetUser(ctx, tenantA, u.ID)
		if got.PasswordHash != "$new" || got.LastLoginAt == nil || !got.LastLoginAt.Equal(at) {
			t.Fatalf("user = %+v", got)
		}
		if err := s.SetUserStatus(ctx, tenantA, u.ID, store.UserDisabled); err != nil {
			t.Fatal(err)
		}
		if got, _ = s.GetUser(ctx, tenantA, u.ID); got.Status != store.UserDisabled {
			t.Errorf("status = %s", got.Status)
		}
		if err := s.SetUserStatus(ctx, tenantA, u.ID, "bogus"); !errors.Is(err, store.ErrInvalidArgument) {
			t.Errorf("status ilegal: err = %v, want ErrInvalidArgument", err)
		}
		if err := s.SetUserStatus(ctx, tenantA, u.ID, store.UserActive); err != nil {
			t.Fatal(err)
		}
		// LockUser bersyarat: hanya dari active, dan hanya sekali
		if ok, err := s.LockUser(ctx, tenantA, u.ID, at); err != nil || !ok {
			t.Fatalf("LockUser = %v, %v", ok, err)
		}
		if ok, err := s.LockUser(ctx, tenantA, u.ID, at); err != nil || ok {
			t.Fatalf("LockUser ulang = %v, %v, want false", ok, err)
		}
		if got, _ = s.GetUser(ctx, tenantA, u.ID); got.Status != store.UserLocked || got.LockedAt == nil {
			t.Fatalf("setelah lock: %+v", got)
		}
		// UnlockUser: kembali active dan menghapus catatan percobaan (jendela lintas IP tidak mengunci ulang)
		for i := 0; i < 3; i++ {
			if err := s.RecordLoginAttempt(ctx, tenantA, u.ID, "198.51.100.1", store.AttemptPassword, false, at); err != nil {
				t.Fatal(err)
			}
		}
		if ok, err := s.UnlockUser(ctx, tenantA, u.ID); err != nil || !ok {
			t.Fatalf("UnlockUser = %v, %v", ok, err)
		}
		if ok, err := s.UnlockUser(ctx, tenantA, u.ID); err != nil || ok {
			t.Fatalf("UnlockUser pada akun tidak terkunci = %v, %v, want false", ok, err)
		}
		got, _ = s.GetUser(ctx, tenantA, u.ID)
		if got.Status != store.UserActive || got.LockedAt != nil {
			t.Errorf("setelah unlock: %+v", got)
		}
		st, err := s.CrossIPFailures(ctx, tenantA, u.ID, at.Add(-time.Hour))
		if err != nil || st.Failures != 0 {
			t.Errorf("percobaan setelah unlock = %+v, %v, want 0", st, err)
		}
		// tenant lain tidak dapat mengunci/membuka
		if ok, err := s.LockUser(ctx, tenantB, u.ID, at); err != nil || ok {
			t.Errorf("LockUser dari tenant B = %v, %v, want false", ok, err)
		}
	})

	t.Run("UnlockUser selalu menghapus catatan percobaan, idempoten, terbatas pada pengguna dan tenant", func(t *testing.T) {
		at := now()
		blocked := newUser(t, s, tenantA, "pairblocked@example.com")
		bystander := newUser(t, s, tenantA, "bystander@example.com")
		const ip = "198.51.100.77"
		for i := 0; i < 25; i++ {
			_ = s.RecordLoginAttempt(ctx, tenantA, blocked.ID, ip, store.AttemptPassword, false, at)
		}
		_ = s.RecordLoginAttempt(ctx, tenantA, bystander.ID, ip, store.AttemptPassword, false, at)
		if st, _ := s.PairFailureState(ctx, tenantA, blocked.ID, ip); st.ConsecutiveFailures != 25 {
			t.Fatalf("persiapan: %+v", st)
		}
		// akun masih active (hanya pasangan yang terblokir): UnlockUser tetap membersihkan pasangan
		if ok, err := s.UnlockUser(ctx, tenantA, blocked.ID); err != nil || ok {
			t.Fatalf("UnlockUser pada akun active = %v, %v, want false (status tidak berubah, tanpa galat)", ok, err)
		}
		if st, _ := s.PairFailureState(ctx, tenantA, blocked.ID, ip); st.ConsecutiveFailures != 0 || st.LastFailureAt != nil {
			t.Errorf("pasangan setelah UnlockUser = %+v, want bersih", st)
		}
		if st, _ := s.CrossIPFailures(ctx, tenantA, blocked.ID, at.Add(-time.Hour)); st.Failures != 0 {
			t.Errorf("lintas IP setelah UnlockUser = %+v, want 0", st)
		}
		got, _ := s.GetUser(ctx, tenantA, blocked.ID)
		if got.Status != store.UserActive {
			t.Errorf("status = %s", got.Status)
		}
		// pengguna lain tidak terpengaruh
		if st, _ := s.PairFailureState(ctx, tenantA, bystander.ID, ip); st.ConsecutiveFailures != 1 {
			t.Errorf("pengguna lain = %+v, want 1", st)
		}
		// idempoten
		if ok, err := s.UnlockUser(ctx, tenantA, blocked.ID); err != nil || ok {
			t.Errorf("UnlockUser ulang = %v, %v", ok, err)
		}
		// akun locked tetap terbuka (dan true)
		_ = s.RecordLoginAttempt(ctx, tenantA, blocked.ID, ip, store.AttemptPassword, false, at)
		if ok, _ := s.LockUser(ctx, tenantA, blocked.ID, at); !ok {
			t.Fatal("lock gagal")
		}
		if ok, err := s.UnlockUser(ctx, tenantA, blocked.ID); err != nil || !ok {
			t.Fatalf("UnlockUser akun locked = %v, %v, want true", ok, err)
		}
		if st, _ := s.PairFailureState(ctx, tenantA, blocked.ID, ip); st.ConsecutiveFailures != 0 {
			t.Errorf("setelah membuka locked = %+v", st)
		}
		// tenant lain tidak dapat membersihkan catatan tenant A (non-anggota di tenant B)
		_ = s.RecordLoginAttempt(ctx, tenantA, bystander.ID, ip, store.AttemptPassword, false, at)
		if _, err := s.UnlockUser(ctx, tenantB, bystander.ID); err != nil {
			t.Fatal(err)
		}
		if st, _ := s.PairFailureState(ctx, tenantA, bystander.ID, ip); st.ConsecutiveFailures != 2 {
			t.Errorf("tenant B menghapus catatan tenant A: %+v, want 2", st)
		}
		// ID bukan UUID: tanpa efek, tanpa galat
		if ok, err := s.UnlockUser(ctx, tenantA, "bukan-uuid"); err != nil || ok {
			t.Errorf("UnlockUser(bukan-uuid) = %v, %v", ok, err)
		}
	})

	t.Run("preferensi", func(t *testing.T) {
		u := newUser(t, s, tenantA, "pref@example.com")
		p, err := s.GetPreferences(ctx, tenantA, u.ID)
		if err != nil || p.Locale != "" || p.Timezone != "" {
			t.Fatalf("preferensi awal = %+v, %v, want kosong", p, err)
		}
		if err := s.UpsertPreferences(ctx, tenantA, u.ID, "id", "Asia/Jakarta"); err != nil {
			t.Fatal(err)
		}
		if err := s.UpsertPreferences(ctx, tenantA, u.ID, "en", "UTC"); err != nil {
			t.Fatal(err)
		}
		if p, _ = s.GetPreferences(ctx, tenantA, u.ID); p.Locale != "en" || p.Timezone != "UTC" {
			t.Errorf("preferensi = %+v", p)
		}
		if err := s.UpsertPreferences(ctx, tenantB, u.ID, "xx", "UTC"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("Upsert dari tenant B: err = %v, want ErrNotFound", err)
		}
		if p, _ = s.GetPreferences(ctx, tenantA, u.ID); p.Locale != "en" {
			t.Errorf("preferensi berubah lewat tenant lain: %+v", p)
		}
	})
}

func TestSigningKeys(t *testing.T) {
	_, s := setup(t)
	created := now()
	pub, priv := bytes.Repeat([]byte{7}, 32), []byte("sealed-private-blob")

	k1 := store.SigningKey{ID: "kid-0000000001", Status: store.KeyActive, PublicKey: pub, PrivateKeySealed: priv, CreatedAt: created}
	if err := s.InsertSigningKey(ctx, k1); err != nil {
		t.Fatal(err)
	}
	k2 := store.SigningKey{ID: "kid-0000000002", Status: store.KeyPending, PublicKey: pub, PrivateKeySealed: priv, CreatedAt: created.Add(time.Second)}
	if err := s.InsertSigningKey(ctx, k2); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertSigningKey(ctx, k2); !errors.Is(err, store.ErrConflict) {
		t.Errorf("kid duplikat: err = %v, want ErrConflict", err)
	}
	bad := k2
	bad.ID = "../etc/passwd"
	if err := s.InsertSigningKey(ctx, bad); !errors.Is(err, store.ErrInvalidArgument) {
		t.Errorf("kid berpath: err = %v, want ErrInvalidArgument", err)
	}
	bad = k2
	bad.ID, bad.Status = "kid-0000000003", "enabled"
	if err := s.InsertSigningKey(ctx, bad); !errors.Is(err, store.ErrInvalidArgument) {
		t.Errorf("status ilegal: err = %v, want ErrInvalidArgument", err)
	}

	list, err := s.ListSigningKeys(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("ListSigningKeys = %+v, %v", list, err)
	}
	if list[0].ID != k1.ID || list[1].ID != k2.ID {
		t.Errorf("urutan = %s, %s; want created_at naik", list[0].ID, list[1].ID)
	}
	if list[0].Status != store.KeyActive || list[0].ActivatedAt == nil || !list[0].ActivatedAt.Equal(created) {
		t.Errorf("k1 = %+v (active wajib ber-ActivatedAt, bawaan CreatedAt)", list[0])
	}
	if !bytes.Equal(list[0].PublicKey, pub) || !bytes.Equal(list[0].PrivateKeySealed, priv) || !list[0].CreatedAt.Equal(created) {
		t.Errorf("round trip rusak: %+v", list[0])
	}
	if list[1].ActivatedAt != nil || list[1].RetiringAt != nil || list[1].RetiredAt != nil {
		t.Errorf("k2 pending = %+v", list[1])
	}

	at := created.Add(5 * time.Minute)
	// from tidak cocok: false tanpa efek
	if ok, err := s.TransitionSigningKey(ctx, k2.ID, store.KeyActive, store.KeyRetiring, at); err != nil || ok {
		t.Fatalf("transisi dari status salah = %v, %v, want false", ok, err)
	}
	// tepi ilegal: galat, bukan false
	if _, err := s.TransitionSigningKey(ctx, k2.ID, store.KeyPending, store.KeyRetiring, at); !errors.Is(err, store.ErrInvalidTransition) {
		t.Errorf("tepi ilegal: err = %v, want ErrInvalidTransition", err)
	}
	if ok, err := s.TransitionSigningKey(ctx, "kid-tidakada-001", store.KeyPending, store.KeyActive, at); err != nil || ok {
		t.Errorf("kid tak ada = %v, %v, want false", ok, err)
	}
	// promosi serentak: tepat satu menang
	wins, errs := race(8, func(int) (bool, error) {
		return s.TransitionSigningKey(ctx, k2.ID, store.KeyPending, store.KeyActive, at)
	})
	if wins != 1 || len(errs) != 0 {
		t.Fatalf("promosi serentak: wins=%d errs=%v", wins, errs)
	}
	if ok, err := s.TransitionSigningKey(ctx, k1.ID, store.KeyActive, store.KeyRetiring, at.Add(time.Second)); err != nil || !ok {
		t.Fatalf("active->retiring = %v, %v", ok, err)
	}
	if ok, err := s.TransitionSigningKey(ctx, k1.ID, store.KeyRetiring, store.KeyRetired, at.Add(time.Minute)); err != nil || !ok {
		t.Fatalf("retiring->retired = %v, %v", ok, err)
	}
	list, _ = s.ListSigningKeys(ctx)
	byID := map[string]store.SigningKey{}
	for _, k := range list {
		byID[k.ID] = k
	}
	if k := byID[k2.ID]; k.Status != store.KeyActive || k.ActivatedAt == nil || !k.ActivatedAt.Equal(at) {
		t.Errorf("k2 = %+v, want active @ %v", k, at)
	}
	if k := byID[k1.ID]; k.Status != store.KeyRetired || k.RetiringAt == nil || !k.RetiringAt.Equal(at.Add(time.Second)) ||
		k.RetiredAt == nil || !k.RetiredAt.Equal(at.Add(time.Minute)) {
		t.Errorf("k1 = %+v", k)
	}
	// kunci retired tidak dapat dihidupkan lagi
	if _, err := s.TransitionSigningKey(ctx, k1.ID, store.KeyRetired, store.KeyActive, at); !errors.Is(err, store.ErrInvalidTransition) {
		t.Errorf("retired->active: err = %v, want ErrInvalidTransition", err)
	}
}

func TestSessionsAndRefreshTokens(t *testing.T) {
	_, s := setup(t)
	ua := newUser(t, s, tenantA, "sess-a@example.com")
	ub := newUser(t, s, tenantB, "sess-b@example.com")

	t.Run("buat sesi, cari token lewat hash, round trip", func(t *testing.T) {
		sess, rt := newSession(t, s, tenantA, ua.ID)
		got, err := s.GetSession(ctx, tenantA, sess.ID)
		if err != nil || got.UserID != ua.ID || got.ClientKind != "web" || len(got.AMR) != 1 || got.AMR[0] != "pwd" ||
			got.RevokedAt != nil || !got.AbsoluteExpiresAt.Equal(sess.AbsoluteExpiresAt) || got.IP != "203.0.113.7" {
			t.Fatalf("GetSession = %+v, %v", got, err)
		}
		f, err := s.FindRefreshTokenByHash(ctx, tenantA, rt.TokenHash)
		if err != nil || f.ID != rt.ID || f.SessionID != sess.ID || f.FamilyID != rt.FamilyID || f.ParentID != "" || f.UsedAt != nil {
			t.Fatalf("FindRefreshTokenByHash = %+v, %v", f, err)
		}
		if _, err := s.FindRefreshTokenByHash(ctx, tenantA, hash(250)); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("hash tak dikenal: err = %v, want ErrNotFound", err)
		}
		if err := s.TouchSession(ctx, tenantA, sess.ID, sess.CreatedAt.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
		got, _ = s.GetSession(ctx, tenantA, sess.ID)
		if !got.LastSeenAt.Equal(sess.CreatedAt.Add(time.Minute)) {
			t.Errorf("LastSeenAt = %v", got.LastSeenAt)
		}
		if ok, err := s.SetSessionAMR(ctx, tenantA, sess.ID, []string{"pwd", "otp"}); err != nil || !ok {
			t.Fatalf("SetSessionAMR = %v, %v", ok, err)
		}
		got, _ = s.GetSession(ctx, tenantA, sess.ID)
		if len(got.AMR) != 2 || got.AMR[1] != "otp" {
			t.Errorf("AMR = %v", got.AMR)
		}
		if _, err := s.SetSessionAMR(ctx, tenantA, sess.ID, nil); !errors.Is(err, store.ErrInvalidArgument) {
			t.Errorf("AMR kosong: err = %v, want ErrInvalidArgument", err)
		}
	})

	t.Run("sesi dan token atomik: kegagalan token membatalkan sesi", func(t *testing.T) {
		_, existing := newSession(t, s, tenantA, ua.ID)
		c := now()
		sess := store.Session{ID: uuid.NewString(), UserID: ua.ID, ClientKind: "web", AuthMethod: "password", AMR: []string{"pwd"},
			CreatedAt: c, LastSeenAt: c, AbsoluteExpiresAt: c.Add(time.Hour)}
		// hash token sudah dipakai token lain: pelanggaran UNIQUE setelah sesi tersimpan, transaksi harus rollback
		rt := store.RefreshToken{ID: uuid.NewString(), SessionID: sess.ID, FamilyID: uuid.NewString(),
			TokenHash: existing.TokenHash, CreatedAt: c, ExpiresAt: c.Add(time.Hour)}
		if err := s.CreateSessionWithRefresh(ctx, tenantA, sess, rt); err == nil {
			t.Fatal("harus gagal karena hash token duplikat")
		}
		if _, err := s.GetSession(ctx, tenantA, sess.ID); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("sesi tidak boleh tersisa setelah rollback: err = %v", err)
		}
		rt.TokenHash = hash(211)
		rt.SessionID = uuid.NewString()
		if err := s.CreateSessionWithRefresh(ctx, tenantA, sess, rt); !errors.Is(err, store.ErrInvalidArgument) {
			t.Errorf("token menunjuk sesi lain: err = %v, want ErrInvalidArgument", err)
		}
	})

	t.Run("used_at tidak dapat diset dua kali, serentak tepat satu menang", func(t *testing.T) {
		_, rt := newSession(t, s, tenantA, ua.ID)
		wins, errs := race(10, func(int) (bool, error) { return s.MarkRefreshTokenUsed(ctx, tenantA, rt.ID, now()) })
		if wins != 1 || len(errs) != 0 {
			t.Fatalf("MarkRefreshTokenUsed serentak: wins=%d errs=%v", wins, errs)
		}
		if ok, err := s.MarkRefreshTokenUsed(ctx, tenantA, rt.ID, now()); err != nil || ok {
			t.Errorf("ulang = %v, %v, want false", ok, err)
		}
		f, _ := s.FindRefreshTokenByHash(ctx, tenantA, rt.TokenHash)
		if f.UsedAt == nil {
			t.Error("UsedAt kosong")
		}
	})

	t.Run("rotasi atomik: pemenang menerbitkan anak, kalah tanpa efek", func(t *testing.T) {
		sess, root := newSession(t, s, tenantA, ua.ID)
		wins, errs := race(10, func(i int) (bool, error) {
			child := store.RefreshToken{ID: uuid.NewString(), SessionID: sess.ID, FamilyID: root.FamilyID, ParentID: root.ID,
				TokenHash: hash(byte(100 + i)), CreatedAt: now(), ExpiresAt: now().Add(14 * 24 * time.Hour)}
			return s.RotateRefreshToken(ctx, tenantA, root.ID, now(), child)
		})
		if wins != 1 || len(errs) != 0 {
			t.Fatalf("rotasi serentak: wins=%d errs=%v", wins, errs)
		}
		n := 0
		for i := 0; i < 10; i++ {
			if f, err := s.FindRefreshTokenByHash(ctx, tenantA, hash(byte(100+i))); err == nil {
				n++
				if f.ParentID != root.ID || f.FamilyID != root.FamilyID || f.UsedAt != nil {
					t.Errorf("anak = %+v", f)
				}
			}
		}
		if n != 1 {
			t.Errorf("jumlah anak yang diterbitkan = %d, want 1", n)
		}
		// anak dengan family/sesi tidak konsisten ditolak DB dan rotasi dibatalkan (induk tetap belum terpakai)
		_, root2 := newSession(t, s, tenantA, ua.ID)
		bad := store.RefreshToken{ID: uuid.NewString(), SessionID: root2.SessionID, FamilyID: uuid.NewString(), ParentID: root2.ID,
			TokenHash: hash(230), CreatedAt: now(), ExpiresAt: now().Add(time.Hour)}
		if ok, err := s.RotateRefreshToken(ctx, tenantA, root2.ID, now(), bad); err == nil || ok {
			t.Fatalf("rotasi dengan family tak konsisten = %v, %v, want galat", ok, err)
		}
		f, _ := s.FindRefreshTokenByHash(ctx, tenantA, root2.TokenHash)
		if f.UsedAt != nil {
			t.Error("rotasi gagal tidak boleh meninggalkan induk berstatus terpakai (transaksi harus rollback)")
		}
	})

	t.Run("pencabutan sesi, family, dan seluruh sesi pengguna", func(t *testing.T) {
		pu := newUser(t, s, tenantA, "revoke@example.com")
		s1, r1 := newSession(t, s, tenantA, pu.ID)
		s2, r2 := newSession(t, s, tenantA, pu.ID)
		s3, _ := newSession(t, s, tenantA, pu.ID)
		at := now()
		if ok, err := s.RevokeSession(ctx, tenantA, s1.ID, "logout", at); err != nil || !ok {
			t.Fatalf("RevokeSession = %v, %v", ok, err)
		}
		if ok, err := s.RevokeSession(ctx, tenantA, s1.ID, "logout", at); err != nil || ok {
			t.Fatalf("RevokeSession ulang = %v, %v, want false", ok, err)
		}
		g, _ := s.GetSession(ctx, tenantA, s1.ID)
		if g.RevokedAt == nil || g.RevokeReason != "logout" {
			t.Errorf("sesi tercabut = %+v", g)
		}
		ids, err := s.RevokeSessionsOfFamily(ctx, tenantA, r1.FamilyID, "refresh_reuse", at)
		if err != nil || len(ids) != 0 {
			t.Errorf("family sesi yang sudah tercabut: %v, %v, want kosong (sudah tercabut)", ids, err)
		}
		ids, err = s.RevokeSessionsOfFamily(ctx, tenantA, r2.FamilyID, "refresh_reuse", at)
		if err != nil || len(ids) != 1 || ids[0] != s2.ID {
			t.Errorf("RevokeSessionsOfFamily = %v, %v, want [%s]", ids, err, s2.ID)
		}
		ids, err = s.RevokeUserSessions(ctx, tenantA, pu.ID, s3.ID, "password_changed", at)
		if err != nil || len(ids) != 0 {
			t.Errorf("RevokeUserSessions kecuali s3 = %v, %v (s1,s2 sudah tercabut)", ids, err)
		}
		s4, _ := newSession(t, s, tenantA, pu.ID)
		ids, err = s.RevokeUserSessions(ctx, tenantA, pu.ID, s3.ID, "password_changed", at)
		if err != nil || len(ids) < 1 || !contains(ids, s4.ID) || contains(ids, s3.ID) {
			t.Errorf("RevokeUserSessions = %v, %v; s4 harus tercabut dan s3 (dikecualikan) tidak", ids, err)
		}
		if g, _ = s.GetSession(ctx, tenantA, s3.ID); g.RevokedAt != nil {
			t.Error("sesi yang dikecualikan tercabut")
		}
		rev, err := s.ListRevokedSessionsSince(ctx, tenantA, at.Add(-time.Minute))
		if err != nil || !contains(revIDs(rev), s1.ID) || !contains(revIDs(rev), s4.ID) || contains(revIDs(rev), s3.ID) {
			t.Errorf("ListRevokedSessionsSince = %v, %v", revIDs(rev), err)
		}
		if rev, _ = s.ListRevokedSessionsSince(ctx, tenantA, at.Add(time.Minute)); len(rev) != 0 {
			t.Errorf("sesudah cutoff harus kosong: %v", revIDs(rev))
		}
		act, err := s.ListUserSessions(ctx, tenantA, pu.ID)
		if err != nil || !contains(sessIDs(act), s3.ID) || contains(sessIDs(act), s1.ID) {
			t.Errorf("ListUserSessions = %v, %v", sessIDs(act), err)
		}
	})

	t.Run("UpgradeSession: CAS atomik, token belum terpakai diganti, family dipertahankan", func(t *testing.T) {
		uu := newUser(t, s, tenantA, "upgrade@example.com")
		sess, root := newSession(t, s, tenantA, uu.ID)
		// satu rotasi dulu: token saat ini adalah anak (belum terpakai), induk berstatus terpakai
		child := store.RefreshToken{ID: uuid.NewString(), SessionID: sess.ID, FamilyID: root.FamilyID, ParentID: root.ID,
			TokenHash: hash(150), CreatedAt: now(), ExpiresAt: now().Add(14 * 24 * time.Hour)}
		if ok, err := s.RotateRefreshToken(ctx, tenantA, root.ID, now(), child); err != nil || !ok {
			t.Fatalf("rotasi: %v %v", ok, err)
		}
		mk := func(b byte) store.RefreshToken {
			return store.RefreshToken{ID: uuid.NewString(), SessionID: sess.ID, FamilyID: uuid.NewString(),
				TokenHash: hash(b), CreatedAt: now(), ExpiresAt: now().Add(14 * 24 * time.Hour)}
		}
		// AMR lama tidak cocok (CAS): kalah tanpa efek
		if ok, err := s.UpgradeSession(ctx, tenantA, sess.ID, []string{"pwd", "otp"}, []string{"pwd", "otp"}, mk(160)); err != nil || ok {
			t.Fatalf("CAS salah = %v, %v, want false", ok, err)
		}
		if _, err := s.FindRefreshTokenByHash(ctx, tenantA, hash(150)); err != nil {
			t.Fatalf("token saat ini tidak boleh hilang saat upgrade kalah: %v", err)
		}
		// pemanggil lain di tenant B tidak dapat menaikkan sesi tenant A
		if ok, err := s.UpgradeSession(ctx, tenantB, sess.ID, []string{"pwd"}, []string{"pwd", "otp"}, mk(161)); err != nil || ok {
			t.Fatalf("tenant B = %v, %v, want false", ok, err)
		}
		next := mk(162)
		if ok, err := s.UpgradeSession(ctx, tenantA, sess.ID, []string{"pwd"}, []string{"pwd", "otp"}, next); err != nil || !ok {
			t.Fatalf("UpgradeSession = %v, %v", ok, err)
		}
		g, _ := s.GetSession(ctx, tenantA, sess.ID)
		if len(g.AMR) != 2 || g.AMR[1] != "otp" || g.RevokedAt != nil {
			t.Errorf("sesi = %+v (harus [pwd otp] dan tetap hidup)", g)
		}
		// token yang belum terpakai (anak) dihapus: lama ditolak sebagai tak dikenal (bukan reuse); yang baru satu family
		if _, err := s.FindRefreshTokenByHash(ctx, tenantA, hash(150)); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("token lama harus hilang: %v", err)
		}
		f, err := s.FindRefreshTokenByHash(ctx, tenantA, hash(162))
		if err != nil || f.FamilyID != root.FamilyID || f.ParentID != "" || f.UsedAt != nil || f.SessionID != sess.ID {
			t.Errorf("token baru = %+v, %v (family harus dipertahankan)", f, err)
		}
		// leluhur yang sudah terpakai tetap ada (deteksi reuse tetap bekerja untuk token lama dari rotasi sebelumnya)
		if r, err := s.FindRefreshTokenByHash(ctx, tenantA, root.TokenHash); err != nil || r.UsedAt == nil {
			t.Errorf("leluhur terpakai = %+v, %v", r, err)
		}
		// sesi tercabut tidak dapat dinaikkan
		if _, err := s.RevokeSession(ctx, tenantA, sess.ID, "logout", now()); err != nil {
			t.Fatal(err)
		}
		if ok, err := s.UpgradeSession(ctx, tenantA, sess.ID, []string{"pwd", "otp"}, []string{"pwd", "otp", "x"}, mk(163)); err != nil || ok {
			t.Errorf("sesi tercabut = %v, %v, want false", ok, err)
		}
	})

	t.Run("UpgradeSession serentak: tepat satu menang", func(t *testing.T) {
		uu := newUser(t, s, tenantA, "upgrade-race@example.com")
		sess, _ := newSession(t, s, tenantA, uu.ID)
		wins, errs := race(10, func(i int) (bool, error) {
			next := store.RefreshToken{ID: uuid.NewString(), SessionID: sess.ID, FamilyID: uuid.NewString(),
				TokenHash: hash(byte(170 + i)), CreatedAt: now(), ExpiresAt: now().Add(time.Hour)}
			return s.UpgradeSession(ctx, tenantA, sess.ID, []string{"pwd"}, []string{"pwd", "otp"}, next)
		})
		if wins != 1 || len(errs) != 0 {
			t.Fatalf("upgrade serentak: wins=%d errs=%v", wins, errs)
		}
		live := 0
		for i := 0; i < 10; i++ {
			if f, err := s.FindRefreshTokenByHash(ctx, tenantA, hash(byte(170+i))); err == nil && f.UsedAt == nil {
				live++
			}
		}
		if live != 1 {
			t.Errorf("token hidup setelah balapan = %d, want 1", live)
		}
	})

	t.Run("CountUserSessions menghitung semua sesi (juga yang tercabut), terisolasi tenant", func(t *testing.T) {
		uu := newUser(t, s, tenantA, "count-sessions@example.com")
		if n, err := s.CountUserSessions(ctx, tenantA, uu.ID); err != nil || n != 0 {
			t.Fatalf("awal = %d, %v", n, err)
		}
		s1, _ := newSession(t, s, tenantA, uu.ID)
		newSession(t, s, tenantA, uu.ID)
		if _, err := s.RevokeSession(ctx, tenantA, s1.ID, "logout", now()); err != nil {
			t.Fatal(err)
		}
		if n, err := s.CountUserSessions(ctx, tenantA, uu.ID); err != nil || n != 2 {
			t.Errorf("setelah 2 sesi (1 tercabut) = %d, %v, want 2", n, err)
		}
		if n, err := s.CountUserSessions(ctx, tenantB, uu.ID); err != nil || n != 0 {
			t.Errorf("tenant B = %d, %v, want 0", n, err)
		}
		if n, err := s.CountUserSessions(ctx, tenantA, "bukan-uuid"); err != nil || n != 0 {
			t.Errorf("id cacat = %d, %v, want 0", n, err)
		}
	})

	t.Run("isolasi tenant (IDOR)", func(t *testing.T) {
		sess, rt := newSession(t, s, tenantA, ua.ID)
		if _, err := s.GetSession(ctx, tenantB, sess.ID); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("GetSession dari tenant B: err = %v, want ErrNotFound", err)
		}
		if _, err := s.FindRefreshTokenByHash(ctx, tenantB, rt.TokenHash); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("FindRefreshTokenByHash dari tenant B: err = %v, want ErrNotFound", err)
		}
		if ok, err := s.MarkRefreshTokenUsed(ctx, tenantB, rt.ID, now()); err != nil || ok {
			t.Errorf("MarkRefreshTokenUsed dari tenant B = %v, %v, want false", ok, err)
		}
		if ok, err := s.RevokeSession(ctx, tenantB, sess.ID, "x", now()); err != nil || ok {
			t.Errorf("RevokeSession dari tenant B = %v, %v, want false", ok, err)
		}
		if ids, err := s.RevokeUserSessions(ctx, tenantB, ua.ID, "", "x", now()); err != nil || len(ids) != 0 {
			t.Errorf("RevokeUserSessions dari tenant B = %v, %v", ids, err)
		}
		if ids, err := s.RevokeSessionsOfFamily(ctx, tenantB, rt.FamilyID, "x", now()); err != nil || len(ids) != 0 {
			t.Errorf("RevokeSessionsOfFamily dari tenant B = %v, %v", ids, err)
		}
		if rev, _ := s.ListRevokedSessionsSince(ctx, tenantB, now().Add(-time.Hour)); len(rev) != 0 {
			t.Errorf("tenant B melihat sesi tercabut tenant A: %v", revIDs(rev))
		}
		if g, _ := s.GetSession(ctx, tenantA, sess.ID); g.RevokedAt != nil {
			t.Error("sesi tenant A tercabut oleh tenant B")
		}
		// sesi di tenant B milik pengguna yang bukan anggota tenant B ditolak
		c := now()
		other := store.Session{ID: uuid.NewString(), UserID: ua.ID, ClientKind: "web", AuthMethod: "password", AMR: []string{"pwd"},
			CreatedAt: c, LastSeenAt: c, AbsoluteExpiresAt: c.Add(time.Hour)}
		if err := s.CreateSession(ctx, tenantB, other); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("CreateSession untuk non-anggota tenant: err = %v, want ErrNotFound", err)
		}
		_ = ub
	})
}

func TestMFA(t *testing.T) {
	_, s := setup(t)
	u := newUser(t, s, tenantA, "mfa@example.com")
	secret := []byte("sealed-totp-secret")

	t.Run("daftar, ganti pending, konfirmasi", func(t *testing.T) {
		if _, err := s.GetMFAFactor(ctx, tenantA, u.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("faktor awal: err = %v, want ErrNotFound", err)
		}
		if ok, err := s.UpsertPendingMFAFactor(ctx, tenantA, u.ID, secret, now()); err != nil || !ok {
			t.Fatalf("UpsertPendingMFAFactor = %v, %v", ok, err)
		}
		if ok, err := s.UpsertPendingMFAFactor(ctx, tenantA, u.ID, []byte("secret-2"), now()); err != nil || !ok {
			t.Fatalf("pending diganti = %v, %v", ok, err)
		}
		f, _ := s.GetMFAFactor(ctx, tenantA, u.ID)
		if f.Type != "totp" || string(f.SecretSealed) != "secret-2" || f.ConfirmedAt != nil || f.LastUsedStep != 0 {
			t.Fatalf("faktor = %+v", f)
		}
		// verifikasi belum dikonfirmasi tidak menaikkan step
		if ok, err := s.AdvanceMFAStep(ctx, tenantA, u.ID, 5); err != nil || ok {
			t.Fatalf("AdvanceMFAStep pada faktor belum terkonfirmasi = %v, %v, want false", ok, err)
		}
		// konfirmasi serentak: tepat satu; last_used_step = step konfirmasi (kode pendaftaran tak dapat dipakai ulang)
		wins, errs := race(8, func(int) (bool, error) { return s.ConfirmMFAFactor(ctx, tenantA, u.ID, 1000, now()) })
		if wins != 1 || len(errs) != 0 {
			t.Fatalf("konfirmasi serentak: wins=%d errs=%v", wins, errs)
		}
		f, _ = s.GetMFAFactor(ctx, tenantA, u.ID)
		if f.ConfirmedAt == nil || f.LastUsedStep != 1000 {
			t.Fatalf("faktor terkonfirmasi = %+v", f)
		}
		// faktor terkonfirmasi tidak dapat ditimpa oleh pendaftaran baru
		if ok, err := s.UpsertPendingMFAFactor(ctx, tenantA, u.ID, []byte("hijack"), now()); err != nil || ok {
			t.Fatalf("timpa faktor terkonfirmasi = %v, %v, want false", ok, err)
		}
		if f, _ = s.GetMFAFactor(ctx, tenantA, u.ID); string(f.SecretSealed) != "secret-2" {
			t.Error("secret faktor terkonfirmasi berubah")
		}
	})

	t.Run("anti-replay: step harus lebih besar, atomik", func(t *testing.T) {
		for _, step := range []int64{1000, 999, 0} {
			if ok, err := s.AdvanceMFAStep(ctx, tenantA, u.ID, step); err != nil || ok {
				t.Errorf("AdvanceMFAStep(%d) = %v, %v, want false (replay/lebih lama)", step, ok, err)
			}
		}
		if ok, err := s.AdvanceMFAStep(ctx, tenantA, u.ID, 1001); err != nil || !ok {
			t.Fatalf("AdvanceMFAStep(1001) = %v, %v", ok, err)
		}
		wins, errs := race(10, func(int) (bool, error) { return s.AdvanceMFAStep(ctx, tenantA, u.ID, 2000) })
		if wins != 1 || len(errs) != 0 {
			t.Fatalf("kode sama serentak: wins=%d errs=%v, want 1", wins, errs)
		}
		if f, _ := s.GetMFAFactor(ctx, tenantA, u.ID); f.LastUsedStep != 2000 {
			t.Errorf("LastUsedStep = %d", f.LastUsedStep)
		}
	})

	t.Run("recovery code sekali pakai", func(t *testing.T) {
		hs := make([][]byte, 10)
		for i := range hs {
			hs[i] = hash(byte(i + 1))
		}
		if err := s.ReplaceRecoveryCodes(ctx, tenantA, u.ID, hs, now()); err != nil {
			t.Fatal(err)
		}
		if n, err := s.CountUnusedRecoveryCodes(ctx, tenantA, u.ID); err != nil || n != 10 {
			t.Fatalf("CountUnused = %d, %v", n, err)
		}
		wins, errs := race(10, func(int) (bool, error) { return s.UseRecoveryCode(ctx, tenantA, u.ID, hs[0], now()) })
		if wins != 1 || len(errs) != 0 {
			t.Fatalf("recovery serentak: wins=%d errs=%v", wins, errs)
		}
		if ok, _ := s.UseRecoveryCode(ctx, tenantA, u.ID, hash(99), now()); ok {
			t.Error("kode tak dikenal diterima")
		}
		if n, _ := s.CountUnusedRecoveryCodes(ctx, tenantA, u.ID); n != 9 {
			t.Errorf("CountUnused = %d, want 9", n)
		}
		// regenerasi mengganti semua (termasuk yang terpakai) dan mencabut kode lama
		if err := s.ReplaceRecoveryCodes(ctx, tenantA, u.ID, [][]byte{hash(50), hash(51)}, now()); err != nil {
			t.Fatal(err)
		}
		if ok, _ := s.UseRecoveryCode(ctx, tenantA, u.ID, hs[1], now()); ok {
			t.Error("kode lama masih berlaku setelah regenerasi")
		}
		if n, _ := s.CountUnusedRecoveryCodes(ctx, tenantA, u.ID); n != 2 {
			t.Errorf("CountUnused = %d, want 2", n)
		}
		if err := s.ReplaceRecoveryCodes(ctx, tenantA, u.ID, [][]byte{hash(60), hash(60)}, now()); err == nil {
			t.Error("kode ganda dalam satu set harus ditolak")
		}
		if n, _ := s.CountUnusedRecoveryCodes(ctx, tenantA, u.ID); n != 2 {
			t.Errorf("set gagal tidak boleh mengubah kode lama: %d", n)
		}
	})

	t.Run("isolasi tenant (IDOR)", func(t *testing.T) {
		if _, err := s.GetMFAFactor(ctx, tenantB, u.ID); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("GetMFAFactor dari tenant B: err = %v, want ErrNotFound", err)
		}
		if ok, err := s.AdvanceMFAStep(ctx, tenantB, u.ID, 99999); err != nil || ok {
			t.Errorf("AdvanceMFAStep dari tenant B = %v, %v, want false", ok, err)
		}
		if ok, err := s.UseRecoveryCode(ctx, tenantB, u.ID, hash(50), now()); err != nil || ok {
			t.Errorf("UseRecoveryCode dari tenant B = %v, %v, want false", ok, err)
		}
		if n, _ := s.CountUnusedRecoveryCodes(ctx, tenantB, u.ID); n != 0 {
			t.Errorf("CountUnused dari tenant B = %d", n)
		}
		if ok, err := s.UpsertPendingMFAFactor(ctx, tenantB, u.ID, []byte("x"), now()); !errors.Is(err, store.ErrNotFound) || ok {
			t.Errorf("Upsert dari tenant B = %v, %v, want ErrNotFound", ok, err)
		}
		if err := s.ResetMFA(ctx, tenantB, u.ID); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("ResetMFA dari tenant B: err = %v, want ErrNotFound", err)
		}
		if f, err := s.GetMFAFactor(ctx, tenantA, u.ID); err != nil || f.ConfirmedAt == nil {
			t.Errorf("faktor tenant A terganggu: %+v, %v", f, err)
		}
	})

	t.Run("reset MFA menghapus faktor dan recovery code", func(t *testing.T) {
		if err := s.ResetMFA(ctx, tenantA, u.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetMFAFactor(ctx, tenantA, u.ID); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("faktor setelah reset: err = %v", err)
		}
		if n, _ := s.CountUnusedRecoveryCodes(ctx, tenantA, u.ID); n != 0 {
			t.Errorf("recovery code tersisa: %d", n)
		}
	})
}

func TestPasswordResets(t *testing.T) {
	_, s := setup(t)
	u := newUser(t, s, tenantA, "reset@example.com")
	mk := func(b byte, ttl time.Duration) store.PasswordReset {
		r := store.PasswordReset{ID: uuid.NewString(), UserID: u.ID, TokenHash: hash(b), CreatedAt: now(), ExpiresAt: now().Add(ttl)}
		if err := s.InsertPasswordReset(ctx, tenantA, r); err != nil {
			t.Fatal(err)
		}
		return r
	}

	t.Run("dipakai sekali, serentak tepat satu", func(t *testing.T) {
		r := mk(1, 30*time.Minute)
		f, err := s.FindPasswordResetByHash(ctx, tenantA, r.TokenHash)
		if err != nil || f.ID != r.ID || f.UserID != u.ID || f.UsedAt != nil {
			t.Fatalf("FindPasswordResetByHash = %+v, %v", f, err)
		}
		wins, errs := race(10, func(int) (bool, error) { return s.ConsumePasswordReset(ctx, tenantA, r.ID, now()) })
		if wins != 1 || len(errs) != 0 {
			t.Fatalf("konsumsi serentak: wins=%d errs=%v", wins, errs)
		}
		if f, _ = s.FindPasswordResetByHash(ctx, tenantA, r.TokenHash); f.UsedAt == nil {
			t.Error("UsedAt kosong")
		}
	})

	t.Run("kedaluwarsa tidak dapat dipakai", func(t *testing.T) {
		r := mk(2, time.Minute)
		if ok, err := s.ConsumePasswordReset(ctx, tenantA, r.ID, r.ExpiresAt.Add(time.Second)); err != nil || ok {
			t.Errorf("konsumsi sesudah kedaluwarsa = %v, %v, want false", ok, err)
		}
		if ok, err := s.ConsumePasswordReset(ctx, tenantA, r.ID, r.ExpiresAt.Add(-time.Second)); err != nil || !ok {
			t.Errorf("konsumsi sebelum kedaluwarsa = %v, %v, want true", ok, err)
		}
	})

	t.Run("permintaan baru membatalkan tautan lama", func(t *testing.T) {
		old := mk(3, 30*time.Minute)
		if n, err := s.InvalidatePasswordResets(ctx, tenantA, u.ID, now()); err != nil || n < 1 {
			t.Fatalf("InvalidatePasswordResets = %d, %v", n, err)
		}
		if ok, _ := s.ConsumePasswordReset(ctx, tenantA, old.ID, now()); ok {
			t.Error("tautan lama masih berlaku")
		}
	})

	t.Run("isolasi tenant", func(t *testing.T) {
		r := mk(4, 30*time.Minute)
		if _, err := s.FindPasswordResetByHash(ctx, tenantB, r.TokenHash); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("Find dari tenant B: err = %v, want ErrNotFound", err)
		}
		if ok, err := s.ConsumePasswordReset(ctx, tenantB, r.ID, now()); err != nil || ok {
			t.Errorf("Consume dari tenant B = %v, %v, want false", ok, err)
		}
	})

	t.Run("ResetPassword atomik: sandi, buka kunci, cabut semua sesi, hapus percobaan", func(t *testing.T) {
		v := newUser(t, s, tenantA, "reset2@example.com")
		s1, _ := newSession(t, s, tenantA, v.ID)
		s2, _ := newSession(t, s, tenantA, v.ID)
		at := now()
		if ok, _ := s.LockUser(ctx, tenantA, v.ID, at); !ok {
			t.Fatal("lock gagal")
		}
		_ = s.RecordLoginAttempt(ctx, tenantA, v.ID, "198.51.100.9", store.AttemptPassword, false, at)
		r := store.PasswordReset{ID: uuid.NewString(), UserID: v.ID, TokenHash: hash(77), CreatedAt: at, ExpiresAt: at.Add(30 * time.Minute)}
		if err := s.InsertPasswordReset(ctx, tenantA, r); err != nil {
			t.Fatal(err)
		}
		type res struct {
			ok  bool
			ids []string
		}
		var mu sync.Mutex
		var all []res
		wins, errs := race(6, func(int) (bool, error) {
			out, ok, err := s.ResetPassword(ctx, tenantA, r.ID, "$argon2id$baru", at)
			mu.Lock()
			all = append(all, res{ok, out.RevokedSessionIDs})
			mu.Unlock()
			return ok, err
		})
		if wins != 1 || len(errs) != 0 {
			t.Fatalf("ResetPassword serentak: wins=%d errs=%v", wins, errs)
		}
		got, _ := s.GetUser(ctx, tenantA, v.ID)
		if got.PasswordHash != "$argon2id$baru" || got.Status != store.UserActive || got.LockedAt != nil {
			t.Errorf("user setelah reset = %+v", got)
		}
		for _, id := range []string{s1.ID, s2.ID} {
			if g, _ := s.GetSession(ctx, tenantA, id); g.RevokedAt == nil || g.RevokeReason != "password_reset" {
				t.Errorf("sesi %s = %+v, want tercabut password_reset", id, g)
			}
		}
		for _, r := range all {
			if r.ok && len(r.ids) != 2 {
				t.Errorf("RevokedSessionIDs = %v, want 2 sesi", r.ids)
			}
		}
		if st, _ := s.CrossIPFailures(ctx, tenantA, v.ID, at.Add(-time.Hour)); st.Failures != 0 {
			t.Errorf("percobaan login tersisa: %+v", st)
		}
		// tautan yang sama tidak berlaku lagi
		if _, ok, err := s.ResetPassword(ctx, tenantA, r.ID, "$lagi", at); err != nil || ok {
			t.Errorf("ResetPassword ulang = %v, %v, want false", ok, err)
		}
		if g, _ := s.GetUser(ctx, tenantA, v.ID); g.PasswordHash != "$argon2id$baru" {
			t.Error("sandi diganti oleh pemakaian ulang tautan")
		}
		// tenant lain tidak dapat mereset
		r2 := store.PasswordReset{ID: uuid.NewString(), UserID: v.ID, TokenHash: hash(78), CreatedAt: at, ExpiresAt: at.Add(time.Hour)}
		if err := s.InsertPasswordReset(ctx, tenantA, r2); err != nil {
			t.Fatal(err)
		}
		if _, ok, err := s.ResetPassword(ctx, tenantB, r2.ID, "$evil", at); err != nil || ok {
			t.Errorf("ResetPassword dari tenant B = %v, %v, want false", ok, err)
		}
	})

	t.Run("ChangePassword: sandi baru, sesi lain tercabut, sesi saat ini tetap", func(t *testing.T) {
		v := newUser(t, s, tenantA, "change@example.com")
		cur, _ := newSession(t, s, tenantA, v.ID)
		other, _ := newSession(t, s, tenantA, v.ID)
		ids, err := s.ChangePassword(ctx, tenantA, v.ID, "$argon2id$ganti", cur.ID, now())
		if err != nil || len(ids) != 1 || ids[0] != other.ID {
			t.Fatalf("ChangePassword = %v, %v, want [%s]", ids, err, other.ID)
		}
		if g, _ := s.GetSession(ctx, tenantA, cur.ID); g.RevokedAt != nil {
			t.Error("sesi saat ini tercabut")
		}
		if g, _ := s.GetUser(ctx, tenantA, v.ID); g.PasswordHash != "$argon2id$ganti" {
			t.Error("sandi tidak berubah")
		}
		if _, err := s.ChangePassword(ctx, tenantB, v.ID, "$evil", "", now()); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("ChangePassword dari tenant B: err = %v, want ErrNotFound", err)
		}
	})
}

func TestLoginAttempts(t *testing.T) {
	_, s := setup(t)
	u := newUser(t, s, tenantA, "attempts@example.com")
	v := newUser(t, s, tenantA, "attempts2@example.com")
	base := now().Add(-30 * time.Minute)
	rec := func(user, ip string, success bool, at time.Time) {
		t.Helper()
		if err := s.RecordLoginAttempt(ctx, tenantA, user, ip, store.AttemptPassword, success, at); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("hitungan per pasangan direset oleh sukses", func(t *testing.T) {
		st, err := s.PairFailureState(ctx, tenantA, u.ID, "203.0.113.1")
		if err != nil || st.ConsecutiveFailures != 0 || st.LastFailureAt != nil {
			t.Fatalf("awal = %+v, %v", st, err)
		}
		for i := 0; i < 5; i++ {
			rec(u.ID, "203.0.113.1", false, base.Add(time.Duration(i)*time.Second))
		}
		st, _ = s.PairFailureState(ctx, tenantA, u.ID, "203.0.113.1")
		if st.ConsecutiveFailures != 5 || st.LastFailureAt == nil || !st.LastFailureAt.Equal(base.Add(4*time.Second)) {
			t.Fatalf("setelah 5 gagal = %+v", st)
		}
		// IP lain dan akun lain tidak terpengaruh
		if st, _ = s.PairFailureState(ctx, tenantA, u.ID, "203.0.113.2"); st.ConsecutiveFailures != 0 {
			t.Errorf("IP lain = %+v", st)
		}
		if st, _ = s.PairFailureState(ctx, tenantA, v.ID, "203.0.113.1"); st.ConsecutiveFailures != 0 {
			t.Errorf("akun lain = %+v", st)
		}
		rec(u.ID, "203.0.113.1", true, base.Add(10*time.Second))
		if st, _ = s.PairFailureState(ctx, tenantA, u.ID, "203.0.113.1"); st.ConsecutiveFailures != 0 {
			t.Errorf("setelah sukses = %+v, want 0", st)
		}
		rec(u.ID, "203.0.113.1", false, base.Add(20*time.Second))
		if st, _ = s.PairFailureState(ctx, tenantA, u.ID, "203.0.113.1"); st.ConsecutiveFailures != 1 {
			t.Errorf("gagal sesudah sukses = %+v, want 1", st)
		}
	})

	t.Run("jendela lintas IP tidak direset oleh sukses dari IP lain", func(t *testing.T) {
		w := newUser(t, s, tenantA, "attempts3@example.com")
		for i := 0; i < 6; i++ {
			ip := []string{"198.51.100.1", "198.51.100.2", "198.51.100.3"}[i%3]
			rec(w.ID, ip, false, base.Add(time.Duration(i)*time.Second))
		}
		rec(w.ID, "198.51.100.200", true, base.Add(time.Minute))
		st, err := s.CrossIPFailures(ctx, tenantA, w.ID, base.Add(-time.Minute))
		if err != nil || st.Failures != 6 || st.DistinctIPs != 3 {
			t.Fatalf("CrossIPFailures = %+v, %v, want 6 gagal dari 3 IP", st, err)
		}
		if st, _ = s.CrossIPFailures(ctx, tenantA, w.ID, base.Add(4*time.Second)); st.Failures != 2 || st.DistinctIPs != 2 {
			t.Errorf("jendela lebih sempit = %+v, want 2 gagal dari 2 IP", st)
		}
	})

	t.Run("jendela waktu: hanya kegagalan dalam jendela yang dihitung, sukses mereset", func(t *testing.T) {
		w := newUser(t, s, tenantA, "window@example.com")
		const ip = "203.0.113.50"
		ref := now()
		since := ref.Add(-24 * time.Hour)
		// 3 gagal lebih tua dari 24 jam (tidak dihitung), 2 gagal dalam 24 jam (dihitung)
		for i := 0; i < 3; i++ {
			rec(w.ID, ip, false, ref.Add(-25*time.Hour).Add(time.Duration(i)*time.Minute))
		}
		for i := 0; i < 2; i++ {
			rec(w.ID, ip, false, ref.Add(-23*time.Hour).Add(time.Duration(i)*time.Minute))
		}
		st, err := s.PairFailureStateSince(ctx, tenantA, w.ID, ip, since)
		if err != nil || st.ConsecutiveFailures != 2 || st.LastFailureAt == nil || !st.LastFailureAt.Equal(ref.Add(-23*time.Hour).Add(time.Minute)) {
			t.Fatalf("PairFailureStateSince = %+v, %v, want 2 gagal dalam jendela", st, err)
		}
		// tanpa jendela (zero) dan API lama menghitung semua 5: perilaku lama tetap utuh
		if st, _ = s.PairFailureStateSince(ctx, tenantA, w.ID, ip, time.Time{}); st.ConsecutiveFailures != 5 {
			t.Errorf("since nol = %+v, want 5 (tanpa batas)", st)
		}
		if st, _ = s.PairFailureState(ctx, tenantA, w.ID, ip); st.ConsecutiveFailures != 5 {
			t.Errorf("PairFailureState lama = %+v, want 5", st)
		}
		// batas tepat: baris pada waktu == since dihitung (>= since)
		rec(w.ID, ip, false, since)
		if st, _ = s.PairFailureStateSince(ctx, tenantA, w.ID, ip, since); st.ConsecutiveFailures != 3 {
			t.Errorf("baris tepat di batas = %+v, want 3", st)
		}
		// semua kegagalan lebih tua dari jendela: nol, tanpa waktu gagal terakhir
		if st, _ = s.PairFailureStateSince(ctx, tenantA, w.ID, ip, ref.Add(time.Hour)); st.ConsecutiveFailures != 0 || st.LastFailureAt != nil {
			t.Errorf("jendela di masa depan = %+v, want kosong", st)
		}
		// sukses dalam jendela mereset; gagal sesudahnya dihitung
		rec(w.ID, ip, true, ref.Add(-time.Hour))
		if st, _ = s.PairFailureStateSince(ctx, tenantA, w.ID, ip, since); st.ConsecutiveFailures != 0 {
			t.Errorf("setelah sukses = %+v, want 0", st)
		}
		rec(w.ID, ip, false, ref.Add(-30*time.Minute))
		if st, _ = s.PairFailureStateSince(ctx, tenantA, w.ID, ip, since); st.ConsecutiveFailures != 1 {
			t.Errorf("gagal sesudah sukses = %+v, want 1", st)
		}
		// pasangan/tenant lain tidak terpengaruh
		if st, _ = s.PairFailureStateSince(ctx, tenantB, w.ID, ip, since); st.ConsecutiveFailures != 0 {
			t.Errorf("tenant B = %+v", st)
		}
	})

	t.Run("isolasi tenant dan pembersihan", func(t *testing.T) {
		if st, err := s.CrossIPFailures(ctx, tenantB, u.ID, base.Add(-time.Hour)); err != nil || st.Failures != 0 {
			t.Errorf("tenant B = %+v, %v", st, err)
		}
		if st, _ := s.PairFailureState(ctx, tenantB, u.ID, "203.0.113.1"); st.ConsecutiveFailures != 0 {
			t.Errorf("PairFailureState tenant B = %+v", st)
		}
		if n, err := s.PruneLoginAttempts(ctx, tenantB, now()); err != nil || n != 0 {
			t.Errorf("Prune dari tenant B = %d, %v, want 0", n, err)
		}
		if n, err := s.PruneLoginAttempts(ctx, tenantA, base.Add(15*time.Second)); err != nil || n < 6 {
			t.Errorf("Prune tenant A = %d, %v, want >= 6", n, err)
		}
	})

	t.Run("pengguna tak ada ditolak", func(t *testing.T) {
		if err := s.RecordLoginAttempt(ctx, tenantA, uuid.NewString(), "1.2.3.4", store.AttemptPassword, false, now()); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("user tak ada: err = %v, want ErrNotFound", err)
		}
		if err := s.RecordLoginAttempt(ctx, tenantB, u.ID, "1.2.3.4", store.AttemptPassword, false, now()); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("non-anggota tenant: err = %v, want ErrNotFound", err)
		}
	})
}

func TestRoles(t *testing.T) {
	_, s := setup(t)
	u := uuid.NewString()

	if ok, err := s.HasRole(ctx, tenantA, u, "administrator"); err != nil || ok {
		t.Fatalf("HasRole awal = %v, %v", ok, err)
	}
	if granted, err := s.GrantRole(ctx, tenantA, u, "administrator", now()); err != nil || !granted {
		t.Fatalf("GrantRole = %v, %v", granted, err)
	}
	if granted, err := s.GrantRole(ctx, tenantA, u, "administrator", now()); err != nil || granted {
		t.Fatalf("GrantRole ulang = %v, %v, want false", granted, err)
	}
	if _, err := s.GrantRole(ctx, tenantA, u, "tidak-ada", now()); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("role tak ada: err = %v, want ErrNotFound", err)
	}
	if _, err := s.GrantRole(ctx, tenantA, "bukan-uuid", "administrator", now()); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("user bukan uuid: err = %v, want ErrNotFound", err)
	}
	keys, err := s.UserRoleKeys(ctx, tenantA, u)
	sort.Strings(keys)
	if err != nil || len(keys) != 1 || keys[0] != "administrator" {
		t.Fatalf("UserRoleKeys = %v, %v", keys, err)
	}
	if ok, _ := s.HasRole(ctx, tenantA, u, "administrator"); !ok {
		t.Error("HasRole = false")
	}
	if n, err := s.CountRoleHolders(ctx, tenantA, "administrator"); err != nil || n != 1 {
		t.Errorf("CountRoleHolders = %d, %v", n, err)
	}
	// tenant tak dikenal connector: galat (gagal tertutup), bukan data tenant lain
	other := tenancy.TenantID(uuid.New())
	if _, err := s.HasRole(ctx, other, u, "administrator"); !errors.Is(err, database.ErrUnknownTenant) {
		t.Errorf("tenant tak dikenal: err = %v, want ErrUnknownTenant", err)
	}
	if ok, err := s.RevokeRole(ctx, tenantA, u, "administrator"); err != nil || !ok {
		t.Fatalf("RevokeRole = %v, %v", ok, err)
	}
	if ok, err := s.RevokeRole(ctx, tenantA, u, "administrator"); err != nil || ok {
		t.Fatalf("RevokeRole ulang = %v, %v, want false", ok, err)
	}
	if ok, _ := s.HasRole(ctx, tenantA, u, "administrator"); ok {
		t.Error("HasRole masih true setelah revoke")
	}
}

// Store tanpa konektor tenant gagal tertutup pada operasi peran (tidak panik).
func TestRolesRequireTenantConnector(t *testing.T) {
	db, _ := setup(t)
	s := store.New(database.ControlDB{DB: db}, nil)
	if _, err := s.HasRole(ctx, tenantA, uuid.NewString(), "administrator"); err == nil {
		t.Fatal("harus galat tanpa TenantConnector")
	}
}

func contains(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}
func revIDs(r []store.RevokedSession) []string {
	var o []string
	for _, x := range r {
		o = append(o, x.ID)
	}
	return o
}
func sessIDs(r []store.Session) []string {
	var o []string
	for _, x := range r {
		o = append(o, x.ID)
	}
	return o
}

// TestReserveLoginAttempt: reservasi atomik per pasangan lintas koneksi (kunci advisory) dan pelepasannya.
func TestReserveLoginAttempt(t *testing.T) {
	db, s := setup(t)
	u := newUser(t, s, tenantA, "reserve@example.com")
	other := newUser(t, s, tenantA, "reserve2@example.com")
	const ip = "198.51.100.9"
	since := now().Add(-24 * time.Hour)

	t.Run("serentak: hanya batas yang diizinkan admit yang lolos", func(t *testing.T) {
		const limit, workers = 3, 24
		var wg sync.WaitGroup
		var mu sync.Mutex
		admittedN := 0
		start := make(chan struct{})
		for range workers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, ok, id, err := s.ReserveLoginAttempt(ctx, tenantA, u.ID, ip, store.AttemptPassword, now(), since,
					func(ps store.PairState) bool { return ps.ConsecutiveFailures < limit })
				if err != nil {
					t.Errorf("ReserveLoginAttempt: %v", err)
					return
				}
				if ok != (id != 0) {
					t.Errorf("ok=%v id=%d tidak konsisten", ok, id)
				}
				if ok {
					mu.Lock()
					admittedN++
					mu.Unlock()
				}
			}()
		}
		close(start)
		wg.Wait()
		if admittedN != limit {
			t.Fatalf("diterima %d, want tepat %d (pembacaan keadaan dan sisipan harus atomik)", admittedN, limit)
		}
		if st, _ := s.PairFailureStateSince(ctx, tenantA, u.ID, ip, since); st.ConsecutiveFailures != limit {
			t.Fatalf("baris tercatat = %d, want %d", st.ConsecutiveFailures, limit)
		}
	})

	t.Run("ditolak tidak menulis; pasangan lain tidak terpengaruh", func(t *testing.T) {
		before, _ := s.PairFailureStateSince(ctx, tenantA, other.ID, ip, since)
		_, ok, id, err := s.ReserveLoginAttempt(ctx, tenantA, other.ID, ip, store.AttemptMFA, now(), since, func(store.PairState) bool { return false })
		if err != nil || ok || id != 0 {
			t.Fatalf("ok=%v id=%d err=%v", ok, id, err)
		}
		after, _ := s.PairFailureStateSince(ctx, tenantA, other.ID, ip, since)
		if after.ConsecutiveFailures != before.ConsecutiveFailures {
			t.Fatal("penolakan tidak boleh menulis baris")
		}
	})

	t.Run("pelepasan menghapus hanya reservasi itu dan milik pengguna itu", func(t *testing.T) {
		_, ok, id, err := s.ReserveLoginAttempt(ctx, tenantA, other.ID, ip, store.AttemptPassword, now(), since, func(store.PairState) bool { return true })
		if err != nil || !ok {
			t.Fatal(err)
		}
		if err := s.ReleaseLoginAttempt(ctx, tenantA, u.ID, id); err != nil { // pengguna salah: tidak menghapus apa pun
			t.Fatal(err)
		}
		if st, _ := s.PairFailureStateSince(ctx, tenantA, other.ID, ip, since); st.ConsecutiveFailures != 1 {
			t.Fatalf("setelah rilis oleh pengguna lain = %d, want 1", st.ConsecutiveFailures)
		}
		if err := s.ReleaseLoginAttempt(ctx, tenantA, other.ID, id); err != nil {
			t.Fatal(err)
		}
		if err := s.ReleaseLoginAttempt(ctx, tenantA, other.ID, id); err != nil { // idempoten
			t.Fatal(err)
		}
		if st, _ := s.PairFailureStateSince(ctx, tenantA, other.ID, ip, since); st.ConsecutiveFailures != 0 {
			t.Fatalf("setelah rilis = %d, want 0", st.ConsecutiveFailures)
		}
	})

	t.Run("pengguna fiktif yang diterima menghasilkan ErrNotFound tanpa baris; non-anggota ditolak", func(t *testing.T) {
		_, ok, id, err := s.ReserveLoginAttempt(ctx, tenantA, uuid.NewString(), ip, store.AttemptPassword, now(), since, func(store.PairState) bool { return true })
		if !errors.Is(err, store.ErrNotFound) || ok || id != 0 {
			t.Fatalf("fiktif: ok=%v id=%d err=%v", ok, id, err)
		}
		if _, _, _, err := s.ReserveLoginAttempt(ctx, tenantB, u.ID, ip, store.AttemptPassword, now(), since, func(store.PairState) bool { return true }); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("non-anggota tenant: %v", err)
		}
		if _, _, _, err := s.ReserveLoginAttempt(ctx, tenantA, u.ID, ip, "lain", now(), since, func(store.PairState) bool { return true }); !errors.Is(err, store.ErrInvalidArgument) {
			t.Fatalf("kind tak valid: %v", err)
		}
	})

	t.Run("status keanggotaan ikut dimuat", func(t *testing.T) {
		got, err := s.FindUserByEmail(ctx, tenantA, u.Email)
		if err != nil || got.MembershipStatus != store.MembershipActive {
			t.Fatalf("%+v %v", got, err)
		}
		if err := db.Exec(`UPDATE tenant_memberships SET status = 'suspended' WHERE tenant_id = ? AND user_id = ?`, uuid.UUID(tenantA), u.ID).Error; err != nil {
			t.Fatal(err)
		}
		for name, load := range map[string]func() (store.User, error){
			"FindUserByEmail": func() (store.User, error) { return s.FindUserByEmail(ctx, tenantA, u.Email) },
			"GetUser":         func() (store.User, error) { return s.GetUser(ctx, tenantA, u.ID) },
		} {
			got, err := load()
			if err != nil || got.MembershipStatus != store.MembershipSuspended {
				t.Errorf("%s: %+v %v, want suspended", name, got, err)
			}
		}
	})
}

// Q30 butir 5: penjaga akun disabled pada ResetPassword harus atomik terhadap perubahan status yang belum commit.
// Balapan: transaksi admin menonaktifkan akun (lock baris users, belum commit); ResetPassword mulai, melewati syarat
// EXISTS pada tautan (snapshot lama: masih active), lalu UPDATE users menunggu lock. Setelah admin commit, UPDATE
// bersyarat status harus menolak (0 baris), seluruh transaksi di-rollback: won=false, hash tidak berubah, tautan
// tidak terpakai, sesi tidak dicabut.
func TestResetPasswordRaceWithDisable(t *testing.T) {
	db, s := setup(t)
	u := newUser(t, s, tenantA, "race-disable@example.com")
	sess, _ := newSession(t, s, tenantA, u.ID)
	at := now()
	r := store.PasswordReset{ID: uuid.NewString(), UserID: u.ID, TokenHash: hash(91), CreatedAt: at, ExpiresAt: at.Add(30 * time.Minute)}
	if err := s.InsertPasswordReset(ctx, tenantA, r); err != nil {
		t.Fatal(err)
	}
	before, err := s.GetUser(ctx, tenantA, u.ID)
	if err != nil {
		t.Fatal(err)
	}

	admin := db.Begin()
	if admin.Error != nil {
		t.Fatal(admin.Error)
	}
	defer admin.Rollback()
	if err := admin.Exec(`UPDATE users SET status = 'disabled' WHERE id = ?`, u.ID).Error; err != nil {
		t.Fatal(err)
	}

	type out struct {
		ok  bool
		err error
	}
	done := make(chan out, 1)
	go func() {
		_, ok, err := s.ResetPassword(ctx, tenantA, r.ID, "$argon2id$penyerang", at)
		done <- out{ok, err}
	}()
	select {
	case o := <-done:
		t.Fatalf("ResetPassword selesai sebelum admin commit (tidak menunggu lock): %+v", o)
	case <-time.After(500 * time.Millisecond):
	}
	if err := admin.Commit().Error; err != nil {
		t.Fatal(err)
	}
	var o out
	select {
	case o = <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("ResetPassword tidak selesai setelah commit")
	}
	if o.err != nil || o.ok {
		t.Fatalf("ResetPassword = ok:%v err:%v, want ok=false tanpa galat", o.ok, o.err)
	}
	got, err := s.GetUser(ctx, tenantA, u.ID)
	if err != nil || got.Status != store.UserDisabled || got.PasswordHash != before.PasswordHash {
		t.Fatalf("akun berubah: %+v %v (hash awal %q)", got, err, before.PasswordHash)
	}
	rec, err := s.FindPasswordResetByHash(ctx, tenantA, r.TokenHash)
	if err != nil || rec.UsedAt != nil {
		t.Fatalf("tautan harus tetap belum terpakai (rollback): %+v %v", rec, err)
	}
	var revoked int64
	if err := db.Raw(`SELECT count(*) FROM auth_sessions WHERE id = ? AND revoked_at IS NOT NULL`, sess.ID).Scan(&revoked).Error; err != nil || revoked != 0 {
		t.Errorf("sesi tercabut padahal reset ditolak: %d %v", revoked, err)
	}
}

// WithEmailLock menserialkan pemegang email yang sama (advisory lock transaksi): pemegang kedua baru masuk setelah
// yang pertama selesai; email berbeda tidak saling menunggu; galat fn dikembalikan apa adanya dan lock dilepas.
func TestWithEmailLock_SerializesSameEmailOnly(t *testing.T) {
	_, s := setup(t)
	var inside, maxInside int32
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := s.WithEmailLock(ctx, "  Sama@Example.test ", func(context.Context) error {
				n := atomic.AddInt32(&inside, 1)
				for {
					m := atomic.LoadInt32(&maxInside)
					if n <= m || atomic.CompareAndSwapInt32(&maxInside, m, n) {
						break
					}
				}
				time.Sleep(80 * time.Millisecond)
				atomic.AddInt32(&inside, -1)
				return nil
			})
			if err != nil {
				t.Errorf("WithEmailLock: %v", err)
			}
		}()
	}
	wg.Wait()
	if maxInside != 1 {
		t.Fatalf("pemegang serentak = %d, want 1 (email sama harus diserialkan)", maxInside)
	}

	// Email berbeda tidak saling menunggu: B selesai saat A masih memegang lock.
	hold, release := make(chan struct{}), make(chan struct{})
	go func() {
		_ = s.WithEmailLock(ctx, "a@example.test", func(context.Context) error { close(hold); <-release; return nil })
	}()
	<-hold
	done := make(chan error, 1)
	go func() { done <- s.WithEmailLock(ctx, "b@example.test", func(context.Context) error { return nil }) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("email berbeda tidak boleh menunggu")
	}
	close(release)

	// Galat fn diteruskan apa adanya dan lock dilepas (pemanggilan berikutnya tidak menunggu).
	boom := errors.New("boom")
	if err := s.WithEmailLock(ctx, "a@example.test", func(context.Context) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if err := s.WithEmailLock(ctx, "a@example.test", func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := s.WithEmailLock(ctx, "   ", func(context.Context) error { return nil }); !errors.Is(err, store.ErrInvalidArgument) {
		t.Fatalf("email kosong: %v", err)
	}
}
