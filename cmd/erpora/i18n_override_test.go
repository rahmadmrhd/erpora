package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"erpora/internal/app"
	"erpora/internal/platform/i18n"
	"erpora/internal/platform/i18n/store"
	"erpora/internal/platform/tenancy"
)

// fakeI18n memenuhi i18nOverrideBackend dan merekam permintaan.
type fakeI18n struct {
	rows       []store.Override
	setCalls   []i18n.Override
	setActors  []string
	setChanged bool
	setErr     error
	delCalls   [][3]string
	delExisted bool
	delErr     error
	listErr    error
	guardErr   error
	guardCalls int
	lastFilter store.Filter
}

func (f *fakeI18n) GuardWrite(context.Context) error { f.guardCalls++; return f.guardErr }

func (f *fakeI18n) SetOverride(_ context.Context, _ tenancy.TenantID, o i18n.Override, actor string, _ store.Precondition) (bool, error) {
	f.setCalls = append(f.setCalls, o)
	f.setActors = append(f.setActors, actor)
	return f.setChanged, f.setErr
}

func (f *fakeI18n) DeleteOverride(_ context.Context, _ tenancy.TenantID, ns, loc, key, _ string, _ store.Precondition) (bool, error) {
	f.delCalls = append(f.delCalls, [3]string{ns, loc, key})
	return f.delExisted, f.delErr
}

func (f *fakeI18n) ListOverrides(_ context.Context, _ tenancy.TenantID, flt store.Filter, page, size int) ([]store.Override, int64, error) {
	f.lastFilter = flt
	if f.listErr != nil {
		return nil, 0, f.listErr
	}
	from := min((page-1)*size, len(f.rows))
	to := min(from+size, len(f.rows))
	return f.rows[from:to], int64(len(f.rows)), nil
}

func canonicalOverrides() []store.Override {
	at := time.Date(2026, 10, 29, 10, 0, 0, 0, time.UTC)
	return []store.Override{
		{Namespace: "example", Locale: "id", Key: "note.title", Message: "Klien", UpdatedBy: "cli:budi", UpdatedAt: at},
		{Namespace: "ui", Locale: "en", Key: "actions.save", Message: "Store \"now\"\nline2", UpdatedBy: "api:dev", UpdatedAt: at.Add(time.Hour)},
	}
}

func runI18n(t *testing.T, f *fakeI18n, stdin string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	root := &cobra.Command{Use: "erpora", SilenceUsage: true, SilenceErrors: true}
	root.AddGroup(&cobra.Group{ID: groupRuntime, Title: "Runtime"})
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetIn(strings.NewReader(stdin))
	root.AddCommand(newI18nCmd(&rootOpts{}, func(context.Context, *rootOpts, io.Writer) (i18nOverrideBackend, tenancy.TenantID, func(), error) {
		return f, cliTenant, func() {}, nil
	}))
	root.SetArgs(append([]string{"i18n"}, args...))
	err = root.Execute()
	return out.String(), errOut.String(), err
}

func TestI18nOverride_ListGolden(t *testing.T) {
	f := &fakeI18n{rows: canonicalOverrides()}
	out, _, err := runI18n(t, f, "", "override", "list")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "i18n_override_list.golden", out)
	js, _, err := runI18n(t, f, "", "override", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "i18n_override_list_json.golden", js)
	empty, _, err := runI18n(t, &fakeI18n{}, "", "override", "list", "--json")
	if err != nil || strings.TrimSpace(empty) != "[]" {
		t.Errorf("daftar kosong JSON = %q, %v", empty, err)
	}
}

func TestI18nOverride_ListFiltersAndReadsAllPages(t *testing.T) {
	var rows []store.Override
	for i := 0; i < 450; i++ {
		rows = append(rows, store.Override{Namespace: "ui", Locale: "id", Key: "k" + strings.Repeat("x", i%5), Message: "m"})
	}
	f := &fakeI18n{rows: rows}
	out, _, err := runI18n(t, f, "", "override", "list", "--namespace", "ui", "--locale", "id", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(out, `"namespace"`); n != 450 {
		t.Errorf("seluruh halaman harus terbaca: %d baris dari 450", n)
	}
	if f.lastFilter.Namespace != "ui" || f.lastFilter.Locale != "id" {
		t.Errorf("filter = %+v", f.lastFilter)
	}
}

func TestI18nOverride_SetSendsCLIActorAndReportsOutcome(t *testing.T) {
	f := &fakeI18n{setChanged: true}
	out, _, err := runI18n(t, f, "", "override", "set", "example", "id", "note.title", "Klien")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.setCalls) != 1 || f.setCalls[0] != (i18n.Override{Namespace: "example", Locale: "id", Key: "note.title", Message: "Klien"}) {
		t.Fatalf("setCalls = %+v", f.setCalls)
	}
	if !strings.HasPrefix(f.setActors[0], "cli:") {
		t.Errorf("actor = %q, want cli:<user>", f.setActors[0])
	}
	if !strings.Contains(out, "disimpan") || !strings.Contains(out, "example/id/note.title") {
		t.Errorf("keluaran = %q", out)
	}
	f2 := &fakeI18n{setChanged: false}
	out, _, err = runI18n(t, f2, "", "override", "set", "example", "id", "note.title", "Klien")
	if err != nil || !strings.Contains(out, "tidak ada perubahan") {
		t.Errorf("nilai sama: %q %v", out, err)
	}
	if f.guardCalls != 1 {
		t.Errorf("guard dipanggil %d kali sebelum mutasi, want 1", f.guardCalls)
	}
}

