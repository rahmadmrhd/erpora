package i18n

import "golang.org/x/text/language"

// maxAcceptLanguageLen adalah panjang maksimal header Accept-Language yang diproses; yang lebih panjang
// diabaikan (Requirement 3.4; mencegah pekerjaan parse berlebih).
const maxAcceptLanguageLen = 1024

// Negotiate memilih bahasa registri untuk nilai header Accept-Language memakai language.Matcher. Hasilnya
// selalu anggota registri (P9): header kosong, terlalu panjang, rusak, atau tanpa kecocokan menghasilkan
// bahasa default. Nilai header tidak pernah dipakai sebagai path berkas (Requirement 3.4).
func (r *Registry) Negotiate(header string) string {
	if header == "" || len(header) > maxAcceptLanguageLen {
		return r.Default
	}
	tags, _, err := language.ParseAcceptLanguage(header)
	if err != nil || len(tags) == 0 {
		return r.Default
	}
	_, idx, conf := r.matcher.Match(tags...)
	if conf == language.No || idx < 0 || idx >= len(r.order) {
		return r.Default
	}
	return r.order[idx].Code
}
