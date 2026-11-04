// Package cli berisi command CLI modul core (`core:*`; PRD-001 C1.12). Command memanggil identity.Admin yang dipasang
// platform pada context command modul core (ADR-0013 §7a butir 5) dan hanya berhasil di bawah hak sistem berawalan
// `cli:core:` (PRD-001 Q30 butir 1). Command tidak mengimpor platform (D9).
package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	sdkidentity "erpora/sdk/identity"
	"erpora/sdk/module"
)

const (
	// DefaultAdminName adalah nama tampilan administrator bila --name tidak diberikan.
	DefaultAdminName = "Administrator"
	// PublicURLKey adalah satu-satunya kunci yang dikenal core:settings:set pada C1 (PRD-001 Q24).
	PublicURLKey = "platform.public_url"
	// maxPasswordInput membatasi byte yang dibaca untuk satu sandi (kebijakan FR-101 menolak > 1024 byte; batas ini
	// hanya mencegah membaca masukan tak terbatas).
	maxPasswordInput = 4096
)

// Commands mengembalikan command CLI modul core. Nama tercatat sebagai `core:<Name>`.
func Commands() []module.Command {
	return []module.Command{
		{
			Name:  "create-admin",
			Short: "Buat administrator pertama (sandi dibaca dari prompt tersembunyi atau stdin)",
			Flags: []module.Flag{
				{Name: "email", Usage: "email administrator (wajib)"},
				{Name: "name", Usage: "nama tampilan", Default: DefaultAdminName},
			},
			Run: createAdmin,
		},
		{
			Name:  "rotate-keys",
			Short: "Buat kunci penandatangan JWT baru berstatus pending dan laporkan waktu aktivasinya",
			Run:   rotateKeys,
		},
		{
			Name:  "reset-mfa",
			Short: "Hapus faktor MFA dan recovery code pengguna, serta cabut seluruh sesinya",
			Flags: []module.Flag{{Name: "email", Usage: "email pengguna (wajib)"}},
			Run:   resetMFA,
		},
		{
			Name:  "unlock",
			Short: "Buka akun yang terkunci (locked) dan hapus jeda login pasangannya",
			Flags: []module.Flag{{Name: "email", Usage: "email pengguna (wajib)"}},
			Run:   unlock,
		},
		{
			Name:  "settings:set",
			Short: "Isi setting platform (di C1 hanya platform.public_url)",
			Flags: []module.Flag{
				{Name: "key", Usage: "kunci setting; didukung: " + PublicURLKey},
				{Name: "value", Usage: "nilai setting"},
			},
			Run: settingsSet,
		},
	}
}

// admin mengambil identity.Admin dari context command. Hanya modul core yang menerimanya (platform menegakkan ini di
// runtime.Services.installIdentity); tidak ada Admin berarti command salah rakit.
func admin(ctx context.Context) (sdkidentity.Admin, error) {
	a, ok := sdkidentity.AdminFrom(ctx)
	if !ok {
		return nil, errors.New("identity.Admin tidak tersedia pada context command (perakitan salah)")
	}
	return a, nil
}

func requireFlag(inv module.Invocation, name string) (string, error) {
	v := strings.TrimSpace(inv.Flags[name])
	if v == "" {
		return "", fmt.Errorf("flag --%s wajib diisi", name)
	}
	return v, nil
}

func createAdmin(ctx context.Context, inv module.Invocation) error {
	email, err := requireFlag(inv, "email")
	if err != nil {
		return err
	}
	name := strings.TrimSpace(inv.Flags["name"])
	if name == "" {
		name = DefaultAdminName
	}
	a, err := admin(ctx)
	if err != nil {
		return err
	}
	pw, err := readPassword(inv)
	if err != nil {
		return err
	}
	sum, err := a.CreateAdmin(ctx, sdkidentity.CreateAdminInput{Email: email, Name: name, Password: pw})
	if err != nil {
		return describe(err)
	}
	_, err = fmt.Fprintf(inv.Out, "administrator siap: %s (id %s). Login pertama akan diminta mendaftarkan MFA.\n", sum.Email, sum.ID)
	return err
}

