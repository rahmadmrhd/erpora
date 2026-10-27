package lifecycle

import (
	"context"
	"errors"
	"testing"
)

// RFC 9110 section 13.1.1: "*" cocok dengan representasi yang ada; daftar dipisah koma; perbandingan kuat (W/ tidak pernah cocok).
func TestMatchIfMatch_Table(t *testing.T) {
	tests := []struct {
		header string
		rev    int64
		want   bool
	}{
		{`"7"`, 7, true},
		{`"6"`, 7, false},
		{`*`, 7, true},
		{` * `, 0, true},
		{`"5", "7", "9"`, 7, true},
		{`"5","9"`, 7, false},
		{`"5", *`, 7, false}, // "*" hanya sah bila seluruh header (RFC 9110: If-Match = "*" / #entity-tag)
		{`*, "7"`, 7, false},
		{`**`, 7, false},
		{`"7"x`, 7, false}, // tanpa pemisah setelah entity-tag
		{`"7""7"`, 7, false},
		{`"5"x, "7"`, 7, false},
		{`"7",`, 7, true}, // elemen kosong diizinkan (RFC 9110 section 5.6.1)
		{`, "7"`, 7, true},
		{`W/"7"`, 7, false},
		{`W/"5", "7"`, 7, true},
		{`7`, 7, false}, // tanpa kutip bukan entity-tag
		{`"7`, 7, false},
		{``, 7, false},
	}
	for _, tc := range tests {
		if got := MatchIfMatch(tc.header, tc.rev); got != tc.want {
			t.Errorf("MatchIfMatch(%q, %d) = %v, want %v", tc.header, tc.rev, got, tc.want)
		}
	}
}

// B-5: pemeriksaan If-Match terjadi di dalam advisory lock, sebelum simulate: revision basi ditolak tanpa menjalankan apa pun.
func TestExecute_IfMatchChecksRevisionInsideLock(t *testing.T) {
	rec := &fxRec{}
	a := newFx(rec, fxSpec{key: "acme.a"})
	e := newEnv(t, rec, a)
	rev, err := e.st.Revision(context.Background(), tenantA)
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.Execute(context.Background(), Request{Tenant: tenantA, Op: OpInstall, Key: "acme.a", Actor: "cli:t", IfMatch: `"999"`})
	var le *Error
	if !errors.As(err, &le) || le.Code != CodeVersionMismatch || le.Revision != rev {
		t.Fatalf("err = %v, want version_mismatch revision=%d", err, rev)
	}
	if len(rec.list()) != 0 || e.st.row("acme.a").Status != "" && e.st.row("acme.a").Status != "available" {
		t.Fatalf("operasi tidak boleh berjalan: calls=%v row=%+v", rec.list(), e.st.row("acme.a"))
	}
	// Daftar yang memuat revision saat ini lolos (satu eksekusi sukses cukup; operasi berikutnya no-op pada revision berbeda).
	if _, err := e.svc.Execute(context.Background(), Request{Tenant: tenantA, Op: OpInstall, Key: "acme.a", Actor: "cli:t", IfMatch: `"1", "` + itoa(rev) + `"`}); err != nil {
		t.Fatalf("If-Match daftar yang memuat revision harus lolos: %v", err)
	}
}

func TestRetryModule_IfMatchMismatch(t *testing.T) {
	e, _, _ := failedInstall(t)
	_, err := e.svc.RetryModule(context.Background(), tenantA, "acme.a", "cli:t", nil, `"999"`)
	var le *Error
	if !errors.As(err, &le) || le.Code != CodeVersionMismatch {
		t.Fatalf("err = %v, want version_mismatch", err)
	}
}

// R-6: kunci modul tidak dikenal harus 404 (module_not_found), bukan 412, walau If-Match tidak cocok.
func TestExecute_UnknownKeyBeforeIfMatch(t *testing.T) {
	rec := &fxRec{}
	a := newFx(rec, fxSpec{key: "acme.a"})
	e := newEnv(t, rec, a)
	_, err := e.svc.Execute(context.Background(), Request{Tenant: tenantA, Op: OpInstall, Key: "acme.nope", Actor: "cli:t", IfMatch: `"999"`})
	var le *Error
	if !errors.As(err, &le) || le.Code != CodeModuleNotFound {
		t.Fatalf("err = %v, want module_not_found", err)
	}
}

func TestRetryModule_UnknownKeyBeforeIfMatch(t *testing.T) {
	e, _, _ := failedInstall(t)
	_, err := e.svc.RetryModule(context.Background(), tenantA, "acme.nope", "cli:t", nil, `"999"`)
	var le *Error
	if !errors.As(err, &le) || le.Code != CodeModuleNotFound {
		t.Fatalf("err = %v, want module_not_found", err)
	}
}

func itoa(n int64) string {
	const digits = "0123456789"
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{digits[n%10]}, b...)
	}
	return string(b)
}
