// Package contract adalah port publik modul stock-opname untuk modul lain; tidak mengimpor internal/.
//
// Tambahkan antarmuka lewat `erpora make:contract stock-opname <Nama>`. Implementasi kontrak wajib menegakkan otorisasi sendiri
// terhadap principal di context, dan parameter kontrak tidak boleh membawa tenantId atau userId.
package contract
