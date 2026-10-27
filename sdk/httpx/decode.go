package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"reflect"
	"strings"
)

type decodeConfig struct {
	allowEmpty bool
	maxDepth   int
	maxArray   int
}

// DecodeOption mengubah perilaku DecodeJSON.
type DecodeOption func(*decodeConfig)

// AllowEmptyBody membuat body kosong menghasilkan zero T (Content-Type boleh tidak ada).
func AllowEmptyBody() DecodeOption { return func(c *decodeConfig) { c.allowEmpty = true } }

// MaxDepth mengganti batas kedalaman JSON.
func MaxDepth(n int) DecodeOption { return func(c *decodeConfig) { c.maxDepth = n } }

// MaxArrayLen mengganti batas panjang array.
func MaxArrayLen(n int) DecodeOption { return func(c *decodeConfig) { c.maxArray = n } }

func badRequest(code, message, field string) *Error {
	return NewError(http.StatusBadRequest, "bad_request", message, Detail{Field: field, Code: code, Message: message})
}

// DecodeJSON membaca body (batas = BodyLimit(ctx) atau DefaultMaxBodyBytes) ke DTO T. T wajib struct;
// selain itu panic (kesalahan pemrograman). Semua kegagalan input adalah *Error: kerusakan struktur
// 400 bad_request, field tak dikenal dan tipe salah 422 validation_failed, dengan kode spesifik di
// error.details[].code (api-guidelines §8).
func DecodeJSON[T any](w http.ResponseWriter, r *http.Request, opts ...DecodeOption) (T, error) {
	var zero T
	rt := reflect.TypeFor[T]()
	if rt.Kind() != reflect.Struct {
		panic("httpx: DecodeJSON hanya untuk DTO struct")
	}
	cfg := decodeConfig{maxDepth: DefaultMaxDepth, maxArray: DefaultMaxArrayLen}
	for _, o := range opts {
		o(&cfg)
	}

	limit := DefaultMaxBodyBytes
	if n, ok := BodyLimit(r.Context()); ok {
		limit = n
	}
	var body []byte
	if r.Body != nil {
		var err error
		body, err = io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
		if err != nil {
			var mbe *http.MaxBytesError
			if errors.As(err, &mbe) {
				return zero, badRequest("body_too_large", "Body request melebihi batas ukuran.", "")
			}
			return zero, badRequest("malformed_json", "Body request tidak dapat dibaca.", "")
		}
	}
	if len(bytes.TrimSpace(body)) == 0 {
		if cfg.allowEmpty {
			return zero, nil
		}
		return zero, badRequest("body_required", "Body request wajib diisi.", "")
	}
	if !jsonContentType(r.Header.Get("Content-Type")) {
		return zero, badRequest("unsupported_media_type", "Content-Type harus application/json (charset utf-8).", "")
	}

	wk := &walker{dec: json.NewDecoder(bytes.NewReader(body)), maxDepth: cfg.maxDepth, maxArray: cfg.maxArray}
	wk.dec.UseNumber()
	if err := wk.root(rt); err != nil {
		return zero, err
	}
	if len(wk.details) > 0 {
		return zero, NewError(http.StatusUnprocessableEntity, "validation_failed", "Data tidak valid.", wk.details...)
	}

	var v T
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return zero, mapDecodeError(err)
	}
	return v, nil
}

func jsonContentType(ct string) bool {
	if ct == "" {
		return false
	}
	mt, params, err := mime.ParseMediaType(ct)
	if err != nil || mt != "application/json" {
		return false
	}
	if cs, ok := params["charset"]; ok && !strings.EqualFold(cs, "utf-8") {
		return false
	}
	return true
}

// mapDecodeError memetakan sisa kesalahan lapis kedua (jarang: mis. overflow atau Unmarshaler kustom).
func mapDecodeError(err error) error {
	var ute *json.UnmarshalTypeError
	if errors.As(err, &ute) {
		return NewError(http.StatusUnprocessableEntity, "validation_failed", "Data tidak valid.",
			Detail{Field: ute.Field, Code: "invalid_type", Message: "Tipe nilai tidak sesuai."})
	}
	var se *json.SyntaxError
	if errors.As(err, &se) || errors.Is(err, io.ErrUnexpectedEOF) {
		return badRequest("malformed_json", "Body request bukan JSON yang valid.", "")
	}
	return NewError(http.StatusUnprocessableEntity, "validation_failed", "Data tidak valid.",
		Detail{Code: "invalid_type", Message: "Nilai tidak dapat diproses."})
}
