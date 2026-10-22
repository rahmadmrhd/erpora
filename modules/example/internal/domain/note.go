// Package domain berisi entity dan aturan domain modul example, tanpa I/O.
package domain

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// MaxTitleLength adalah panjang maksimum judul (karakter).
const MaxTitleLength = 200

// ErrInvalidTitle dikembalikan bila judul kosong atau terlalu panjang.
var ErrInvalidTitle = errors.New("judul harus 1 sampai 200 karakter")

// Note adalah catatan sederhana. ID dan CreatedAt diisi saat disimpan.
type Note struct {
	ID        string
	Title     string
	CreatedAt time.Time
}

// NewNote membuat Note baru dengan judul yang tervalidasi.
func NewNote(title string) (Note, error) {
	n := utf8.RuneCountInString(title)
	if strings.TrimSpace(title) == "" || n < 1 || n > MaxTitleLength {
		return Note{}, ErrInvalidTitle
	}
	return Note{Title: title}, nil
}
