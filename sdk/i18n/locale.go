package i18n

// Locale adalah kode bahasa terdaftar di registri bahasa platform (mis. "id", "en-GB"). Paket ini tidak
// memvalidasi nilainya: registri (internal/platform/i18n) memastikan Locale yang sampai ke sini
// anggota registri. Locale yang tidak dikenal x/text diperlakukan sebagai bahasa tak tentu (und):
// kategori plural selalu other dan angka memakai pemformatan bawaan.
type Locale string
