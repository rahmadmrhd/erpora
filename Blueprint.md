# Blueprint Arsitektur ERP Modular — Hybrid, Multi-Tenant, dan Extensible

Dokumen rancangan teknis · Versi 1.0 · 20 Oktober 2026

Blueprint ini dirancang untuk ERP yang akan dikembangkan sebagai platform jangka panjang, dengan pengalaman pengguna modern, modul yang dapat diaktifkan atau dinonaktifkan, dukungan instalasi SaaS maupun self-hosted, dan ekosistem ekstensi publik.

Saya akan memperlakukan sistem ini bukan sekadar aplikasi ERP, melainkan ERP Platform: sebuah fondasi yang menyediakan identitas, tenant management, module runtime, authorization, konfigurasi, event system, migrasi, dan mekanisme ekstensi bagi modul-modul bisnis.

Keputusan arsitektur harus mendukung dua tujuan sekaligus:

1. Kemudahan pengembangan saat ini: satu tim dapat membangun dan menguji ERP tanpa mengelola banyak microservices.
2. Kemampuan berkembang di masa depan: perusahaan dapat memiliki database terisolasi, modul dapat berkembang secara independen, dan ekstensi pihak ketiga dapat diinstal dengan batas keamanan yang jelas.

# 1. Ringkasan keputusan arsitektur

Arsitektur utama: Modular Monolith

Satu aplikasi backend untuk core dan modul bisnis, dengan batas modul yang ditegakkan secara teknis. Tidak menggunakan microservices sejak awal.

Arsitektur internal: Clean Architecture selektif

Domain, Application, Infrastructure, dan Presentation dipisahkan pada modul yang memiliki aturan bisnis penting.

Extensibility: Manifest + Module Registry + Public SDK

Modul terdaftar secara eksplisit, memiliki dependensi, kontrak, versi, permission, dan lifecycle yang dapat divalidasi.

Multi-tenancy: Database per tenant

Control plane menyimpan metadata platform; setiap tenant mempunyai database bisnis sendiri, dengan kredensial dan proses provisioning yang terisolasi.

Integrasi: Contracts + Domain Events + Transactional Outbox

Pemanggilan langsung untuk operasi yang membutuhkan konsistensi segera; event untuk proses lanjutan dan integrasi.

## Keputusan teknologi awal

| Area             | Rekomendasi                                                                                          |
| ---------------- | ---------------------------------------------------------------------------------------------------- |
| Backend          | PHP + Laravel                                                                                        |
| Frontend         | React + TypeScript                                                                                   |
| Integrasi UI     | Inertia.js                                                                                           |
| UI system        | Tailwind CSS + shadcn/ui                                                                             |
| Database awal    | PostgreSQL sebagai kandidat utama; MySQL tetap layak                                                 |
| Cache dan queue  | Redis                                                                                                |
| File dan dokumen | S3-compatible object storage                                                                         |
| Testing          | Pest/PHPUnit, React Testing Library, browser E2E                                                     |
| CI/CD            | GitHub Actions                                                                                       |
| Observability    | Centralized logs, metrics, error tracking, tracing                                                   |
| Deployment       | Containerized deployment untuk SaaS dan self-hosted                                                  |
| Modul publik     | Registry, package distribution, signed releases, sandboxed execution untuk kode yang tidak tepercaya |

Rekomendasi PostgreSQL masih berupa keputusan awal, bukan keharusan. Kita akan menentukan pilihan final berdasarkan kebutuhan akuntansi, concurrency, reporting, tooling, serta biaya operasional. Karena database-per-tenant sudah dipilih, arsitektur sebaiknya tidak membuat business logic bergantung pada fitur spesifik satu vendor tanpa alasan yang kuat.

Batas penting: database-per-tenant mengisolasi data antarperusahaan, tetapi tidak otomatis menjamin keamanan tenant. Pemilihan tenant, credential database, cache key, file storage, queue, dan proses background tetap harus menggunakan konteks tenant yang benar.

# 2. Gambaran arsitektur sistem

Arsitektur harus memisahkan tiga area yang memiliki siklus hidup dan kebutuhan keamanan berbeda:

1. Control Plane: mengelola organisasi, tenant, database, lisensi, instalasi, dan katalog modul.
2. ERP Runtime: menjalankan transaksi bisnis pada konteks tenant tertentu.
3. Extension Platform: mendistribusikan dan mengelola modul dari tim internal maupun pengembang pihak ketiga.

## 2.1. Diagram arsitektur tingkat tinggi

## Clients

Web browser · React · TypeScript · Inertia

### Application Edge

TLS · Host routing · Authentication · Rate limiting

### Control Plane

Tenant registry, provisioning, module catalog, licensing

### ERP Runtime

Tenant context, module registry, use cases, domain rules

Control Database

Tenant metadata dan katalog platform

Tenant Databases

Data bisnis dan konfigurasi tenant

Platform Services

Queue workers · Event outbox · Object storage · Observability · Package registry

Diagram tersebut menunjukkan batas logis, bukan keharusan membuat setiap kotak menjadi aplikasi atau service terpisah. Pada fase awal, Control Plane dan ERP Runtime dapat berada dalam satu codebase Laravel, tetapi modul, otorisasi, dan koneksi datanya tetap memiliki batas yang jelas.

### Alur request

Contoh: pengguna membuka halaman Purchase Orders.

1. Request masuk melalui domain tenant atau URL yang sudah terdaftar.
2. Middleware mengidentifikasi tenant berdasarkan pemetaan yang terpercaya.
3. Sistem memverifikasi bahwa pengguna memiliki akses ke tenant tersebut.
4. Tenant context diinisialisasi, termasuk koneksi database, konfigurasi, dan modul yang aktif.
5. Backend memeriksa permission serta status modul.
6. Controller meneruskan permintaan ke use case Purchasing.
7. Use case menjalankan aturan bisnis dan mengakses data melalui kontrak persistence.
8. Inertia mengembalikan data halaman kepada React.

Jangan mengambil nama database langsung dari parameter URL, header yang tidak tervalidasi, atau input pengguna. Resolusi tenant harus menggunakan registry yang dipercaya.

# 3. Database-per-tenant: desain dan konsekuensinya

Kamu memilih database terpisah untuk setiap tenant. Saya setuju dengan pilihan ini jika isolasi data, kemudahan backup/restore per perusahaan, dan fleksibilitas self-hosted menjadi prioritas.

Namun, pendekatan ini juga memengaruhi migrasi, koneksi database, background jobs, pelaporan lintas perusahaan, dan biaya operasional.

## 3.1. Control database dan tenant database

Pisahkan metadata platform dari transaksi bisnis.

## Control Database

Dikelola oleh platform, bukan oleh modul bisnis.

`tenants`

`tenant_databases`

`users`

`tenant_memberships`

`module_catalog`

`tenant_modules`

## Tenant Database

Satu database bisnis untuk setiap tenant.

`companies`

`roles_permissions`

`items_warehouses`

`sales_purchasing`

`journal_entries`

`audit_outbox_jobs`

### Mengapa `users` bisa berada di Control Database?

Untuk SaaS, pengguna dapat memiliki akun platform dan bergabung ke beberapa tenant. Identitas pengguna disimpan terpusat, sedangkan peran dan hak akses bisnis dapat ditetapkan per tenant.

Pada self-hosted, instalasi dapat memakai deployment mandiri dengan control database lokal. Jika satu instalasi hanya melayani satu perusahaan, model ini tetap dapat digunakan, walaupun sebagian komponen provisioning mungkin disederhanakan.

Jangan menjadikan Control Database sebagai tempat semua transaksi bisnis. Ia harus tetap menjadi metadata plane, bukan sumber kebenaran untuk saldo stok, invoice, atau jurnal.

## 3.2. ERD inti

Berikut ERD konseptual untuk control database. Ini belum mencakup seluruh tabel transaksi ERP.

