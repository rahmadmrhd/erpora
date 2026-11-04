//go:build integration

package app

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"erpora/internal/platform/config"
	identitystore "erpora/internal/platform/identity/store"
	"erpora/internal/platform/modules/lifecycle"
	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/tenancy"
	sdkauthz "erpora/sdk/authz"
	sdkidentity "erpora/sdk/identity"
	"erpora/sdk/module"
)

// Tes command CLI core (C1.12) lewat RunModuleCommand pada database sungguhan, dengan stack CLI (tanpa Service).

// cliResult adalah hasil satu eksekusi command: keluaran, galat, dan log proses CLI (untuk memeriksa audit dan kebocoran).
type cliResult struct {
	out string
	log string
	err error
}

func (e *svcEnv) cli(name string, flags map[string]string, stdin string) cliResult {
	return e.cliWith(e.cfg, name, flags, stdin)
}

func (e *svcEnv) cliWith(cfg config.Config, name string, flags map[string]string, stdin string) cliResult {
	e.t.Helper()
	var out bytes.Buffer
	logs := &syncBuffer{}
	err := RunModuleCommand(context.Background(), cfg, newBufLogger(logs), CommandRun{
		Command: coreCommand(e.t, name), Flags: flags, Out: &out, In: strings.NewReader(stdin), Actor: "cli:tester",
	})
	return cliResult{out: out.String(), log: logs.String(), err: err}
}

func (e *svcEnv) userStatus(email string) (identitystore.User, error) {
	e.t.Helper()
	return e.a.Identity.Store.FindUserByEmail(e.ctx, e.a.Tenant, email)
}

// Admin dibuat lewat command (sandi dari stdin), dapat login, wajib mendaftarkan MFA; sandi tidak pernah muncul di
// keluaran maupun log; audit mencatat pelaku "cli:core:create-admin".
func TestCoreCLI_CreateAdminThenLogin(t *testing.T) {
	e := newCoreEnv(t)
	e.remember(pwAdmin)
	r := e.cli("core:create-admin", map[string]string{"email": "Admin.Cli@Example.test", "name": "Admin CLI"}, pwAdmin+"\n")
	if r.err != nil {
		t.Fatalf("create-admin: %v", r.err)
	}
	if strings.Contains(r.out+r.log, pwAdmin) {
		t.Error("sandi bocor ke keluaran atau log command")
	}
	if !strings.Contains(r.out, "admin.cli@example.test") {
		t.Errorf("keluaran = %q", r.out)
	}
	if !strings.Contains(r.log, "identity.admin_created") || !strings.Contains(r.log, "cli:core:create-admin") {
		t.Errorf("audit pembuatan admin tidak tercatat dengan pelaku cli:core:create-admin:\n%s", r.log)
	}
	l := e.mustLogin("admin.cli@example.test", pwAdmin)
	if l.MFAState != sdkidentity.MFAEnrollRequired {
		t.Errorf("administrator baru harus diminta mendaftarkan MFA, dapat %s", l.MFAState)
	}
	u, err := e.userStatus("admin.cli@example.test")
	if err != nil || u.Name != "Admin CLI" {
		t.Fatalf("pengguna = %+v, %v", u, err)
	}
}

func TestCoreCLI_CreateAdminDefaultName(t *testing.T) {
	e := newCoreEnv(t)
	if r := e.cli("core:create-admin", map[string]string{"email": "tanpa.nama@example.test"}, pwAdmin+"\n"); r.err != nil {
		t.Fatal(r.err)
	}
	if u, err := e.userStatus("tanpa.nama@example.test"); err != nil || u.Name == "" {
		t.Fatalf("nama bawaan kosong: %+v, %v", u, err)
	}
}

// Sandi lemah ditolak oleh kebijakan FR-101 tanpa membuat akun dan tanpa membocorkan sandi.
func TestCoreCLI_CreateAdminWeakPasswordRejected(t *testing.T) {
	e := newCoreEnv(t)
	const weak = "pendek123"
	r := e.cli("core:create-admin", map[string]string{"email": "lemah@example.test"}, weak+"\n")
	if r.err == nil || !strings.Contains(r.err.Error(), "kebijakan") || !strings.Contains(r.err.Error(), "too_short") {
		t.Fatalf("err = %v, want penolakan kebijakan too_short", r.err)
	}
	if strings.Contains(r.out+r.log+r.err.Error(), weak) {
		t.Error("sandi bocor")
	}
	if _, err := e.userStatus("lemah@example.test"); err == nil {
		t.Error("akun tidak boleh terbentuk untuk sandi lemah")
	}
}

