package identity

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"erpora/internal/platform/identity/mfa"
	"erpora/internal/platform/identity/store"
	"erpora/internal/platform/settings"
	"erpora/sdk/authz"
	sdkidentity "erpora/sdk/identity"
)

func systemCtx(reason string) context.Context { return authz.AsSystem(tenantCtx(), reason) }

const goodPassword = "kata-sandi-admin-yang-kuat-2026"

func TestAdmin_RequiresPrincipalAdminOrSystem(t *testing.T) {
	e := newFakeEnv()
	e.store.addUser(userB, "b@example.test", "h:x")
	e.store.roles[userA] = []string{mfa.AdministratorRole} // A administrator; B bukan
	adm := e.admin()
	ops := map[string]func(ctx context.Context) error{
		"CreateAdmin": func(ctx context.Context) error {
			_, err := adm.CreateAdmin(ctx, sdkidentity.CreateAdminInput{Email: "n@example.test", Name: "N", Password: goodPassword})
			return err
		},
		"RotateSigningKeys": func(ctx context.Context) error { _, err := adm.RotateSigningKeys(ctx); return err },
		"ResetMFA":          func(ctx context.Context) error { return adm.ResetMFA(ctx, "b@example.test") },
		"UnlockAccount":     func(ctx context.Context) error { return adm.UnlockAccount(ctx, "b@example.test") },
		"LogoutAllDevices":  func(ctx context.Context) error { return adm.LogoutAllDevices(ctx, userB) },
		"SetPublicURL":      func(ctx context.Context) error { return adm.SetPublicURL(ctx, "https://erp.example.com") },
	}
	for name, op := range ops {
		t.Run(name, func(t *testing.T) {
			// tanpa Principal dan tanpa hak sistem
			if err := op(tenantCtx()); !errors.Is(err, sdkidentity.ErrForbidden) {
				t.Errorf("tanpa hak: %v", err)
			}
			// Principal bukan administrator, walau membawa hak sistem
			if err := op(authz.AsSystem(principalCtx(userB, sessB, "pwd", "otp"), "cli:core:x")); !errors.Is(err, sdkidentity.ErrForbidden) {
				t.Errorf("non-admin dengan hak sistem: %v", err)
			}
			// administrator tetapi sesi belum MFA
			if err := op(principalCtx(userA, sessA, "pwd")); !errors.Is(err, sdkidentity.ErrForbidden) {
				t.Errorf("admin tanpa otp: %v", err)
			}
		})
	}
	// Jalur sah: administrator ber-MFA dan hak sistem.
	if _, err := adm.RotateSigningKeys(principalCtx(userA, sessA, "pwd", "otp")); err != nil {
		t.Errorf("admin sah: %v", err)
	}
	if _, err := adm.RotateSigningKeys(systemCtx("cli:core:rotate-keys")); err != nil {
		t.Errorf("hak sistem: %v", err)
	}
}

