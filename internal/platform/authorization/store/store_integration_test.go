//go:build integration

package store_test

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	"erpora/internal/platform/authorization/store"
	"erpora/internal/platform/config"
	"erpora/internal/platform/database"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/tenancy"
	"erpora/internal/platform/testsupport/pgtest"
)

var (
	tenantA = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"))
	tenantB = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c"))
)

// perTenant memetakan tiap tenant ke database terpisah (ADR-0004: isolasi per database).
type perTenant map[tenancy.TenantID]database.TenantDB

func (p perTenant) For(_ context.Context, id tenancy.TenantID) (database.TenantDB, error) {
	db, ok := p[id]
	if !ok {
		return database.TenantDB{}, database.ErrUnknownTenant
	}
	return db, nil
}

func openMigrated(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := database.Open(context.Background(),
		config.Config{DatabaseURL: pgtest.Start(t), DBSlowQuery: 200 * time.Millisecond}, noop.NewTracerProvider(), slog.Default())
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
	return db
}

func ctxOf(t tenancy.TenantID) context.Context { return tenancy.WithTenant(context.Background(), t) }

type env struct {
	t   *testing.T
	db  *gorm.DB
	s   *store.Store
	ctx context.Context
}

func setup(t *testing.T) *env {
	t.Helper()
	db := openMigrated(t)
	return &env{t: t, db: db, s: store.New(perTenant{tenantA: {DB: db}}), ctx: ctxOf(tenantA)}
}

func (e *env) rev() int64 {
	e.t.Helper()
	r, err := e.s.Revision(e.ctx)
	if err != nil {
		e.t.Fatalf("Revision: %v", err)
	}
	return r
}

func (e *env) role(key string) store.Role {
	e.t.Helper()
	r, _, err := e.s.CreateRole(e.ctx, store.NewRole{Key: key, Label: "Role " + key, Kind: store.KindCustom})
	if err != nil {
		e.t.Fatalf("CreateRole(%s): %v", key, err)
	}
	return r
}

