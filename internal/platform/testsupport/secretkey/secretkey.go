// Package secretkey menyediakan ERP_SECRET_KEY untuk tes: satu kunci acak 32 byte per proses uji, dibangkitkan
// dari crypto/rand saat pertama dipakai. Satu-satunya tempat kunci uji dibuat; jangan memakai kunci tetap
// (mis. semua-nol) di tes atau CI. Kunci tidak pernah ditulis ke disk atau log.
package secretkey

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"testing"
)

// EnvVar adalah nama variabel environment yang diisi helper ini.
const EnvVar = "ERP_SECRET_KEY"

var (
	once sync.Once
	key  []byte
)

func gen() {
	key = make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic("secretkey: crypto/rand gagal: " + err.Error())
	}
}

// Bytes mengembalikan salinan kunci uji proses ini (32 byte).
func Bytes() []byte {
	once.Do(gen)
	return append([]byte(nil), key...)
}

// B64 mengembalikan kunci uji proses ini dalam base64 standar (bentuk ERP_SECRET_KEY).
func B64() string { return base64.StdEncoding.EncodeToString(Bytes()) }

// Env mengembalikan "ERP_SECRET_KEY=<kunci>" untuk ditambahkan ke environment subproses.
func Env() string { return EnvVar + "=" + B64() }

// Setenv memasang kunci uji proses ini pada environment tes (dipulihkan otomatis oleh testing).
func Setenv(t testing.TB) {
	t.Helper()
	t.Setenv(EnvVar, B64())
}
