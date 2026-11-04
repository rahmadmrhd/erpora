package commands

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// secretKeyBytes adalah panjang ERP_SECRET_KEY (AES-256).
const secretKeyBytes = 32

// generateSecretKey membaca 32 byte dari r dan mengembalikannya sebagai base64 standar (setara
// `openssl rand -base64 32`).
func generateSecretKey(r io.Reader) (string, error) {
	b := make([]byte, secretKeyBytes)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", fmt.Errorf("bangkitkan kunci acak: %w", err)
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func newSecretGenerate() *cobra.Command {
	return &cobra.Command{
		Use:   "secret:generate",
		Short: "Cetak nilai ERP_SECRET_KEY baru (32 byte acak, base64); tidak menyimpan apa pun",
		Long: "Mencetak satu baris: 32 byte acak dari crypto/rand dalam base64 standar, setara `openssl rand -base64 32`.\n" +
			"Tidak menulis berkas dan tidak membaca konfigurasi. Salin hasilnya ke ERP_SECRET_KEY di .env (atau secret manager).\n" +
			"Jalankan SEKALI per lingkungan dan cadangkan terpisah dari backup database: mengganti kunci membuat data yang\n" +
			"sudah tersegel (kunci JWT, secret MFA, secret settings) tidak terbaca.\n\n" +
			"PERINGATAN CI: keluaran command ini adalah rahasia dan akan tercetak di log bila dijalankan di CI. Jangan jalankan di\n" +
			"CI tanpa menyamarkannya dulu (`echo \"::add-mask::$key\"` pada GitHub Actions); lebih baik pakai pola openssl seperti\n" +
			"langkah \"Bangkitkan ERP_SECRET_KEY acak\" di .github/workflows/ci.yml (`openssl rand -base64 32` lalu ::add-mask:: dan $GITHUB_ENV).",
		GroupID: GroupDevtools,
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			key, err := generateSecretKey(rand.Reader)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(c.OutOrStdout(), key)
			return err
		},
	}
}
