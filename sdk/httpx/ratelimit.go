package httpx

import (
	"net/http"
	"strconv"
	"time"
)

// CodeRateLimited adalah kode galat 429 (api-guidelines §8).
const CodeRateLimited = "rate_limited"

// HeaderRetryAfter adalah header RFC 9110 §10.2.3 yang menyatakan berapa lama klien menunggu.
const HeaderRetryAfter = "Retry-After"

// TooManyRequests menjawab 429 `rate_limited` dengan header Retry-After berupa bilangan bulat detik (delta-seconds
// RFC 9110): retryAfter dibulatkan ke atas dan minimal 1, sehingga klien tidak pernah disuruh menunggu 0 detik dan tidak
// pernah mencoba lebih awal dari yang diizinkan. Amplop galat standar (api-guidelines §8); pesan terlokalisasi dari
// katalog errors.rate_limited bila context membawa penerjemah. Pesan tidak membedakan sebab pembatasan (per IP, per
// identitas sasaran, jeda lockout, atau pembatas hash), agar tidak membocorkan apakah sebuah akun ada.
func TooManyRequests(w http.ResponseWriter, r *http.Request, retryAfter time.Duration) {
	w.Header().Set(HeaderRetryAfter, strconv.FormatInt(retryAfterSeconds(retryAfter), 10))
	WriteError(w, r, http.StatusTooManyRequests, CodeRateLimited, "Terlalu banyak permintaan; coba lagi nanti.", nil)
}

// retryAfterSeconds membulatkan d ke atas ke detik utuh, minimal 1.
func retryAfterSeconds(d time.Duration) int64 {
	if d <= time.Second {
		return 1
	}
	secs := int64(d / time.Second)
	if d%time.Second != 0 {
		secs++
	}
	return secs
}