// Q30 butir 1: hak sistem hanya diterima untuk alasan berawalan "cli:core:" (command CLI core). Alasan lain (hook
// lifecycle, seed, modul lain) ditolak ErrForbidden dan dicatat identity.admin_denied; tidak ada efek samping.
func TestAdmin_SystemRightsOnlyForCoreCLIReason(t *testing.T) {
	e := newFakeEnv()
	var logBuf bytes.Buffer
	d := e.deps()
	d.Log = slog.New(slog.NewTextHandler(&logBuf, nil))
	adm, err := NewAdmin(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, reason := range []string{
		"lifecycle:install", "cli:db:seed", "cli:other:create-admin", "cli:core", "cli:", "core:create-admin",
		" cli:core:create-admin", "CLI:core:create-admin", "system:cli:core:x",
	} {
		logBuf.Reset()
		_, err := adm.CreateAdmin(systemCtx(reason), sdkidentity.CreateAdminInput{Email: "n@example.test", Name: "N", Password: goodPassword})
		if !errors.Is(err, sdkidentity.ErrForbidden) {
			t.Errorf("alasan %q: err = %v, want ErrForbidden", reason, err)
		}
		if !strings.Contains(logBuf.String(), "identity.admin_denied") {
			t.Errorf("alasan %q: tidak ada log identity.admin_denied: %s", reason, logBuf.String())
		}
	}
	if len(e.store.users) != 0 {
		t.Errorf("penolakan meninggalkan %d akun", len(e.store.users))
	}
	if _, err := adm.CreateAdmin(systemCtx("cli:core:create-admin"), sdkidentity.CreateAdminInput{Email: "n@example.test", Name: "N", Password: goodPassword}); err != nil {
		t.Errorf("cli:core:create-admin: %v", err)
	}
}

func TestAdmin_CreateAdmin(t *testing.T) {
	e := newFakeEnv()
	adm := e.admin()
	ctx := systemCtx("cli:core:create-admin")
	sum, err := adm.CreateAdmin(ctx, sdkidentity.CreateAdminInput{Email: "  Admin@Example.TEST ", Name: " Budi ", Password: goodPassword})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Email != "admin@example.test" || sum.Name != "Budi" || sum.ID == "" {
		t.Fatalf("ringkasan = %+v", sum)
	}
	if roles := e.store.roles[sum.ID]; len(roles) != 1 || roles[0] != mfa.AdministratorRole {
		t.Errorf("role = %v", roles)
	}
	if got := e.store.users[sum.ID].PasswordHash; got != "h:"+goodPassword {
		t.Errorf("hash tersimpan = %q (harus lewat HashGate)", got)
	}
	if len(e.audit.events) != 1 || e.audit.events[0].Action != "identity.admin_created" || e.audit.events[0].Actor != "cli:core:create-admin" || e.audit.events[0].Target != sum.ID {
		t.Errorf("audit = %+v", e.audit.events)
	}
	if strings.Contains(sum.ID+sum.Email+sum.Name, goodPassword) {
		t.Error("sandi tidak boleh muncul di hasil")
	}

	// Email ganda (berbeda huruf/spasi) ditolak.
	if _, err := adm.CreateAdmin(ctx, sdkidentity.CreateAdminInput{Email: "ADMIN@example.test", Name: "X", Password: goodPassword}); !errors.Is(err, sdkidentity.ErrEmailTaken) {
		t.Errorf("email ganda: %v", err)
	}
	// Sandi lemah ditolak tanpa membuat akun.
	n := len(e.store.users)
	_, err = adm.CreateAdmin(ctx, sdkidentity.CreateAdminInput{Email: "lain@example.test", Name: "L", Password: "pendek"})
	var pe *sdkidentity.PasswordPolicyError
	if !errors.As(err, &pe) || len(e.store.users) != n {
		t.Errorf("sandi lemah: %v", err)
	}
	// Nama/email kosong ditolak.
	for _, in := range []sdkidentity.CreateAdminInput{{Email: "", Name: "A", Password: goodPassword}, {Email: "a@b.test", Name: "  ", Password: goodPassword}} {
		if _, err := adm.CreateAdmin(ctx, in); !errors.Is(err, sdkidentity.ErrInvalidInput) {
			t.Errorf("masukan %+v: %v", in, err)
		}
	}
}

// Akun dibuat nonaktif, role diberikan, dan status active TERAKHIR: akun tidak pernah aktif tanpa role.
func TestAdmin_CreateAdmin_ActivatesLast(t *testing.T) {
	e := newFakeEnv()
	sum, err := e.admin().CreateAdmin(systemCtx("cli:core:create-admin"), sdkidentity.CreateAdminInput{Email: "baru@example.test", Name: "B", Password: goodPassword})
	if err != nil {
		t.Fatal(err)
	}
	if e.store.users[sum.ID].Status != store.UserActive || e.store.statusSet[sum.ID] != store.UserActive {
		t.Errorf("akun harus aktif di akhir: %+v %v", e.store.users[sum.ID], e.store.statusSet)
	}
}

// Pemberian role gagal: akun tetap nonaktif tanpa role (sisa yang dapat dilanjutkan), tanpa audit sukses.
func TestAdmin_CreateAdmin_RoleFailureLeavesResumableRemnant(t *testing.T) {
	e := newFakeEnv()
	ctx := systemCtx("cli:core:create-admin")
	e.store.grantErr = errBoom
	in := sdkidentity.CreateAdminInput{Email: "z@example.test", Name: "Z", Password: goodPassword}
	_, err := e.admin().CreateAdmin(ctx, in)
	if err == nil {
		t.Fatal("harus galat")
	}
	u, ferr := e.store.FindUserByEmail(ctx, svcTenant, "z@example.test")
	if ferr != nil || u.Status != store.UserDisabled || len(e.store.roles[u.ID]) != 0 {
		t.Fatalf("sisa = %+v %v", u, ferr)
	}
	if len(e.audit.events) != 0 {
		t.Error("tidak ada audit sukses untuk operasi gagal")
	}
	// Panggilan ulang melanjutkan akun yang sama (idempoten).
	e.store.grantErr = nil
	sum, err := e.admin().CreateAdmin(ctx, in)
	if err != nil || sum.ID != u.ID {
		t.Fatalf("lanjutan: %+v %v", sum, err)
	}
	if got := e.store.users[u.ID]; got.Status != store.UserActive || !slices.Equal(e.store.roles[u.ID], []string{mfa.AdministratorRole}) {
		t.Errorf("setelah lanjut: %+v %v", got, e.store.roles[u.ID])
	}
	if len(e.audit.events) != 1 || e.audit.events[0].Action != "identity.admin_resumed" || e.audit.events[0].Target != u.ID {
		t.Errorf("audit = %+v", e.audit.events)
	}
}

// Aktivasi gagal: role dicabut (upaya terbaik) sehingga akun kembali ke sisa yang dapat dilanjutkan.
func TestAdmin_CreateAdmin_ActivationFailureRevokesRole(t *testing.T) {
	e := newFakeEnv()
	ctx := systemCtx("cli:core:create-admin")
	e.store.activateErr = errBoom
	in := sdkidentity.CreateAdminInput{Email: "y@example.test", Name: "Y", Password: goodPassword}
	if _, err := e.admin().CreateAdmin(ctx, in); err == nil {
		t.Fatal("harus galat")
	}
	u, _ := e.store.FindUserByEmail(ctx, svcTenant, "y@example.test")
	if u.Status != store.UserDisabled || len(e.store.roles[u.ID]) != 0 || len(e.store.revoked) != 1 {
		t.Fatalf("akun = %+v role=%v dicabut=%v", u, e.store.roles[u.ID], e.store.revoked)
	}
	e.store.activateErr = nil
	if _, err := e.admin().CreateAdmin(ctx, in); err != nil {
		t.Fatalf("lanjutan: %v", err)
	}
	if e.store.users[u.ID].Status != store.UserActive {
		t.Error("akun harus aktif setelah dilanjutkan")
	}
}

// Lanjutan mengatur ulang sandi dari masukan baru; semua keadaan lain tetap ErrEmailTaken tanpa mengubah akun.
func TestAdmin_CreateAdmin_ResumeBranches(t *testing.T) {
	login := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		setup     func(e *fakeEnv, id string)
		wantTaken bool
	}{
		{"sisa nonaktif tanpa role, tanpa login, tanpa sesi", func(*fakeEnv, string) {}, false},
		{"akun aktif", func(e *fakeEnv, id string) {
			u := e.store.users[id]
			u.Status = store.UserActive
			e.store.users[id] = u
		}, true},
		{"nonaktif tetapi punya role (mis. role lain)", func(e *fakeEnv, id string) { e.store.roles[id] = []string{"viewer"} }, true},
		{"nonaktif tetapi punya role administrator", func(e *fakeEnv, id string) { e.store.roles[id] = []string{mfa.AdministratorRole} }, true},
		{"nonaktif tetapi pernah login", func(e *fakeEnv, id string) { u := e.store.users[id]; u.LastLoginAt = &login; e.store.users[id] = u }, true},
		{"nonaktif tetapi punya sesi (termasuk tercabut)", func(e *fakeEnv, id string) { e.store.sessionCnt[id] = 1 }, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newFakeEnv()
			e.store.users[userA] = store.User{ID: userA, Email: "dupe@example.test", Name: "Lama", Status: store.UserDisabled, PasswordHash: "h:lama"}
			c.setup(e, userA)
			before := e.store.users[userA]
			rolesBefore := slices.Clone(e.store.roles[userA])
			sum, err := e.admin().CreateAdmin(systemCtx("cli:core:create-admin"), sdkidentity.CreateAdminInput{Email: "Dupe@example.test", Name: "Baru", Password: goodPassword})
			if c.wantTaken {
				if !errors.Is(err, sdkidentity.ErrEmailTaken) {
					t.Fatalf("err = %v, want ErrEmailTaken", err)
				}
				if e.store.users[userA].PasswordHash != before.PasswordHash || e.store.users[userA].Status != before.Status {
					t.Errorf("akun yang ada tidak boleh berubah: %+v", e.store.users[userA])
				}
				if !slices.Equal(e.store.roles[userA], rolesBefore) || len(e.audit.events) != 0 || len(e.store.hashSet) != 0 {
					t.Errorf("tidak boleh ada perubahan: roles=%v audit=%v hash=%v", e.store.roles[userA], e.audit.events, e.store.hashSet)
				}
				return
			}
			if err != nil || sum.ID != userA {
				t.Fatalf("lanjutan: %+v %v", sum, err)
			}
			got := e.store.users[userA]
			if got.Status != store.UserActive || got.PasswordHash != "h:"+goodPassword {
				t.Errorf("akun = %+v (sandi harus dari masukan baru lewat HashGate)", got)
			}
			if !slices.Equal(e.store.roles[userA], []string{mfa.AdministratorRole}) {
				t.Errorf("role = %v", e.store.roles[userA])
			}
		})
	}
}