\#chatgpt-mermaid-\_r_t2\_{font-family:-apple-system-body,ui-sans-serif,-apple-system,system-ui,"Segoe UI",Helvetica,"Apple Color Emoji",Arial,sans-serif,"Segoe UI Emoji","Segoe UI Symbol";font-size:16px;fill:rgb(237, 237, 237);}@keyframes edge-animation-frame{from{stroke-dashoffset:0;}}@keyframes dash{to{stroke-dashoffset:0;}}#chatgpt-mermaid-\_r_t2\_ .edge-animation-slow{stroke-dasharray:9,5!important;stroke-dashoffset:900;animation:dash 50s linear infinite;stroke-linecap:round;}#chatgpt-mermaid-\_r_t2\_ .edge-animation-fast{stroke-dasharray:9,5!important;stroke-dashoffset:900;animation:dash 20s linear infinite;stroke-linecap:round;}#chatgpt-mermaid-\_r_t2\_ .error-icon{fill:rgb(48, 48, 48);}#chatgpt-mermaid-\_r_t2\_ .error-text{fill:rgb(237, 237, 237);stroke:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_t2\_ .edge-thickness-normal{stroke-width:1px;}#chatgpt-mermaid-\_r_t2\_ .edge-thickness-thick{stroke-width:3.5px;}#chatgpt-mermaid-\_r_t2\_ .edge-pattern-solid{stroke-dasharray:0;}#chatgpt-mermaid-\_r_t2\_ .edge-thickness-invisible{stroke-width:0;fill:none;}#chatgpt-mermaid-\_r_t2\_ .edge-pattern-dashed{stroke-dasharray:3;}#chatgpt-mermaid-\_r_t2\_ .edge-pattern-dotted{stroke-dasharray:2;}#chatgpt-mermaid-\_r_t2\_ .marker{fill:rgb(175, 175, 175);stroke:rgb(175, 175, 175);}#chatgpt-mermaid-\_r_t2\_ .marker.cross{stroke:rgb(175, 175, 175);}#chatgpt-mermaid-\_r_t2\_ svg{font-family:-apple-system-body,ui-sans-serif,-apple-system,system-ui,"Segoe UI",Helvetica,"Apple Color Emoji",Arial,sans-serif,"Segoe UI Emoji","Segoe UI Symbol";font-size:16px;}#chatgpt-mermaid-\_r_t2\_ p{margin:0;}#chatgpt-mermaid-\_r_t2\_ .entityBox{fill:rgb(9, 23, 44);stroke:rgb(31, 78, 148);}#chatgpt-mermaid-\_r_t2\_ .relationshipLabelBox{fill:rgb(48, 48, 48);opacity:0.7;background-color:rgb(48, 48, 48);}#chatgpt-mermaid-\_r_t2\_ .relationshipLabelBox rect{opacity:0.5;}#chatgpt-mermaid-\_r_t2\_ .labelBkg{background-color:rgba(48, 48, 48, 0.5);}#chatgpt-mermaid-\_r_t2\_ .edgeLabel{background-color:rgb(0, 0, 0);}#chatgpt-mermaid-\_r_t2\_ .edgeLabel .label rect{fill:rgb(0, 0, 0);}#chatgpt-mermaid-\_r_t2\_ .edgeLabel .label text{fill:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_t2\_ .edgeLabel .label{fill:rgb(31, 78, 148);font-size:14px;}#chatgpt-mermaid-\_r_t2\_ .label{font-family:-apple-system-body,ui-sans-serif,-apple-system,system-ui,"Segoe UI",Helvetica,"Apple Color Emoji",Arial,sans-serif,"Segoe UI Emoji","Segoe UI Symbol";color:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_t2\_ .edge-pattern-dashed{stroke-dasharray:8,8;}#chatgpt-mermaid-\_r_t2\_ .node rect,#chatgpt-mermaid-\_r_t2\_ .node circle,#chatgpt-mermaid-\_r_t2\_ .node ellipse,#chatgpt-mermaid-\_r_t2\_ .node polygon{fill:rgb(9, 23, 44);stroke:rgb(31, 78, 148);stroke-width:1px;}#chatgpt-mermaid-\_r_t2\_ .relationshipLine{stroke:rgb(175, 175, 175);stroke-width:1px;fill:none;}#chatgpt-mermaid-\_r_t2\_ .marker{fill:none!important;stroke:rgb(175, 175, 175)!important;stroke-width:1;}#chatgpt-mermaid-\_r_t2\_ [data-look=neo].labelBkg{background-color:rgba(48, 48, 48, 0.5);}#chatgpt-mermaid-\_r_t2\_ .node .neo-node{stroke:rgb(31, 78, 148);}#chatgpt-mermaid-\_r_t2\_ [data-look="neo"].node rect,#chatgpt-mermaid-\_r_t2\_ [data-look="neo"].cluster rect,#chatgpt-mermaid-\_r_t2\_ [data-look="neo"].node polygon{stroke:url(#chatgpt-mermaid-\_r_t2\_-gradient);filter:drop-shadow( 1px 2px 2px rgba(185,185,185,1));}#chatgpt-mermaid-\_r_t2\_ [data-look="neo"].swimlane.cluster rect{filter:none;}#chatgpt-mermaid-\_r_t2\_ [data-look="neo"].node path{stroke:url(#chatgpt-mermaid-\_r_t2\_-gradient);stroke-width:1px;}#chatgpt-mermaid-\_r_t2\_ [data-look="neo"].node .outer-path{filter:drop-shadow( 1px 2px 2px rgba(185,185,185,1));}#chatgpt-mermaid-\_r_t2\_ [data-look="neo"].node .neo-line path{stroke:rgb(31, 78, 148);filter:none;}#chatgpt-mermaid-\_r_t2\_ [data-look="neo"].node circle{stroke:url(#chatgpt-mermaid-\_r_t2\_-gradient);filter:drop-shadow( 1px 2px 2px rgba(185,185,185,1));}#chatgpt-mermaid-\_r_t2\_ [data-look="neo"].node circle .state-start{fill:#000000;}#chatgpt-mermaid-\_r_t2\_ [data-look="neo"].icon-shape .icon{fill:url(#chatgpt-mermaid-\_r_t2\_-gradient);filter:drop-shadow( 1px 2px 2px rgba(185,185,185,1));}#chatgpt-mermaid-\_r_t2\_ [data-look="neo"].icon-shape .icon-neo path{stroke:url(#chatgpt-mermaid-\_r_t2\_-gradient);filter:drop-shadow( 1px 2px 2px rgba(185,185,185,1));}#chatgpt-mermaid-\_r_t2\_ :root{--mermaid-font-family:-apple-system-body,ui-sans-serif,-apple-system,system-ui,"Segoe UI",Helvetica,"Apple Color Emoji",Arial,sans-serif,"Segoe UI Emoji","Segoe UI Symbol";}TENANTSuuididPKstringslugUKstringnamestringdeployment_modestringstatustimestampcreated_atTENANT_DATABASESuuididPKuuidtenant_idFKstringconnection_keystringenginestringdatabase_namestringsecret_referencestringstatusTENANT_MEMBERSHIPSuuididPKuuidtenant_idFKuuiduser_idFKstringstatusstringplatform_roleUSERSuuididPKstringemailUKstringpassword_hashstringstatusTENANT_MODULESuuididPKuuidtenant_idFKuuidmodule_idFKstringinstalled_versionstringstatusjsonsettingsMODULE_CATALOGuuididPKstringmodule_keyUKstringcurrent_versionstringpublisher_idstringtrust_statusMODULE_DEPENDENCIESuuididPKuuidmodule_idFKstringdependency_keystringversion_constraintbooleanoptionalPROVISIONING_OPERATIONSuuididPKuuidtenant_idFKstringoperationstringstatusjsonprogressTENANT_RELEASESuuididPKuuidtenant_idFKstringcore_versionstringschema_versionstringstatusownshasjoinsenablesconfigured_forparentdependencyprovisionsruns

### Catatan desain ERD

- `tenant_databases.secret_reference` harus menunjuk ke secret manager atau mekanisme secret terenkripsi, bukan menyimpan password database dalam bentuk plaintext.
- `tenant_modules` perlu unique constraint `(tenant_id, module_id)`.
- `module_catalog` idealnya menyimpan identitas publisher, versi, checksum artifact, signature metadata, dan hasil pemeriksaan keamanan melalui tabel terkait.
- `tenant_releases` berguna untuk melacak versi core dan status migrasi setiap tenant.
- `provisioning_operations` diperlukan karena pembuatan database, migrasi, dan pemasangan modul dapat berlangsung cukup lama dan gagal di tengah proses.

Jangan menyimpan seluruh kredensial koneksi database sebagai JSON biasa di tabel tenant. Gunakan identitas koneksi, referensi secret, enkripsi yang dikelola dengan benar, dan pembatasan akses.

## 3.3. ERD konseptual untuk transaksi ERP

Setiap tenant mempunyai tabel bisnisnya sendiri. Misalnya, pada tenant database terdapat:

\#chatgpt-mermaid-\_r_tb\_{font-family:-apple-system-body,ui-sans-serif,-apple-system,system-ui,"Segoe UI",Helvetica,"Apple Color Emoji",Arial,sans-serif,"Segoe UI Emoji","Segoe UI Symbol";font-size:16px;fill:rgb(237, 237, 237);}@keyframes edge-animation-frame{from{stroke-dashoffset:0;}}@keyframes dash{to{stroke-dashoffset:0;}}#chatgpt-mermaid-\_r_tb\_ .edge-animation-slow{stroke-dasharray:9,5!important;stroke-dashoffset:900;animation:dash 50s linear infinite;stroke-linecap:round;}#chatgpt-mermaid-\_r_tb\_ .edge-animation-fast{stroke-dasharray:9,5!important;stroke-dashoffset:900;animation:dash 20s linear infinite;stroke-linecap:round;}#chatgpt-mermaid-\_r_tb\_ .error-icon{fill:rgb(48, 48, 48);}#chatgpt-mermaid-\_r_tb\_ .error-text{fill:rgb(237, 237, 237);stroke:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_tb\_ .edge-thickness-normal{stroke-width:1px;}#chatgpt-mermaid-\_r_tb\_ .edge-thickness-thick{stroke-width:3.5px;}#chatgpt-mermaid-\_r_tb\_ .edge-pattern-solid{stroke-dasharray:0;}#chatgpt-mermaid-\_r_tb\_ .edge-thickness-invisible{stroke-width:0;fill:none;}#chatgpt-mermaid-\_r_tb\_ .edge-pattern-dashed{stroke-dasharray:3;}#chatgpt-mermaid-\_r_tb\_ .edge-pattern-dotted{stroke-dasharray:2;}#chatgpt-mermaid-\_r_tb\_ .marker{fill:rgb(175, 175, 175);stroke:rgb(175, 175, 175);}#chatgpt-mermaid-\_r_tb\_ .marker.cross{stroke:rgb(175, 175, 175);}#chatgpt-mermaid-\_r_tb\_ svg{font-family:-apple-system-body,ui-sans-serif,-apple-system,system-ui,"Segoe UI",Helvetica,"Apple Color Emoji",Arial,sans-serif,"Segoe UI Emoji","Segoe UI Symbol";font-size:16px;}#chatgpt-mermaid-\_r_tb\_ p{margin:0;}#chatgpt-mermaid-\_r_tb\_ .entityBox{fill:rgb(9, 23, 44);stroke:rgb(31, 78, 148);}#chatgpt-mermaid-\_r_tb\_ .relationshipLabelBox{fill:rgb(48, 48, 48);opacity:0.7;background-color:rgb(48, 48, 48);}#chatgpt-mermaid-\_r_tb\_ .relationshipLabelBox rect{opacity:0.5;}#chatgpt-mermaid-\_r_tb\_ .labelBkg{background-color:rgba(48, 48, 48, 0.5);}#chatgpt-mermaid-\_r_tb\_ .edgeLabel{background-color:rgb(0, 0, 0);}#chatgpt-mermaid-\_r_tb\_ .edgeLabel .label rect{fill:rgb(0, 0, 0);}#chatgpt-mermaid-\_r_tb\_ .edgeLabel .label text{fill:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_tb\_ .edgeLabel .label{fill:rgb(31, 78, 148);font-size:14px;}#chatgpt-mermaid-\_r_tb\_ .label{font-family:-apple-system-body,ui-sans-serif,-apple-system,system-ui,"Segoe UI",Helvetica,"Apple Color Emoji",Arial,sans-serif,"Segoe UI Emoji","Segoe UI Symbol";color:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_tb\_ .edge-pattern-dashed{stroke-dasharray:8,8;}#chatgpt-mermaid-\_r_tb\_ .node rect,#chatgpt-mermaid-\_r_tb\_ .node circle,#chatgpt-mermaid-\_r_tb\_ .node ellipse,#chatgpt-mermaid-\_r_tb\_ .node polygon{fill:rgb(9, 23, 44);stroke:rgb(31, 78, 148);stroke-width:1px;}#chatgpt-mermaid-\_r_tb\_ .relationshipLine{stroke:rgb(175, 175, 175);stroke-width:1px;fill:none;}#chatgpt-mermaid-\_r_tb\_ .marker{fill:none!important;stroke:rgb(175, 175, 175)!important;stroke-width:1;}#chatgpt-mermaid-\_r_tb\_ [data-look=neo].labelBkg{background-color:rgba(48, 48, 48, 0.5);}#chatgpt-mermaid-\_r_tb\_ .node .neo-node{stroke:rgb(31, 78, 148);}#chatgpt-mermaid-\_r_tb\_ [data-look="neo"].node rect,#chatgpt-mermaid-\_r_tb\_ [data-look="neo"].cluster rect,#chatgpt-mermaid-\_r_tb\_ [data-look="neo"].node polygon{stroke:url(#chatgpt-mermaid-\_r_tb\_-gradient);filter:drop-shadow( 1px 2px 2px rgba(185,185,185,1));}#chatgpt-mermaid-\_r_tb\_ [data-look="neo"].swimlane.cluster rect{filter:none;}#chatgpt-mermaid-\_r_tb\_ [data-look="neo"].node path{stroke:url(#chatgpt-mermaid-\_r_tb\_-gradient);stroke-width:1px;}#chatgpt-mermaid-\_r_tb\_ [data-look="neo"].node .outer-path{filter:drop-shadow( 1px 2px 2px rgba(185,185,185,1));}#chatgpt-mermaid-\_r_tb\_ [data-look="neo"].node .neo-line path{stroke:rgb(31, 78, 148);filter:none;}#chatgpt-mermaid-\_r_tb\_ [data-look="neo"].node circle{stroke:url(#chatgpt-mermaid-\_r_tb\_-gradient);filter:drop-shadow( 1px 2px 2px rgba(185,185,185,1));}#chatgpt-mermaid-\_r_tb\_ [data-look="neo"].node circle .state-start{fill:#000000;}#chatgpt-mermaid-\_r_tb\_ [data-look="neo"].icon-shape .icon{fill:url(#chatgpt-mermaid-\_r_tb\_-gradient);filter:drop-shadow( 1px 2px 2px rgba(185,185,185,1));}#chatgpt-mermaid-\_r_tb\_ [data-look="neo"].icon-shape .icon-neo path{stroke:url(#chatgpt-mermaid-\_r_tb\_-gradient);filter:drop-shadow( 1px 2px 2px rgba(185,185,185,1));}#chatgpt-mermaid-\_r_tb\_ :root{--mermaid-font-family:-apple-system-body,ui-sans-serif,-apple-system,system-ui,"Segoe UI",Helvetica,"Apple Color Emoji",Arial,sans-serif,"Segoe UI Emoji","Segoe UI Symbol";}COMPANIESuuididPKstringnamestringbase_currencyWAREHOUSESuuididPKstringcodeUKstringnameITEMSuuididPKstringskuUKstringnamestringstock_uomSTOCK_MOVEMENTSuuididPKuuiditem_idFKuuidwarehouse_idFKdecimalquantitystringsource_typeuuidsource_idtimestampposted_atSUPPLIERSPURCHASE_ORDERSuuididPKstringnumberUKstringstatusstringcurrencydecimaltotalPURCHASE_ORDER_LINESuuididPKuuidpurchase_order_idFKuuiditem_idFKdecimalquantitydecimalunit_priceGOODS_RECEIPTSuuididPKuuidpurchase_order_idFKstringstatusGOODS_RECEIPT_LINESuuididPKuuidgoods_receipt_idFKuuiditem_idFKdecimalquantity_receivedJOURNAL_ENTRIESuuididPKstringnumberUKdateposting_datestringstatusJOURNAL_LINESuuididPKuuidjournal_entry_idFKuuidaccount_idFKdecimaldebitdecimalcreditACCOUNTSuuididPKstringcodeUKstringaccount_typeownstracksrecordsreceivescontainsfulfilled_bycontainsreferencescontainsposts_toowns

