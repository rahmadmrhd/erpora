package check

import (
	"regexp"
	"strings"
)

// Nama sensitif (Requirement 2.7): sama dengan aturan Spectral erp-sensitive-needs-classification
// (contracts/spectral-functions/classification.ts). Semua istilah ini wajib highly_confidential, yang belum
// didukung sistem metadata (K17), sehingga ditolak.
var (
	sensitiveTokens = map[string]bool{"password": true, "passwd": true, "secret": true, "token": true, "nik": true, "npwp": true, "salary": true}
	sensitivePairs  = map[string]bool{"api key": true, "private key": true, "bank account": true, "access token": true, "refresh token": true}

	reLowerUpper = regexp.MustCompile(`([a-z0-9])([A-Z])`)
	reAcronym    = regexp.MustCompile(`([A-Z]+)([A-Z][a-z])`)
	reSplit      = regexp.MustCompile(`[^A-Za-z0-9]+`)
)

func nameTokens(name string) []string {
	s := reLowerUpper.ReplaceAllString(name, "$1 $2")
	s = reAcronym.ReplaceAllString(s, "$1 $2")
	var out []string
	for _, t := range reSplit.Split(s, -1) {
		// pemisahan spasi dari penggantian di atas juga memecah token
		for _, p := range strings.Fields(t) {
			out = append(out, strings.ToLower(p))
		}
	}
	return out
}

// IsSensitiveName menyatakan nama properti tampak sensitif (per token, pasangan token berurutan, dan bentuk
// gabungan istilah dua kata seperti apikey atau bankaccount).
func IsSensitiveName(name string) bool {
	toks := nameTokens(name)
	var joined []string
	for p := range sensitivePairs {
		joined = append(joined, strings.ReplaceAll(p, " ", ""))
	}
	for i, tok := range toks {
		if sensitiveTokens[tok] {
			return true
		}
		if i+1 < len(toks) && sensitivePairs[tok+" "+toks[i+1]] {
			return true
		}
		for _, j := range joined {
			if strings.Contains(tok, j) {
				return true
			}
		}
	}
	return false
}
