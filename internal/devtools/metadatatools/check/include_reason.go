package check

import "strings"

// checkIncludeSetReasons menegakkan bahwa setiap entri include yang memakai set (pelonggaran atas properti
// turunan) menyatakan reason tertulis, tidak kosong setelah trim (keputusan pemilik produk, 2 Nov 2026;
// Requirement 3.6 dan 14.5). Alasan tercatat di model resolver (Resolved.IncludeSets); daftar pelonggaran beserta reason dicetak oleh metadata:check (commands.writeIncludeRelaxations).
func (x *rctx) checkIncludeSetReasons() {
	for _, is := range x.r.IncludeSets {
		if strings.TrimSpace(is.Reason) != "" {
			continue
		}
		x.atOrigin(SeverityError, is.Origin, "/include", CodeIncludeSetMissingReason,
			"entri include %s memakai set (%s) tanpa reason: pelonggaran atas properti turunan wajib beralasan tertulis (kunci reason, tidak kosong)",
			is.Source, joinNames(is.Fields))
	}
}
