package secretkey

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestKeyIsRandom32BytesStablePerProcess(t *testing.T) {
	k := Bytes()
	if len(k) != 32 || bytes.Equal(k, make([]byte, 32)) {
		t.Fatalf("kunci harus 32 byte non-nol, len=%d", len(k))
	}
	if !bytes.Equal(k, Bytes()) {
		t.Error("kunci harus stabil dalam satu proses")
	}
	dec, err := base64.StdEncoding.DecodeString(B64())
	if err != nil || !bytes.Equal(dec, k) {
		t.Errorf("B64 harus base64 standar dari Bytes: %v", err)
	}
	k[0] ^= 0xff // mengubah salinan tidak boleh memengaruhi kunci
	if bytes.Equal(k, Bytes()) {
		t.Error("Bytes harus mengembalikan salinan")
	}
}

func TestSetenv(t *testing.T) {
	Setenv(t)
	if got := Env(); got != EnvVar+"="+B64() {
		t.Errorf("Env = %q", got)
	}
}
