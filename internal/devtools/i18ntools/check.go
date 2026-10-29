package i18ntools

import "errors"

// CheckOptions mengonfigurasi Check.
type CheckOptions struct {
	// Strict mengubah semua peringatan menjadi galat (Requirement 8.4).
	Strict bool
}

// Check memeriksa semua katalog di root repo (belum diimplementasikan).
func Check(root string, opts CheckOptions) (*Report, error) {
	return nil, errors.New("belum diimplementasikan")
}

// StatusRow adalah kelengkapan satu bahasa pada satu namespace.
type StatusRow struct {
	Namespace  string  `json:"namespace"`
	Locale     string  `json:"locale"`
	Default    bool    `json:"default"`
	Partial    bool    `json:"partial"`
	Total      int     `json:"total"`
	Translated int     `json:"translated"`
	Percent    float64 `json:"percent"`
}

// ValidationError berarti katalog tidak valid sehingga operasi tidak dapat dilanjutkan (exit 2).
type ValidationError struct{ Msg string }

// Error mengimplementasi error.
func (e *ValidationError) Error() string { return e.Msg }

// Status menghitung kelengkapan per bahasa per namespace (belum diimplementasikan).
func Status(root string) ([]StatusRow, error) {
	return nil, errors.New("belum diimplementasikan")
}