func (e *env) systemRole(key string) store.Role {
	e.t.Helper()
	roles, err := e.s.ListRoles(e.ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	for _, r := range roles {
		if r.Key == key {
			return r
		}
	}
	e.t.Fatalf("role %s tidak ada", key)
	return store.Role{}
}

func row(res string, actions []string, scope ...string) store.PermissionRow {
	return store.PermissionRow{Resource: res, Actions: actions, Scope: scope}
}

func fa(res, group, access string) store.FieldAccessRow {
	return store.FieldAccessRow{Resource: res, FieldGroup: group, Access: access}
}

func TestRevisionStartsAtOneAndNeedsTenant(t *testing.T) {
	e := setup(t)
	if got := e.rev(); got != 1 {
		t.Fatalf("revisi awal = %d, want 1", got)
	}
	if _, err := e.s.Revision(context.Background()); !errors.Is(err, store.ErrNoTenant) {
		t.Errorf("tanpa tenant: err = %v, want ErrNoTenant", err)
	}
	if _, err := e.s.Revision(ctxOf(tenantB)); !errors.Is(err, database.ErrUnknownTenant) {
		t.Errorf("tenant tak dikenal: err = %v, want ErrUnknownTenant (gagal tertutup)", err)
	}
	if _, err := store.New(nil).Revision(e.ctx); err == nil {
		t.Error("TenantConnector nil harus galat")
	}
}

func TestBumpRevisionRollbackDoesNotIncrement(t *testing.T) {
	e := setup(t)
	boom := errors.New("batal")
	err := e.db.Transaction(func(tx *gorm.DB) error {
		if n, err := store.BumpRevision(tx); err != nil || n != 2 {
			t.Errorf("BumpRevision dalam tx = %d, %v, want 2", n, err)
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if got := e.rev(); got != 1 {
		t.Fatalf("revisi setelah rollback = %d, want 1", got)
	}
	if err := e.db.Transaction(func(tx *gorm.DB) error { _, err := store.BumpRevision(tx); return err }); err != nil {
		t.Fatal(err)
	}
	if got := e.rev(); got != 2 {
		t.Fatalf("revisi setelah commit = %d, want 2", got)
	}
}

// Dua transaksi yang naik serentak menerima nilai berbeda (pembaruan baris authz_state mengunci sampai commit).
func TestBumpRevisionConcurrentTransactionsGetDistinctValues(t *testing.T) {
	e := setup(t)
	const n = 6
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		got  []int64
		errs []error
	)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			err := e.db.Transaction(func(tx *gorm.DB) error {
				v, err := store.BumpRevision(tx)
				if err != nil {
					return err
				}
				time.Sleep(20 * time.Millisecond) // tahan kunci agar yang lain benar-benar bersaing
				mu.Lock()
				got = append(got, v)
				mu.Unlock()
				return nil
			})
			if err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()
	if len(errs) > 0 {
		t.Fatalf("galat: %v", errs)
	}
	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	for i, v := range got {
		if v != int64(i)+2 {
			t.Fatalf("nilai revisi = %v, want 2..%d berurutan tanpa duplikat", got, n+1)
		}
	}
	if e.rev() != int64(n)+1 {
		t.Fatalf("revisi akhir = %d", e.rev())
	}
}

// Setiap metode tulis menaikkan revisi tepat satu kali dan mengembalikan nilai barunya.
func TestEveryWriteBumpsRevisionExactlyOnce(t *testing.T) {
	e := setup(t)
	user := uuid.NewString()
	var r1, r2 store.Role
	steps := []struct {
		name string
		do   func() (int64, error)
	}{
		{"CreateRole", func() (int64, error) {
			r, rev, err := e.s.CreateRole(e.ctx, store.NewRole{Key: "kasir", Label: "Kasir", Kind: store.KindCustom})
			r1 = r
			return rev, err
		}},
		{"UpdateRole", func() (int64, error) {
			lbl := "Kasir Bandung"
			_, rev, err := e.s.UpdateRole(e.ctx, r1.ID, store.RoleUpdate{Label: &lbl})
			return rev, err
		}},
		{"ReplaceRoleGrants", func() (int64, error) {
			return e.s.ReplaceRoleGrants(e.ctx, r1.ID, store.RoleGrants{
				Permissions: []store.PermissionRow{row("example.note", []string{"read"}, "all")},
				FieldAccess: []store.FieldAccessRow{fa("example.note", "profit", store.AccessNone)},
			}, nil)
		}},
		{"DuplicateRole", func() (int64, error) {
			r, rev, err := e.s.DuplicateRole(e.ctx, r1.ID, store.NewRole{Key: "kasir.salinan", Label: "Salinan", Kind: store.KindCustom})
			r2 = r
			return rev, err
		}},
		{"SetUserRoles", func() (int64, error) { return e.s.SetUserRoles(e.ctx, user, []string{r1.ID}) }},
		{"SetUserRoles kosong", func() (int64, error) { return e.s.SetUserRoles(e.ctx, user, nil) }},
		{"SetScopeAssignments", func() (int64, error) {
			return e.s.SetScopeAssignments(e.ctx, user, []store.ScopeAssignment{{Dimension: "inventory.warehouse", Values: []string{"wh-01"}}})
		}},
		{"SetScopeAssignments kosong", func() (int64, error) { return e.s.SetScopeAssignments(e.ctx, user, nil) }},
		{"SyncModuleDefaultRoles", func() (int64, error) {
			res, err := e.s.SyncModuleDefaultRoles(e.ctx, "inventory", []store.DefaultRoleDef{{Key: "inventory.staff", Label: "Staf"}})
			return res.Revision, err
		}},
		{"DeleteRole", func() (int64, error) { return e.s.DeleteRole(e.ctx, r2.ID) }},
	}
	for _, st := range steps {
		before := e.rev()
		got, err := st.do()
		if err != nil {
			t.Fatalf("%s: %v", st.name, err)
		}
		if after := e.rev(); after != before+1 || got != after {
			t.Errorf("%s: revisi %d -> %d (dikembalikan %d), want naik tepat satu", st.name, before, after, got)
		}
	}
}

// Penulisan yang gagal membatalkan seluruhnya: data lama utuh dan revisi tidak naik.
func TestFailedWriteRollsBackDataAndRevision(t *testing.T) {
	e := setup(t)
	user := uuid.NewString()
	keep := e.role("simpan")
	other := e.role("lain")
	if _, err := e.s.SetUserRoles(e.ctx, user, []string{keep.ID}); err != nil {
		t.Fatal(err)
	}
	before := e.rev()

	// role kedua tidak ada: pencabutan "simpan" tidak boleh terjadi
	if _, err := e.s.SetUserRoles(e.ctx, user, []string{other.ID, uuid.NewString()}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("role tak ada: err = %v, want ErrNotFound", err)
	}
	ids, err := e.s.UserRoleIDs(e.ctx, user)
	if err != nil || len(ids) != 1 || ids[0] != keep.ID {
		t.Fatalf("role pengguna setelah gagal = %v, %v, want tetap [simpan]", ids, err)
	}
	// role authenticated implisit tidak dapat diberikan lewat baris
	auth := e.systemRole(store.RoleAuthenticated)
	if _, err := e.s.SetUserRoles(e.ctx, user, []string{auth.ID}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("authenticated eksplisit: err = %v, want ErrNotFound", err)
	}
	// kunci sudah dipakai
	if _, _, err := e.s.CreateRole(e.ctx, store.NewRole{Key: "simpan", Label: "Ganda", Kind: store.KindCustom}); !errors.Is(err, store.ErrConflict) {
		t.Errorf("key ganda: err = %v, want ErrConflict", err)
	}
	if _, _, err := e.s.DuplicateRole(e.ctx, keep.ID, store.NewRole{Key: "simpan", Label: "Ganda", Kind: store.KindCustom}); !errors.Is(err, store.ErrConflict) {
		t.Errorf("duplikasi key ganda: err = %v, want ErrConflict", err)
	}
	if _, _, err := e.s.DuplicateRole(e.ctx, uuid.NewString(), store.NewRole{Key: "z", Label: "Z", Kind: store.KindCustom}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("sumber tak ada: err = %v, want ErrNotFound", err)
	}
	// stale
	old := keep.UpdatedAt.Add(-time.Hour)
	lbl := "Baru"
	if _, _, err := e.s.UpdateRole(e.ctx, keep.ID, store.RoleUpdate{Label: &lbl, IfUpdatedAt: &old}); !errors.Is(err, store.ErrStaleVersion) {
		t.Errorf("versi basi: err = %v, want ErrStaleVersion", err)
	}
	if _, err := e.s.ReplaceRoleGrants(e.ctx, keep.ID, store.RoleGrants{}, &old); !errors.Is(err, store.ErrStaleVersion) {
		t.Errorf("versi basi grants: err = %v, want ErrStaleVersion", err)
	}
	// role dipegang
	if _, err := e.s.DeleteRole(e.ctx, keep.ID); !errors.Is(err, store.ErrRoleInUse) {
		t.Errorf("hapus role dipegang: err = %v, want ErrRoleInUse", err)
	}
	// konflik sinkronisasi: key milik role custom, bukan bawaan modul; sinkronisasi dibatalkan seluruhnya
	if _, err := e.s.SyncModuleDefaultRoles(e.ctx, "inventory", []store.DefaultRoleDef{
		{Key: "inventory.baru", Label: "Baru"}, {Key: "simpan", Label: "Bentrok"},
	}); !errors.Is(err, store.ErrConflict) {
		t.Errorf("sinkronisasi bentrok: err = %v, want ErrConflict", err)
	}
	for _, r := range mustRoles(t, e) {
		if r.Key == "inventory.baru" {
			t.Error("role inventory.baru tersisa setelah sinkronisasi dibatalkan")
		}
	}
	if after := e.rev(); after != before {
		t.Errorf("revisi %d -> %d setelah rangkaian penulisan yang semuanya gagal, want tidak berubah", before, after)
	}
}

func mustRoles(t *testing.T, e *env) []store.Role {
	t.Helper()
	rs, err := e.s.ListRoles(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

func TestRoleLifecycleAndImmutability(t *testing.T) {
	e := setup(t)
	admin := e.systemRole(store.RoleAdministrator)
	auth := e.systemRole(store.RoleAuthenticated)
	if admin.Kind != store.KindSystem || auth.Kind != store.KindSystem {
		t.Fatalf("role sistem: %+v %+v", admin, auth)
	}
	before := e.rev()
	lbl := "X"
	if _, _, err := e.s.UpdateRole(e.ctx, admin.ID, store.RoleUpdate{Label: &lbl}); !errors.Is(err, store.ErrImmutable) {
		t.Errorf("ubah administrator: err = %v, want ErrImmutable", err)
	}
	if _, err := e.s.ReplaceRoleGrants(e.ctx, admin.ID, store.RoleGrants{Permissions: []store.PermissionRow{row("a.b", []string{"read"}, "all")}}, nil); !errors.Is(err, store.ErrImmutable) {
		t.Errorf("ganti matriks administrator: err = %v, want ErrImmutable", err)
	}
	if _, err := e.s.DeleteRole(e.ctx, admin.ID); !errors.Is(err, store.ErrImmutable) {
		t.Errorf("hapus administrator: err = %v, want ErrImmutable", err)
	}
	if _, err := e.s.DeleteRole(e.ctx, auth.ID); !errors.Is(err, store.ErrImmutable) {
		t.Errorf("hapus authenticated: err = %v, want ErrImmutable", err)
	}
	// matriks authenticated dapat diubah (FR-113)
	if _, err := e.s.ReplaceRoleGrants(e.ctx, auth.ID, store.RoleGrants{Permissions: []store.PermissionRow{row("example.ping", []string{"read"}, "all")}}, nil); err != nil {
		t.Errorf("ganti matriks authenticated: %v", err)
	}
	if e.rev() != before+1 {
		t.Errorf("revisi = %d, want %d (hanya penulisan yang berhasil)", e.rev(), before+1)
	}
	// ID bukan UUID dan role tak ada
	for _, id := range []string{"bukan-uuid", uuid.NewString()} {
		if _, err := e.s.GetRole(e.ctx, id); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("GetRole(%q) err = %v", id, err)
		}
		if _, err := e.s.DeleteRole(e.ctx, id); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("DeleteRole(%q) err = %v", id, err)
		}
		if _, err := e.s.ReplaceRoleGrants(e.ctx, id, store.RoleGrants{}, nil); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("ReplaceRoleGrants(%q) err = %v", id, err)
		}
		if _, err := e.s.RoleGrantsOf(e.ctx, id); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("RoleGrantsOf(%q) err = %v", id, err)
		}
	}
	// role custom: buat, ubah, hapus (matriks ikut terhapus)
	r := e.role("kustom")
	if _, err := e.s.ReplaceRoleGrants(e.ctx, r.ID, store.RoleGrants{
		Permissions: []store.PermissionRow{row("example.note", []string{"read"}, "all")},
		FieldAccess: []store.FieldAccessRow{fa("example.note", "profit", store.AccessNone)},
	}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.DeleteRole(e.ctx, r.ID); err != nil {
		t.Fatalf("hapus custom: %v", err)
	}
	var n int
	if err := e.db.Raw(`SELECT (SELECT count(*) FROM role_permissions WHERE role_id = ?::uuid) + (SELECT count(*) FROM role_field_access WHERE role_id = ?::uuid)`, r.ID, r.ID).Scan(&n).Error; err != nil || n != 0 {
		t.Errorf("sisa hak role terhapus = %d, %v", n, err)
	}
	// label dirapikan dan dibatasi
	long := strings.Repeat("a", 121)
	if _, _, err := e.s.CreateRole(e.ctx, store.NewRole{Key: "panjang", Label: long, Kind: store.KindCustom}); !errors.Is(err, store.ErrInvalidArgument) {
		t.Errorf("label 121 karakter: err = %v", err)
	}
}

func TestReplaceRoleGrantsRoundTripReplacesWholly(t *testing.T) {
	e := setup(t)
	r := e.role("matriks")
	g1 := store.RoleGrants{
		Permissions: []store.PermissionRow{
			row("example.note", []string{"read"}, "all"),
			row("example.note", []string{"update", "delete"}, "owner", "assigned:inventory.warehouse"),
			row("example-addon.summary", []string{"read"}, "custom:example.mine"),
		},
		FieldAccess: []store.FieldAccessRow{fa("example.note", "profit", store.AccessNone), fa("example.note", "general", store.AccessWrite)},
	}
	if _, err := e.s.ReplaceRoleGrants(e.ctx, r.ID, g1, nil); err != nil {
		t.Fatal(err)
	}
	got, err := e.s.RoleGrantsOf(e.ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Permissions) != 3 || len(got.FieldAccess) != 2 {
		t.Fatalf("grants = %+v", got)
	}
	found := false
	for _, p := range got.Permissions {
		if p.ID == "" || p.ScopeInvalid {
			t.Errorf("baris %+v", p)
		}
		if p.Resource == "example.note" && len(p.Scope) == 2 {
			found = true
			if strings.Join(p.Actions, ",") != "update,delete" || p.Scope[0] != "owner" || p.Scope[1] != "assigned:inventory.warehouse" {
				t.Errorf("urutan aksi/scope tidak dipertahankan: %+v", p)
			}
		}
	}
	if !found {
		t.Error("baris AND (owner + assigned) tidak ditemukan")
	}
	// penggantian utuh: baris lama hilang
	g2 := store.RoleGrants{Permissions: []store.PermissionRow{row("example.ping", []string{"read"}, "all")}}
	if _, err := e.s.ReplaceRoleGrants(e.ctx, r.ID, g2, nil); err != nil {
		t.Fatal(err)
	}
	got, _ = e.s.RoleGrantsOf(e.ctx, r.ID)
	if len(got.Permissions) != 1 || got.Permissions[0].Resource != "example.ping" || len(got.FieldAccess) != 0 {
		t.Fatalf("setelah penggantian = %+v", got)
	}
	// kosongkan
	if _, err := e.s.ReplaceRoleGrants(e.ctx, r.ID, store.RoleGrants{}, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ = e.s.RoleGrantsOf(e.ctx, r.ID); len(got.Permissions) != 0 {
		t.Fatalf("setelah dikosongkan = %+v", got)
	}
}

// Masukan tak sah ditolak sebelum menyentuh DB: revisi tidak naik dan isi lama tetap.
func TestReplaceRoleGrantsRejectsInvalidScopeAndKeepsOldGrants(t *testing.T) {
	e := setup(t)
	r := e.role("tolak")
	if _, err := e.s.ReplaceRoleGrants(e.ctx, r.ID, store.RoleGrants{Permissions: []store.PermissionRow{row("example.note", []string{"read"}, "all")}}, nil); err != nil {
		t.Fatal(err)
	}
	before := e.rev()
	bad := map[string]store.RoleGrants{
		"scope kosong":  {Permissions: []store.PermissionRow{{Resource: "example.note", Actions: []string{"read"}, Scope: []string{}}}},
		"scope nil":     {Permissions: []store.PermissionRow{{Resource: "example.note", Actions: []string{"read"}}}},
		"token tak sah": {Permissions: []store.PermissionRow{row("example.note", []string{"read"}, "semua")}},
		"aksi kosong":   {Permissions: []store.PermissionRow{{Resource: "example.note", Scope: []string{"all"}}}},
		"access all":    {FieldAccess: []store.FieldAccessRow{fa("example.note", "profit", "all")}},
		"baris kedua rusak setelah yang sah": {Permissions: []store.PermissionRow{
			row("example.note", []string{"read"}, "all"), {Resource: "example.note", Actions: []string{"update"}, Scope: nil}}},
	}
	for name, g := range bad {
		if _, err := e.s.ReplaceRoleGrants(e.ctx, r.ID, g, nil); !errors.Is(err, store.ErrInvalidArgument) {
			t.Errorf("%s: err = %v, want ErrInvalidArgument", name, err)
		}
	}
	got, _ := e.s.RoleGrantsOf(e.ctx, r.ID)
	if len(got.Permissions) != 1 || got.Permissions[0].Actions[0] != "read" {
		t.Errorf("isi lama berubah: %+v", got)
	}
	if e.rev() != before {
		t.Errorf("revisi berubah oleh penulisan yang ditolak")
	}
}

// Nilai tersimpan rusak (hanya mungkin bila CHECK dilepas) ditandai ScopeInvalid oleh pemuat: mesin menolak, tidak
// pernah menganggapnya "all". (Review Focus 1, lapisan store.)
func TestLoadUserGrantsMarksCorruptStoredScopeInvalid(t *testing.T) {
	e := setup(t)
	r := e.role("rusak")
	user := uuid.NewString()
	if _, err := e.s.SetUserRoles(e.ctx, user, []string{r.ID}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{"role_permissions_scope_nonempty_array", "role_permissions_scope_tokens"} {
		if err := e.db.Exec(`ALTER TABLE role_permissions DROP CONSTRAINT ` + c).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, sc := range []string{`[]`, `"all"`, `{"all":true}`, `[1]`, `["bogus"]`} {
		if err := e.db.Exec(`INSERT INTO role_permissions (role_id, resource, actions, scope) VALUES (?::uuid, 'example.note', '{read}', ?::jsonb)`, r.ID, sc).Error; err != nil {
			t.Fatal(err)
		}
	}
	g, err := e.s.LoadUserGrants(e.ctx, user)
	if err != nil {
		t.Fatalf("LoadUserGrants: %v", err)
	}
	var n int
	for _, p := range g.Permissions {
		if p.RoleID != r.ID {
			continue
		}
		n++
		if !p.ScopeInvalid || p.Scope != nil {
			t.Errorf("baris rusak lolos: %+v", p)
		}
	}
	if n != 5 {
		t.Errorf("baris rusak terbaca = %d, want 5 (tidak dibuang diam-diam)", n)
	}
}

func TestDuplicateRoleCopiesGrants(t *testing.T) {
	e := setup(t)
	src := e.role("sumber")
	if _, err := e.s.ReplaceRoleGrants(e.ctx, src.ID, store.RoleGrants{
		Permissions: []store.PermissionRow{row("example.note", []string{"read", "update"}, "owner"), row("example.note", []string{"read"}, "all")},
		FieldAccess: []store.FieldAccessRow{fa("example.note", "profit", store.AccessNone)},
	}, nil); err != nil {
		t.Fatal(err)
	}
	dup, _, err := e.s.DuplicateRole(e.ctx, src.ID, store.NewRole{Key: "salinan", Label: "  Salinan  ", Description: "d", Kind: store.KindCustom})
	if err != nil {
		t.Fatal(err)
	}
	if dup.ID == src.ID || dup.Key != "salinan" || dup.Label != "Salinan" || dup.Kind != store.KindCustom || dup.Customized {
		t.Errorf("salinan = %+v", dup)
	}
	got, _ := e.s.RoleGrantsOf(e.ctx, dup.ID)
	if len(got.Permissions) != 2 || len(got.FieldAccess) != 1 {
		t.Fatalf("salinan grants = %+v", got)
	}
	// menyalin administrator: tanpa baris (izin penuhnya bukan baris)
	admin := e.systemRole(store.RoleAdministrator)
	adm, _, err := e.s.DuplicateRole(e.ctx, admin.ID, store.NewRole{Key: "salinan.admin", Label: "Salinan Admin", Kind: store.KindCustom})
	if err != nil {
		t.Fatal(err)
	}
	if g, _ := e.s.RoleGrantsOf(e.ctx, adm.ID); len(g.Permissions) != 0 {
		t.Errorf("salinan administrator = %+v", g)
	}
}

func TestModuleDefaultRoleBecomesCustomizedWhenEditedByAdmin(t *testing.T) {
	e := setup(t)
	if _, err := e.s.SyncModuleDefaultRoles(e.ctx, "inventory", []store.DefaultRoleDef{{Key: "inventory.manager", Label: "Manajer"}}); err != nil {
		t.Fatal(err)
	}
	r := e.systemRole("inventory.manager")
	if r.Kind != store.KindModuleDefault || r.SourceModule != "inventory" || r.Customized {
		t.Fatalf("role bawaan = %+v", r)
	}
	if _, err := e.s.ReplaceRoleGrants(e.ctx, r.ID, store.RoleGrants{Permissions: []store.PermissionRow{row("inventory.item", []string{"read"}, "all")}}, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.s.GetRole(e.ctx, r.ID); !got.Customized || !got.UpdatedAt.After(r.UpdatedAt) {
		t.Errorf("setelah diubah = %+v, want Customized dan updated_at naik", got)
	}
	// ubah label role bawaan lain juga menandai customized
	if _, err := e.s.SyncModuleDefaultRoles(e.ctx, "inventory", []store.DefaultRoleDef{{Key: "inventory.staff", Label: "Staf"}}); err != nil {
		t.Fatal(err)
	}
	staff := e.systemRole("inventory.staff")
	lbl := "Staf Gudang"
	if _, _, err := e.s.UpdateRole(e.ctx, staff.ID, store.RoleUpdate{Label: &lbl}); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.s.GetRole(e.ctx, staff.ID); !got.Customized || got.Label != "Staf Gudang" {
		t.Errorf("staff = %+v", got)
	}
	// role custom tidak pernah menjadi customized
	c := e.role("kustom")
	if _, err := e.s.ReplaceRoleGrants(e.ctx, c.ID, store.RoleGrants{}, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.s.GetRole(e.ctx, c.ID); got.Customized {
		t.Error("role custom ditandai customized")
	}
}

func TestSyncModuleDefaultRoles(t *testing.T) {
	e := setup(t)
	defs := []store.DefaultRoleDef{
		{Key: "inventory.manager", Label: "Manajer", Description: "m", RoleGrants: store.RoleGrants{
			Permissions: []store.PermissionRow{row("inventory.item", []string{"read", "update"}, "all")},
			FieldAccess: []store.FieldAccessRow{fa("inventory.item", "profit", store.AccessRead)},
		}},
		{Key: "inventory.staff", Label: "Staf", RoleGrants: store.RoleGrants{
			Permissions: []store.PermissionRow{row("inventory.item", []string{"read"}, "assigned:inventory.warehouse")},
		}},
	}
	before := e.rev()
	res, err := e.s.SyncModuleDefaultRoles(e.ctx, "inventory", defs)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Created) != 2 || len(res.Updated) != 0 || len(res.SkippedCustomized) != 0 || res.Revision != before+1 || e.rev() != before+1 {
		t.Fatalf("sinkronisasi pertama = %+v (revisi sebelum %d)", res, before)
	}
	mgr := e.systemRole("inventory.manager")
	if g, _ := e.s.RoleGrantsOf(e.ctx, mgr.ID); len(g.Permissions) != 1 || len(g.FieldAccess) != 1 {
		t.Fatalf("grants manager = %+v", g)
	}

	// manifest berubah: role belum customized mengikuti manifest (diganti utuh)
	defs[0].Label = "Manajer Gudang"
	defs[0].Permissions = []store.PermissionRow{row("inventory.item", []string{"read", "update", "delete"}, "all")}
	defs[0].FieldAccess = nil
	res, err = e.s.SyncModuleDefaultRoles(e.ctx, "inventory", defs)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Created) != 0 || len(res.Updated) != 2 {
		t.Fatalf("sinkronisasi kedua = %+v", res)
	}
	mgr = e.systemRole("inventory.manager")
	g, _ := e.s.RoleGrantsOf(e.ctx, mgr.ID)
	if mgr.Label != "Manajer Gudang" || len(g.Permissions[0].Actions) != 3 || len(g.FieldAccess) != 0 || mgr.Customized {
		t.Fatalf("manager setelah sinkron = %+v %+v", mgr, g)
	}

	// admin mengubah staff -> customized: sinkronisasi berikutnya tidak menyentuhnya, dilaporkan
	staff := e.systemRole("inventory.staff")
	if _, err := e.s.ReplaceRoleGrants(e.ctx, staff.ID, store.RoleGrants{Permissions: []store.PermissionRow{row("inventory.item", []string{"read"}, "all")}}, nil); err != nil {
		t.Fatal(err)
	}
	defs[1].Label = "Staf Baru"
	res, err = e.s.SyncModuleDefaultRoles(e.ctx, "inventory", defs)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Updated) != 1 || res.Updated[0] != "inventory.manager" || len(res.SkippedCustomized) != 1 || res.SkippedCustomized[0] != "inventory.staff" {
		t.Fatalf("sinkronisasi ketiga = %+v", res)
	}
	staff = e.systemRole("inventory.staff")
	sg, _ := e.s.RoleGrantsOf(e.ctx, staff.ID)
	if staff.Label != "Staf" || sg.Permissions[0].Scope[0] != "all" {
		t.Errorf("role customized tersentuh: %+v %+v", staff, sg)
	}

	// tanpa perubahan apa pun yang ditulis (semua customized/kosong): revisi tidak naik
	cur := e.rev()
	res, err = e.s.SyncModuleDefaultRoles(e.ctx, "inventory", defs[1:])
	if err != nil || res.Revision != cur || e.rev() != cur || len(res.SkippedCustomized) != 1 {
		t.Fatalf("sinkronisasi tanpa tulis = %+v, %v (revisi %d)", res, err, cur)
	}
	res, err = e.s.SyncModuleDefaultRoles(e.ctx, "inventory", nil)
	if err != nil || res.Revision != cur {
		t.Fatalf("sinkronisasi kosong = %+v, %v", res, err)
	}

	// modul lain tidak boleh merebut key; masukan tak sah ditolak
	if _, err := e.s.SyncModuleDefaultRoles(e.ctx, "other", []store.DefaultRoleDef{{Key: "inventory.manager", Label: "Rebut"}}); !errors.Is(err, store.ErrConflict) {
		t.Errorf("rebut key modul lain: err = %v, want ErrConflict", err)
	}
	if _, err := e.s.SyncModuleDefaultRoles(e.ctx, "", defs); !errors.Is(err, store.ErrInvalidArgument) {
		t.Errorf("modul kosong: err = %v", err)
	}
	if _, err := e.s.SyncModuleDefaultRoles(e.ctx, "inventory", []store.DefaultRoleDef{{Key: "a", Label: "A"}, {Key: "a", Label: "B"}}); !errors.Is(err, store.ErrInvalidArgument) {
		t.Errorf("key ganda: err = %v", err)
	}
	if _, err := e.s.SyncModuleDefaultRoles(e.ctx, "inventory", []store.DefaultRoleDef{{Key: "inventory.x", Label: "X", RoleGrants: store.RoleGrants{
		Permissions: []store.PermissionRow{{Resource: "inventory.item", Actions: []string{"read"}, Scope: []string{}}}}}}); !errors.Is(err, store.ErrInvalidArgument) {
		t.Errorf("scope kosong di manifest: err = %v", err)
	}
	// role sistem tidak dapat direbut
	if _, err := e.s.SyncModuleDefaultRoles(e.ctx, "inventory", []store.DefaultRoleDef{{Key: "administrator", Label: "Rebut"}}); !errors.Is(err, store.ErrConflict) {
		t.Errorf("rebut administrator: err = %v, want ErrConflict", err)
	}
}

func TestSetUserRolesReplacesSetAndKeepsGrantedAt(t *testing.T) {
	e := setup(t)
	a, b, c := e.role("a"), e.role("b"), e.role("c")
	user, other := uuid.NewString(), uuid.NewString()
	if _, err := e.s.SetUserRoles(e.ctx, other, []string{a.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.SetUserRoles(e.ctx, user, []string{a.ID, b.ID, a.ID}); err != nil { // duplikat diabaikan
		t.Fatal(err)
	}
	var grantedA time.Time
	if err := e.db.Raw(`SELECT granted_at FROM user_roles WHERE user_id = ?::uuid AND role_id = ?::uuid`, user, a.ID).Scan(&grantedA).Error; err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if _, err := e.s.SetUserRoles(e.ctx, user, []string{a.ID, c.ID}); err != nil {
		t.Fatal(err)
	}
	ids, _ := e.s.UserRoleIDs(e.ctx, user)
	want := []string{a.ID, c.ID}
	sort.Strings(want)
	if strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Fatalf("role = %v, want %v", ids, want)
	}
	var after time.Time
	if err := e.db.Raw(`SELECT granted_at FROM user_roles WHERE user_id = ?::uuid AND role_id = ?::uuid`, user, a.ID).Scan(&after).Error; err != nil || !after.Equal(grantedA) {
		t.Errorf("granted_at role yang dipertahankan berubah: %v -> %v (%v)", grantedA, after, err)
	}
	// pengguna lain tidak tersentuh
	if ids, _ := e.s.UserRoleIDs(e.ctx, other); len(ids) != 1 || ids[0] != a.ID {
		t.Errorf("role pengguna lain = %v", ids)
	}
	// kosong mencabut semua
	if _, err := e.s.SetUserRoles(e.ctx, user, nil); err != nil {
		t.Fatal(err)
	}
	if ids, _ := e.s.UserRoleIDs(e.ctx, user); len(ids) != 0 {
		t.Errorf("role setelah dikosongkan = %v", ids)
	}
	if _, err := e.s.SetUserRoles(e.ctx, "bukan-uuid", nil); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("user bukan uuid: err = %v", err)
	}
	if _, err := e.s.SetUserRoles(e.ctx, user, []string{"bukan-uuid"}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("role bukan uuid: err = %v", err)
	}
}

func TestSetScopeAssignments(t *testing.T) {
	e := setup(t)
	user, other := uuid.NewString(), uuid.NewString()
	if _, err := e.s.SetScopeAssignments(e.ctx, other, []store.ScopeAssignment{{Dimension: "inventory.warehouse", Values: []string{"o-1"}}}); err != nil {
		t.Fatal(err)
	}
	// nilai berbahaya tetap sekadar data (parameter terikat)
	evil := "x' OR 1=1; DROP TABLE roles; --"
	if _, err := e.s.SetScopeAssignments(e.ctx, user, []store.ScopeAssignment{
		{Dimension: "inventory.warehouse", Values: []string{"wh-01", evil}},
		{Dimension: "core.org_unit", Values: []string{"u-1"}, IncludeDescendants: true},
		{Dimension: "sales.region", Values: nil}, // kosong = tidak ada penugasan
	}); err != nil {
		t.Fatal(err)
	}
	got, err := e.s.UserScopeAssignments(e.ctx, user)
	if err != nil || len(got) != 2 {
		t.Fatalf("penugasan = %+v, %v", got, err)
	}
	if got[0].Dimension != "core.org_unit" || !got[0].IncludeDescendants || got[1].Dimension != "inventory.warehouse" || got[1].IncludeDescendants ||
		len(got[1].Values) != 2 || got[1].Values[1] != evil {
		t.Errorf("penugasan = %+v", got)
	}
	if _, err := e.s.ListRoles(e.ctx); err != nil {
		t.Fatalf("tabel roles rusak: %v", err)
	}
	// penggantian utuh: dimensi yang tak disebut dihapus, yang disebut diperbarui
	if _, err := e.s.SetScopeAssignments(e.ctx, user, []store.ScopeAssignment{{Dimension: "inventory.warehouse", Values: []string{"wh-09"}}}); err != nil {
		t.Fatal(err)
	}
	got, _ = e.s.UserScopeAssignments(e.ctx, user)
	if len(got) != 1 || got[0].Values[0] != "wh-09" || len(got[0].Values) != 1 {
		t.Fatalf("setelah penggantian = %+v", got)
	}
	// pengguna lain tidak tersentuh
	if o, _ := e.s.UserScopeAssignments(e.ctx, other); len(o) != 1 || o[0].Values[0] != "o-1" {
		t.Errorf("penugasan pengguna lain = %+v", o)
	}
	bad := map[string][]store.ScopeAssignment{
		"dimensi ganda":  {{Dimension: "a.b", Values: []string{"1"}}, {Dimension: "a.b", Values: []string{"2"}}},
		"dimensi besar":  {{Dimension: "A.B", Values: []string{"1"}}},
		"nilai kosong":   {{Dimension: "a.b", Values: []string{""}}},
		"nilai ganda":    {{Dimension: "a.b", Values: []string{"1", "1"}}},
		"dimensi kosong": {{Dimension: "", Values: []string{"1"}}},
	}
	before := e.rev()
	for name, as := range bad {
		if _, err := e.s.SetScopeAssignments(e.ctx, user, as); !errors.Is(err, store.ErrInvalidArgument) {
			t.Errorf("%s: err = %v, want ErrInvalidArgument", name, err)
		}
	}
	if e.rev() != before {
		t.Error("revisi naik oleh masukan yang ditolak")
	}
	if _, err := e.s.SetScopeAssignments(e.ctx, "bukan-uuid", nil); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("user bukan uuid: err = %v", err)
	}
	if _, err := e.s.SetScopeAssignments(e.ctx, user, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ = e.s.UserScopeAssignments(e.ctx, user); len(got) != 0 {
		t.Errorf("setelah dikosongkan = %+v", got)
	}
}

func TestLoadUserGrants(t *testing.T) {
	e := setup(t)
	admin := e.systemRole(store.RoleAdministrator)
	auth := e.systemRole(store.RoleAuthenticated)
	rA, rB, rOther := e.role("peran.a"), e.role("peran.b"), e.role("peran.lain")
	mustGrants := func(roleID string, g store.RoleGrants) {
		t.Helper()
		if _, err := e.s.ReplaceRoleGrants(e.ctx, roleID, g, nil); err != nil {
			t.Fatal(err)
		}
	}
	mustGrants(auth.ID, store.RoleGrants{Permissions: []store.PermissionRow{row("example.ping", []string{"read"}, "all")}})
	mustGrants(rA.ID, store.RoleGrants{
		Permissions: []store.PermissionRow{row("example.note", []string{"read"}, "all")},
		FieldAccess: []store.FieldAccessRow{fa("example.note", "profit", store.AccessNone)},
	})
	mustGrants(rB.ID, store.RoleGrants{Permissions: []store.PermissionRow{row("example.note", []string{"update"}, "owner")}})
	mustGrants(rOther.ID, store.RoleGrants{
		Permissions: []store.PermissionRow{row("secret.thing", []string{"read"}, "all")},
		FieldAccess: []store.FieldAccessRow{fa("secret.thing", "x", store.AccessWrite)},
	})
	user, stranger := uuid.NewString(), uuid.NewString()
	if _, err := e.s.SetUserRoles(e.ctx, user, []string{rA.ID, rB.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.SetUserRoles(e.ctx, stranger, []string{rOther.ID, admin.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.SetScopeAssignments(e.ctx, user, []store.ScopeAssignment{{Dimension: "inventory.warehouse", Values: []string{"wh-01"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.SetScopeAssignments(e.ctx, stranger, []store.ScopeAssignment{{Dimension: "inventory.warehouse", Values: []string{"wh-99"}}}); err != nil {
		t.Fatal(err)
	}

	g, err := e.s.LoadUserGrants(e.ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	if g.Revision != e.rev() {
		t.Errorf("Revision = %d, want %d (revisi terkini)", g.Revision, e.rev())
	}
	var keys []string
	for _, r := range g.Roles {
		keys = append(keys, r.Key)
		if r.Implicit != (r.Key == store.RoleAuthenticated) {
			t.Errorf("Implicit salah: %+v", r)
		}
	}
	if strings.Join(keys, ",") != "peran.a,peran.b,authenticated" {
		t.Errorf("role = %v (eksplisit dulu, lalu authenticated implisit)", keys)
	}
	resources := map[string]int{}
	for _, p := range g.Permissions {
		resources[p.Resource]++
		if p.Resource == "secret.thing" {
			t.Error("baris role pengguna lain bocor ke pengguna ini")
		}
	}
	if resources["example.note"] != 2 || resources["example.ping"] != 1 {
		t.Errorf("baris matriks = %v", resources)
	}
	if len(g.FieldAccess) != 1 || g.FieldAccess[0].RoleID != rA.ID || g.FieldAccess[0].Access != store.AccessNone {
		t.Errorf("field access = %+v", g.FieldAccess)
	}
	if len(g.Assignments) != 1 || g.Assignments[0].Values[0] != "wh-01" {
		t.Errorf("penugasan = %+v (harus milik pengguna ini saja)", g.Assignments)
	}

	// pengguna tanpa role dan tanpa penugasan: hanya hak authenticated
	bare, err := e.s.LoadUserGrants(e.ctx, uuid.NewString())
	if err != nil || len(bare.Roles) != 1 || !bare.Roles[0].Implicit || len(bare.Permissions) != 1 || len(bare.Assignments) != 0 {
		t.Errorf("pengguna tanpa role = %+v, %v", bare, err)
	}
	// authenticated yang diberikan eksplisit lewat user_roles (mis. SQL langsung) tidak menggandakan baris
	if err := e.db.Exec(`INSERT INTO user_roles (user_id, role_id) VALUES (?::uuid, ?::uuid)`, user, auth.ID).Error; err != nil {
		t.Fatal(err)
	}
	g2, _ := e.s.LoadUserGrants(e.ctx, user)
	ping := 0
	for _, p := range g2.Permissions {
		if p.Resource == "example.ping" {
			ping++
		}
	}
	if len(g2.Roles) != 3 || ping != 1 {
		t.Errorf("authenticated eksplisit: role=%d ping=%d, want 3 dan 1", len(g2.Roles), ping)
	}

	if _, err := e.s.LoadUserGrants(e.ctx, "bukan-uuid"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("user bukan uuid: err = %v", err)
	}
	if _, err := e.s.LoadUserGrants(context.Background(), user); !errors.Is(err, store.ErrNoTenant) {
		t.Errorf("tanpa tenant: err = %v", err)
	}
	if _, err := e.s.LoadUserGrants(ctxOf(tenantB), user); !errors.Is(err, database.ErrUnknownTenant) {
		t.Errorf("tenant tak dikenal: err = %v", err)
	}
}

// Dua tenant = dua database: data dan revisi satu tenant tidak terlihat dan tidak terpengaruh tenant lain.
func TestTenantIsolation(t *testing.T) {
	dbA, dbB := openMigrated(t), openMigrated(t)
	s := store.New(perTenant{tenantA: {DB: dbA}, tenantB: {DB: dbB}})
	a, b := ctxOf(tenantA), ctxOf(tenantB)
	user := uuid.NewString()
	r, _, err := s.CreateRole(a, store.NewRole{Key: "hanya.a", Label: "A", Kind: store.KindCustom})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetUserRoles(a, user, []string{r.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetScopeAssignments(a, user, []store.ScopeAssignment{{Dimension: "inventory.warehouse", Values: []string{"wh-01"}}}); err != nil {
		t.Fatal(err)
	}
	if revB, _ := s.Revision(b); revB != 1 {
		t.Errorf("revisi tenant B = %d, want 1 (tak terpengaruh A)", revB)
	}
	if _, err := s.GetRole(b, r.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("role tenant A terlihat dari B: %v", err)
	}
	gb, err := s.LoadUserGrants(b, user)
	if err != nil || len(gb.Assignments) != 0 || len(gb.Roles) != 1 {
		t.Errorf("hak pengguna dari tenant B = %+v, %v", gb, err)
	}
	if ids, _ := s.UserRoleIDs(b, user); len(ids) != 0 {
		t.Errorf("user_roles tenant A terlihat dari B: %v", ids)
	}
}
