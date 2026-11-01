// Package sqlgen membangkitkan teks SQL dari model skema turunan metadata (paket dbmodel): DDL tabel,
// constraint, indeks, kolom turunan, penerjemah ekspresi dan kondisi dari tata bahasa tertutup, dan badan
// pemicu dari templat tetap (spec resource-metadata Requirement 5.3, 10.4 sampai 10.6, 11.5, 19.7, 19.8).
//
// Aturan keamanan paket ini (design D-32):
//   - Identifier selalu dikutip lewat QuoteIdent; tidak ada nama yang disisipkan tanpa kutip.
//   - Semua literal teks lewat satu fungsi, Literal; literal numerik lewat NumericLiteral yang menolak apa pun
//     di luar string desimal kanonik. Tidak ada penyalinan teks berkas metadata ke keluaran.
//   - Badan fungsi memakai kutip dolar dengan penanda yang dipilih agar tidak muncul di badan (DollarQuote).
//   - Setiap pernyataan keluaran adalah satu pernyataan utuh (diuji dengan pemecah pernyataan tingkat atas).
package sqlgen

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"erpora/internal/devtools/metadatatools/dbmodel"
	"erpora/sdk/metadata"
)

// maxIdentBytes adalah batas identifier PostgreSQL (NAMEDATALEN-1); lebih panjang dipotong diam-diam oleh
// server sehingga ditolak di sini.
const maxIdentBytes = 63

// QuoteIdent mengutip identifier: dibungkus tanda kutip ganda dengan kutip ganda di dalamnya digandakan.
// Nama kosong, mengandung NUL, bukan UTF-8 sah, atau melebihi 63 byte ditolak.
func QuoteIdent(name string) (string, error) {
	switch {
	case name == "":
		return "", errors.New("sqlgen: identifier kosong")
	case !utf8.ValidString(name):
		return "", errors.New("sqlgen: identifier bukan UTF-8 sah")
	case strings.ContainsRune(name, 0):
		return "", errors.New("sqlgen: identifier mengandung NUL")
	case len(name) > maxIdentBytes:
		return "", fmt.Errorf("sqlgen: identifier %d byte melebihi batas %d", len(name), maxIdentBytes)
	}
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`, nil
}

// Literal adalah SATU-SATUNYA fungsi yang mengubah teks menjadi literal string SQL. Kutip tunggal digandakan.
// Bila teks memuat backslash, hasilnya literal E'...' dengan backslash digandakan, sehingga benar baik
// standard_conforming_strings aktif maupun tidak. NUL dan UTF-8 tak sah ditolak (PostgreSQL tidak dapat
// menyimpannya). Karakter lain, termasuk $, titik koma, tanda komentar, dan baris baru, aman di dalam literal.
func Literal(s string) (string, error) {
	if !utf8.ValidString(s) {
		return "", errors.New("sqlgen: literal bukan UTF-8 sah")
	}
	if strings.ContainsRune(s, 0) {
		return "", errors.New("sqlgen: literal mengandung NUL")
	}
	q := strings.ReplaceAll(s, "'", "''")
	if strings.ContainsRune(q, '\\') {
		return "E'" + strings.ReplaceAll(q, `\`, `\\`) + "'", nil
	}
	return "'" + q + "'", nil
}

// DollarQuote membungkus badan fungsi dengan kutip dolar. Penanda dipilih ($erpora$, $erpora1$, ...) sehingga
// "$<penanda>" tidak muncul di badan; dengan begitu isi badan, termasuk literal dari masukan, tidak dapat
// menutup kutip lebih awal.
func DollarQuote(body string) string {
	tag := "erpora"
	for i := 1; strings.Contains(body, "$"+tag); i++ {
		tag = fmt.Sprintf("erpora%d", i)
	}
	return "$" + tag + "$" + body + "$" + tag + "$"
}

// NumericLiteral memvalidasi teks desimal kanonik (-?(0|[1-9][0-9]*)(.[0-9]+)?, paling banyak 64 digit) dan
// mengembalikannya apa adanya sebagai literal numerik. Selain itu ditolak.
func NumericLiteral(s string) (string, error) {
	if _, ok := metadata.ParseDecimal(s); !ok {
		return "", fmt.Errorf("sqlgen: %q bukan string desimal kanonik", truncate(s))
	}
	return s, nil
}

func truncate(s string) string {
	if len(s) > 40 {
		return s[:40] + "..."
	}
	return s
}

func isDigitsN(s string, minN, maxN int) bool {
	if len(s) < minN || len(s) > maxN {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// DefaultLiteral mengubah nilai bawaan kolom menjadi literal SQL menurut tipe metadatanya.
func DefaultLiteral(d *dbmodel.Default) (string, error) {
	switch d.Type {
	case metadata.TypeBoolean:
		if d.Value == "true" || d.Value == "false" {
			return d.Value, nil
		}
		return "", fmt.Errorf("sqlgen: default boolean %q tidak sah", truncate(d.Value))
	case metadata.TypeInteger:
		v := strings.TrimPrefix(d.Value, "-")
		if !isDigitsN(v, 1, 18) {
			return "", fmt.Errorf("sqlgen: default integer %q tidak sah", truncate(d.Value))
		}
		return d.Value, nil
	case metadata.TypeDecimal:
		return NumericLiteral(d.Value)
	case metadata.TypeString, metadata.TypeText, metadata.TypeEnum, metadata.TypeUUID, metadata.TypeDate, metadata.TypeDateTime:
		return Literal(d.Value)
	}
	return "", fmt.Errorf("sqlgen: tipe default %q tidak dikenal", d.Type)
}
