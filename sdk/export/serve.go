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

func (t *trackWriter) Write(p []byte) (int, error) {
	if len(p) > 0 {
		t.wrote = true
	}
	return t.w.Write(p)
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
		h.Del("Content-Type")
		h.Del("Content-Disposition")
		h.Del("Cache-Control")
		return err
	}
	slog.ErrorContext(ctx, "ekspor terputus di tengah stream", "resource", spec.Resource, "error", err)
	panic(http.ErrAbortHandler)
}
