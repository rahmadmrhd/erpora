package identity_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"erpora/sdk/identity"
)

// MFAVerified diturunkan dari AMR (bukan sumber kebenaran kedua yang bisa tak sinkron).
func TestPrincipalMFAVerifiedDerivedFromAMR(t *testing.T) {
	cases := []struct {
		amr  []string
		want bool
	}{
		{nil, false},
		{[]string{identity.AMRPassword}, false},
		{[]string{identity.AMRPassword, identity.AMROTP}, true},
		{[]string{identity.AMROTP}, true},
	}
	for _, c := range cases {
		if got := (identity.Principal{AMR: c.amr}).MFAVerified(); got != c.want {
			t.Errorf("AMR=%v MFAVerified=%v, want %v", c.amr, got, c.want)
		}
	}
	if _, ok := reflect.TypeOf(identity.Principal{}).FieldByName("MFAVerified"); ok {
		t.Error("Principal tidak boleh punya field MFAVerified")
	}
}

func TestKeyInfoStatus(t *testing.T) {
	if identity.KeyStatusPending != "pending" || identity.KeyStatusActive != "active" ||
		identity.KeyStatusRetiring != "retiring" || identity.KeyStatusRetired != "retired" {
		t.Fatal("nilai KeyStatus berubah")
	}
	at := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	raw, err := json.Marshal(identity.KeyInfo{KeyID: "k1", Algorithm: "EdDSA", Status: identity.KeyStatusPending, CreatedAt: at, ActivatesAt: at.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, w := range []string{`"status":"pending"`, `"activatesAt":"2026-11-03T11:00:00Z"`, `"keyId":"k1"`} {
		if !strings.Contains(s, w) {
			t.Errorf("JSON tanpa %s: %s", w, s)
		}
	}
}
