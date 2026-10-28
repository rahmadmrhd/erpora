package httpx

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"erpora/sdk/i18n"
)

// mapResolver adalah i18n.Resolver tetap: "ns|loc|key" -> sumber pesan.
type mapResolver map[string]string

func (m mapResolver) Resolve(_ context.Context, ns, key string, loc i18n.Locale) (string, bool) {
	v, ok := m[ns+"|"+string(loc)+"|"+key]
	return v, ok
}

func writeWith(t *testing.T, ctx context.Context, status int, code, msg string, details any) map[string]any {
	t.Helper()
	r := httptest.NewRequest("GET", "/x", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	WriteError(w, r, status, code, msg, details)
	var env struct {
		Error map[string]any `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	return env.Error
}

func i18nCtx(res i18n.Resolver, scope string, loc i18n.Locale) context.Context {
	ctx := i18n.WithResolver(context.Background(), res)
	if scope != "" {
		ctx = i18n.WithScope(ctx, scope)
	}
	return i18n.WithLocale(ctx, loc)
}

// Pesan galat dilokalkan lewat errors.<code>: namespace modul pemilik route dulu, lalu platform::, lalu
// message dari pemanggil. code dan details tidak berubah (Requirement 4.4, 12.9).
func TestWriteError_LocalizesMessageByCode(t *testing.T) {
	res := mapResolver{
		"example|en|errors.not_found":    "Note not found",
		"platform|en|errors.not_found":   "Not found",
		"platform|en|errors.bad_request": "Bad request",
		"platform|id|errors.bad_request": "Permintaan tidak valid",
	}
	ctx := i18nCtx(res, "example", "en")
	if got := writeWith(t, ctx, 404, "not_found", "bawaan", nil); got["message"] != "Note not found" || got["code"] != "not_found" {
		t.Errorf("scope modul harus menang: %v", got)
	}
	if got := writeWith(t, ctx, 400, "bad_request", "bawaan", nil); got["message"] != "Bad request" {
		t.Errorf("fallback ke platform: %v", got)
	}
	if got := writeWith(t, ctx, 409, "conflict", "pesan pemanggil", nil); got["message"] != "pesan pemanggil" {
		t.Errorf("tanpa kunci, message pemanggil dipertahankan: %v", got)
	}
	idCtx := i18nCtx(res, "example", "id")
	if got := writeWith(t, idCtx, 400, "bad_request", "bawaan", nil); got["message"] != "Permintaan tidak valid" {
		t.Errorf("bahasa id: %v", got)
	}
}

func TestWriteError_DetailsAndCodeUntouched(t *testing.T) {
	res := mapResolver{"platform|en|errors.validation_failed": "Invalid data"}
	got := writeWith(t, i18nCtx(res, "", "en"), 422, "validation_failed", "Data tidak valid.",
		[]Detail{{Field: "title", Code: "required", Message: "wajib diisi"}})
	if got["message"] != "Invalid data" || got["code"] != "validation_failed" {
		t.Fatalf("got %v", got)
	}
	d, _ := got["details"].([]any)
	if len(d) != 1 || d[0].(map[string]any)["message"] != "wajib diisi" {
		t.Errorf("details tidak boleh berubah: %v", got["details"])
	}
}

func TestWriteError_5xxMessageIsStaticCatalogText(t *testing.T) {
	res := mapResolver{"platform|en|errors.internal_error": "Something went wrong"}
	got := writeWith(t, i18nCtx(res, "example", "en"), 500, "internal_error", "terjadi kesalahan pada server", nil)
	if got["message"] != "Something went wrong" {
		t.Errorf("got %v", got)
	}
	if _, has := got["details"]; has {
		t.Error("5xx tidak boleh memuat details")
	}
}

func TestWriteError_WithoutResolverOrWithInvalidMessageKeepsOriginal(t *testing.T) {
	if got := writeWith(t, context.Background(), 400, "bad_request", "asli", nil); got["message"] != "asli" {
		t.Errorf("tanpa resolver: %v", got)
	}
	// pesan katalog yang butuh argumen tidak boleh membocorkan placeholder mentah; Format tetap sukses
	res := mapResolver{"platform|en|errors.bad_request": "Halo {x"}
	if got := writeWith(t, i18nCtx(res, "", "en"), 400, "bad_request", "asli", nil); got["message"] != "asli" {
		t.Errorf("pesan katalog rusak: message pemanggil harus dipertahankan: %v", got)
	}
}
