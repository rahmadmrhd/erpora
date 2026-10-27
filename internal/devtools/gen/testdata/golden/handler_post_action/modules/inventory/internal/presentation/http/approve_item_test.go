package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"erpora/modules/inventory/internal/domain"
)

// approveItemPrincipalKey membawa "principal" uji lewat context; menggantikan sdk/authz sampai PRD-001 tersedia.
type approveItemPrincipalKey struct{}

const approveItemSampleID = "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"

// approveItemDo mengirim request ke handler lewat ServeMux (agar r.PathValue terisi) sebagai principal tertentu.
func approveItemDo(principal, id, body string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.Handle("POST /items/{id}/approve", ApproveItem())
	target := strings.ReplaceAll("/items/{id}/approve", "{id}", id)
	req := httptest.NewRequest("POST", target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if principal != "" {
		req = req.WithContext(context.WithValue(req.Context(), approveItemPrincipalKey{}, principal))
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

const approveItemBody = `{"reason":"Contoh"}`

// approveItemRestore mengembalikan titik sambung use case setelah test.
func approveItemRestore(t *testing.T) {
	t.Helper()
	orig := approveItemFunc
	t.Cleanup(func() { approveItemFunc = orig })
}

// approveItemIsDefault melaporkan apakah titik sambung masih fungsi bawaan kerangka (501).
func approveItemIsDefault() bool {
	return reflect.ValueOf(approveItemFunc).Pointer() == reflect.ValueOf(defaultApproveItem).Pointer()
}

// Kerangka belum disambungkan ke use case: harus menjawab 501, bukan panik atau 200 palsu.
func TestApproveItem_NotImplementedByDefault(t *testing.T) {
	rec := approveItemDo("owner", approveItemSampleID, approveItemBody)
	if rec.Code != http.StatusNotImplemented || errCode(t, rec) != "not_implemented" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

// ID yang bukan UUID dijawab 404 (tidak dibedakan dari record di luar scope).
func TestApproveItem_InvalidIDIs404(t *testing.T) {
	rec := approveItemDo("owner", "bukan-uuid", approveItemBody)
	if rec.Code != http.StatusNotFound || errCode(t, rec) != "not_found" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

// Test IDOR (CLAUDE.md, PRD-001 FR-119): principal lain tidak boleh melihat atau mengubah record milik pemilik; jawabannya
// 404 not_found (bukan 403) tanpa membocorkan isi record. Kerangka ini memakai "principal" palsu di context; setelah
// sdk/authz.Check/authz.Filter tersedia, ganti dengan dua principal sungguhan (user A vs user B) dan pertahankan asersinya.
func TestApproveItem_IDOR_OtherPrincipalGets404(t *testing.T) {
	if !approveItemIsDefault() {
		t.Fatal("approveItemFunc sudah disambungkan ke use case sungguhan, sedangkan test IDOR kerangka ini memakai stub (principal palsu) yang menimpa titik sambung dan tetap hijau walau pembatasan scope di use case hilang. Tulis ulang test ini terhadap use case sungguhan di atas repository yang menolak record milik principal lain (dua principal: A vs B), lalu hapus pemeriksaan ini.")
	}
	approveItemRestore(t)
	approveItemFunc = func(ctx context.Context, id, _, _ string, _ ApproveItemRequest) (ApproveItemData, error) {
		if ctx.Value(approveItemPrincipalKey{}) != "owner" {
			return ApproveItemData{}, domain.ErrNotFound
		}
		return ApproveItemData{ID: id, Title: "rahasia-pemilik", Version: 2}, nil
	}

	intruder := approveItemDo("intruder", approveItemSampleID, approveItemBody)
	if intruder.Code != http.StatusNotFound || errCode(t, intruder) != "not_found" {
		t.Fatalf("principal lain: %d %s, want 404 not_found", intruder.Code, intruder.Body.String())
	}
	if strings.Contains(intruder.Body.String(), "rahasia-pemilik") {
		t.Errorf("isi record bocor ke principal lain: %s", intruder.Body.String())
	}
	owner := approveItemDo("owner", approveItemSampleID, approveItemBody)
	if owner.Code != 200 {
		t.Fatalf("pemilik: %d %s, want 200", owner.Code, owner.Body.String())
	}
}

// api-guidelines §7.2: header Idempotency-Key diteruskan ke use case (yang WAJIB idempoten terhadapnya).
func TestApproveItem_ForwardsIdempotencyKey(t *testing.T) {
	approveItemRestore(t)
	var got string
	approveItemFunc = func(_ context.Context, id, _, key string, _ ApproveItemRequest) (ApproveItemData, error) {
		got = key
		return ApproveItemData{ID: id, Version: 2}, nil
	}
	mux := http.NewServeMux()
	mux.Handle("POST /items/{id}/approve", ApproveItem())
	req := httptest.NewRequest("POST", strings.ReplaceAll("/items/{id}/approve", "{id}", approveItemSampleID), strings.NewReader(approveItemBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "kunci-123")
	mux.ServeHTTP(httptest.NewRecorder(), req)
	if got != "kunci-123" {
		t.Errorf("Idempotency-Key ke use case = %q, want kunci-123", got)
	}
}

func TestApproveItem_UnknownFieldIs422(t *testing.T) {
	rec := approveItemDo("owner", approveItemSampleID, `{"tidakAda":1}`)
	if rec.Code != http.StatusUnprocessableEntity || errCode(t, rec) != "validation_failed" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

// api-guidelines §8: detail 5xx tidak boleh bocor ke respons.
func TestApproveItem_InternalErrorHidesDetail(t *testing.T) {
	approveItemRestore(t)
	approveItemFunc = func(context.Context, string, string, string, ApproveItemRequest) (ApproveItemData, error) {
		return ApproveItemData{}, errors.New("SELECT rahasia")
	}
	rec := approveItemDo("owner", approveItemSampleID, approveItemBody)
	if rec.Code != http.StatusInternalServerError || errCode(t, rec) != "internal_error" || strings.Contains(rec.Body.String(), "rahasia") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}
