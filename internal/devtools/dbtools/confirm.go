// Package dbtools berisi command database destruktif developer tools (db:fresh, db:seed). Semuanya hanya
// berjalan terhadap lingkungan development: ERP_ENV dan penanda DB diperiksa sebelum ada yang diubah (PRD-000 FR-22.5).
package dbtools

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrAborted: konfirmasi nama database tidak cocok (atau tidak diberikan); tidak ada yang diubah.
var ErrAborted = errors.New("dibatalkan: nama database tidak cocok")

// Confirm meminta pengguna mengetik nama database persis (sensitif huruf besar-kecil). Nama berbeda, kosong,
// atau EOF menghasilkan ErrAborted.
func Confirm(in io.Reader, out io.Writer, dbName string) error {
	if _, err := fmt.Fprintf(out, "Semua data di database %q akan DIHAPUS. Ketik nama database untuk melanjutkan: ", dbName); err != nil {
		return err
	}
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if strings.TrimSpace(line) != dbName || dbName == "" {
		return ErrAborted
	}
	return nil
}
