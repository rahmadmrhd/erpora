package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"erpora/modules/inventory/internal/domain"
)

// getItemPrincipalKey membawa "principal" uji lewat context; menggantikan sdk/authz sampai PRD-001 tersedia.
type getItemPrincipalKey struct{}

const getItemSampleID = "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"

// getItemDo mengirim request ke handler lewat ServeMux (agar r.PathValue terisi) sebagai principal tertentu.
func getItemDo(principal, id, body string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.Handle("GET /items/{id}", GetItem())
	target := strings.ReplaceAll("/items/{id}", "{id}", id)
	req := httptest.NewRequest("GET", target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if principal != "" {
		req = req.WithContext(context.WithValue(req.Context(), getItemPrincipalKey{}, principal))
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// getItemRestore mengembalikan titik sambung use case setelah test.
func getItemRestore(t *testing.T) {
	t.Helper()
	orig := getItemFunc
	t.Cleanup(func() { getItemFunc = orig })
}

// Kerangka belum disambungkan ke use case: harus menjawab 501, bukan panik atau 200 palsu.
func TestGetItem_NotImplementedByDefault(t *testing.T) {
	rec := getItemDo("owner", getItemSampleID, "")
	if rec.Code != http.StatusNotImplemented || errCode(t, rec) != "not_implemented" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

// ID yang bukan UUID dijawab 404 (tidak dibedakan dari record di luar scope).
func TestGetItem_InvalidIDIs404(t *testing.T) {
	rec := getItemDo("owner", "bukan-uuid", "")
	if rec.Code != http.StatusNotFound || errCode(t, rec) != "not_found" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

// Test IDOR (CLAUDE.md, PRD-001 FR-119): principal lain tidak boleh melihat atau mengubah record milik pemilik; jawabannya
// 404 not_found (bukan 403) tanpa membocorkan isi record. Kerangka ini memakai "principal" palsu di context; setelah
// sdk/authz.Check/authz.Filter tersedia, ganti dengan dua principal sungguhan (user A vs user B) dan pertahankan asersinya.
func TestGetItem_IDOR_OtherPrincipalGets404(t *testing.T) {
	getItemRestore(t)
	getItemFunc = func(ctx context.Context, id string) (GetItemData, error) {
		if ctx.Value(getItemPrincipalKey{}) != "owner" {
			return GetItemData{}, domain.ErrNotFound
		}
		return GetItemData{ID: id, Title: "rahasia-pemilik", Version: 1}, nil
	}

	intruder := getItemDo("intruder", getItemSampleID, "")
	if intruder.Code != http.StatusNotFound || errCode(t, intruder) != "not_found" {
		t.Fatalf("principal lain: %d %s, want 404 not_found", intruder.Code, intruder.Body.String())
	}
	if strings.Contains(intruder.Body.String(), "rahasia-pemilik") {
		t.Errorf("isi record bocor ke principal lain: %s", intruder.Body.String())
	}
	owner := getItemDo("owner", getItemSampleID, "")
	if owner.Code != 200 {
		t.Fatalf("pemilik: %d %s, want 200", owner.Code, owner.Body.String())
	}
}

// api-guidelines §8: detail 5xx tidak boleh bocor ke respons.
func TestGetItem_InternalErrorHidesDetail(t *testing.T) {
	getItemRestore(t)
	getItemFunc = func(context.Context, string) (GetItemData, error) { return GetItemData{}, errors.New("SELECT rahasia") }
	rec := getItemDo("owner", getItemSampleID, "")
	if rec.Code != http.StatusInternalServerError || errCode(t, rec) != "internal_error" || strings.Contains(rec.Body.String(), "rahasia") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}
