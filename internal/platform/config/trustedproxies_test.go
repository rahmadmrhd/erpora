package config

import (
	"fmt"
	"strings"
	"testing"
)

func proxiesEnv(t *testing.T, value string) {
	t.Helper()
	mailEnv(t, "development", map[string]string{"ERP_TRUSTED_PROXIES": value})
}

func TestLoad_TrustedProxiesDefaultEmpty(t *testing.T) {
	proxiesEnv(t, "")
	c, err := Load("")
	if err != nil || len(c.TrustedProxies) != 0 {
		t.Fatalf("default = %v, %v", c.TrustedProxies, err)
	}
}

func TestLoad_TrustedProxiesParses(t *testing.T) {
	proxiesEnv(t, " 10.0.0.0/8 , 192.0.2.5,2001:db8::/32, ::1 ,10.1.2.3/24")
	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range c.TrustedProxies {
		got = append(got, p.String())
	}
	// IP tunggal menjadi /32 atau /128; CIDR dinormalisasi ke alamat jaringan.
	want := "10.0.0.0/8 192.0.2.5/32 2001:db8::/32 ::1/128 10.1.2.0/24"
	if strings.Join(got, " ") != want {
		t.Fatalf("TrustedProxies = %v, want %s", got, want)
	}
}

func TestLoad_TrustedProxiesRejectsInvalid(t *testing.T) {
	many := make([]string, MaxTrustedProxies+1)
	for i := range many {
		many[i] = fmt.Sprintf("10.0.%d.1", i)
	}
	cases := map[string]string{
		"bukan ip":              "bukan-ip",
		"cidr cacat":            "10.0.0.0/33",
		"prefiks nol IPv4":      "0.0.0.0/0",
		"prefiks nol IPv6":      "::/0",
		"entri kosong":          "10.0.0.1,,10.0.0.2",
		"koma di ujung":         "10.0.0.1,",
		"spasi di dalam entri":  "10.0.0. 1",
		"spasi di dalam cidr":   "10.0.0.0 /8",
		"zona IPv6":             "fe80::1%eth0",
		"hostname":              "proxy.example.com",
		"dengan port":           "10.0.0.1:8080",
		"tab di dalam entri":    "10.0.0.	1",
		"terlalu banyak entri":  strings.Join(many, ","),
		"pemisah titik koma":    "10.0.0.1;10.0.0.2",
		"hanya spasi di antara": "10.0.0.1 10.0.0.2",
	}
	for name, v := range cases {
		t.Run(name, func(t *testing.T) {
			proxiesEnv(t, v)
			_, err := Load("")
			ve, ok := err.(ValidationError)
			if !ok || ve.Var != "ERP_TRUSTED_PROXIES" {
				t.Fatalf("err = %v, want ValidationError ERP_TRUSTED_PROXIES", err)
			}
		})
	}
}

func TestLoad_TrustedProxiesMaxAccepted(t *testing.T) {
	ok := make([]string, MaxTrustedProxies)
	for i := range ok {
		ok[i] = fmt.Sprintf("10.0.%d.1", i)
	}
	proxiesEnv(t, strings.Join(ok, ","))
	if c, err := Load(""); err != nil || len(c.TrustedProxies) != MaxTrustedProxies {
		t.Fatalf("batas maksimum harus diterima: %d, %v", len(c.TrustedProxies), err)
	}
}

// NUL tidak dapat dipasang lewat environment; periksa penguraian langsung.
func TestLoadTrustedProxies_RejectsControlCharacter(t *testing.T) {
	var c Config
	if err := loadTrustedProxies(&c, "10.0.0.1"+string(rune(0))); err == nil {
		t.Fatal("karakter kontrol harus ditolak")
	}
	if err := loadTrustedProxies(&c, "::ffff:0.0.0.0/96"); err == nil {
		t.Fatal("IPv4-mapped /96 (setara /0) harus ditolak")
	}
}

func TestLoadTrustedProxies_ErrorDoesNotEchoInput(t *testing.T) {
	var c Config
	err := loadTrustedProxies(&c, "rahasia-bukan-ip")
	if err == nil || strings.Contains(err.Error(), "rahasia-bukan-ip") {
		t.Fatalf("pesan galat tidak boleh mengulang masukan: %v", err)
	}
}
