package httpserver

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"erpora/internal/platform/bootstrap"
	"erpora/internal/platform/tenancy"
	sdkidentity "erpora/sdk/identity"
)

const (
	testUserA = "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4aa1"
	testUserB = "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4bb2"
)

// principalFromHeader meniru Authenticate (yang nyata diuji di package identity): memasang Principal dari header uji
// X-Test-User; tanpa header tidak ada Principal (seperti rantai yang salah rakit, bukan seperti Authenticate nyata).
func principalFromHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if uid := r.Header.Get("X-Test-User"); uid != "" {
			p := sdkidentity.Principal{UserID: uid, Kind: sdkidentity.KindHuman}
			if sc := r.Header.Get("X-Test-Scope"); sc != "" {
				p.TokenScope = strings.Split(sc, ",")
			}
			r = r.WithContext(sdkidentity.WithPrincipal(r.Context(), p))
		}
		next.ServeHTTP(w, r)
	})
}

// userBootstrap membentuk manifest dengan user dari Principal di ctx, seperti sumber nyata di internal/app.
func userBootstrap(ctx context.Context, tn tenancy.TenantID) bootstrap.Manifest {
	m := bootstrap.Build("9.9.9", tn, nil, 5)
	if p, ok := sdkidentity.FromContext(ctx); ok {
		switch p.UserID {
		case testUserA:
			m.User = &bootstrap.User{ID: p.UserID, Email: "a@example.test", DisplayName: "Ani"}
		case testUserB:
			m.User = &bootstrap.User{ID: p.UserID, Email: "b@example.test", DisplayName: "Budi"}
		}
	}
	return m
}

func asUser(uid string) map[string]string { return map[string]string{"X-Test-User": uid} }

