// Package metadata memuat evaluator kecil untuk model kondisi, ekspresi, templat label, aritmetika desimal,
// dan validator subset `pattern` dari sistem metadata resource (spec resource-metadata, Requirement 2, 4, 5, 19).
//
// Paket ini adalah daun SDK: tidak mengimpor platform maupun modul. Kode bangkitan menanam AST sebagai
// data bertipe (Condition, Expr) dan memanggil evaluator ini; tidak ada berkas metadata yang ditafsirkan
// saat runtime (design D-2). Evaluator TypeScript di @erpora/platform-web (src/metadata) berperilaku
// identik, dibuktikan oleh korpus vektor bersama contracts/metadata-vectors dan property test paritas.
//
// # Semantik
//
// Semua evaluasi adalah fungsi total: input apa pun menghasilkan hasil, tidak pernah panik, dan berbiaya
// linear terhadap jumlah simpul (batas: MaxConditionNodes, MaxConditionDepth, MaxExprNodes, MaxInValues,
// MaxStringLen). Tipe tak cocok, field tak ada, atau operator tak didukung menghasilkan hasil "tak tentu";
// pada kondisi, hasil tak tentu menjadi false dan disertai Reason. Logika tiga-nilai (Kleene) dipakai di grup:
// all/any boleh tetap pasti bila simpul lain menentukan, dan not tidak pernah mengubah tak tentu menjadi true.
//
// Tipe field menentukan penafsiran nilai: decimal selalu string desimal (tidak pernah float), integer adalah
// bilangan bulat aman (|n| <= 2^53-1), date YYYY-MM-DD, datetime RFC 3339 UTC (akhiran Z, pecahan <= 9 digit),
// uuid huruf kecil kanonik. Teks dibandingkan berurutan menurut code point (identik dengan urutan byte UTF-8);
// "contains" tidak peka huruf besar-kecil hanya untuk huruf ASCII.
//
// # Semantik null (acuan bagi penerjemah SQL, task T2)
//
// null hanya sama dengan null (eq aman-null; ne adalah negasinya, jadi ne atas null bernilai benar); gt, gte, lt,
// lte, contains, dan in atas null bernilai salah pasti (bukan tak tentu); isnull tanpa nilai berarti isnull true.
// Penerjemah SQL (kolom stored, CHECK, pemicu lockWhen) harus mempertahankannya (IS [NOT] DISTINCT FROM dan
// COALESCE(..., false)) karena logika tiga-nilai SQL berbeda.
package metadata
