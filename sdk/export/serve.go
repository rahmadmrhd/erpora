package export

import (
	"io"
	"iter"
	"log/slog"
	"net/http"

	"erpora/sdk/module"
)

const xlsxContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

type trackWriter struct {
	w     io.Writer
	wrote bool
}

// Write meneruskan ke writer dasar; wrote hanya true bila ada byte yang benar-benar diterima (byte pertama terkirim).
func (t *trackWriter) Write(p []byte) (int, error) {
	n, err := t.w.Write(p)
	if n > 0 {
		t.wrote = true
	}
	return n, err
}

// Serve memvalidasi spec, memasang header unduhan (Content-Type, Content-Disposition, dan
// Cache-Control: no-store bila ada kolom confidential ke atas), lalu menulis ekspor. Kesalahan sebelum
// byte pertama dikembalikan (handler memanggil httpx.RespondError); kesalahan di tengah stream dicatat
// lalu koneksi diputus dengan panic(http.ErrAbortHandler) agar klien tidak menerima berkas terpotong
// yang tampak utuh.
func Serve(w http.ResponseWriter, r *http.Request, spec Spec, rows iter.Seq2[Row, error]) error {
	ctx := r.Context()
	if err := Validate(ctx, spec); err != nil {
		return err
	}
	h := w.Header()
	// Nilai awal disimpan: bila gagal sebelum byte pertama, header dikembalikan ke keadaan semula (no-store yang sudah
	// dipasang platform berdasarkan klasifikasi route tidak boleh ikut terhapus).
	saved := map[string][]string{}
	for _, k := range []string{"Content-Type", "Content-Disposition", "Cache-Control"} {
		if v, ok := h[k]; ok {
			saved[k] = append([]string(nil), v...)
		}
	}
	ext, ctype := "csv", "text/csv; charset=utf-8"
	if spec.Format == XLSX {
		ext, ctype = "xlsx", xlsxContentType
	}
	name := spec.Filename + "." + ext
	h.Set("Content-Type", ctype)
	h.Set("Content-Disposition", `attachment; filename="`+name+`"; filename*=UTF-8''`+name)
	if highestClass(spec.Columns).AtLeast(module.ClassConfidential) {
		h.Set("Cache-Control", "no-store")
	}

	tw := &trackWriter{w: w}
	_, err := Write(ctx, tw, spec, rows)
	if err == nil {
		return nil
	}
	if !tw.wrote {
		for _, k := range []string{"Content-Type", "Content-Disposition", "Cache-Control"} {
			if v, ok := saved[k]; ok {
				h[k] = v
			} else {
				h.Del(k)
			}
		}
		return err
	}
	slog.ErrorContext(ctx, "ekspor terputus di tengah stream", "resource", spec.Resource, "error", err)
	panic(http.ErrAbortHandler)
}