Ini hanya model domain tingkat tinggi. Implementasi produksi harus menambahkan audit trail, source document references, unit conversion, warehouse dimensions, currency precision, tax details, accounting periods, reversal entries, serta aturan posting yang sesuai.

Jangan menganggap semua modul harus memiliki relasi foreign key lintas modul yang bebas. Jika data dimiliki modul lain, akseslah melalui kontrak domain atau referensi dokumen yang stabil. Foreign key tetap berguna di dalam batas kepemilikan data yang jelas.

# 4. Keputusan database: PostgreSQL atau MySQL?

Keduanya dapat digunakan oleh Laravel. Dokumentasi Laravel 13 menyediakan dukungan untuk MySQL maupun PostgreSQL.&#x20;

[image](https://www.google.com/s2/favicons?domain=https://laravel.com\&sz=32)

Laravel



Saya cenderung memilih PostgreSQL untuk proyek baru ini karena kebutuhan ERP mencakup transaksi kompleks, data terstruktur dan semi-terstruktur, serta kebutuhan pelaporan yang dapat berkembang. Namun, jika kamu jauh lebih berpengalaman dengan MySQL atau deployment target sudah mengoptimalkannya, MySQL tetap merupakan pilihan yang valid.

| Kriteria                    | PostgreSQL                      | MySQL                           |
| --------------------------- | ------------------------------- | ------------------------------- |
| Transaksi dan concurrency   | Sangat kuat                     | Sangat kuat                     |
| Constraint dan tipe data    | Kaya                            | Memadai dan terus berkembang    |
| JSON dan query kompleks     | Sangat baik                     | Mendukung JSON                  |
| Ekosistem hosting           | Luas                            | Sangat luas                     |
| Keahlian tim                | Harus dinilai                   | Harus dinilai                   |
| Portabilitas business logic | Bergantung pada desain aplikasi | Bergantung pada desain aplikasi |

Yang lebih penting daripada perbedaan fitur adalah menghindari ketergantungan vendor yang tidak diperlukan. Buat kontrak persistence dan uji query yang benar-benar penting pada database target.

Jika menggunakan PostgreSQL, jangan menggunakan schema-per-tenant sebagai pengganti database-per-tenant tanpa alasan khusus. PostgreSQL sendiri menjelaskan bahwa database adalah batas yang berbeda dari schema, dan koneksi terhubung ke satu database pada satu waktu.&#x20;

[image](https://www.google.com/s2/favicons?domain=https://www.postgresql.org\&sz=32)

PostgreSQL



Untuk target hybrid, tetapkan satu database engine sebagai standar utama pada versi pertama. Mendukung MySQL dan PostgreSQL sekaligus sejak awal akan memperbesar beban testing, migration, query compatibility, dan dukungan instalasi self-hosted.

# 5. Struktur folder final

Struktur berikut menjadi usulan struktur repository utama. Tujuannya bukan membuat folder sebanyak mungkin, melainkan memisahkan core platform, modul bisnis, kontrak publik, dan infrastruktur deployment.

```
erp-platform/
├── app/
│   ├── Core/
│   │   ├── Identity/
│   │   ├── Tenancy/
│   │   ├── Authorization/
│   │   ├── Modules/
│   │   │   ├── Contracts/
│   │   │   ├── Registry/
│   │   │   ├── Lifecycle/
│   │   │   ├── DependencyResolver/
│   │   │   └── Compatibility/
│   │   ├── Settings/
│   │   ├── Events/
│   │   ├── Audit/
│   │   ├── Files/
│   │   └── Observability/
│   │
│   ├── Modules/
│   │   ├── Inventory/
│   │   ├── Purchasing/
│   │   ├── Sales/
│   │   ├── Finance/
│   │   └── Assets/
│   │
│   └── Http/
│       └── Middleware/
│
├── packages/
│   ├── platform-contracts/
│   ├── module-sdk/
│   └── ui-contracts/
│
├── resources/
│   ├── js/
│   │   ├── app.tsx
│   │   ├── layouts/
│   │   ├── components/
│   │   ├── features/
│   │   └── pages/
│   └── css/
│
├── database/
│   ├── migrations/
│   │   ├── control/
│   │   └── tenant/
│   └── factories/
│
├── routes/
│   ├── web.php
│   └── api.php
│
├── tests/
│   ├── Architecture/
│   ├── Feature/
│   ├── Integration/
│   ├── Security/
│   └── E2E/
│
├── infrastructure/
│   ├── docker/
│   ├── deployment/
│   ├── monitoring/
│   └── backup/
│
├── docs/
│   ├── architecture/
│   ├── adr/
│   ├── module-sdk/
│   ├── security/
│   └── operations/
│
├── composer.json
├── package.json
├── pnpm-lock.yaml
└── README.md
```

## 5.1. Struktur internal setiap modul

Setiap modul bisnis menggunakan pola yang konsisten, tetapi boleh disederhanakan sesuai kompleksitas domain.

```
app/Modules/Purchasing/
├── module.json
├── Providers/
│   └── PurchasingServiceProvider.php
│
├── Domain/
│   ├── Entities/
│   ├── ValueObjects/
│   ├── Events/
│   ├── Exceptions/
│   ├── Policies/
│   └── Repositories/
│
├── Application/
│   ├── Commands/
│   ├── Queries/
│   ├── DTOs/
│   ├── Handlers/
│   └── Contracts/
│
├── Infrastructure/
│   ├── Persistence/
│   │   ├── Eloquent/
│   │   └── Repositories/
│   ├── Integrations/
│   └── Messaging/
│
├── Presentation/
│   ├── Http/
│   │   ├── Controllers/
│   │   ├── Requests/
│   │   └── Resources/
│   └── Routes/
│
├── Database/
│   ├── Migrations/
│   └── Seeders/
│
├── Resources/
│   ├── js/
│   └── lang/
│
└── Tests/
    ├── Unit/
    ├── Feature/
    ├── Integration/
    └── Contract/
```

### Aturan dependensi

Tetapkan aturan ini sebagai standar arsitektur:

- `Domain` tidak mengimpor Laravel, Eloquent, HTTP, atau React.
- `Application` bergantung pada domain dan kontrak yang dibutuhkan use case.
- `Infrastructure` mengimplementasikan kontrak persistence dan integrasi.
- `Presentation` menerima request dan memanggil use case.
- Modul bisnis lain tidak mengakses implementasi privat modul Purchasing secara langsung.
- `Core` tidak boleh bergantung pada modul bisnis tertentu.

Jangan menganggap struktur ini sebagai kewajiban bagi setiap operasi. CRUD sederhana dapat memakai implementasi yang lebih ringkas. Clean Architecture diterapkan terutama pada domain dengan aturan bisnis dan konsekuensi transaksi yang signifikan.

# 6. Module System: kontrak yang harus dirancang

Module Registry adalah salah satu bagian paling kritis dari platform ini. Saya menyarankan modul memiliki lima kontrak yang terpisah:

1. Manifest kontrak untuk metadata dan dependensi.
2. Runtime contract untuk registrasi kemampuan.
3. Lifecycle contract untuk install, enable, disable, dan upgrade.
4. Capability contract untuk authorization.
5. Integration contract untuk komunikasi antarmodul.

## 6.1. Manifest modul

Contoh manifest untuk modul Inventory:

```
{
  "schema_version": "1",
  "key": "acme.inventory",
  "name": "Inventory",
  "version": "1.0.0",
  "publisher": {
    "id": "acme",
    "name": "Acme Software"
  },
  "compatibility": {
    "platform": "^1.0",
    "php": "^8.3",
    "laravel": "^13.0"
  },
  "dependencies": [
    {
      "module": "platform.core",
      "version": "^1.0",
      "optional": false
    }
  ],
  "capabilities": [
    "inventory.item.read",
    "inventory.stock.read",
    "inventory.stock.adjust"
  ],
  "provides": [
    "inventory.stock-management"
  ],
  "migrations": {
    "strategy": "versioned"
  },
  "entrypoint": "Acme\\Inventory\\InventoryModuleProvider"
}
```

Ini adalah contoh kontrak yang kamu definisikan sendiri.

Untuk marketplace publik, manifest juga perlu mencakup identitas artifact, checksum, signature, deklarasi izin yang diminta, lisensi, dan metadata kompatibilitas. Jangan menganggap nilai manifest yang menyatakan `publisher.id` sebagai bukti bahwa publisher tersebut benar-benar tepercaya.

### SemVer dan kompatibilitas

Gunakan Semantic Versioning untuk versi modul:

- `MAJOR`: perubahan kontrak yang tidak kompatibel.
- `MINOR`: fitur baru yang kompatibel.
- `PATCH`: perbaikan yang kompatibel.

Namun, versi modul saja tidak cukup. Simpan juga versi platform SDK dan kontrak yang digunakan. Sebuah modul dapat memiliki versi `2.4.0` tetapi tetap tidak kompatibel dengan versi platform tertentu.

## 6.2. Kontrak runtime

Contoh interface PHP:

```
<?php

namespace App\Core\Modules\Contracts;

interface ErpModule
{
    public function manifest(): ModuleManifest;

    public function register(
        ModuleRegistrar $registrar
    ): void;
}
```

`ModuleRegistrar` menyediakan API yang diizinkan, misalnya untuk mendaftarkan:

- Route dan halaman.
- Navigation items.
- Capabilities.
- Event subscribers.
- Settings schema.
- Scheduled jobs.
- Integration handlers.

Jangan memberikan akses tanpa batas ke container internal atau service provider lain. Public SDK harus menjadi batas kompatibilitas yang sengaja dirancang.

Contoh implementasi modul:

```
<?php

namespace Acme\Inventory;

use App\Core\Modules\Contracts\ErpModule;
use App\Core\Modules\Contracts\ModuleManifest;
use App\Core\Modules\Contracts\ModuleRegistrar;

final class InventoryModuleProvider implements ErpModule
{
    public function manifest(): ModuleManifest
    {
        return ModuleManifest::fromFile(
            __DIR__.'/../module.json'
        );
    }

    public function register(
        ModuleRegistrar $registrar
    ): void {
        $registrar->capability('inventory.item.read');
        $registrar->capability('inventory.stock.read');
        $registrar->capability('inventory.stock.adjust');

        $registrar->routes(
            __DIR__.'/../Presentation/Routes/web.php'
        );
    }
}
```

Kode ini menggambarkan kontrak desain. `ModuleManifest`, `ModuleRegistrar`, dan metode `capability()` serta `routes()` harus kamu implementasikan sendiri.

# 7. Lifecycle modul secara formal

Jangan menyamakan status modul dengan satu boolean `is_active`. Ada perbedaan antara ketersediaan paket, instalasi pada tenant, dan aktivasi operasional.

Saya menyarankan model status berikut:

| Status            | Makna                                               |
| ----------------- | --------------------------------------------------- |
| `available`       | Modul tersedia di katalog atau repository           |
| `verified`        | Artifact lolos pemeriksaan dan signature validation |
| `installed`       | Kode atau paket tersedia pada deployment            |
| `enabled`         | Modul diaktifkan pada tenant tertentu               |
| `disabled`        | Modul terpasang tetapi tidak aktif pada tenant      |
| `upgrade_pending` | Ada versi baru atau migrasi yang belum selesai      |
| `failed`          | Operasi lifecycle gagal dan memerlukan penanganan   |
| `uninstalled`     | Modul tidak lagi terpasang pada tenant              |

Status `verified` adalah hasil pemeriksaan artifact, bukan jaminan bahwa modul sepenuhnya aman.

## 7.1. Alur instalasi dan aktivasi

\#chatgpt-mermaid-\_r_ul\_{font-family:-apple-system-body,ui-sans-serif,-apple-system,system-ui,"Segoe UI",Helvetica,"Apple Color Emoji",Arial,sans-serif,"Segoe UI Emoji","Segoe UI Symbol";font-size:16px;fill:rgb(237, 237, 237);}@keyframes edge-animation-frame{from{stroke-dashoffset:0;}}@keyframes dash{to{stroke-dashoffset:0;}}#chatgpt-mermaid-\_r_ul\_ .edge-animation-slow{stroke-dasharray:9,5!important;stroke-dashoffset:900;animation:dash 50s linear infinite;stroke-linecap:round;}#chatgpt-mermaid-\_r_ul\_ .edge-animation-fast{stroke-dasharray:9,5!important;stroke-dashoffset:900;animation:dash 20s linear infinite;stroke-linecap:round;}#chatgpt-mermaid-\_r_ul\_ .error-icon{fill:rgb(48, 48, 48);}#chatgpt-mermaid-\_r_ul\_ .error-text{fill:rgb(237, 237, 237);stroke:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_ul\_ .edge-thickness-normal{stroke-width:1px;}#chatgpt-mermaid-\_r_ul\_ .edge-thickness-thick{stroke-width:3.5px;}#chatgpt-mermaid-\_r_ul\_ .edge-pattern-solid{stroke-dasharray:0;}#chatgpt-mermaid-\_r_ul\_ .edge-thickness-invisible{stroke-width:0;fill:none;}#chatgpt-mermaid-\_r_ul\_ .edge-pattern-dashed{stroke-dasharray:3;}#chatgpt-mermaid-\_r_ul\_ .edge-pattern-dotted{stroke-dasharray:2;}#chatgpt-mermaid-\_r_ul\_ .marker{fill:rgb(175, 175, 175);stroke:rgb(175, 175, 175);}#chatgpt-mermaid-\_r_ul\_ .marker.cross{stroke:rgb(175, 175, 175);}#chatgpt-mermaid-\_r_ul\_ svg{font-family:-apple-system-body,ui-sans-serif,-apple-system,system-ui,"Segoe UI",Helvetica,"Apple Color Emoji",Arial,sans-serif,"Segoe UI Emoji","Segoe UI Symbol";font-size:16px;}#chatgpt-mermaid-\_r_ul\_ p{margin:0;}#chatgpt-mermaid-\_r_ul\_ .label{font-family:-apple-system-body,ui-sans-serif,-apple-system,system-ui,"Segoe UI",Helvetica,"Apple Color Emoji",Arial,sans-serif,"Segoe UI Emoji","Segoe UI Symbol";color:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_ul\_ .cluster-label text{fill:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_ul\_ .cluster-label span{color:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_ul\_ .cluster-label span p{background-color:transparent;}#chatgpt-mermaid-\_r_ul\_ .label text,#chatgpt-mermaid-\_r_ul\_ span{fill:rgb(237, 237, 237);color:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_ul\_ .node rect,#chatgpt-mermaid-\_r_ul\_ .node circle,#chatgpt-mermaid-\_r_ul\_ .node ellipse,#chatgpt-mermaid-\_r_ul\_ .node polygon,#chatgpt-mermaid-\_r_ul\_ .node path{fill:rgb(9, 23, 44);stroke:rgb(31, 78, 148);stroke-width:1px;}#chatgpt-mermaid-\_r_ul\_ .rough-node .label text,#chatgpt-mermaid-\_r_ul\_ .node .label text,#chatgpt-mermaid-\_r_ul\_ .image-shape .label,#chatgpt-mermaid-\_r_ul\_ .icon-shape .label{text-anchor:middle;}#chatgpt-mermaid-\_r_ul\_ .node .katex path{fill:#000;stroke:#000;stroke-width:1px;}#chatgpt-mermaid-\_r_ul\_ .rough-node .label,#chatgpt-mermaid-\_r_ul\_ .node .label,#chatgpt-mermaid-\_r_ul\_ .image-shape .label,#chatgpt-mermaid-\_r_ul\_ .icon-shape .label{text-align:center;}#chatgpt-mermaid-\_r_ul\_ .node.clickable{cursor:pointer;}#chatgpt-mermaid-\_r_ul\_ .root .anchor path{fill:rgb(175, 175, 175)!important;stroke-width:0;stroke:rgb(175, 175, 175);}#chatgpt-mermaid-\_r_ul\_ .arrowheadPath{fill:rgb(175, 175, 175);}#chatgpt-mermaid-\_r_ul\_ .edgePath .path{stroke:rgb(175, 175, 175);stroke-width:1px;}#chatgpt-mermaid-\_r_ul\_ .flowchart-link{stroke:rgb(175, 175, 175);fill:none;}#chatgpt-mermaid-\_r_ul\_ .edgeLabel{background-color:rgb(0, 0, 0);text-align:center;}#chatgpt-mermaid-\_r_ul\_ .edgeLabel p{background-color:rgb(0, 0, 0);}#chatgpt-mermaid-\_r_ul\_ .edgeLabel rect{opacity:0.5;background-color:rgb(0, 0, 0);fill:rgb(0, 0, 0);}#chatgpt-mermaid-\_r_ul\_ .labelBkg{background-color:rgba(0, 0, 0, 0.5);}#chatgpt-mermaid-\_r_ul\_ .cluster rect{fill:rgb(48, 48, 48);stroke:rgba(255, 255, 255, 0.15);stroke-width:1px;}#chatgpt-mermaid-\_r_ul\_ .cluster text{fill:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_ul\_ .cluster span{color:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_ul\_ div.mermaidTooltip{position:absolute;text-align:center;max-width:200px;padding:2px;font-family:-apple-system-body,ui-sans-serif,-apple-system,system-ui,"Segoe UI",Helvetica,"Apple Color Emoji",Arial,sans-serif,"Segoe UI Emoji","Segoe UI Symbol";font-size:12px;background:rgb(48, 48, 48);border:1px solid rgba(255, 255, 255, 0.15);border-radius:2px;pointer-events:none;z-index:100;}#chatgpt-mermaid-\_r_ul\_ .flowchartTitleText{text-anchor:middle;font-size:18px;fill:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_ul\_ rect.text{fill:none;stroke-width:0;}#chatgpt-mermaid-\_r_ul\_ .icon-shape,#chatgpt-mermaid-\_r_ul\_ .image-shape{background-color:rgb(0, 0, 0);text-align:center;}#chatgpt-mermaid-\_r_ul\_ .icon-shape p,#chatgpt-mermaid-\_r_ul\_ .image-shape p{background-color:rgb(0, 0, 0);padding:2px;}#chatgpt-mermaid-\_r_ul\_ .icon-shape .label rect,#chatgpt-mermaid-\_r_ul\_ .image-shape .label rect{opacity:0.5;background-color:rgb(0, 0, 0);fill:rgb(0, 0, 0);}#chatgpt-mermaid-\_r_ul\_ .label-icon{display:inline-block;height:1em;overflow:visible;vertical-align:-0.125em;}#chatgpt-mermaid-\_r_ul\_ .node .label-icon path{fill:currentColor;stroke:revert;stroke-width:revert;}#chatgpt-mermaid-\_r_ul\_ .node .neo-node{stroke:rgb(31, 78, 148);}#chatgpt-mermaid-\_r_ul\_ [data-look="neo"].node rect,#chatgpt-mermaid-\_r_ul\_ [data-look="neo"].cluster rect,#chatgpt-mermaid-\_r_ul\_ [data-look="neo"].node polygon{stroke:url(#chatgpt-mermaid-\_r_ul\_-gradient);filter:drop-shadow( 1px 2px 2px rgba(185,185,185,1));}#chatgpt-mermaid-\_r_ul\_ [data-look="neo"].swimlane.cluster rect{filter:none;}#chatgpt-mermaid-\_r_ul\_ [data-look="neo"].node path{stroke:url(#chatgpt-mermaid-\_r_ul\_-gradient);stroke-width:1px;}#chatgpt-mermaid-\_r_ul\_ [data-look="neo"].node .outer-path{filter:drop-shadow( 1px 2px 2px rgba(185,185,185,1));}#chatgpt-mermaid-\_r_ul\_ [data-look="neo"].node .neo-line path{stroke:rgb(31, 78, 148);filter:none;}#chatgpt-mermaid-\_r_ul\_ [data-look="neo"].node circle{stroke:url(#chatgpt-mermaid-\_r_ul\_-gradient);filter:drop-shadow( 1px 2px 2px rgba(185,185,185,1));}#chatgpt-mermaid-\_r_ul\_ [data-look="neo"].node circle .state-start{fill:#000000;}#chatgpt-mermaid-\_r_ul\_ [data-look="neo"].icon-shape .icon{fill:url(#chatgpt-mermaid-\_r_ul\_-gradient);filter:drop-shadow( 1px 2px 2px rgba(185,185,185,1));}#chatgpt-mermaid-\_r_ul\_ [data-look="neo"].icon-shape .icon-neo path{stroke:url(#chatgpt-mermaid-\_r_ul\_-gradient);filter:drop-shadow( 1px 2px 2px rgba(185,185,185,1));}#chatgpt-mermaid-\_r_ul\_ .node text{font-size:14px;font-weight:600;letter-spacing:normal;fill:rgb(153, 206, 255);}#chatgpt-mermaid-\_r_ul\_ .edgeLabels text{font-size:13px;font-weight:600;letter-spacing:-0.08px;fill:rgb(153, 206, 255);}#chatgpt-mermaid-\_r_ul\_ .node tspan[font-weight="normal"],#chatgpt-mermaid-\_r_ul\_ .edgeLabels tspan[font-weight="normal"]{font-weight:600;}#chatgpt-mermaid-\_r_ul\_ .edgeLabel .label rect{opacity:1;rx:13px;ry:13px;fill:rgb(0, 14, 26);stroke:rgb(26, 62, 95);stroke-width:1px;}#chatgpt-mermaid-\_r_ul\_ .node rect,#chatgpt-mermaid-\_r_ul\_ .node circle,#chatgpt-mermaid-\_r_ul\_ .node ellipse,#chatgpt-mermaid-\_r_ul\_ .node polygon,#chatgpt-mermaid-\_r_ul\_ .node path{fill:rgb(0, 40, 77);stroke:rgba(255, 255, 255, 0.1);stroke-width:1px;}#chatgpt-mermaid-\_r_ul\_ .node rect{rx:16px;ry:16px;}#chatgpt-mermaid-\_r_ul\_ .node.mermaid-decision .label-container{fill:rgb(0, 14, 26);stroke:rgb(26, 62, 95);stroke-dasharray:2,2;}#chatgpt-mermaid-\_r_ul\_ .edgePaths .flowchart-link{stroke:rgb(175, 175, 175);stroke-width:1px;stroke-linecap:round;stroke-linejoin:round;}#chatgpt-mermaid-\_r_ul\_ .marker{fill:rgb(175, 175, 175);stroke:rgb(175, 175, 175);}#chatgpt-mermaid-\_r_ul\_ :root{--mermaid-font-family:-apple-system-body,ui-sans-serif,-apple-system,system-ui,"Segoe UI",Helvetica,"Apple Color Emoji",Arial,sans-serif,"Segoe UI Emoji","Segoe UI Symbol";}Discover ModuleVerify Publisher and ArtifactSignature and Policy Valid?Reject PackageResolve DependenciesCompatible?Reject or Request UpgradeInstall Package on DeploymentProvision Tenant SchemaRun Tenant MigrationsRegister Capabilities andRoutesEnable Module for TenantHealth Check and AuditHealthy?EnabledDisable and Record FailureNoYesNoYesYesNo

Urutan di atas adalah alur konseptual. Dalam implementasi produksi, registrasi route dan provider tidak boleh membuka akses sebelum status modul serta dependensinya tervalidasi. Setiap tahap harus memiliki status operasi yang persisten agar kegagalan dapat dipulihkan.

### Enable dan disable harus transaksional secara operasional

Misalnya, tenant ingin menonaktifkan Purchasing.

1. Sistem memeriksa apakah modul lain bergantung padanya.
2. Sistem memeriksa apakah ada proses atau job aktif yang memerlukan modul.
3. Operasi baru yang tidak diizinkan dihentikan.
4. Modul ditandai `disabled`.
5. Route, menu, job, dan event subscription dinonaktifkan sesuai kebijakan.
6. Histori transaksi tetap disimpan.
7. Audit log mencatat siapa yang melakukan perubahan dan kapan.

Penting: jika sebuah job sudah masuk queue sebelum modul dinonaktifkan, worker tetap harus memeriksa status modul dan tenant saat mengeksekusi job. Menyembunyikan menu saja tidak cukup.

### Uninstall lebih berisiko daripada disable

Untuk ERP, uninstall modul bisnis sebaiknya tidak otomatis menghapus histori transaksi. Buat kebijakan retensi dan penghapusan data yang eksplisit, dengan konfirmasi, backup, pemeriksaan dependensi, serta mekanisme pemulihan.

Untuk modul yang telah menghasilkan jurnal akuntansi atau pergerakan stok, penghapusan fisik data dapat merusak integritas audit. Dalam kasus tersebut, menonaktifkan modul atau melakukan prosedur archival lebih aman daripada menghapus data secara otomatis.

# 8. Public Marketplace: keamanan harus menjadi bagian dari desain awal

Ini perbedaan besar dibandingkan ERP internal. Ketika pihak ketiga boleh membuat modul, kamu tidak lagi hanya mengelola kode yang ditulis oleh tim sendiri.

Karena itu, saya menyarankan dua kelas ekstensi.

Class A — Trusted in-process modules

Modul internal dan publisher yang sudah melalui proses persetujuan. Kode PHP berjalan dalam proses aplikasi dan harus diperlakukan sebagai kode tepercaya.

Contoh: Inventory, Purchasing, laporan khusus yang disetujui.

Class B — Isolated extensions

Ekstensi publik yang belum dianggap tepercaya harus dijalankan di luar proses utama melalui worker atau service terisolasi, dengan API terbatas.

Contoh: integrasi eksternal, custom automation, atau transformasi dokumen dari publisher pihak ketiga.

Untuk tahap awal, kamu dapat mendukung marketplace dengan proses verifikasi dan persetujuan publisher. Namun, jika kamu benar-benar mengizinkan siapa pun mengunggah kode yang langsung berjalan di server ERP, sandboxing bukan fitur opsional.

## 8.1. Pipeline distribusi modul

\#chatgpt-mermaid-\_r_uu\_{font-family:-apple-system-body,ui-sans-serif,-apple-system,system-ui,"Segoe UI",Helvetica,"Apple Color Emoji",Arial,sans-serif,"Segoe UI Emoji","Segoe UI Symbol";font-size:16px;fill:rgb(237, 237, 237);}@keyframes edge-animation-frame{from{stroke-dashoffset:0;}}@keyframes dash{to{stroke-dashoffset:0;}}#chatgpt-mermaid-\_r_uu\_ .edge-animation-slow{stroke-dasharray:9,5!important;stroke-dashoffset:900;animation:dash 50s linear infinite;stroke-linecap:round;}#chatgpt-mermaid-\_r_uu\_ .edge-animation-fast{stroke-dasharray:9,5!important;stroke-dashoffset:900;animation:dash 20s linear infinite;stroke-linecap:round;}#chatgpt-mermaid-\_r_uu\_ .error-icon{fill:rgb(48, 48, 48);}#chatgpt-mermaid-\_r_uu\_ .error-text{fill:rgb(237, 237, 237);stroke:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_uu\_ .edge-thickness-normal{stroke-width:1px;}#chatgpt-mermaid-\_r_uu\_ .edge-thickness-thick{stroke-width:3.5px;}#chatgpt-mermaid-\_r_uu\_ .edge-pattern-solid{stroke-dasharray:0;}#chatgpt-mermaid-\_r_uu\_ .edge-thickness-invisible{stroke-width:0;fill:none;}#chatgpt-mermaid-\_r_uu\_ .edge-pattern-dashed{stroke-dasharray:3;}#chatgpt-mermaid-\_r_uu\_ .edge-pattern-dotted{stroke-dasharray:2;}#chatgpt-mermaid-\_r_uu\_ .marker{fill:rgb(175, 175, 175);stroke:rgb(175, 175, 175);}#chatgpt-mermaid-\_r_uu\_ .marker.cross{stroke:rgb(175, 175, 175);}#chatgpt-mermaid-\_r_uu\_ svg{font-family:-apple-system-body,ui-sans-serif,-apple-system,system-ui,"Segoe UI",Helvetica,"Apple Color Emoji",Arial,sans-serif,"Segoe UI Emoji","Segoe UI Symbol";font-size:16px;}#chatgpt-mermaid-\_r_uu\_ p{margin:0;}#chatgpt-mermaid-\_r_uu\_ .label{font-family:-apple-system-body,ui-sans-serif,-apple-system,system-ui,"Segoe UI",Helvetica,"Apple Color Emoji",Arial,sans-serif,"Segoe UI Emoji","Segoe UI Symbol";color:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_uu\_ .cluster-label text{fill:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_uu\_ .cluster-label span{color:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_uu\_ .cluster-label span p{background-color:transparent;}#chatgpt-mermaid-\_r_uu\_ .label text,#chatgpt-mermaid-\_r_uu\_ span{fill:rgb(237, 237, 237);color:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_uu\_ .node rect,#chatgpt-mermaid-\_r_uu\_ .node circle,#chatgpt-mermaid-\_r_uu\_ .node ellipse,#chatgpt-mermaid-\_r_uu\_ .node polygon,#chatgpt-mermaid-\_r_uu\_ .node path{fill:rgb(9, 23, 44);stroke:rgb(31, 78, 148);stroke-width:1px;}#chatgpt-mermaid-\_r_uu\_ .rough-node .label text,#chatgpt-mermaid-\_r_uu\_ .node .label text,#chatgpt-mermaid-\_r_uu\_ .image-shape .label,#chatgpt-mermaid-\_r_uu\_ .icon-shape .label{text-anchor:middle;}#chatgpt-mermaid-\_r_uu\_ .node .katex path{fill:#000;stroke:#000;stroke-width:1px;}#chatgpt-mermaid-\_r_uu\_ .rough-node .label,#chatgpt-mermaid-\_r_uu\_ .node .label,#chatgpt-mermaid-\_r_uu\_ .image-shape .label,#chatgpt-mermaid-\_r_uu\_ .icon-shape .label{text-align:center;}#chatgpt-mermaid-\_r_uu\_ .node.clickable{cursor:pointer;}#chatgpt-mermaid-\_r_uu\_ .root .anchor path{fill:rgb(175, 175, 175)!important;stroke-width:0;stroke:rgb(175, 175, 175);}#chatgpt-mermaid-\_r_uu\_ .arrowheadPath{fill:rgb(175, 175, 175);}#chatgpt-mermaid-\_r_uu\_ .edgePath .path{stroke:rgb(175, 175, 175);stroke-width:1px;}#chatgpt-mermaid-\_r_uu\_ .flowchart-link{stroke:rgb(175, 175, 175);fill:none;}#chatgpt-mermaid-\_r_uu\_ .edgeLabel{background-color:rgb(0, 0, 0);text-align:center;}#chatgpt-mermaid-\_r_uu\_ .edgeLabel p{background-color:rgb(0, 0, 0);}#chatgpt-mermaid-\_r_uu\_ .edgeLabel rect{opacity:0.5;background-color:rgb(0, 0, 0);fill:rgb(0, 0, 0);}#chatgpt-mermaid-\_r_uu\_ .labelBkg{background-color:rgba(0, 0, 0, 0.5);}#chatgpt-mermaid-\_r_uu\_ .cluster rect{fill:rgb(48, 48, 48);stroke:rgba(255, 255, 255, 0.15);stroke-width:1px;}#chatgpt-mermaid-\_r_uu\_ .cluster text{fill:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_uu\_ .cluster span{color:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_uu\_ div.mermaidTooltip{position:absolute;text-align:center;max-width:200px;padding:2px;font-family:-apple-system-body,ui-sans-serif,-apple-system,system-ui,"Segoe UI",Helvetica,"Apple Color Emoji",Arial,sans-serif,"Segoe UI Emoji","Segoe UI Symbol";font-size:12px;background:rgb(48, 48, 48);border:1px solid rgba(255, 255, 255, 0.15);border-radius:2px;pointer-events:none;z-index:100;}#chatgpt-mermaid-\_r_uu\_ .flowchartTitleText{text-anchor:middle;font-size:18px;fill:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_uu\_ rect.text{fill:none;stroke-width:0;}#chatgpt-mermaid-\_r_uu\_ .icon-shape,#chatgpt-mermaid-\_r_uu\_ .image-shape{background-color:rgb(0, 0, 0);text-align:center;}#chatgpt-mermaid-\_r_uu\_ .icon-shape p,#chatgpt-mermaid-\_r_uu\_ .image-shape p{background-color:rgb(0, 0, 0);padding:2px;}#chatgpt-mermaid-\_r_uu\_ .icon-shape .label rect,#chatgpt-mermaid-\_r_uu\_ .image-shape .label rect{opacity:0.5;background-color:rgb(0, 0, 0);fill:rgb(0, 0, 0);}#chatgpt-mermaid-\_r_uu\_ .label-icon{display:inline-block;height:1em;overflow:visible;vertical-align:-0.125em;}#chatgpt-mermaid-\_r_uu\_ .node .label-icon path{fill:currentColor;stroke:revert;stroke-width:revert;}#chatgpt-mermaid-\_r_uu\_ .node .neo-node{stroke:rgb(31, 78, 148);}#chatgpt-mermaid-\_r_uu\_ [data-look="neo"].node rect,#chatgpt-mermaid-\_r_uu\_ [data-look="neo"].cluster rect,#chatgpt-mermaid-\_r_uu\_ [data-look="neo"].node polygon{stroke:url(#chatgpt-mermaid-\_r_uu\_-gradient);filter:drop-shadow( 1px 2px 2px rgba(185,185,185,1));}#chatgpt-mermaid-\_r_uu\_ [data-look="neo"].swimlane.cluster rect{filter:none;}#chatgpt-mermaid-\_r_uu\_ [data-look="neo"].node path{stroke:url(#chatgpt-mermaid-\_r_uu\_-gradient);stroke-width:1px;}#chatgpt-mermaid-\_r_uu\_ [data-look="neo"].node .outer-path{filter:drop-shadow( 1px 2px 2px rgba(185,185,185,1));}#chatgpt-mermaid-\_r_uu\_ [data-look="neo"].node .neo-line path{stroke:rgb(31, 78, 148);filter:none;}#chatgpt-mermaid-\_r_uu\_ [data-look="neo"].node circle{stroke:url(#chatgpt-mermaid-\_r_uu\_-gradient);filter:drop-shadow( 1px 2px 2px rgba(185,185,185,1));}#chatgpt-mermaid-\_r_uu\_ [data-look="neo"].node circle .state-start{fill:#000000;}#chatgpt-mermaid-\_r_uu\_ [data-look="neo"].icon-shape .icon{fill:url(#chatgpt-mermaid-\_r_uu\_-gradient);filter:drop-shadow( 1px 2px 2px rgba(185,185,185,1));}#chatgpt-mermaid-\_r_uu\_ [data-look="neo"].icon-shape .icon-neo path{stroke:url(#chatgpt-mermaid-\_r_uu\_-gradient);filter:drop-shadow( 1px 2px 2px rgba(185,185,185,1));}#chatgpt-mermaid-\_r_uu\_ .node text{font-size:14px;font-weight:600;letter-spacing:normal;fill:rgb(153, 206, 255);}#chatgpt-mermaid-\_r_uu\_ .edgeLabels text{font-size:13px;font-weight:600;letter-spacing:-0.08px;fill:rgb(153, 206, 255);}#chatgpt-mermaid-\_r_uu\_ .node tspan[font-weight="normal"],#chatgpt-mermaid-\_r_uu\_ .edgeLabels tspan[font-weight="normal"]{font-weight:600;}#chatgpt-mermaid-\_r_uu\_ .edgeLabel .label rect{opacity:1;rx:13px;ry:13px;fill:rgb(0, 14, 26);stroke:rgb(26, 62, 95);stroke-width:1px;}#chatgpt-mermaid-\_r_uu\_ .node rect,#chatgpt-mermaid-\_r_uu\_ .node circle,#chatgpt-mermaid-\_r_uu\_ .node ellipse,#chatgpt-mermaid-\_r_uu\_ .node polygon,#chatgpt-mermaid-\_r_uu\_ .node path{fill:rgb(0, 40, 77);stroke:rgba(255, 255, 255, 0.1);stroke-width:1px;}#chatgpt-mermaid-\_r_uu\_ .node rect{rx:16px;ry:16px;}#chatgpt-mermaid-\_r_uu\_ .node.mermaid-decision .label-container{fill:rgb(0, 14, 26);stroke:rgb(26, 62, 95);stroke-dasharray:2,2;}#chatgpt-mermaid-\_r_uu\_ .edgePaths .flowchart-link{stroke:rgb(175, 175, 175);stroke-width:1px;stroke-linecap:round;stroke-linejoin:round;}#chatgpt-mermaid-\_r_uu\_ .marker{fill:rgb(175, 175, 175);stroke:rgb(175, 175, 175);}#chatgpt-mermaid-\_r_uu\_ :root{--mermaid-font-family:-apple-system-body,ui-sans-serif,-apple-system,system-ui,"Segoe UI",Helvetica,"Apple Color Emoji",Arial,sans-serif,"Segoe UI Emoji","Segoe UI Symbol";}Publisher Submits PackageValidate ManifestDependency and License ScanStatic Analysis and TestsSecurity ReviewBuild Immutable ArtifactSign and PublishCustomer Requests InstallationVerify Signature andCompatibilityInstall in ApprovedEnvironmentHealth Check and Audit

### Persyaratan minimum marketplace

- Identitas publisher terverifikasi.
- Package immutable dan memiliki checksum.
- Tanda tangan digital diverifikasi dengan public key yang tepercaya.
- Versi dan dependency constraints divalidasi.
- Hak akses yang diminta ditampilkan sebelum instalasi.
- Review keamanan dan pemindaian dependency.
- Kebijakan update, penghentian dukungan, dan pencabutan paket.
- Audit log untuk instalasi, aktivasi, upgrade, dan uninstall.
- Prosedur penanganan vulnerability serta distribusi patch darurat.

Jangan mengizinkan modul publik mengubah core framework secara langsung atau mengakses secret seluruh tenant tanpa batas. Gunakan capability-based access, API yang sempit, dan identitas workload terpisah untuk ekstensi yang diisolasi.

Rekomendasi fase pertama: bangun format package dan SDK publik sejak awal, tetapi batasi eksekusi ke modul bawaan dan publisher tepercaya. Marketplace terbuka sepenuhnya diluncurkan setelah model keamanan dan isolasi eksekusinya matang.

# 9. Implementasi multi-tenancy

Tenant context harus menjadi komponen fundamental, bukan sekadar middleware yang mengganti nama database.

## 9.1. Tenant resolver

Alur yang disarankan:

```
Incoming Request
    ↓
Resolve tenant from trusted hostname / route mapping
    ↓
Authenticate user
    ↓
Verify tenant membership and status
    ↓
Load tenant database reference
    ↓
Resolve database credentials securely
    ↓
Initialize tenant connection
    ↓
Load tenant module configuration
    ↓
Execute authorized use case
    ↓
Clear tenant-scoped state
```

Untuk SaaS, pemetaan domain seperti `company-a.example.com` dapat mengarah ke tenant yang terdaftar. Untuk self-hosted, konfigurasi instalasi dapat menentukan tenant database secara lokal.

Untuk PostgreSQL, misalnya, konfigurasi charset dan parameter koneksi perlu disesuaikan dengan driver yang digunakan.

### Pengamanan wajib

1. Jangan mengizinkan tenant menentukan `host`, nama database, atau credential melalui request.
2. Jangan menyimpan koneksi tenant sebagai global state yang bertahan tanpa batas.
3. Bersihkan atau pulihkan koneksi setelah request selesai, terutama ketika memakai persistent workers.
4. Setiap queued job harus membawa `tenant_id` yang tervalidasi, bukan hanya model atau connection name.
5. Cache, file storage, log context, dan idempotency keys harus dibatasi berdasarkan tenant.
6. Proses scheduled jobs harus memilih tenant secara eksplisit dan terisolasi.
7. Gunakan database account dengan hak akses minimum yang diperlukan.




## 9.2. Tenant context untuk background jobs

Jangan mengasumsikan bahwa tenant context pada HTTP request akan otomatis tersedia di worker.

Contoh kontrak job:

```
final class RebuildInventoryProjection implements ShouldQueue
{
    public function __construct(
        public readonly string $tenantId,
        public readonly string $warehouseId,
    ) {}

    public function handle(
        TenantContextInitializer $tenantContext,
    ): void {
        $tenant = Tenant::findOrFail($this->tenantId);

        $tenantContext->initialize($tenant);

        try {
            app(RebuildProjectionUseCase::class)->execute(
                $this->warehouseId
            );
        } finally {
            $tenantContext->clear();
        }
    }
}
```

Contoh tersebut mengasumsikan `Tenant` dapat diambil dari control database dan initializer mengelola koneksi serta status context dengan benar.

Worker perlu memastikan bahwa tenant context lama tidak bocor ke job berikutnya. worker juga perlu di-restart dengan benar saat deployment.&#x20;


# 10. Contoh kontrak bisnis antarmodul

Ambil kasus: Purchasing perlu menerima barang ke Inventory.

Jangan membuat Purchasing mengubah tabel `stock_movements` milik Inventory secara langsung. Buat kontrak yang menyatakan maksud bisnisnya.

## 10.1. Application port

```
<?php

namespace App\Modules\Inventory\Application\Contracts;

interface ReceiveStock
{
    public function execute(
        ReceiveStockCommand $command
    ): ReceiveStockResult;
}
```

DTO command dapat memuat:

- ID tenant dan perusahaan dalam konteks terverifikasi.
- Referensi Goods Receipt.
- Baris item, warehouse, kuantitas, dan unit.
- Identitas actor atau execution context.

Jangan mempercayai `tenant_id` dari payload mentah hanya karena field tersebut tersedia. Gunakan tenant context yang telah divalidasi.

## 10.2. Use case Inventory

```
final class ReceiveStockHandler
{
    public function __construct(
        private StockMovementRepository $movements,
        private GoodsReceiptPolicy $policy,
    ) {}

    public function handle(
        ReceiveStockCommand $command
    ): ReceiveStockResult {
        $this->policy->assertCanReceive($command);

        return DB::connection('tenant')->transaction(
            function () use ($command) {
                // Validate idempotency and lock affected records.
                // Apply inventory rules.
                // Persist stock ledger and receipt status.
                // Record an outbox event.
                return $this->postReceipt($command);
            }
        );
    }
}
```

Ini merupakan pseudocode yang harus dilengkapi. Implementasi produksi perlu menangani concurrent receipts, stock reservation, validasi kuantitas, unique source references, idempotency, dan penulisan outbox pada transaksi yang sama.

### Kapan memakai synchronous call dan event?

| Kondisi                                                      | Mekanisme                                               |
| ------------------------------------------------------------ | ------------------------------------------------------- |
| Goods Receipt harus ditolak jika aturan stok tidak terpenuhi | Synchronous use case                                    |
| Mengirim notifikasi setelah penerimaan barang                | Event / queued listener                                 |
| Memperbarui dashboard statistik                              | Event dan read projection                               |
| Mengintegrasikan warehouse eksternal                         | Integration event atau API                              |
| Posting jurnal harus berhasil bersama transaksi tertentu     | Desain transaksi eksplisit, bukan event fire-and-forget |

Untuk outbox, buat tabel seperti `outbox_messages` di tenant database. Simpan event dalam transaksi bisnis yang sama, lalu worker mempublikasikannya dan menandai status pengiriman. Konsumen tetap harus idempoten karena pesan dapat terkirim lebih dari sekali.

Jangan menggunakan event sebagai pengganti semua pemanggilan method. Gunakan kontrak langsung ketika pemanggil membutuhkan hasil atau kegagalan secara segera.

# 11. Strategi frontend dan Design System

Frontend sebaiknya mengikuti arsitektur bisnis tanpa menjadi salinan struktur backend secara membabi buta.

Saya menyarankan satu aplikasi React + TypeScript yang memiliki application shell bersama, komponen UI reusable, serta halaman yang didaftarkan berdasarkan kapabilitas modul aktif.

## 11.1. Struktur frontend

```
resources/js/
├── app.tsx
├── core/
│   ├── navigation/
│   ├── permissions/
│   ├── tenant-context/
│   ├── module-registry/
│   └── settings/
│
├── components/
│   ├── ui/
│   ├── data-grid/
│   ├── forms/
│   ├── relation-select/
│   ├── document-status/
│   └── audit-timeline/
│
├── layouts/
│   ├── app-layout.tsx
│   └── auth-layout.tsx
│
├── modules/
│   ├── inventory/
│   │   ├── pages/
│   │   ├── components/
│   │   ├── schemas/
│   │   └── types/
│   ├── purchasing/
│   ├── sales/
│   └── finance/
│
├── shared/
│   ├── api/
│   ├── hooks/
│   ├── types/
│   └── utilities/
│
└── styles/
```

### Aturan frontend

- Setiap modul memiliki halaman dan komponen spesifiknya sendiri.
- Komponen umum tidak boleh mengimpor detail implementasi modul tertentu.
- Backend menyediakan daftar modul aktif, navigation items, dan capability yang boleh digunakan.
- Route tetap divalidasi di server.
- TypeScript types untuk DTO penting sebaiknya dihasilkan atau divalidasi dari kontrak backend agar tidak mudah drift.
- Jangan memuat kode JavaScript dari package publik secara dinamis hanya karena modul diaktifkan. Aset harus melalui pipeline build dan proses distribusi yang tepercaya.

Untuk marketplace publik, UI extension dapat dikembangkan melalui SDK dan entry point yang terdefinisi. Modul eksternal tidak seharusnya bisa menyisipkan script arbitrer ke seluruh halaman ERP tanpa pemeriksaan.

## 11.2. UX yang konsisten tetapi tetap fleksibel

Bangun fondasi berikut sebelum memperluas modul bisnis:

- Navigation registry dan command palette.
- Data grid dengan filter, sorting, pagination, dan saved views.
- Form components dengan validasi konsisten.
- Relation selector yang mendukung pencarian data lintas layar sesuai authorization.
- Document status dan workflow action.
- Activity timeline dan audit history.
- Settings schema untuk konfigurasi tenant.
- Accessibility, keyboard navigation, dan responsive layout.

Jangan membuat seluruh halaman generik berdasarkan metadata sejak awal. UI metadata-driven bagus untuk konfigurasi dan CRUD standar, tetapi proses seperti journal posting, stock reservation, atau period closing sering membutuhkan interaksi khusus dan validasi yang lebih eksplisit.

# 12. Strategi testing dan quality gates

Untuk ERP, test suite tidak cukup hanya membuktikan bahwa halaman berhasil dimuat. Kamu harus menguji konsistensi transaksi, isolasi tenant, dan kompatibilitas modul.

Unit tests

Aturan domain, perhitungan pajak, kurs, pembulatan, status dokumen, dan validasi transaksi.

Integration tests

Repository, database transactions, migration, locking, outbox, dan tenant database resolver.

Contract dan architecture tests

Kompatibilitas SDK, larangan dependensi antarmodul, public API, dan versi manifest.

Security tests

Tenant isolation, authorization, package integrity, privilege escalation, dan secret handling.

End-to-end tests

Alur pengguna dari pembuatan dokumen hingga posting, menggunakan browser dan database yang realistis.

## 12.1. Test wajib untuk Module System

| Skenario                              | Hasil yang diharapkan                                          |
| ------------------------------------- | -------------------------------------------------------------- |
| Modul diaktifkan                      | Route, menu, capability, dan fitur tersedia sesuai konfigurasi |
| Modul dinonaktifkan                   | Akses baru ditolak dan histori tetap tersimpan                 |
| Dependency tidak tersedia             | Aktivasi ditolak dengan alasan yang jelas                      |
| Manifest tidak valid                  | Package tidak didaftarkan                                      |
| Versi tidak kompatibel                | Instalasi atau upgrade ditolak                                 |
| Migration gagal                       | Status operasi tercatat; prosedur pemulihan dapat dijalankan   |
| Job lama dieksekusi setelah disable   | Job memeriksa status modul dan tenant                          |
| Package signature tidak valid         | Instalasi ditolak                                              |
| Tenant A mencoba mengakses tenant B   | Akses ditolak                                                  |
| Dua transaksi mengubah stok bersamaan | Tidak terjadi stok atau ledger yang inkonsisten                |

### Test isolasi tenant

Jangan hanya menguji tenant A dengan database A dan tenant B dengan database B secara terpisah. Buat pengujian adversarial:

1. Pengguna tenant A mengubah ID pada URL untuk mengakses dokumen tenant B.
2. Job tenant A dimanipulasi agar menggunakan referensi tenant B.
3. Cache key tenant A digunakan untuk mencoba membaca cache tenant B.
4. Worker memproses beberapa tenant secara berurutan.
5. Tenant dinonaktifkan ketika masih ada request atau job yang berjalan.

Tujuannya bukan hanya memastikan tenant resolver bekerja, tetapi membuktikan bahwa kegagalan pada satu batas tidak menyebabkan kebocoran data.

## 12.2. Quality gates pada CI/CD

Pipeline minimal:

```
Pull Request
    ↓
Formatting + Static Analysis
    ↓
Backend Unit Tests
    ↓
Frontend Type Checks + Tests
    ↓
Architecture Tests
    ↓
Database Integration Tests
    ↓
Security and Dependency Scan
    ↓
Build Immutable Artifact
    ↓
Deployment Staging
    ↓
Smoke Tests + Migration Verification
    ↓
Production Rollout
```

Gunakan PHPStan atau alat analisis statis yang sesuai, serta ESLint dan TypeScript checks. Quality gate harus berbasis hasil yang dapat diukur, bukan sekadar keberhasilan build.

# 13. Upgrade framework, core, dan modul

Karena kamu menginginkan platform yang mudah di-upgrade, tetapkan kebijakan kompatibilitas sejak versi pertama.

## 13.1. Pisahkan tiga versi

Platform version

Versi core ERP, module SDK, dan kontrak publik.

Module version

Versi setiap modul bisnis atau ekstensi.

Schema version

Versi struktur data yang sudah diterapkan pada database tenant.

Ketiganya tidak selalu berubah bersamaan. Modul dapat merilis patch tanpa mengubah skema. Sebaliknya, perubahan skema besar memerlukan prosedur migrasi tersendiri.

## 13.2. Migration strategy

Gunakan migration per modul dengan pencatatan versi yang eksplisit, misalnya:

```
module_migrations
- id
- module_key
- module_version
- migration_key
- applied_at
- execution_status
- checksum
```

Pencatatan ini merupakan tabel operasional tambahan di tenant database; bukan pengganti migration tracking standar framework.

Aturan yang perlu ditegakkan:

- Migration bersifat versioned dan dapat diaudit.
- Migration produksi tidak dijalankan secara otomatis pada setiap HTTP request.
- Instalasi atau upgrade dijalankan melalui orchestrator yang memiliki status operasi persisten.
- Perubahan skema yang berisiko menggunakan pendekatan expand-and-contract.
- Backup dan restore diuji sebelum upgrade berisiko tinggi.
- Upgrade tenant dapat dijalankan bertahap dengan kemampuan retry.
- Versi core, modul, dan skema setiap tenant harus dapat diketahui.


### Strategi deployment hybrid

Untuk SaaS dan self-hosted, usahakan menggunakan artifact rilis yang sama, lalu membedakan konfigurasi deployment.

SaaS:

- Tenant registry terpusat.
- Provisioning database otomatis.
- Upgrade tenant bertahap.
- Observability dan backup terpusat.
- Canary rollout sebelum seluruh tenant diperbarui.

Self-hosted:

- Artifact atau container image yang dapat diverifikasi.
- File konfigurasi dan secret lokal.
- Perintah install, migrate, upgrade, backup, dan restore yang terdokumentasi.
- Health checks sebelum aplikasi menerima traffic.
- Mekanisme lisensi atau update yang tetap dapat menangani lingkungan dengan konektivitas terbatas, bila itu merupakan kebutuhan produk.

Jangan membangun dua codebase berbeda untuk SaaS dan self-hosted. Perbedaan sebaiknya berada pada deployment profile, konfigurasi, dan layanan control plane yang memang dibutuhkan masing-masing model.

# 14. Roadmap implementasi dari nol hingga scaling

Berikut urutan implementasi yang saya sarankan. 

## 01

Architecture foundation

Estimasi 1–2 minggu

- Buat Architecture Decision Records (ADR).
- Tetapkan database engine.
- Definisikan tenant boundary dan threat model.
- Buat struktur monorepo, coding standards, dan CI.
- Definisikan module manifest dan SDK contract awal.

Exit gate: keputusan arsitektur dapat diuji dan diterjemahkan menjadi struktur kode.

## 02

Core platform dan tenancy

Estimasi 2–4 minggu

- Control database dan tenant registry.
- Database provisioning serta secret management.
- Authentication, membership, RBAC, dan audit.
- Tenant resolver dan isolation tests.
- Mekanisme backup serta restore per tenant.

Exit gate: tenant dapat dibuat dan dipakai tanpa risiko akses silang dalam skenario pengujian yang ditetapkan.

## 03

Module Registry dan lifecycle

Estimasi 2–3 minggu

- Manifest parser dan dependency resolver.
- Install, enable, disable, dan upgrade.
- Capability registration.
- Tenant module settings.
- Migration orchestration dan audit trail.

Exit gate: modul contoh dapat diaktifkan dan dinonaktifkan dengan aman tanpa menghapus data transaksi.

## 04

UI platform dan vertical slice pertama

Estimasi 3–5 minggu

- Application shell dan design system.
- Navigation registry.
- Item, warehouse, Purchase Order, dan Goods Receipt.
- Stock ledger dan validasi transaksi.
- End-to-end tests untuk satu alur bisnis.

Exit gate: satu alur bisnis lengkap dapat dijalankan, diuji, dan diaudit.

## 05

Domain expansion

Estimasi 6–12 minggu atau lebih

- Sales dan Purchasing yang lebih lengkap.
- Finance dengan posting jurnal, payment, dan tax engine.
- Assets dan fixed asset accounting.
- Event outbox, retry, idempotency, serta integrasi.
- Pengujian transaksi dan contract compatibility.

Exit gate: alur bisnis lintas modul memenuhi invariant domain dan target kualitas yang ditetapkan.

## 06

Hybrid release dan marketplace beta

Estimasi 3–6 minggu atau lebih

- Container image dan deployment profiles.
- Self-hosted installer dan upgrade commands.
- Signed packages, publisher verification, dan SDK documentation.
- Isolated extension execution untuk ekstensi yang tidak tepercaya.
- Release channel dan rollback procedures.

Exit gate: instalasi SaaS dan self-hosted menggunakan release yang sama, dan package lifecycle dapat diaudit.

## 07

Production hardening dan scaling

Berlanjut sepanjang produk berkembang

- Load testing dan query profiling.
- Horizontal scaling dan queue workers.
- Tenant-aware observability.
- Backup drills, disaster recovery, dan security review.
- Evaluasi pemisahan service berdasarkan bottleneck aktual.

Exit gate: target performa, availability, recovery, dan biaya operasional terukur.

## 14.1. Urutan modul bisnis

Jangan mengembangkan seluruh modul sekaligus. Gunakan urutan yang menguji fondasi secara bertahap.

| Urutan | Modul                   | Alasan                                                |
| ------ | ----------------------- | ----------------------------------------------------- |
| 1      | Core + Settings         | Identitas, konfigurasi, permission, dan lifecycle     |
| 2      | Master Data + Inventory | Menguji ownership data, ledger, dan transaksi         |
| 3      | Purchasing              | Menguji integrasi dokumen dan penerimaan barang       |
| 4      | Sales                   | Menguji reservasi stok, pengiriman, dan invoice       |
| 5      | Finance                 | Menguji posting jurnal dan konsistensi lintas dokumen |
| 6      | Assets                  | Menguji depresiasi dan integrasi akuntansi            |
| 7      | Marketplace extensions  | Menguji SDK dan kontrak eksternal                     |

Finance tidak boleh dianggap hanya sebagai modul CRUD. Sebelum implementasi posting jurnal, kamu harus menentukan aturan pembukuan, multi-currency, tax handling, reversal, accounting period, dan sumber kebenaran saldo.

# 15. Strategi scaling untuk target growing

Jangan menetapkan ambang scaling berdasarkan jumlah pengguna semata. Tetapkan target SLO dan ukur bottleneck yang benar-benar terjadi.

Tahap A — Single deployment

Satu deployment aplikasi, database terpisah per tenant, cache, dan queue sesuai kebutuhan. Fokus pada konsistensi domain, kualitas query, serta backup.

Tahap B — Horizontal application scaling

Tambahkan beberapa web instances, load balancer, shared cache/session, queue workers, serta pengelolaan koneksi database yang terkontrol. Pastikan tenant context aman di setiap instance.

Tahap C — Data and workload scaling

Evaluasi query optimization, indexing, connection pooling, read replicas, reporting database, dan pemisahan workload tenant besar dari tenant kecil.

Tahap D — Selective service extraction

Pisahkan pekerjaan seperti document rendering, integration gateway, atau reporting jika kebutuhan scaling, isolasi kegagalan, atau deployment independen membenarkannya.

### Risiko khusus database-per-tenant

Database-per-tenant memberi isolasi yang baik, tetapi ketika jumlah tenant tumbuh, jumlah koneksi, migrasi, backup, monitoring, dan upgrade juga meningkat.

Rancang sejak awal:

- Database placement dan kapasitas per cluster.
- Connection pooling dan batas koneksi per worker.
- Tenant migration scheduler dengan concurrency limits.
- Backup dan restore per tenant.
- Tenant-aware metrics, seperti query latency dan queue backlog.
- Prosedur memindahkan tenant dari satu database server ke server lain.
- Jalur khusus bagi tenant besar yang membutuhkan resource terisolasi.

Jangan menjalankan migrasi semua tenant secara paralel tanpa batas. Upgrade harus bisa di-resume, di-throttle, dan dipantau.

# 16. Architecture Decision Records yang harus dibuat

Sebelum tim mulai mengimplementasikan core, buat ADR untuk keputusan yang berpotensi mahal jika diubah belakangan.

| ADR     | Keputusan yang perlu ditetapkan                |
| ------- | ---------------------------------------------- |
| ADR-001 | Modular Monolith sebagai struktur awal         |
| ADR-002 | Clean Architecture dan aturan dependensi modul |
| ADR-003 | Database engine utama                          |
| ADR-004 | Database-per-tenant dan tenant resolution      |
| ADR-005 | Control Plane vs ERP Runtime                   |
| ADR-006 | Module manifest, SDK, dan semantic versioning  |
| ADR-007 | Install/enable/disable/uninstall semantics     |
| ADR-008 | Extension trust model dan sandboxing           |
| ADR-009 | Domain events, outbox, dan idempotency         |
| ADR-010 | Database migration dan upgrade compatibility   |
| ADR-011 | UI extension contract dan design system        |
| ADR-012 | SaaS/self-hosted release strategy              |
| ADR-013 | Authentication, authorization, dan audit       |
| ADR-014 | Observability, SLO, backup, dan recovery       |

Setiap ADR harus memuat konteks masalah, opsi yang dipertimbangkan, keputusan, konsekuensi, dan kondisi yang akan memicu evaluasi ulang.

# 17. Risiko arsitektur terbesar

Sebagai penilaian teknis, saya akan memprioritaskan risiko berikut daripada langsung mengejar jumlah fitur.

Kritis

Ekstensi publik berjalan dengan privilege aplikasi utama

Manifest dan interface tidak cukup. Tanpa trust boundary yang benar, modul pihak ketiga dapat mengakses data atau secret yang seharusnya tidak boleh dijangkau.

Kritis

Kebocoran tenant context

Persistent workers, queue, cache, file storage, atau database connection yang tidak dibersihkan dapat menyebabkan akses silang antartenant.

Mayor

Module system terlalu generik

Jika semua perilaku dipaksakan ke manifest dan generic CRUD engine, aturan domain kompleks menjadi sulit dipahami dan diuji.

Mayor

Upgrade tenant tidak terkendali

Migrasi yang gagal di tengah proses, versi modul yang tidak kompatibel, dan perubahan skema tanpa recovery plan dapat membuat sebagian tenant tidak dapat digunakan.

Menengah

Premature microservices

Pemisahan service sebelum ada kebutuhan terukur meningkatkan kompleksitas operasional tanpa menjamin modularitas yang lebih baik.

# 18. Rekomendasi akhir

Saya akan memulai proyek dengan kombinasi berikut:

Core architecture

Modular Monolith + Clean Architecture selektif + DDD pada domain yang kompleks.

Technology

Go + React + TypeScript + Inertia, dengan PostgreSQL sebagai kandidat utama dan keputusan final setelah proof of concept.

Tenant isolation

Control Database + database-per-tenant + tenant context yang tervalidasi untuk request, jobs, cache, dan storage.

Extensibility

Module Registry + manifest + SDK + lifecycle + dependency resolver + version compatibility.

Marketplace

Signed immutable packages, publisher verification, permission declarations, dan isolated execution bagi kode yang tidak tepercaya.

Scaling

Optimasi monolith lebih dahulu, kemudian scale aplikasi dan database secara terukur, lalu ekstraksi service bila diperlukan.

Urutan tindakan paling tepat sekarang:

1. Finalisasi ADR untuk database engine, tenant isolation, dan module SDK.
2. Implementasikan Control Database dan Tenant Resolver.
3. Buat satu modul contoh dengan manifest, dependency resolver, serta lifecycle lengkap.
4. Buktikan tenant isolation dan kemampuan disable tanpa kehilangan data.
5. Bangun application shell dan satu vertical slice Inventory → Purchasing.
6. Baru perluas domain bisnis dan buka marketplace secara bertahap.

Prinsip yang harus dijaga sepanjang pengembangan adalah: modul boleh berkembang secara independen, tetapi aturan bisnis, transaksi, keamanan tenant, dan kompatibilitas versi harus tetap dapat dikendalikan oleh platform.


Dokumen ini merupakan blueprint arsitektur tingkat platform. Sebelum implementasi produksi, detail kontrak SDK, ERD transaksi lengkap, threat model marketplace, dan prosedur recovery harus diturunkan menjadi spesifikasi teknis yang dapat diuji.