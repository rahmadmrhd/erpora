# Studi UI/UX ERPNext (Desk v16) sebagai acuan frontend core

Catatan pengamatan, bukan spesifikasi mengikat. Sumber: deployment milik pemilik produk (`erpnext.rhd.my.id`: ERPNext, Frappe HR, CRM, Helpdesk), diamati 31 Okt 2026 pada tema gelap, viewport ±800 px, data nyaris kosong (satu user, satu lead). Hanya membaca: tidak ada pengaturan yang disimpan atau dokumen yang diubah. Tujuannya menjadi bahan spec revisi M3 (shell, M3b–M3d), dengan komponen **persis shadcn** dan ERP Design System sebagai acuan visual (keputusan 31 Okt 2026).

Batas pengamatan: `/hrms` adalah portal swa-layanan karyawan (menolak akun tanpa data karyawan), sehingga tidak diamati. Tampilan Kanban, laporan, dan dashboard Desk tidak diamati karena data kosong; tampilan Kanban hanya dilihat di CRM.

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
- **Dock (rel ikon)**: logo app di atas (membuka peluncur), Search, Notifications, satu ikon per Desk, avatar di dasar. Mode **Pinned** (menempel di sisi sidebar) atau **Floating** (melayang, muncul dari tepi kiri); `Shift+Ctrl+/` membuka dock. Menu avatar: **My Space, Settings, Manage Dock, Reload, Logout**.
- **Manage Dock** (dialog): daftar entri dengan seret-untuk-urut dan ikon mata (sembunyikan), tombol Add / Reset / **Reset for everyone**, pemilih cakupan "For everyone", panel **Preview** ("dock seperti hasil susunan ini"), tombol Save.
- **Panel menu**: judul Desk berdropdown (`Setup ▾`), item dengan ikon, sebagian bersarang dengan chevron dan membuka inline (Reports membuka daftar anak).
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

## 7. Belum diamati (perlu rujukan tambahan sebelum spec blok terkait)

Tampilan Report View, Dashboard View, Image View, Kanban Desk, Calendar/Gantt; popover filter dengan beberapa kondisi dan filter tersimpan; sidebar group-by di daftar (pengaturan "Show sidebar" ada, tetapi tidak terlihat pada viewport ini); form dokumen yang dapat di-Submit dengan status Draft/Submitted/Cancelled; dialog form (quick entry); keadaan Floating dock secara langsung (hanya dilihat lewat tangkapan layar pemilik produk); lebar Compact vs Full Width; perilaku mobile.