// Kegagalan di tengah lanjutan: akun tidak pernah aktif tanpa role.
func TestAdmin_CreateAdmin_ResumeFailureKeepsAccountDisabled(t *testing.T) {
	e := newFakeEnv()
	ctx := systemCtx("cli:core:create-admin")
	e.store.users[userA] = store.User{ID: userA, Email: "r@example.test", Name: "R", Status: store.UserDisabled, PasswordHash: "h:lama"}
	e.store.grantErr = errBoom
	in := sdkidentity.CreateAdminInput{Email: "r@example.test", Name: "R", Password: goodPassword}
	if _, err := e.admin().CreateAdmin(ctx, in); err == nil {
		t.Fatal("harus galat")
	}
	if e.store.users[userA].Status != store.UserDisabled || len(e.store.roles[userA]) != 0 {
		t.Fatalf("akun = %+v role=%v", e.store.users[userA], e.store.roles[userA])
	}
	// Sandi yang tak memenuhi kebijakan ditolak sebelum menyentuh akun mana pun.
	e.store.grantErr = nil
	e.store.hashSet = map[string]string{}
	if _, err := e.admin().CreateAdmin(ctx, sdkidentity.CreateAdminInput{Email: "r@example.test", Name: "R", Password: "pendek"}); err == nil || len(e.store.hashSet) != 0 {
		t.Errorf("sandi lemah: %v %v", err, e.store.hashSet)
	}
}