// PRD-001 Q28: sisa kegagalan (akun nonaktif, tanpa role, belum login, tanpa sesi) dilanjutkan; akun yang sudah aktif
// atau berrole ditolak ErrEmailTaken dan sandinya tidak berubah.
func TestCoreCLI_CreateAdminResumesLeftoverOtherwiseEmailTaken(t *testing.T) {
	e := newCoreEnv(t)
	phc, err := e.a.Identity.Hasher.Hash("sandi-lama-sisa-2026")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.a.Identity.Store.CreateUser(e.ctx, e.a.Tenant, identitystore.NewUser{
		Email: "sisa@example.test", Name: "Sisa", PasswordHash: phc, Status: identitystore.UserDisabled,
	}); err != nil {
		t.Fatal(err)
	}
	r := e.cli("core:create-admin", map[string]string{"email": "sisa@example.test", "name": "Sisa"}, pwAdmin+"\n")
	if r.err != nil {
		t.Fatalf("sisa harus dilanjutkan: %v", r.err)
	}
	if !strings.Contains(r.log, "identity.admin_resumed") {
		t.Errorf("audit harus mencatat admin_resumed:\n%s", r.log)
	}
	e.mustLogin("sisa@example.test", pwAdmin)

	// Kini aktif dan berrole: panggilan ketiga tidak boleh mengambil alih akun.
	r = e.cli("core:create-admin", map[string]string{"email": "sisa@example.test"}, "sandi-pengambilalihan-2026\n")
	if r.err == nil || !strings.Contains(r.err.Error(), "sudah dipakai") {
		t.Fatalf("akun aktif harus ditolak ErrEmailTaken: %v", r.err)
	}
	if _, err := e.login("sisa@example.test", pwAdmin); err != nil {
		t.Errorf("sandi akun yang ada tidak boleh berubah: %v", err)
	}
}