func TestBootstrap_RequiresPrincipal(t *testing.T) {
	r, _ := newTestRouter(t, nil)
	rec := do(r, "GET", "/api/bootstrap", nil)
	if rec.Code != http.StatusUnauthorized || decode(t, rec)["error"].(map[string]any)["code"] != "unauthenticated" {
		t.Fatalf("tanpa principal = %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "appVersion") || rec.Header().Get("ETag") != "" {
		t.Errorf("401 tidak boleh membawa isi atau ETag: %s", rec.Body)
	}
}

// Pengguna tak ditemukan (manifest tanpa user) gagal tertutup 401, bukan manifest anonim.
func TestBootstrap_ManifestWithoutUserIs401(t *testing.T) {
	r := newBootstrapRouter(t)
	if rec := do(r, "GET", "/api/bootstrap", asUser("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4cc3")); rec.Code != http.StatusUnauthorized {
		t.Fatalf("pengguna tak dikenal = %d %s", rec.Code, rec.Body)
	}
}

// Token ber-scope terbatas tidak boleh membaca bootstrap (PRD-001 Q29 butir 2 hanya mengecualikan aturan MFA, bukan
// scope): 403 forbidden tanpa isi manifest. Token tanpa scope (sesi biasa) tetap 200.
func TestBootstrap_ScopedTokenIs403(t *testing.T) {
	r := newBootstrapRouter(t)
	hdr := asUser(testUserA)
	hdr["X-Test-Scope"] = "reports:read"
	rec := do(r, "GET", "/api/bootstrap", hdr)
	if rec.Code != http.StatusForbidden || decode(t, rec)["error"].(map[string]any)["code"] != "permission_denied" {
		t.Fatalf("token ber-scope = %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "Ani") || strings.Contains(rec.Body.String(), "appVersion") || rec.Header().Get("ETag") != "" {
		t.Errorf("403 tidak boleh membawa isi atau ETag: %s", rec.Body)
	}
	if rec := do(r, "GET", "/api/bootstrap", asUser(testUserA)); rec.Code != 200 {
		t.Fatalf("tanpa scope = %d %s", rec.Code, rec.Body)
	}
}

func TestBootstrap_CarriesUserAndPermRev(t *testing.T) {
	r := newBootstrapRouter(t)
	rec := do(r, "GET", "/api/bootstrap", asUser(testUserA))
	if rec.Code != 200 {
		t.Fatalf("code %d %s", rec.Code, rec.Body)
	}
	d := decode(t, rec)["data"].(map[string]any)
	u, _ := d["user"].(map[string]any)
	if u["id"] != testUserA || u["email"] != "a@example.test" || u["displayName"] != "Ani" || len(u) != 3 {
		t.Errorf("user = %v", d["user"])
	}
	if pr, ok := d["permRev"]; !ok || pr != float64(0) {
		t.Errorf("permRev = %v (ada=%v)", pr, ok)
	}
	if _, ok := d["permissions"].([]any); !ok {
		t.Error("permissions tetap ada sampai C2")
	}
}

// Cache per pengguna: Cache-Control private, Vary Authorization, ETag berbeda antar pengguna dan 304 hanya untuk
// pemilik ETag itu (If-None-Match dari A tidak pernah menghasilkan 304 untuk B).
func TestBootstrap_PerUserCachingAndIDOR(t *testing.T) {
	r := newBootstrapRouter(t)
	a, b := do(r, "GET", "/api/bootstrap", asUser(testUserA)), do(r, "GET", "/api/bootstrap", asUser(testUserB))
	for name, rec := range map[string]interface{ Header() http.Header }{"A": a, "B": b} {
		h := rec.Header()
		if cc := h.Get("Cache-Control"); !strings.Contains(cc, "private") || strings.Contains(cc, "public") {
			t.Errorf("%s: Cache-Control = %q", name, cc)
		}
		if v := h.Get("Vary"); !strings.Contains(v, "Authorization") {
			t.Errorf("%s: Vary = %q", name, v)
		}
	}
	etagA, etagB := a.Header().Get("ETag"), b.Header().Get("ETag")
	if etagA == "" || etagA == etagB {
		t.Fatalf("ETag A=%q B=%q harus ada dan berbeda", etagA, etagB)
	}
	if strings.Contains(b.Body.String(), "a@example.test") || strings.Contains(b.Body.String(), "Ani") || strings.Contains(a.Body.String(), "Budi") {
		t.Errorf("respons memuat data pengguna lain: A=%s B=%s", a.Body, b.Body)
	}
	hdr := asUser(testUserB)
	hdr["If-None-Match"] = etagA
	if rec := do(r, "GET", "/api/bootstrap", hdr); rec.Code != 200 || !strings.Contains(rec.Body.String(), "Budi") {
		t.Fatalf("If-None-Match milik A untuk B = %d %s", rec.Code, rec.Body)
	}
	hdr = asUser(testUserA)
	hdr["If-None-Match"] = etagA
	rec := do(r, "GET", "/api/bootstrap", hdr)
	if rec.Code != 304 || rec.Header().Get("Vary") == "" || rec.Header().Get("Cache-Control") == "" {
		t.Fatalf("304 untuk pemilik = %d vary=%q", rec.Code, rec.Header().Get("Vary"))
	}
}

// Sumber yang mengembalikan user bukan Principal request adalah bug perakitan: 500 tanpa membocorkan isinya.
func TestBootstrap_ManifestUserMismatchIs500(t *testing.T) {
	r := NewRouter(Deps{
		Resolver:     tenancy.SingleTenantResolver{ID: defaultTenant},
		Connector:    newBootstrapRouter(t).deps.Connector,
		Authenticate: principalFromHeader,
		Bootstrap: func(_ context.Context, tn tenancy.TenantID) bootstrap.Manifest {
			m := bootstrap.Build("9.9.9", tn, nil, 1)
			m.User = &bootstrap.User{ID: testUserB, Email: "b@example.test", DisplayName: "Budi"}
			return m
		},
	})
	rec := do(r, "GET", "/api/bootstrap", asUser(testUserA))
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "Budi") || strings.Contains(rec.Body.String(), "b@example.test") {
		t.Fatalf("mismatch = %d %s", rec.Code, rec.Body)
	}
}

// Galat sementara sumber pengguna: 503 (bukan 401) dan tanpa isi.
func TestBootstrap_UnavailableIs503(t *testing.T) {
	r := NewRouter(Deps{
		Resolver:     tenancy.SingleTenantResolver{ID: defaultTenant},
		Connector:    newBootstrapRouter(t).deps.Connector,
		Authenticate: principalFromHeader,
		Bootstrap: func(_ context.Context, tn tenancy.TenantID) bootstrap.Manifest {
			m := bootstrap.Build("9.9.9", tn, nil, 1)
			m.Unavailable = true
			return m
		},
	})
	rec := do(r, "GET", "/api/bootstrap", asUser(testUserA))
	if rec.Code != http.StatusServiceUnavailable || strings.Contains(rec.Body.String(), "appVersion") {
		t.Fatalf("unavailable = %d %s", rec.Code, rec.Body)
	}
}