func TestI18nOverride_ExitCodes(t *testing.T) {
	// 2: argumen salah dan validasi override ditolak
	for _, args := range [][]string{
		{"override", "set", "example", "id", "note.title"},           // pesan hilang
		{"override", "set", "example", "id", "note.title", "a", "b"}, // argumen berlebih
		{"override", "rm", "example", "id"},                          // kunci hilang
		{"override", "list", "extra"},                                // list tanpa argumen
	} {
		if _, _, err := runI18n(t, &fakeI18n{}, "", args...); exitCode(err) != 2 {
			t.Errorf("%v: exit = %d (%v), want 2", args, exitCode(err), err)
		}
	}
	oe := &i18n.OverrideError{Code: i18n.CodeParamMismatch, Field: "message", Detail: "argumen berbeda"}
	_, errOut, err := runI18n(t, &fakeI18n{setErr: oe}, "", "override", "set", "example", "id", "note.title", "x")
	if exitCode(err) != 2 || !strings.Contains(err.Error(), "param_mismatch") {
		t.Errorf("validasi: exit = %d err=%v stderr=%q, want 2 memuat kode", exitCode(err), err, errOut)
	}
	// 3: prasyarat/konfigurasi (penyimpanan tidak terpasang, guard lingkungan)
	if _, _, err := runI18n(t, &fakeI18n{setErr: i18n.ErrOverridesUnavailable}, "", "override", "set", "example", "id", "k", "x"); exitCode(err) != 3 {
		t.Errorf("tanpa store: exit = %d (%v), want 3", exitCode(err), err)
	}
	guard := &app.ExitError{Code: 3, Err: app.ErrProductionDatabase}
	g := &fakeI18n{guardErr: guard}
	if _, _, err := runI18n(t, g, "", "override", "set", "example", "id", "k", "x"); exitCode(err) != 3 || len(g.setCalls) != 0 {
		t.Errorf("guard menolak: exit = %d, setCalls=%d", exitCode(err), len(g.setCalls))
	}
	// 4: operasi gagal
	if _, _, err := runI18n(t, &fakeI18n{setErr: errors.New("db putus")}, "", "override", "set", "example", "id", "k", "x"); exitCode(err) != 4 {
		t.Errorf("operasi gagal: exit = %d (%v), want 4", exitCode(err), err)
	}
	if _, _, err := runI18n(t, &fakeI18n{listErr: errors.New("db putus")}, "", "override", "list"); exitCode(err) != 4 {
		t.Errorf("list gagal: exit = %d, want 4", exitCode(err))
	}
	// list dan rm tak butuh guard penulisan untuk membaca
	r := &fakeI18n{guardErr: guard, rows: canonicalOverrides()}
	if _, _, err := runI18n(t, r, "", "override", "list"); err != nil || r.guardCalls != 0 {
		t.Errorf("list tidak boleh memanggil guard: err=%v calls=%d", err, r.guardCalls)
	}
}

func TestI18nOverride_RmRequiresConfirmation(t *testing.T) {
	f := &fakeI18n{delExisted: true}
	// tanpa --yes dan stdin kosong: konfirmasi hilang, exit 2, tidak ada penghapusan
	if _, _, err := runI18n(t, f, "", "override", "rm", "example", "id", "note.title"); exitCode(err) != 2 || len(f.delCalls) != 0 {
		t.Fatalf("tanpa konfirmasi: exit = %d, delCalls=%d (%v)", exitCode(err), len(f.delCalls), err)
	}
	// jawab n: tidak dihapus
	if out, _, err := runI18n(t, f, "n\n", "override", "rm", "example", "id", "note.title"); err != nil || len(f.delCalls) != 0 || !strings.Contains(out, "dibatalkan") {
		t.Errorf("jawab n: out=%q err=%v calls=%d", out, err, len(f.delCalls))
	}
	// jawab y: dihapus
	if _, _, err := runI18n(t, f, "y\n", "override", "rm", "example", "id", "note.title"); err != nil || len(f.delCalls) != 1 {
		t.Errorf("jawab y: err=%v calls=%d", err, len(f.delCalls))
	}
	// --yes untuk skrip
	f2 := &fakeI18n{delExisted: true}
	out, _, err := runI18n(t, f2, "", "override", "rm", "--yes", "example", "id", "note.title")
	if err != nil || f2.delCalls[0] != [3]string{"example", "id", "note.title"} || !strings.Contains(out, "dihapus") {
		t.Errorf("--yes: out=%q err=%v calls=%v", out, err, f2.delCalls)
	}
	// override tidak ada: informasi, exit 0 (idempoten)
	f3 := &fakeI18n{delExisted: false}
	out, _, err = runI18n(t, f3, "", "override", "rm", "-y", "example", "id", "note.title")
	if err != nil || !strings.Contains(out, "tidak ada") {
		t.Errorf("tidak ada: out=%q err=%v", out, err)
	}
	// namespace/bahasa tak dikenal: exit 2
	f4 := &fakeI18n{delErr: i18n.ErrNotFound}
	if _, _, err := runI18n(t, f4, "", "override", "rm", "-y", "nope", "id", "k"); exitCode(err) != 2 {
		t.Errorf("target tak dikenal: exit = %d, want 2", exitCode(err))
	}
}
