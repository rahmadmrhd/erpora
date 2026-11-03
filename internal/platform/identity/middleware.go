package identity

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"erpora/internal/platform/identity/mfa"
	"erpora/internal/platform/identity/token"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/httpx"
	sdkidentity "erpora/sdk/identity"
)

// Klaim iss dan aud access token (belum ada kunci konfigurasi; ADR-0013 §1). Konstanta cukup: penerbit dan verifier
// selalu satu proses, dan klaim hanya menolak token dari sistem lain. Diekspor agar perakitan (internal/app) dan helper
// uji memakai nilai yang sama.
const (
	TokenIssuer   = "erpora"
	TokenAudience = "erpora-api" //nolint:gosec // G101 salah sasaran: nilai klaim aud JWT, bukan kredensial
)

// Kode galat 401 (api-guidelines §12). Hanya dua: klien tidak pernah diberi tahu mengapa token ditolak, kecuali
// `token_expired` yang hanya keluar untuk token yang tanda tangan, kid, alg, iss, dan aud-nya sah (Verifier) agar
// klien tahu harus refresh; token palsu tidak pernah mendapat jawaban itu, jadi bukan oracle.
const (
	codeUnauthenticated = "unauthenticated"
	codeTokenExpired    = "token_expired"
)

// TokenVerifier memverifikasi access token mentah (dipenuhi *token.Verifier).
type TokenVerifier interface {
	Verify(raw string) (token.Claims, error)
}

// RevocationChecker melaporkan sesi tercabut dari daftar di memori, tanpa query DB (dipenuhi *session.RevocationList).
type RevocationChecker interface {
	IsRevoked(sid string) bool
}

// AuthenticateDeps adalah dependensi Authenticate.
type AuthenticateDeps struct {
	Verifier TokenVerifier     // wajib
	Revoked  RevocationChecker // wajib
	// AcceptedScopes adalah scope token yang boleh melewati Authenticate. Kosong (bawaan C1): token ber-scope
	// terbatas (mis. scope refresh atau MFA parsial) ditolak di semua route biasa; PAT/OAuth kelak menambahkan
	// scope-nya di sini beserta pemeriksaannya.
	AcceptedScopes []string
	Log            *slog.Logger // nil = slog.Default()
}

// Authenticate membangun middleware autentikasi (ADR-0013 §4). Berjalan SETELAH tenantContext, jadi klaim `tid`
// dicocokkan dengan tenant di context. Untuk setiap request ia:
//
//  1. mensyaratkan tepat satu header Authorization berskema Bearer dengan token tunggal tidak kosong, dan menolak
//     token lebih besar dari token.MaxTokenSize SEBELUM diurai;
//  2. memverifikasi token (tanda tangan EdDSA, kid, alg, iss, aud, exp, status kunci);
//  3. mencocokkan klaim tid dengan tenant context (bentuk UUID kanonik persis);
//  4. menolak sid yang tercabut (daftar di memori);
//  5. menolak token ber-scope di luar AcceptedScopes;
//  6. memasang sdk/identity.Principal (selalu dari token, bukan header atau body lain) dan IP klien untuk lockout MFA.
//
// Semua kegagalan menjawab 401 `unauthenticated` generik; satu-satunya pembeda `token_expired` (lihat konstanta).
// Sebab sebenarnya hanya dicatat di log server pada level debug. Rute Public() tidak melewati middleware ini: tidak
// ada Principal yang dipasang walau request membawa token, dan token buruk pada rute publik tidak menggagalkannya.
// Keputusan itu sengaja sederhana: rute publik tidak boleh berperilaku berbeda menurut ada tidaknya header.
func Authenticate(d AuthenticateDeps) func(http.Handler) http.Handler {
	if d.Verifier == nil || d.Revoked == nil {
		panic("identity: Authenticate memerlukan Verifier dan Revoked")
	}
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	accepted := make(map[string]bool, len(d.AcceptedScopes))
	for _, s := range d.AcceptedScopes {
		accepted[s] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			raw, ok := bearerToken(r)
			if !ok {
				reject(w, r, codeUnauthenticated)
				return
			}
			claims, err := d.Verifier.Verify(raw)
			if err != nil {
				code := codeUnauthenticated
				if errors.Is(err, token.ErrTokenExpired) {
					code = codeTokenExpired
				}
				log.DebugContext(ctx, "autentikasi ditolak", "reason", token.Reason(err))
				reject(w, r, code)
				return
			}
			tid, ok := tenancy.FromContext(ctx)
			if !ok || !sameTenant(claims.TenantID, tid) {
				log.DebugContext(ctx, "autentikasi ditolak", "reason", "tenant_mismatch")
				reject(w, r, codeUnauthenticated)
				return
			}
			if claims.Subject == "" || claims.SessionID == "" || d.Revoked.IsRevoked(claims.SessionID) {
				log.DebugContext(ctx, "autentikasi ditolak", "reason", "session")
				reject(w, r, codeUnauthenticated)
				return
			}
			for _, s := range claims.Scope {
				if !accepted[s] {
					log.DebugContext(ctx, "autentikasi ditolak", "reason", "scope")
					reject(w, r, codeUnauthenticated)
					return
				}
			}
			p := sdkidentity.Principal{
				UserID:     claims.Subject,
				Kind:       sdkidentity.KindHuman,
				SessionID:  claims.SessionID,
				AuthMethod: sdkidentity.AuthMethodPassword,
				AMR:        claims.AMR,
				TokenScope: claims.Scope,
			}
			ctx = sdkidentity.WithPrincipal(ctx, p)
			ctx = mfa.WithClientIP(ctx, httpx.ClientIP(ctx))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// reject menjawab 401 generik. WWW-Authenticate: Bearer sesuai RFC 6750 tanpa parameter error (tidak membocorkan sebab).
func reject(w http.ResponseWriter, r *http.Request, code string) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	msg := "autentikasi diperlukan"
	if code == codeTokenExpired {
		msg = "token kedaluwarsa"
	}
	httpx.WriteError(w, r, http.StatusUnauthorized, code, msg, nil)
}

// bearerToken mengekstrak token dari header Authorization. Ditolak: header tidak ada atau lebih dari satu (ambigu,
// bahan request smuggling antar lapisan), skema selain Bearer, token kosong atau memuat spasi/karakter kontrol, dan
// token lebih besar dari token.MaxTokenSize.
func bearerToken(r *http.Request) (string, bool) {
	vals := r.Header.Values("Authorization")
	if len(vals) != 1 {
		return "", false
	}
	scheme, raw, found := strings.Cut(vals[0], " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	if raw == "" || len(raw) > token.MaxTokenSize {
		return "", false
	}
	for i := 0; i < len(raw); i++ {
		if c := raw[i]; c <= ' ' || c == 0x7f {
			return "", false
		}
	}
	return raw, true
}

// sameTenant membandingkan klaim tid dengan tenant context sebagai UUID kanonik persis (36 karakter huruf kecil);
// bentuk lain (kurung, urn:, huruf besar) ditolak agar tidak ada dua representasi untuk satu tenant.
func sameTenant(claim string, t tenancy.TenantID) bool {
	id, err := uuid.Parse(claim)
	if err != nil || id.String() != claim {
		return false
	}
	return id == uuid.UUID(t)
}
