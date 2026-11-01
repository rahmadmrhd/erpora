# Studi UI/UX ERPNext (Desk v16) sebagai acuan frontend core

Catatan pengamatan, bukan spesifikasi mengikat. Sumber: deployment milik pemilik produk (`erpnext.rhd.my.id`: ERPNext, Frappe HR, CRM, Helpdesk), diamati 31 Okt 2026 pada tema gelap, viewport ±800 px, data nyaris kosong (satu user, satu lead). Hanya membaca: tidak ada pengaturan yang disimpan atau dokumen yang diubah. Tujuannya menjadi bahan spec revisi M3 (shell, M3b–M3d), dengan komponen **persis shadcn** dan ERP Design System sebagai acuan visual (keputusan 31 Okt 2026).

Putaran kedua (31 Okt 2026, atas izin pemilik produk untuk mengubah data uji): dibuat enam ToDo ber-prefiks `[Uji UX]`, satu Kanban Board `Uji UX Status`, satu Item `UJI-UX-001` dan satu Material Request `MAT-MR-2026-00001` (sudah di-Submit). Pengaturan pengguna (tema, layout, dock) yang diubah untuk percobaan sudah dikembalikan ke Dark, Full Width, Pinned. Tidak ada data yang dihapus permanen.

## 1. Dua gaya UI dalam satu deployment

| | Desk (`/desk`) | CRM dan Helpdesk (`/crm`, `/helpdesk`) |
| --- | --- | --- |
| Shell | Rel ikon (Dock) + panel menu bersarang | Satu sidebar: selektor app + pengguna di atas, Search `Ctrl K`, Notifications, menu, kartu "Getting started", Collapse di dasar |
| Tema | Mengikuti pengaturan (di sini gelap) | Terang |
| Navbar | Judul halaman; kanan: pemilih tampilan, refresh, `...`, tombol utama | Breadcrumb (`Leads / List ▾`), tombol `+ Create` |
| Halaman dokumen | Form dengan tab, sidebar kanan, Comments, Activity | Tab aktivitas di kiri (Activity, Emails, Comments, Data, Calls, Tasks, Notes, Attachments) dan panel Details inline di kanan |

Keputusan pemilik produk (31 Okt 2026): shell mengikuti **Desk v16**.

## 2. Shell Desk v16

- **Peluncur (`/desk`)**: kisi ikon aplikasi (ERPNext, Frappe HR, CRM, Helpdesk, Framework) di bawah bar Search `Ctrl+K`; pojok kanan atas: lonceng notifikasi dan avatar.
- **Dock (rel ikon)**: logo app di atas (membuka peluncur), Search, Notifications, satu ikon per Desk, avatar di dasar. Mode **Pinned** (menempel di sisi sidebar) atau **Floating** (sidebar menjadi satu panel; dock berupa pil melayang yang meluncur dari tepi kiri; lihat 7.1); `Shift+Ctrl+/` membuka dock. Menu avatar: **My Space, Settings, Manage Dock, Reload, Logout**.
- **Manage Dock** (dialog): daftar entri dengan seret-untuk-urut dan ikon mata (sembunyikan), tombol Add / Reset / **Reset for everyone**, pemilih cakupan "For everyone", panel **Preview** ("dock seperti hasil susunan ini"), tombol Save.
- **Panel menu**: judul Desk berdropdown (`Setup ▾`; menu: All apps, Edit Sidebar, Help), item dengan ikon, sebagian bersarang dengan chevron dan membuka inline (Reports membuka daftar anak). Susunan dan hierarkinya dapat diatur pengguna atau seluruh situs (lihat 7.2).
- **Notifikasi**: panel yang meluncur di sebelah sidebar (kolom ketiga), tab **All / Events**, ikon pengaturan, tandai-semua, tutup; keadaan kosong dengan ikon lonceng dan kalimat penjelas.
- **Awesomebar (`Ctrl+K`)**: palet perintah "Search or type a command"; hasil campuran dokumen dan aksi (User List, User Report, New User, Open Users Workspace, ...) dengan teks cocok disorot; pintasan di footer (↑↓ navigasi, ↵ pilih, Esc tutup, `Ctrl+G` Global Search).
- **Home Desk**: kartu sambutan ("Good evening, Rahmad") dengan progres onboarding 9 langkah (Add User, Import, Skip; dapat di-Dismiss), kartu chart dengan filter dan `...`, deretan tile angka (Annual Sales, Annual Purchase, Total Stock Value).

