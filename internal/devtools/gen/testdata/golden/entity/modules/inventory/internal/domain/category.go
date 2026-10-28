package domain

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// MaxCategoryTitleLength adalah panjang maksimum judul Category (karakter).
const MaxCategoryTitleLength = 200

// ErrInvalidCategoryTitle dikembalikan bila judul kosong atau terlalu panjang.
var ErrInvalidCategoryTitle = errors.New("judul category harus 1 sampai 200 karakter")

// Category adalah entity modul inventory, tanpa I/O. ID, Version, CreatedAt, dan UpdatedAt diisi saat disimpan;
// Version naik setiap perubahan dan mulai dari 1 (api-guidelines §4.3, optimistic concurrency).
type Category struct {
	ID        string
	Title     string
	Version   int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewCategory membuat Category baru dengan judul yang tervalidasi. Tambahkan field dan aturan domain di sini.
func NewCategory(title string) (Category, error) {
	n := utf8.RuneCountInString(title)
	if strings.TrimSpace(title) == "" || n < 1 || n > MaxCategoryTitleLength {
		return Category{}, ErrInvalidCategoryTitle
	}
	return Category{Title: title}, nil
}