func TestAdmin_RotateSigningKeys_ReportsPendingKey(t *testing.T) {
	e := newFakeEnv()
	info, err := e.admin().RotateSigningKeys(systemCtx("cli:core:rotate-keys"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Status != sdkidentity.KeyStatusPending || info.KeyID != "kid-baru-0001" || info.Algorithm != "EdDSA" || !info.ActivatesAt.After(info.CreatedAt) {
		t.Fatalf("info = %+v", info)
	}
	if e.keys.rotated != 1 {
		t.Errorf("Rotate = %d", e.keys.rotated)
	}
}

func TestAdmin_ResetMFA_RevokesSessionsAndInvalidates(t *testing.T) {
	e := newFakeEnv()
	e.store.addUser(userB, "b@example.test", "h:x")
	adm := e.admin()
	if err := adm.ResetMFA(systemCtx("cli:core:reset-mfa"), "B@Example.test"); err != nil {
		t.Fatal(err)
	}
	if e.mfa.resetActor != "cli:core:reset-mfa" || e.mfa.resetUser != userB {
		t.Errorf("Reset(actor=%q, user=%q)", e.mfa.resetActor, e.mfa.resetUser)
	}
	if len(e.sessions.revokedAll) != 1 || e.sessions.revokedAll[0] != userB {
		t.Errorf("sesi harus dicabut: %v", e.sessions.revokedAll)
	}
	if slicesIndex(e.status.invalidated, userB) < 0 {
		t.Error("cache keadaan MFA harus dibuang")
	}
	// Principal administrator: actor = id administrator (mfa.Service menegakkannya lagi).
	e.store.roles[userA] = []string{mfa.AdministratorRole}
	if err := adm.ResetMFA(principalCtx(userA, sessA, "pwd", "otp"), "b@example.test"); err != nil {
		t.Fatal(err)
	}
	if e.mfa.resetActor != userA {
		t.Errorf("actor = %q", e.mfa.resetActor)
	}
	if err := adm.ResetMFA(systemCtx("cli:core:reset-mfa"), "tidak-ada@example.test"); !errors.Is(err, sdkidentity.ErrNotFound) {
		t.Errorf("sasaran tak ada: %v", err)
	}
	// mfa.Service menolak (mis. mereset diri sendiri): ErrForbidden, sesi tidak dicabut.
	e.mfa.resetErr = mfa.ErrForbidden
	e.sessions.revokedAll = nil
	if err := adm.ResetMFA(systemCtx("cli:core:reset-mfa"), "b@example.test"); !errors.Is(err, sdkidentity.ErrForbidden) || len(e.sessions.revokedAll) != 0 {
		t.Errorf("err=%v revokedAll=%v", err, e.sessions.revokedAll)
	}
}

func TestAdmin_UnlockLogoutAllAndSetPublicURL(t *testing.T) {
	e := newFakeEnv()
	e.store.addUser(userB, "b@example.test", "h:x")
	adm := e.admin()
	ctx := systemCtx("cli:core:unlock")
	for i := 0; i < 2; i++ { // idempoten
		if err := adm.UnlockAccount(ctx, "b@example.test"); err != nil {
			t.Fatal(err)
		}
	}
	if len(e.lock.unlocked) != 2 || e.lock.unlocked[0] != userB {
		t.Errorf("unlocked = %v", e.lock.unlocked)
	}
	if err := adm.LogoutAllDevices(ctx, userB); err != nil || e.sessions.revokedAll[0] != userB {
		t.Errorf("LogoutAllDevices: %v %v", err, e.sessions.revokedAll)
	}
	if err := adm.LogoutAllDevices(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, sdkidentity.ErrNotFound) {
		t.Errorf("pengguna tak ada: %v", err)
	}
	if err := adm.SetPublicURL(systemCtx("cli:core:settings:set"), "https://erp.example.com"); err != nil {
		t.Fatal(err)
	}
	if e.settings.got[0] != "https://erp.example.com|cli:core:settings:set" {
		t.Errorf("SetPublicURL = %v", e.settings.got)
	}
	e.settings.err = settings.ErrInvalidURL
	if err := adm.SetPublicURL(ctx, "ftp://x"); !errors.Is(err, sdkidentity.ErrInvalidInput) {
		t.Errorf("URL tak sah: %v", err)
	}
}

func TestMapErr_DoesNotLeakInternalText(t *testing.T) {
	err := mapErr("op", errBoom)
	if errors.Is(err, errBoom) == false {
		t.Fatal("galat internal tetap dibungkus untuk log")
	}
	for _, d := range []error{sdkidentity.ErrInvalidCredentials, sdkidentity.ErrForbidden, sdkidentity.ErrInvalidMFACode} {
		if errors.Is(err, d) {
			t.Errorf("galat infrastruktur tidak boleh jadi %v", d)
		}
	}
}
