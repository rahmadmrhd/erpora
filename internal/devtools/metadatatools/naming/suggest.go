package naming

// SuggestCamel mengubah huruf kapital berurutan menjadi kata biasa (taxID menjadi taxId, orderIDNumber menjadi
// orderIdNumber) agar nama memenuhi camelCase ketat Requirement 1.4. Nama yang sudah sah dikembalikan apa adanya.
func SuggestCamel(s string) string {
	b := []byte(s)
	for i := 1; i < len(b); i++ {
		if isUpper(b[i]) && isUpper(b[i-1]) {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}
