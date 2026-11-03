package config

import (
	"net"
	"strconv"
	"strings"
	"unicode"

	sdkmail "erpora/sdk/mail"
)

// Driver surat dan mode TLS SMTP (FR-108, K-C1.6).
const (
	MailDriverLog  = "log"
	MailDriverSMTP = "smtp"

	SMTPTLSStartTLS = "starttls" // sambungan polos lalu STARTTLS wajib (tanpa downgrade)
	SMTPTLSImplicit = "tls"      // TLS implisit sejak sambungan (biasanya port 465)
	SMTPTLSNone     = "none"     // tanpa TLS: hanya development, tanpa autentikasi
)

// SMTPConfig adalah konfigurasi driver surat smtp (env sementara K-C1.6 sampai Settings C3). Password bertipe Secret
// sehingga tersamar di fmt, JSON, dan slog.
type SMTPConfig struct {
	Host     string // ERP_SMTP_HOST: nama host tanpa skema/port
	Port     int    // ERP_SMTP_PORT: bawaan 587 (starttls), 465 (tls), 25 (none)
	Username string // ERP_SMTP_USERNAME: kosong = tanpa autentikasi
	Password Secret // ERP_SMTP_PASSWORD: tidak dipotong spasinya
	From     string // ERP_SMTP_FROM: alamat polos pengirim (aturan ketat sdk/mail)
	TLSMode  string // ERP_SMTP_TLS_MODE: starttls (bawaan) | tls | none (hanya development)
}

// mailRaw adalah nilai mentah variabel surat dan URL publik.
type mailRaw struct {
	Driver, Host, Port, Username, Password, From, TLSMode string
}

// loadMail mengisi c.MailDriver dan c.SMTP. Format selalu divalidasi bila terisi; kelengkapan (host, from) hanya
// diwajibkan bila driver smtp, sehingga migrate, inspect, dan doctor tetap berjalan tanpa konfigurasi surat.
func loadMail(c *Config, r mailRaw) error {
	driver := strings.ToLower(strings.TrimSpace(r.Driver))
	explicit := driver != ""
	if driver == "" {
		driver = MailDriverLog
	}
	if driver != MailDriverLog && driver != MailDriverSMTP {
		return ValidationError{"ERP_MAIL_DRIVER", "harus log atau smtp"}
	}
	mode := strings.ToLower(strings.TrimSpace(r.TLSMode))
	if mode == "" {
		mode = SMTPTLSStartTLS
	}
	switch mode {
	case SMTPTLSStartTLS, SMTPTLSImplicit, SMTPTLSNone:
	default:
		return ValidationError{"ERP_SMTP_TLS_MODE", "harus starttls, tls, atau none"}
	}
	port := 0
	if p := strings.TrimSpace(r.Port); p != "" {
		v, err := strconv.ParseUint(p, 10, 16)
		if err != nil || v < 1 {
			return ValidationError{"ERP_SMTP_PORT", "harus bilangan bulat 1..65535"}
		}
		port = int(v)
	} else {
		port = map[string]int{SMTPTLSStartTLS: 587, SMTPTLSImplicit: 465, SMTPTLSNone: 25}[mode]
	}
	host := strings.TrimSpace(r.Host)
	if host != "" && !validSMTPHost(host) {
		return ValidationError{"ERP_SMTP_HOST", "harus nama host atau alamat IP tanpa skema, port, atau karakter khusus"}
	}
	if hasCtl(r.Username) {
		return ValidationError{"ERP_SMTP_USERNAME", "tidak boleh memuat karakter kontrol"}
	}
	if (r.Username == "") != (r.Password == "") {
		if r.Username == "" {
			return ValidationError{"ERP_SMTP_USERNAME", "wajib diisi bila ERP_SMTP_PASSWORD diisi"}
		}
		return ValidationError{"ERP_SMTP_PASSWORD", "wajib diisi bila ERP_SMTP_USERNAME diisi"}
	}
	from := strings.TrimSpace(r.From)
	if from != "" {
		if err := sdkmail.Validate(sdkmail.Message{To: []string{from}, Subject: "x", Text: "x"}); err != nil {
			return ValidationError{"ERP_SMTP_FROM", "bukan alamat email polos yang sah (tanpa nama tampilan, sudut, atau karakter kontrol)"}
		}
	}
	if mode == SMTPTLSNone {
		if c.Env == EnvProduction {
			return ValidationError{"ERP_SMTP_TLS_MODE", "none ditolak saat ERP_ENV=production: gunakan starttls atau tls"}
		}
		if r.Username != "" {
			return ValidationError{"ERP_SMTP_TLS_MODE", "none tidak boleh dipakai bersama ERP_SMTP_USERNAME: kredensial tidak pernah dikirim tanpa TLS"}
		}
	}
	if driver == MailDriverSMTP {
		if host == "" {
			return ValidationError{"ERP_SMTP_HOST", "wajib diisi bila ERP_MAIL_DRIVER=smtp"}
		}
		if from == "" {
			return ValidationError{"ERP_SMTP_FROM", "wajib diisi bila ERP_MAIL_DRIVER=smtp"}
		}
	}
	c.MailDriver, c.MailDriverExplicit = driver, explicit
	c.SMTP = SMTPConfig{Host: host, Port: port, Username: r.Username, Password: Secret(r.Password), From: from, TLSMode: mode}
	return nil
}

func hasCtl(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return unicode.IsControl(r) }) >= 0
}

// validSMTPHost menerima nama host LDH atau alamat IP; menolak skema, port, jalur, spasi, dan karakter kontrol.
func validSMTPHost(h string) bool {
	if len(h) > 253 {
		return false
	}
	if net.ParseIP(h) != nil {
		return true
	}
	for _, label := range strings.Split(h, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			ch := label[i]
			if (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') && (ch < '0' || ch > '9') && ch != '-' {
				return false
			}
		}
	}
	return true
}

// RequireMailDriver memastikan konfigurasi surat layak untuk `serve` (fail-closed; keputusan pemilik produk 3 Nov 2026):
// di production driver log sah HANYA bila operator memilihnya eksplisit (ERP_MAIL_DRIVER=log); bila ERP_MAIL_DRIVER
// tidak diisi, serve tidak boleh start, sebab surat (mis. tautan reset sandi) tidak akan pernah terkirim dan kegagalannya
// diam. Driver smtp sudah divalidasi lengkap oleh Load. Di development driver log selalu sah. Hanya `serve` yang
// memanggilnya: migrate, inspect, doctor, modules, dan i18n tetap berjalan tanpa konfigurasi surat. Pesan tidak memuat
// nilai apa pun.
func (c Config) RequireMailDriver() error {
	if c.Env == EnvProduction && c.MailDriver == MailDriverLog && !c.MailDriverExplicit {
		return ValidationError{"ERP_MAIL_DRIVER", "wajib diisi untuk serve di production: smtp (dengan ERP_SMTP_HOST dan ERP_SMTP_FROM) agar surat terkirim, atau log bila Anda sengaja tidak mengirim surat (reset sandi lewat email tidak akan berfungsi)"}
	}
	return nil
}
