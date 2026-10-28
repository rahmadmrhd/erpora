// Package i18n adalah inti sistem terjemahan ERPora (PRD-000 FR-25): format pesan ICU
// MessageFormat (argumen, plural CLDR, select, tag rich), pembacaan katalog JSON bersarang
// (ParseCatalog), dan penerjemah terikat context (T). Paket ini tidak melakukan I/O selain pada
// io.Reader yang diberikan pemanggil dan tidak mengimpor internal/ (D5/D9); katalog, registri bahasa,
// dan override dikelola platform yang menyuntikkan Resolver ke context.
//
// Mesin ICU sengaja disembunyikan di balik Compile dan Message supaya dapat diganti tanpa mengubah
// pemanggil (design.md D-2). Perilaku mengikuti intl-messageformat (FormatJS) yang dipakai frontend;
// kesamaannya dijaga fixture testdata/conformance.json (Requirement 9).
package i18n
