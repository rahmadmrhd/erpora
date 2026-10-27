// Package apicontract memodelkan kontrak OpenAPI 3.1 milik platform dan modul (PRD-000 FR-23): parsing YAML,
// rujukan lintas dokumen `erp:<doc>#/…`, daftar operasi, klasifikasi respons, bundel mandiri, penggabungan
// bernamespace, validasi respons, dan pencocokan route terdaftar dengan operasi kontrak.
//
// Model ditulis sendiri (bukan kin-openapi) karena kontrak memakai OpenAPI 3.1 dan skema URI `erp:` yang tidak
// dikenal library umum, sedangkan kebutuhannya kecil. Paket ini tidak boleh mengimpor registry atau modul.
package apicontract