// rotate-keys hanya membuat kunci pending (terlaporkan), dan pekerja promosi serve menjadikannya active setelah
// JWKSMaxAge (jam tiruan lewat Tick).
func TestCoreCLI_RotateKeysCreatesPendingThenServePromotes(t *testing.T) {
	e := newCoreEnv(t)
	before, err := e.a.Identity.Store.ListSigningKeys(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	r := e.cli("core:rotate-keys", nil, "")
	if r.err != nil {
		t.Fatal(r.err)
	}
	if !strings.Contains(r.out, "pending") || !strings.Contains(r.out, "aktif pada") {
		t.Errorf("keluaran = %q", r.out)
	}
	if !strings.Contains(r.log, "identity.signing_key_rotated") || !strings.Contains(r.log, "cli:core:rotate-keys") {
		t.Errorf("audit rotasi tidak tercatat:\n%s", r.log)
	}
	after, err := e.a.Identity.Store.ListSigningKeys(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before)+1 {
		t.Fatalf("kunci %d -> %d, want +1", len(before), len(after))
	}
	var pending []identitystore.SigningKey
	for _, k := range after {
		if k.Status == sdkidentity.KeyStatusPending {
			pending = append(pending, k)
		}
	}
	if len(pending) != 1 || !strings.Contains(r.out, pending[0].ID) {
		t.Fatalf("harus tepat satu kunci pending yang dilaporkan: %+v\n%s", pending, r.out)
	}
	// Belum matang: Tick pada waktu sekarang tidak mempromosikan.
	if err := e.a.Identity.Promoter.Tick(e.ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if k := e.keyStatus(pending[0].ID); k != sdkidentity.KeyStatusPending {
		t.Fatalf("kunci belum matang tidak boleh dipromosikan, status %s", k)
	}
	// Setelah JWKSMaxAge: dipromosikan menjadi active oleh pekerja serve; kunci lama turun menjadi retiring.
	if err := e.a.Identity.Promoter.Tick(e.ctx, time.Now().Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if k := e.keyStatus(pending[0].ID); k != sdkidentity.KeyStatusActive {
		t.Fatalf("kunci matang harus active, status %s", k)
	}
}

func (e *svcEnv) keyStatus(kid string) string {
	e.t.Helper()
	keys, err := e.a.Identity.Store.ListSigningKeys(e.ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	for _, k := range keys {
		if k.ID == kid {
			return k.Status
		}
	}
	e.t.Fatalf("kunci %s tidak ada", kid)
	return ""
}

// reset-mfa menghapus faktor dan mencabut sesi; keduanya tercatat dengan pelaku cli:core:reset-mfa.
func TestCoreCLI_ResetMFARecorded(t *testing.T) {
	e := newCoreEnv(t)
	e.enrollAdmin("hilang.hp@example.test")
	old := e.mustLogin("hilang.hp@example.test", pwAdmin) // verify_required
	if old.MFAState != sdkidentity.MFAVerifyRequired {
		t.Fatalf("login = %+v", old)
	}
	r := e.cli("core:reset-mfa", map[string]string{"email": "hilang.hp@example.test"}, "")
	if r.err != nil {
		t.Fatal(r.err)
	}
	if !strings.Contains(r.log, "identity.mfa_reset") || !strings.Contains(r.log, "cli:core:reset-mfa") {
		t.Errorf("audit reset MFA tidak tercatat:\n%s", r.log)
	}
	// Pencabutan ditulis ke database oleh proses CLI; instance serve yang berjalan menerimanya lewat muat ulang daftar
	// tercabut (berkala 1 menit; di sini dipaksa).
	if err := e.a.Identity.Revocations.Load(tenancy.WithTenant(context.Background(), e.a.Tenant)); err != nil {
		t.Fatal(err)
	}
	if e.status(old.AccessToken) != 401 {
		t.Errorf("sesi lama harus tercabut, status %d", e.status(old.AccessToken))
	}
	if l := e.mustLogin("hilang.hp@example.test", pwAdmin); l.MFAState != sdkidentity.MFAEnrollRequired {
		t.Errorf("setelah reset, administrator harus mendaftar ulang MFA: %s", l.MFAState)
	}
	if r := e.cli("core:reset-mfa", map[string]string{"email": "tidak.ada@example.test"}, ""); r.err == nil || !strings.Contains(r.err.Error(), "tidak ditemukan") {
		t.Errorf("sasaran tak ada: %v", r.err)
	}
}

// unlock membuka akun locked dan tercatat dengan pelaku cli:core:unlock.
func TestCoreCLI_UnlockRecorded(t *testing.T) {
	e := newCoreEnv(t)
	u := e.createUser("terkunci@example.test", pwA)
	if ok, err := e.a.Identity.Store.LockUser(e.ctx, e.a.Tenant, u.ID, time.Now()); err != nil || !ok {
		t.Fatalf("LockUser = %v, %v", ok, err)
	}
	if got, _ := e.userStatus(u.Email); got.Status != identitystore.UserLocked {
		t.Fatalf("status awal = %s", got.Status)
	}
	r := e.cli("core:unlock", map[string]string{"email": u.Email}, "")
	if r.err != nil {
		t.Fatal(r.err)
	}
	if got, _ := e.userStatus(u.Email); got.Status != identitystore.UserActive {
		t.Errorf("status setelah unlock = %s", got.Status)
	}
	if !strings.Contains(r.log, "identity.account_unlocked") || !strings.Contains(r.log, "cli:core:unlock") {
		t.Errorf("audit unlock tidak tercatat:\n%s", r.log)
	}
}

// settings:set memvalidasi (https wajib di production), tercatat, dan hanya mengenal platform.public_url.
func TestCoreCLI_SettingsSet(t *testing.T) {
	e := newCoreEnv(t)
	tctx := tenancy.WithTenant(context.Background(), e.a.Tenant)
	pubURL := func() string {
		t.Helper()
		u, err := newSettingsService(e.cfg, openTestDB(t, e.cfg.DatabaseURL), e.a.Tenant).PublicURL(tctx)
		if err != nil {
			t.Fatalf("PublicURL: %v", err)
		}
		return u
	}

	// Production: http ditolak, kunci lain ditolak, https diterima dan dinormalisasi.
	prod := e.cfg
	prod.Env = config.EnvProduction
	r := e.cliWith(prod, "core:settings:set", map[string]string{"key": "platform.public_url", "value": "http://erp.example.com"}, "")
	if r.err == nil || !strings.Contains(r.err.Error(), "https wajib") {
		t.Fatalf("http di production harus ditolak: %v", r.err)
	}
	if r := e.cliWith(prod, "core:settings:set", map[string]string{"key": "platform.lain", "value": "x"}, ""); r.err == nil {
		t.Error("kunci tak dikenal harus ditolak")
	}
	r = e.cliWith(prod, "core:settings:set", map[string]string{"key": "platform.public_url", "value": "https://ERP.example.com/"}, "")
	if r.err != nil {
		t.Fatalf("https di production: %v", r.err)
	}
	if got := pubURL(); got != "https://erp.example.com" {
		t.Errorf("URL tersimpan = %q, want dinormalisasi", got)
	}
	if !strings.Contains(r.log, "identity.public_url_set") || !strings.Contains(r.log, "cli:core:settings:set") {
		t.Errorf("audit pengisian URL publik tidak tercatat:\n%s", r.log)
	}
	// Development: http lokal diterima.
	if r := e.cli("core:settings:set", map[string]string{"key": "platform.public_url", "value": "http://localhost:8080"}, ""); r.err != nil {
		t.Fatalf("http di development: %v", r.err)
	}
	if got := pubURL(); got != "http://localhost:8080" {
		t.Errorf("URL tersimpan = %q", got)
	}
}

// core:* tanpa ERP_SECRET_KEY ditolak (exit 2) walau database siap dan core enabled.
func TestCoreCLI_RejectsWithoutSecretKey(t *testing.T) {
	e := newCoreEnv(t)
	cfg := e.cfg
	cfg.SecretKey = nil
	r := e.cliWith(cfg, "core:rotate-keys", nil, "")
	var xe *ExitError
	if !errors.As(r.err, &xe) || xe.Code != 2 || !strings.Contains(r.err.Error(), "ERP_SECRET_KEY") {
		t.Fatalf("err = %v, want exit 2 ERP_SECRET_KEY", r.err)
	}
}

// Command modul non-core menerima stdin (Invocation.In) tetapi tidak pernah identity.Admin/Service; command core
// berjalan di bawah hak sistem berawalan cli:core:.
func TestRunModuleCommand_InputPassedAndIdentityOnlyForCore(t *testing.T) {
	e := newCoreEnv(t)
	if _, err := e.a.Lifecycle.Execute(e.ctx, lifecycle.Request{Tenant: e.a.Tenant, Op: lifecycle.OpInstall, Key: "acme.example", Enable: true, Actor: "cli:test"}); err != nil {
		t.Fatal(err)
	}
	reg, err := BuildRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var seen context.Context
	var gotIn string
	probe := func(name string) registry.CommandInfo {
		for _, c := range reg.Commands() {
			if c.Name == name {
				orig := c.Command.Run
				c.Command.Run = func(ctx context.Context, inv module.Invocation) error {
					seen = ctx
					if inv.In != nil {
						b := new(bytes.Buffer)
						_, _ = b.ReadFrom(inv.In)
						gotIn = b.String()
					}
					return orig(ctx, inv)
				}
				return c
			}
		}
		t.Fatalf("command %s tidak ada", name)
		return registry.CommandInfo{}
	}

	var out bytes.Buffer
	if err := RunModuleCommand(context.Background(), e.cfg, testLog(), CommandRun{
		Command: probe("example:recount"), Out: &out, In: strings.NewReader("masukan-stdin"), Actor: "cli:tester",
	}); err != nil {
		t.Fatal(err)
	}
	if gotIn != "masukan-stdin" {
		t.Errorf("Invocation.In = %q", gotIn)
	}
	if _, ok := sdkidentity.AdminFrom(seen); ok {
		t.Error("modul selain core tidak boleh menerima identity.Admin")
	}
	if _, ok := sdkidentity.ServiceFrom(seen); ok {
		t.Error("modul selain core tidak boleh menerima identity.Service")
	}

	out.Reset()
	if err := RunModuleCommand(context.Background(), e.cfg, testLog(), CommandRun{
		Command: probe("core:rotate-keys"), Out: &out, Actor: "cli:tester",
	}); err != nil {
		t.Fatal(err)
	}
	if reason, ok := sdkauthz.SystemReason(seen); !ok || reason != "cli:core:rotate-keys" {
		t.Errorf("hak sistem = %q, %v", reason, ok)
	}
	if _, ok := sdkidentity.AdminFrom(seen); !ok {
		t.Error("command core harus menerima identity.Admin")
	}
	if _, ok := sdkidentity.ServiceFrom(seen); ok {
		t.Error("varian CLI tidak memasang identity.Service")
	}
}