## 3. Settings (dialog dua kolom: navigasi kiri, konten kanan)

| Bagian | Isi |
| --- | --- |
| Profile | Avatar dan email, Change Password, nama depan/tengah/belakang, username, **Save** |
| Email | Tiga saklar (notifikasi thread, salinan email keluar, tanda terima baca), editor **Email Signature** (rich text) |
| **Appearance** | **Theme**: Automatic / Light / Dark (kartu pratinjau); **Layout**: Compact / Full Width (lebar form); **Dock**: Pinned / Floating. Pilihan berbentuk kartu bergambar, bukan dropdown |
| Preferences | Saklar: Allow notifications, Show search bar, Show My Space, Mute sounds; **Locale**: Language dan Time Zone (tombol Change) |
| Lists | Show sidebar (filter dan group-by), Allow bulk actions, Show view switcher |
| Forms | Show sidebar, Show timeline, Show dashboard, Show absolute datetime in timeline, Show navigation buttons |
| Reports | Split view (buka dokumen di panel samping; tidak ada di mobile) |
| Session Defaults | Nilai bawaan sesi (mis. Default Company) yang mengisi form dan laporan; tombol Configure dan Save |
| Keyboard Shortcuts | Tabel pintasan global: `Ctrl+S` aksi utama, `Ctrl+K` Awesomebar, `Ctrl+G` Global Search, `Shift+/` daftar pintasan, Clear cache and reload, toggle sidebar, switch theme, buka app dock; catatan "tekan Alt untuk pintasan tambahan di Menu dan Sidebar" |

Semua preferensi bersifat per pengguna dan tercatat di riwayat dokumen User (mis. "You changed the value of Dock from Floating to Pinned").

## 4. Tampilan daftar (List View)

- **Header halaman**: judul; kanan: **pemilih tampilan** (List View, Report View ▸, Dashboard View, Kanban View ▸, Image View), refresh, `...`, tombol utama (`+ Add User`).
- **Baris filter standar**: kolom filter inline per field (ID, Full Name, Username, User Type), chip **Filters N** dengan ✕ (hapus semua), pengurutan ("Created On" dengan arah).
- **Popover Filters**: baris `field | operator | nilai`, "+ Add a Filter", Clear Filters, Apply Filters. Hanya AND datar; tidak ada OR bersarang. (Spesifikasi M3c kita lebih kaya: filter bersarang dan group-by.)
- **Menu `...`**: Import, User Permissions, Role Permissions Manager, Customize Quick Filters, List Settings, Documentation.
- **Baris**: checkbox, kolom (nama tebal, badge status, tipe, ID), ujung kanan: waktu relatif, jumlah komentar, ikon suka; penghitung "1 of 1"; footer ukuran halaman 20 / 100 / 500 / 2500.
- **Aksi massal**: memilih baris mengganti bar judul menjadi "N items selected" dan tombol utama menjadi dropdown **Actions**: Copy to Clipboard, Edit, Export, Assign To, Clear Assignment, Apply Assignment Rule, Add Tags, Print, Delete.
- **CRM Kanban**: kolom berdot warna per status dengan "+" di kepala kolom; kartu: avatar inisial, nama, pemilik, waktu relatif, deretan ikon aksi cepat (email, catatan, tugas, komentar) dan "+".

## 5. Form (dokumen)