func rotateKeys(ctx context.Context, inv module.Invocation) error {
	a, err := admin(ctx)
	if err != nil {
		return err
	}
	k, err := a.RotateSigningKeys(ctx)
	if err != nil {
		return describe(err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "kunci baru: %s\nalgoritma : %s\nstatus    : %s\ndibuat    : %s\n",
		k.KeyID, k.Algorithm, k.Status, k.CreatedAt.UTC().Format(time.RFC3339))
	if !k.ActivatesAt.IsZero() {
		fmt.Fprintf(&b, "aktif pada: %s\n", k.ActivatesAt.UTC().Format(time.RFC3339))
	}
	b.WriteString("Kunci pending sudah terpublikasi di JWKS tetapi belum menandatangani. Promosi menjadi active dilakukan oleh\n" +
		"proses `erpora serve` yang berjalan (atau saat serve dimulai bila waktu aktivasi sudah lewat).\n")
	_, err = io.WriteString(inv.Out, b.String())
	return err
}

func resetMFA(ctx context.Context, inv module.Invocation) error {
	email, err := requireFlag(inv, "email")
	if err != nil {
		return err
	}
	a, err := admin(ctx)
	if err != nil {
		return err
	}
	if err := a.ResetMFA(ctx, email); err != nil {
		return describe(err)
	}
	_, err = fmt.Fprintf(inv.Out, "MFA direset untuk %s; seluruh sesinya dicabut.\n", email)
	return err
}

func unlock(ctx context.Context, inv module.Invocation) error {
	email, err := requireFlag(inv, "email")
	if err != nil {
		return err
	}
	a, err := admin(ctx)
	if err != nil {
		return err
	}
	if err := a.UnlockAccount(ctx, email); err != nil {
		return describe(err)
	}
	_, err = fmt.Fprintf(inv.Out, "akun %s dibuka.\n", email)
	return err
}

func settingsSet(ctx context.Context, inv module.Invocation) error {
	key, err := requireFlag(inv, "key")
	if err != nil {
		return err
	}
	if key != PublicURLKey {
		return fmt.Errorf("kunci setting %q tidak dikenal; di C1 hanya %s yang didukung", key, PublicURLKey)
	}
	value, err := requireFlag(inv, "value")
	if err != nil {
		return err
	}
	a, err := admin(ctx)
	if err != nil {
		return err
	}
	if err := a.SetPublicURL(ctx, value); err != nil {
		return describe(err)
	}
	_, err = fmt.Fprintf(inv.Out, "%s diperbarui.\n", key)
	return err
}

// describe mengubah galat domain identity menjadi pesan untuk operator. Pesan tidak pernah memuat sandi (Admin tidak
// pernah mengembalikannya); galat tak dikenal diteruskan apa adanya.
func describe(err error) error {
	var pol *sdkidentity.PasswordPolicyError
	switch {
	case errors.As(err, &pol):
		return fmt.Errorf("sandi tidak memenuhi kebijakan (%s)", strings.Join(pol.Codes, ", "))
	case errors.Is(err, sdkidentity.ErrEmailTaken):
		return errors.New("email sudah dipakai akun yang ada; core:create-admin hanya melanjutkan sisa pembuatan yang terputus")
	case errors.Is(err, sdkidentity.ErrNotFound):
		return errors.New("pengguna dengan email itu tidak ditemukan")
	case errors.Is(err, sdkidentity.ErrForbidden):
		return errors.New("operasi ditolak: hak sistem command tidak sah")
	case errors.Is(err, sdkidentity.ErrInvalidInput):
		return fmt.Errorf("masukan tidak valid: %w", err)
	default:
		return err
	}
}

// readPassword membaca sandi: dari prompt tersembunyi (dua kali, dicocokkan) bila stdin adalah terminal, selain itu
// satu baris dari stdin (mis. `printf '%s\n' "$PW" | erpora core:create-admin ...`). Tidak ada flag sandi: argumen
// proses terlihat oleh pengguna lain di sistem dan tersimpan di riwayat shell.
func readPassword(inv module.Invocation) (string, error) {
	if f, ok := inv.In.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		fd := int(f.Fd())
		fmt.Fprint(inv.Out, "Sandi: ")
		first, err := term.ReadPassword(fd)
		fmt.Fprintln(inv.Out)
		if err != nil {
			return "", fmt.Errorf("baca sandi: %w", err)
		}
		fmt.Fprint(inv.Out, "Ulangi sandi: ")
		second, err := term.ReadPassword(fd)
		fmt.Fprintln(inv.Out)
		if err != nil {
			return "", fmt.Errorf("baca sandi: %w", err)
		}
		if string(first) != string(second) {
			return "", errors.New("kedua masukan sandi tidak sama")
		}
		if len(first) == 0 {
			return "", errors.New("sandi kosong")
		}
		return string(first), nil
	}
	if inv.In == nil {
		return "", errors.New("stdin tidak tersedia: beri sandi lewat stdin atau jalankan di terminal")
	}
	line, err := bufio.NewReaderSize(io.LimitReader(inv.In, maxPasswordInput+1), maxPasswordInput+1).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("baca sandi dari stdin: %w", err)
	}
	if len(line) > maxPasswordInput {
		return "", errors.New("sandi dari stdin terlalu panjang")
	}
	line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	if line == "" {
		return "", errors.New("sandi kosong: beri sandi lewat stdin atau jalankan di terminal")
	}
	return line, nil
}
