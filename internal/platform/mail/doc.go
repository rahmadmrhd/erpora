// Package mail adalah driver pengirim surat platform (PRD-001 FR-108, K-C1.6). Paket ini memenuhi seam sdk/mail.Sender;
// modul tidak pernah mengimpornya (modul hanya memakai sdk/mail.Send).
//
// # Driver
//
//	NewSMTP(config.SMTPConfig, ...SMTPOption) (sdkmail.Sender, error)
//	NewLog(*slog.Logger, ...LogOption) sdkmail.Sender
//
// smtp: net/smtp + crypto/tls. Mode starttls (bawaan; STARTTLS WAJIB, tanpa downgrade: ErrSTARTTLSRequired), tls
// (implisit), none (hanya development, tanpa autentikasi). Autentikasi PLAIN atau LOGIN hanya di atas TLS yang sudah
// terverifikasi (diperiksa sendiri, tidak mengandalkan pengecualian localhost net/smtp). Sertifikat diverifikasi
// terhadap ERP_SMTP_HOST dan tidak ada opsi untuk melewatinya (WithRootCAs hanya mengganti kumpulan CA, mis. CA
// internal). Timeout sambung 10 detik dan 30 detik per percobaan; retry hanya atas kegagalan sementara (sambung gagal,
// putus, balasan 4xx), maksimum 3 percobaan dengan jeda 300 ms berlipat dua; 5xx, galat sertifikat/TLS, dan galat
// autentikasi tidak di-retry. Setiap pesan lolos sdkmail.Validate SEBELUM jaringan disentuh; subjek non-ASCII atau
// panjang di-encode RFC 2047 (base64, dilipat), isi quoted-printable, boundary acak crypto/rand. Sandi SMTP bertipe
// config.Secret dan tidak pernah masuk log, galat, atau `inspect config`.
//
// log: tidak membuka soket dan tidak mengirim apa pun. Bawaan hanya mencatat penerima tersamar ("b***@domain"),
// subjek, dan ukuran isi, sehingga tautan/token tidak pernah tertulis di log produksi. WithBody() menulis penerima dan
// isi penuh agar alur reset sandi dapat dicoba di development tanpa SMTP; perakit memasangnya HANYA bila
// ERP_ENV=development (isi surat reset setara kredensial).
//
// # Cara merakit (internal/app, C1.10/C1.12)
//
//	var sender sdkmail.Sender
//	switch cfg.MailDriver {
//	case config.MailDriverSMTP:
//		sender, err = mail.NewSMTP(cfg.SMTP, mail.WithSMTPLogger(logger))
//	default: // config.MailDriverLog
//		var opts []mail.LogOption
//		if cfg.Env == config.EnvDevelopment {
//			opts = append(opts, mail.WithBody())
//		}
//		sender = mail.NewLog(logger, opts...)
//	}
//	ctx = sdkmail.WithSender(ctx, sender) // untuk modul; layanan platform (reset) menerima sender langsung
//
// Catatan: ERP_MAIL_DRIVER bawaan adalah log hanya di development. Di production `serve` GAGAL START bila variabel
// tidak diisi (config.Config.RequireMailDriver; keputusan pemilik produk 3 Nov 2026): isi smtp, atau log secara eksplisit
// bila memang tidak mengirim surat. Perakit (C1.10/C1.12) memanggil cfg.RequireMailDriver() sebelum merakit pengirim.
package mail