- **Header**: `User / Rahmad Maulana` dengan badge status; kanan: dropdown konteks (mis. Permissions, Password), tombol aksi sekunder (Create User Email), panah dokumen sebelumnya/berikutnya, `...`, **Save** (primer, `Ctrl+S`).
- **Menu `...`**: Toggle Sidebar, Email (`Ctrl+E`), Rename, Reload, Delete, Duplicate, New (`Ctrl+B`), **Jump to field** (`Ctrl+J`), Show Links, Copy to Clipboard, Remind Me, **Undo/Redo** (`Ctrl+Z`/`Ctrl+Y`), Documentation.
- **Tata letak**: tab (User Details, Roles & Permissions, More Information, Settings, Connections, Sessions), bagian bernama (Basic Info) dengan kolom grid, tanda wajib `*`, kolom baca-saja diredam.
- **Sidebar kanan**: kartu avatar/nama/email dengan ikon edit/cetak/suka; **Assign**, **Attachments**, **Tags**, **Share** (masing-masing dengan "+"), "Last Edited By", "Created By".
- **Comments** (kolom "Type a reply / comment") dan **Activity** (timeline perubahan nilai, aksi "New Email").
- **CRM, halaman dokumen**: breadcrumb mempertahankan konteks tampilan asal (`Leads / Kanban / Dr SAS`); aksi di navbar (Enrich, pemilik, status dropdown, tombol primer kontekstual "Convert to Deal"); panel Details dan Person dengan **field inline-edit** ("Add Organization...", simpan otomatis); bar bawah **Reply / Comment**.

## 6. Pola lintas halaman yang layak ditiru

1. Preferensi tampilan dikelola lewat dialog Settings berkartu pratinjau, per pengguna.
2. Dock dapat diatur pengguna (urutan, sembunyikan) dan administrator ("Reset for everyone").
3. Tombol utama halaman selalu satu dan berubah konteks (Add / Save / Actions / Convert).
4. Pintasan keyboard mendalam dan dapat ditemukan (`Shift+/`, Alt-hints).
5. Keadaan kosong, onboarding, dan "Getting started" sebagai bagian pengalaman, bukan tempelan.
6. Waktu relatif di daftar, absolut opsional di timeline.
7. Satu palet perintah untuk navigasi dan aksi.
8. Form dan daftar menyimpan konteks asal di breadcrumb.

## 7. Temuan putaran kedua

### 7.1 Dock Floating (diuji langsung)
- Hanya mengubah **mode**, bukan isi: pada Floating, rel ikon hilang dan sidebar menjadi **satu panel lebar** (mirip CRM): kepala berisi pemilih Desk (`Setup ▾`), di bawahnya **Search** dan **Notification** (pada Pinned keduanya ada di rel), lalu menu, dan di dasar **avatar + nama + email**.
- Dock menjadi **pil vertikal** (lebar 56 px, tinggi menyesuaikan, maks `100vh − 32px`, di tengah tinggi layar, `left: 8px`) yang diparkir di luar layar (`translate(-150%, -50%)`) dan meluncur masuk dengan transisi 0,28 dtk (`cubic-bezier(0.4, 0, 0.2, 1)`); isi: logo di atas, garis pemisah, ikon Desk, yang aktif disorot. Pemicunya (tepi kiri atau pintasan) tidak berhasil saya picu lewat hover; yang terverifikasi hanya wujud dan animasinya.
- Setelan tersimpan seketika (toast "Saved · Refresh to see changes") dan kelas `body` berganti (`dock-pinned` / `dock-active`); tema `full-width` juga kelas `body`.
- **Compact vs Full Width**: Compact mengunci lebar form (±921 px dari area ±1199 px), Full Width memakai seluruh area.
- Pengaturan tampilan disimpan sebagai objek `desk_settings` per pengguna: `search_bar`, `notifications`, `list_sidebar`, `bulk_actions`, `view_switcher`, `form_sidebar`, `form_navigation_buttons`, `timeline`, `dashboard`, `report_split_view`, `show_my_space`, `dock_mode`.

### 7.1a Pembatas sidebar yang dapat diklik (diuji di Pinned dan Floating)
- Pembatas sidebar adalah elemen `.sidebar-edge` selebar 8 px setinggi layar; kursor `w-resize` saat terbuka dan `e-resize` saat diciutkan. Tidak ada gagang visual saat hover; kursor saja penandanya.
- **Klik menciutkan atau membuka**; `body` mendapat kelas `sidebar-collapsed`. Klik pertama setelah perubahan keadaan kadang tidak terdaftar pada pengujian otomatis (klik kedua berhasil), jadi pada implementasi area klik sebaiknya lebih lebar dari 8 px.
- **Pinned:** menciutkan **panel menu saja**; rel Desk tetap (logo, Search, Notification, ikon Desk, avatar). **Floating:** sidebar menjadi **strip ikon** (±48 px) berisi ikon Desk (kepala), Search, Notification, ikon menu, dan avatar di dasar.
- Ada juga tombol `sidebar-collapse-arrow` (24 px) dan pintasan "Toggle sidebar" di daftar pintasan.

