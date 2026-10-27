package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// listStockCountsPrincipalKey membawa "principal" uji lewat context; menggantikan sdk/authz sampai PRD-001 tersedia.
type listStockCountsPrincipalKey struct{}

const listStockCountsSampleID = "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"

// listStockCountsDo mengirim request ke handler lewat ServeMux (agar r.PathValue terisi) sebagai principal tertentu.
func listStockCountsDo(principal, id, body string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.Handle("GET /stock-counts", ListStockCounts())
	target := strings.ReplaceAll("/stock-counts", "{id}", id)
	req := httptest.NewRequest("GET", target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if principal != "" {
		req = req.WithContext(context.WithValue(req.Context(), listStockCountsPrincipalKey{}, principal))
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// listStockCountsRestore mengembalikan titik sambung use case setelah test.
func listStockCountsRestore(t *testing.T) {
	t.Helper()
	orig := listStockCountsFunc
	t.Cleanup(func() { listStockCountsFunc = orig })
}

// Kerangka belum disambungkan ke use case: harus menjawab 501, bukan panik atau 200 palsu.
func TestListStockCounts_NotImplementedByDefault(t *testing.T) {
	rec := listStockCountsDo("owner", listStockCountsSampleID, "")
	if rec.Code != http.StatusNotImplemented || errCode(t, rec) != "not_implemented" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestListStockCounts_ListsAndPaginates(t *testing.T) {
	listStockCountsRestore(t)
	var gotPage, gotSize int
	listStockCountsFunc = func(_ context.Context, page, size int) ([]ListStockCountsData, int64, error) {
		gotPage, gotSize = page, size
		return []ListStockCountsData{{ID: listStockCountsSampleID, Title: "a", Version: 1}}, 60, nil
	}
	rec := listStockCountsDo("owner", "", "")
	if rec.Code != http.StatusOK || gotPage != 1 || gotSize != 25 {
		t.Fatalf("%d page=%d size=%d %s", rec.Code, gotPage, gotSize, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"hasMore":true`) {
		t.Errorf("page = %s", rec.Body.String())
	}
}

func TestListStockCounts_InvalidParams(t *testing.T) {
	for _, q := range []string{"page=0", "pageSize=201", "pageSize=abc"} {
		mux := http.NewServeMux()
		mux.Handle("GET /stock-counts", ListStockCounts())
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", "/stock-counts?"+q, nil))
		if rec.Code != http.StatusBadRequest || errCode(t, rec) != "bad_request" {
			t.Errorf("%s: %d %s", q, rec.Code, rec.Body.String())
		}
	}
}

func TestListStockCounts_EmptyListIsArray(t *testing.T) {
	listStockCountsRestore(t)
	listStockCountsFunc = func(context.Context, int, int) ([]ListStockCountsData, int64, error) { return nil, 0, nil }
	rec := listStockCountsDo("owner", "", "")
	if !strings.Contains(rec.Body.String(), `"data":[]`) {
		t.Errorf("data harus array kosong: %s", rec.Body.String())
	}
}

// api-guidelines §8: detail 5xx tidak boleh bocor ke respons.
func TestListStockCounts_InternalErrorHidesDetail(t *testing.T) {
	listStockCountsRestore(t)
	listStockCountsFunc = func(context.Context, int, int) ([]ListStockCountsData, int64, error) {
		return nil, 0, errors.New("SELECT rahasia")
	}
	rec := listStockCountsDo("owner", listStockCountsSampleID, "")
	if rec.Code != http.StatusInternalServerError || errCode(t, rec) != "internal_error" || strings.Contains(rec.Body.String(), "rahasia") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}
