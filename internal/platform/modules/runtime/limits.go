package runtime

import (
	"net/http"

	"erpora/internal/platform/modules/registry"
	"erpora/sdk/httpx"
	"erpora/sdk/module"
)

// requestLimitsMW menegakkan batas request per route sebelum handler modul (FR-05, Q26 butir 13):
// body dibatasi http.MaxBytesReader (juga untuk handler yang membaca body sendiri), batasnya dibagikan
// ke httpx.DecodeJSON lewat context, dan data confidential ke atas ditandai Cache-Control: no-store
// (security-guidelines §6) sebelum handler berjalan sehingga ikut menempel pada respons error. Services
// platform (masker field, auditor ekspor) dipasang di context yang sama.
func requestLimitsMW(rt registry.RouteInfo, svc Services, next http.Handler) http.Handler {
	limit := rt.MaxBodyBytes
	if limit <= 0 {
		limit = module.DefaultMaxBodyBytes
	}
	noStore := rt.Classification.AtLeast(module.ClassConfidential)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if noStore {
			w.Header().Set("Cache-Control", "no-store")
			nw := &noStoreWriter{ResponseWriter: w}
			w = nw
			// Handler yang tidak menulis apa pun lalu menghapus header: paksa lagi sebelum net/http mengirim respons kosong.
			defer nw.force()
		}
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
		ctx := httpx.WithBodyLimit(r.Context(), limit)
		ctx = svc.install(ctx)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// noStoreWriter memaksa Cache-Control: no-store tepat sebelum header terkirim (WriteHeader/Write/Flush pertama),
// sehingga handler tidak dapat menimpa atau menghapusnya (Codex PR#7 C5). Unwrap menjaga http.ResponseController
// (Flush, Hijack, SetWriteDeadline, dst) tetap bekerja.
type noStoreWriter struct {
	http.ResponseWriter
	sent bool
}

func (w *noStoreWriter) force() {
	if !w.sent {
		w.sent = true
		w.Header().Set("Cache-Control", "no-store")
	}
}

func (w *noStoreWriter) WriteHeader(code int) {
	// Respons informasional 1xx (kecuali 101) tidak mengirim header akhir; header dipaksa saat status akhir ditulis.
	if code >= 200 || code == http.StatusSwitchingProtocols {
		w.force()
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *noStoreWriter) Write(b []byte) (int, error) {
	w.force()
	return w.ResponseWriter.Write(b)
}

func (w *noStoreWriter) Flush() {
	w.force()
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *noStoreWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