### 7.2 Edit Sidebar (pengaturan panel menu)
- Menu kepala panel (`Setup ▾`) berisi **All apps, Edit Sidebar, Help ▸**. Edit Sidebar membuka dialog **Manage <Desk> Sidebar** (pola yang sama dengan Manage Dock): daftar entri hierarkis dengan **seret untuk mengurutkan** dan **seret ke dalam bagian untuk menyarangkan**, pensil = ganti nama, mata = sembunyikan, label **Section** pada entri induk, panel **Preview** ("sidebar seperti hasil susunan ini"), tombol Add, Reset, **Reset to Standard**, Save.
- Pemilih cakupan di pojok dialog: `user` (susunan pribadi) atau `site` ("For everyone").
- **Add to the Sidebar**: *Kind* = Link / Section; *Links To* = DocType / Page / Report / Workspace / Dashboard / URL; *Item*, *Label*, *Icon* (pemilih ikon), tombol Add.

### 7.3 Frappe HR (`/hrms`)
PWA **mobile-first** khusus swa-layanan karyawan: header (judul, lonceng, avatar), kartu sambutan dengan tombol **Check In**, daftar **Quick Links** (Request Attendance, Request a Shift, Request Leave, Request Compensatory Leave, Claim an Expense, Request an Advance, View Salary Slips), tab **My Requests / Team Requests**, dan navigasi bawah (Home, Attendance, Leaves, Expenses, Salary). Menolak akun tanpa data karyawan dengan dialog "Login Failed". Pelajaran untuk M3g (PWA): mobile punya tata letak sendiri dengan navigasi bawah, bukan sidebar yang diciutkan.

### 7.4 Daftar (ToDo, 8 data)
- Badge berwarna per nilai: Status (Open merah, Closed hijau, Cancelled abu-abu), Priority (High merah, Medium oranye, Low abu-abu); kolom referensi memuat tombol kecil "Open" ke dokumen terkait.
- Baris filter standar mengikuti jenis field: field *select* menjadi dropdown (Status, Priority), tanggal menjadi pemilih tanggal, teks memakai ikon kondisi (≈).
- Pintasan dan menu `...`: User Permissions, Role Permissions Manager, **Customize (`Ctrl+Y`)**, Settings, Customize Quick Filters, List Settings, Documentation.
- **Tidak ada sidebar group-by** di tampilan daftar (panel kirinya disembunyikan, `display: none`); group-by dipindah ke Report View. Pengaturan "Lists → Show sidebar" ada, tetapi pada instalasi ini tidak menampilkan apa pun.

### 7.5 Report View
Grid bernomor dengan checkbox, **filter per kolom** di bawah header, dan tombol **Add Group**: pilih satu field (Allocated To, Created By, Priority, Reference Type, Status, Tags, ...) dan **agregat** Count / Sum / Average (+ field agregat). Hasil berupa tabel ringkas (mis. Open 6, Closed 1, Cancelled 1) dengan penghitung "1 of 8 (3 rows with children)". Status group-by tersimpan di URL (`?_group_by=[...]`). Hanya **satu level** (spesifikasi M3c kita menargetkan sampai 4).

### 7.6 Kanban (Desk)
- Dialog **New Kanban Board**: nama (privat) dan "Columns based on" (hanya field *select*: Status, Priority).
- Halaman: judul = nama board, **pemilih board** di kanan atas, filter, tombol **Group** (None / Assigned To / Priority → swimlane) dan **Settings**.
- Kolom: titik + nama + jumlah + "+"; kartu memuat judul, field terpilih (Priority berbadge, Due Date, Reference Type), tombol assign "+", dan waktu relatif.
- **Seret-lepas antar kolom** langsung menyimpan nilai field (diuji: Open 6→5, Closed 1→2).
- **Kanban Settings** (dialog dengan navigasi kiri Config / Columns / Cards / Swimlanes): Board Name, Footer Date (Modified), Title Field, Image Field, Show Assigned To, Show Tags on Cards.

