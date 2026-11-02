// Runtime HTTP untuk DTO bangkitan metadata resource (spec resource-metadata, Requirement 9).
//
// Berkas ini adalah SUMBER yang disalin apa adanya (hanya klausa package diganti) oleh `metadata:generate` ke
// `modules/<dir>/internal/presentation/http/meta_runtime.gen.go`. Pembacaan body sepenuhnya didelegasikan ke
// httpx.DecodeJSON (batas body, kedalaman, panjang array, kunci ganda, tipe skalar, field tak dikenal), sehingga
// tidak ada logika dekode kedua yang harus dijaga. Galat memakai amplop api-guidelines section 8 dan tidak
// pernah memuat nilai field.
package httprt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"erpora/sdk/httpx"
	"erpora/sdk/i18n"
)

// maxViolations membatasi jumlah details pada satu respons (selaras httpx).
const maxViolations = 20

// violation menyalin domain.Violation (struktur identik) agar runtime ini tidak bergantung pada paket domain.
type violation struct {
	Field      string
	Code       string
	MessageKey string
	Business   bool
}

// decodeBody membaca body JSON ke T lewat httpx.DecodeJSON dan mengembalikan himpunan kunci tingkat atas yang
// dikirim klien (membedakan field hilang dari null eksplisit). Field tak dikenal menghasilkan 422
// validation_failed dengan details[].code unknown_field; field dalam readOnly menghasilkan kode read_only.
func decodeBody[T any](w http.ResponseWriter, r *http.Request, readOnly map[string]struct{}) (T, map[string]bool, error) {
	var buf bytes.Buffer
	body := r.Body
	if body == nil {
		body = http.NoBody
	}
	r2 := r.WithContext(r.Context())
	r2.Body = io.NopCloser(io.TeeReader(body, &buf))
	v, err := httpx.DecodeJSON[T](w, r2)
	if err != nil {
		return v, nil, relabelReadOnly(err, readOnly)
	}
	var raw map[string]json.RawMessage
	if jerr := json.Unmarshal(buf.Bytes(), &raw); jerr != nil {
		return v, nil, httpx.NewError(http.StatusBadRequest, "bad_request", "Body request bukan JSON yang valid.",
			httpx.Detail{Code: "malformed_json", Message: "Body request bukan JSON yang valid."})
	}
	present := make(map[string]bool, len(raw))
	for k := range raw {
		present[k] = true
	}
	return v, present, nil
}

// relabelReadOnly mengganti unknown_field menjadi read_only untuk field hanya-baca yang dikenal.
func relabelReadOnly(err error, readOnly map[string]struct{}) error {
	var he *httpx.Error
	if !errors.As(err, &he) {
		return err
	}
	details := make([]httpx.Detail, len(he.Details))
	for i, d := range he.Details {
		if _, ro := readOnly[d.Field]; ro && d.Code == "unknown_field" {
			d = httpx.Detail{Field: d.Field, Code: "read_only", Message: "Field hanya-baca dan tidak dapat diubah."}
		}
		details[i] = d
	}
	return &httpx.Error{Status: he.Status, Code: he.Code, Message: he.Message, Details: details, Err: he.Err}
}

// violationsError memetakan pelanggaran ke *httpx.Error: pelanggaran struktur menghasilkan 422
// validation_failed; bila hanya ada pelanggaran aturan bisnis, 422 business_rule_violated dengan kode aturan
// pada details[].code (api-guidelines section 7.3). nil bila tidak ada pelanggaran.
func violationsError[V ~struct {
	Field      string
	Code       string
	MessageKey string
	Business   bool
}](ctx context.Context, items []V) error {
	if len(items) == 0 {
		return nil
	}
	var structuralDetails, businessDetails []httpx.Detail
	for _, item := range items {
		v := violation(item)
		if v.Business {
			if len(businessDetails) < maxViolations {
				businessDetails = append(businessDetails, httpx.Detail{
					Field: v.Field, Code: v.Code, Message: businessMessage(ctx, v.MessageKey),
				})
			}
			continue
		}
		if len(structuralDetails) < maxViolations {
			structuralDetails = append(structuralDetails, httpx.Detail{
				Field: v.Field, Code: v.Code, Message: structuralMessage(v.Code),
			})
		}
	}
	if len(structuralDetails) > 0 {
		return httpx.NewError(http.StatusUnprocessableEntity, "validation_failed", "Data tidak valid.", structuralDetails...)
	}
	return httpx.NewError(http.StatusUnprocessableEntity, "business_rule_violated", "Aturan bisnis dilanggar.", businessDetails...)
}

// businessMessage menerjemahkan kunci pesan aturan dari katalog modul; tanpa terjemahan dipakai pesan umum.
func businessMessage(ctx context.Context, key string) string {
	if key != "" {
		if s, ok := i18n.Lookup(ctx, key); ok {
			return s
		}
	}
	return "Aturan bisnis dilanggar."
}

// structuralMessage memberi pesan umum per kode (tanpa nilai field).
func structuralMessage(code string) string {
	switch code {
	case "required":
		return "Field wajib diisi."
	case "null_not_allowed":
		return "Field tidak boleh null."
	case "read_only":
		return "Field hanya-baca dan tidak dapat diubah."
	case "invalid_type":
		return "Tipe nilai tidak sesuai."
	case "invalid_characters":
		return "Nilai memuat karakter yang tidak diizinkan."
	case "too_short":
		return "Nilai terlalu pendek."
	case "too_long":
		return "Nilai terlalu panjang."
	case "pattern_mismatch":
		return "Nilai tidak cocok dengan pola yang diizinkan."
	case "invalid_format":
		return "Format nilai tidak sah."
	case "invalid_enum_value":
		return "Nilai bukan salah satu pilihan yang sah."
	case "invalid_decimal":
		return "Nilai bukan bilangan desimal yang sah."
	case "decimal_scale_exceeded":
		return "Jumlah angka di belakang koma melebihi batas."
	case "decimal_precision_exceeded":
		return "Jumlah angka melebihi batas presisi."
	case "below_minimum":
		return "Nilai di bawah batas minimum."
	case "above_maximum":
		return "Nilai di atas batas maksimum."
	case "invalid_uuid":
		return "Nilai bukan UUID yang sah."
	case "invalid_date":
		return "Nilai bukan tanggal yang sah (YYYY-MM-DD)."
	case "invalid_datetime":
		return "Nilai bukan waktu UTC yang sah (RFC 3339 berakhiran Z)."
	case "conflicting_deprecated_field":
		return "Field lama dan penggantinya bernilai berbeda."
	}
	return "Nilai tidak sah."
}

// val mengembalikan nilai yang ditunjuk p, atau nil bila p nil (nilai untuk metadata.Record).
func val[T any](p *T) any {
	if p == nil {
		return nil
	}
	return *p
}

// clone menyalin nilai yang ditunjuk p ke pointer baru (nil tetap nil); dipakai mengisi bentuk pasangan rename
// tanpa berbagi pointer.
func clone[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