### 7.7 Dashboard View dan Dashboard Chart
- Keadaan kosong: "No charts or cards yet — Click Customize to add your first widget".
- Mode kustomisasi: tombol **Save Customizations / Discard**, area putus-putus **+ New Number Card** dan **+ New Chart**; menambah chart membuka dialog "Choose an existing chart or create a new chart".
- Chart dibangun sebagai form DocType biasa (**Dashboard Chart**): Chart Name, **Chart Type** (Count / Sum / Average / Group By), Document Type, Time Series Based On, Timespan (Last Year, ...), Time Interval (Yearly, ...), Type (Line, ...), Show Values over Chart, Currency, tabel **Filters**, **Custom Options** (JSON), Color, Empty State Message, Roles, "Is Public", "Is Standard".

### 7.8 Dokumen Submit (Material Request, Stock Desk)
- **Draft**: lencana "Draft", banner biru "Submit this document to confirm", tombol utama **Submit**, dropdown konteks (Get Items From), tab (Details, Terms, More Info, Connections); tabel anak dengan nomor, checkbox, ikon pensil per baris, **Add row / Add multiple / Download / Upload**.
- **Konfirmasi**: dialog "Permanently Submit MAT-MR-...?" dengan **No / Yes**; tombol utama berubah menjadi "Submitting...".
- **Submitted**: lencana berubah mengikuti status ("Pending"), semua field menjadi baca-saja, tombol: **Stop**, **Create ▾** (dokumen turunan), panah, `...`, **Cancel**; toast "Submitted"; timeline mencatat status.
- Peralihan menutup tab Terms dan menyembunyikan banner serta tombol edit tabel.
- Dokumen non-submittable (ToDo): setelah simpan header menampilkan lencana status (Open), tombol **Close** dan **New**, serta toast "Saved"; layar diredupkan selama menyimpan.

## 8. Belum diamati

Image View, Calendar, Gantt; filter tersimpan dan beberapa kondisi sekaligus pada popover filter; dialog form (quick entry) dan field Link; alur Cancel/Amend dan kolom docstatus di daftar; pemicu dock Floating (hover tepi kiri / `Ctrl+Shift+/`) dan keadaan tersembunyi saat tidak dipakai; perilaku mobile Desk; halaman Workspace yang dapat diedit; Helpdesk dan CRM di luar Leads (Deals, Contacts, Notes, Tasks, Call Logs).

## 9. Arahan pemilik produk setelah studi (31 Okt 2026)

Bukan salinan ERPNext: ini keputusan dan pengecualian pemilik produk terhadap yang diamati di atas. Status: dicatat, belum dibahas per butir dan belum menjadi spec.

1. **Halaman Start Guide**: tahapan setup per modul (padanan kartu onboarding "Good evening, ... 0 of 9 steps done", tetapi sebagai halaman tersendiri).
2. **Manajemen sidebar kustom diperluas** agar mendukung tautan spesifik (dokumen tertentu), laporan, dan jenis lain. **Hanya dicatat; jangan dikerjakan sekarang.**
3. **Editor Tiptap berfitur lengkap** sebagai komponen editor teks kaya.
4. **Comments di halaman Form** juga memakai Tiptap dengan **toolbar terlihat** dan **mention pengguna yang punya akses ke Form saat itu** (daftar mention mengikuti otorisasi dokumen; bergantung pada PRD-001).
5. **Halaman Desks**: satu desk per **modul** (ERPNext v16: per app). Desk dapat dikustomisasi seperti di ERP Design System, dengan **pengelompokan desk/folder** di halaman Desks.
6. **Dock di halaman utama berisi daftar Desk**, sehingga berpindah desk tidak perlu kembali ke halaman Desks.
7. **Breadcrumb**: `ikon Home` (menuju halaman Desks) `>` **desk saat ini** (menuju Dashboard desk itu) `>` ... Diringkas **bertahap menurut lebar** (rincian tahap dan pengecualian ponsel di PRD-000 Q40 butir 4): utuh bila muat; Opsi A (Home, Desk, "…", dua langkah terakhir); Opsi B (Home, "…", dua langkah terakhir bila muat); ponsel (`… › langkah terakhir`, Home di dalam "…").
8. **Setiap desk punya satu halaman Dashboard** (menggantikan pasangan Home + Dashboard di ERPNext, yang memakai komponen sama); hanya "Dashboard" yang ada, setara Home ERPNext.

### Arahan lanjutan (31 Okt 2026, setelah mockup sidebar)

9. **Struktur sidebar**: header Desk dihapus; grup tetap **Cari, Notifikasi, ToDo** (boleh bertambah kelak), divider, menu Desk saat ini; popover pada logo berlaku di Pinned dan Floating; pembatas sidebar dapat diklik untuk menciutkan atau membuka (hasil uji di 7.1a).
10. **Dashboard wajib per Desk**, tetap berupa item di grup menu, **terkunci** pada pengaturan sidebar.
11. **Penentuan Desk saat ini** (tiap tahap memeriksa izin): memori klien per tab, lalu cookie/`localStorage`, lalu `default_desk` pengguna, lalu Desk pemilik utama menu, lalu Desk pertama dari menu itu yang diizinkan.
12. **Aturan Desk boleh tampil**: kepemilikan Desk kustom (pribadi, atau terlihat untuk pengguna/peran), minimal satu menu diizinkan, tidak dinonaktifkan, dan seterusnya.

Rincian dan status resmi ada di PRD-000 Q40 butir 9–12.

### Pengujian penentuan Desk di ERPNext (31 Okt 2026)

| Aksi | Hasil |
| --- | --- |
| Buka `/desk/item` (tanpa Desk di URL, muat penuh) | Dialihkan ke `/desk/stock/item`: Desk **pemilik utama** Item |
| Dari Desk Selling, klik "Items › Item" di sidebar | Tautan `/desk/selling/item`: **tetap di Selling** (Item ada di sidebar Selling) |
| Buka langsung `/desk/selling/item` | Tetap di Selling (pasangan valid) |
| Buka `/desk/buying/user` (User tidak ada di Buying) | Dialihkan ke `/desk/users/user`: pasangan tidak valid diganti Desk pemilik utama |

Pelajaran: di ERPNext **Desk adalah bagian rute** (`/desk/<desk>/<halaman>`), sehingga "Desk saat ini per tab" otomatis berasal dari URL dan validasi terhadap halaman dilakukan di router. Sidebar Selling juga memuat **Home dan Dashboard sekaligus** (arahan produk: hanya Dashboard).

13. **URL bersih, Desk di state** (menolak `/desk/<desk>/<halaman>` milik ERPNext): `/` = Dashboard Desk saat ini, `/desks` = halaman Desks, `/<slug-menu>` = halaman menu (`/sales-order`). Tautan yang dibagikan terbuka di Desk yang biasa dipakai penerima. Rinci di PRD-000 Q40 butir 13.

14. **Koreksi rute (31 Okt 2026)**: rute modul **non-core wajib diawali slug modul** (`/example/notes`); rute modul **core tidak berawalan**. Menggantikan usulan "semua rute datar" pada butir 13.
15. **Setiap modul wajib punya definisi Desk bawaan** (sidebar dan pengaturan relevan lain) sebagai default yang masih dapat diedit (timpaan di atas default, "Reset to Standard").
16. **Aturan mengedit Desk**: Desk bawaan modul dan Desk yang diatur untuk beberapa pengguna/peran hanya dapat diedit pengguna ber-permission atas Desk itu; Desk kustom pribadi dapat diedit pemiliknya dan pengguna ber-permission atas Desk. Rincian di PRD-000 Q40 butir 13–15.

17. **Koreksi akhir URL (31 Okt 2026)**, menggantikan butir 13 dan 14: **semua modul, termasuk core, berawalan slug modul** (`/example`, `/core`, `/core/users`). `/` hanya pintu masuk: menentukan Desk (state tab, `localStorage`, `default_desk`) lalu mengarah ke Dashboard-nya, atau menampilkan halaman Desks bila tidak ada. Halaman Desks tidak punya URL (`/desks` dihapus); Desk kustom memakai `/desks/<slug-desk>`. Ikon Home membuka Desks dengan melewati proses penentuan. Rinci di PRD-000 Q40 butir 13.
