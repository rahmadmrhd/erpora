// Dijalankan sinkron sebelum paint pertama (M3a K-18): memasang tema eksplisit dari localStorage.
// Mandiri (tanpa impor) dan tanpa <script> inline agar kompatibel dengan CSP `script-src 'self'`.
// Pilihan "system" (atau nilai asing/akses diblokir) tidak memasang kelas: CSS `prefers-color-scheme` yang bekerja.
// Kunci dan perilaku harus sama dengan packages/platform-web/src/theme (diuji di task 9.1).
try {
  const pref = localStorage.getItem("erpora.theme");
  if (pref === "light" || pref === "dark") {
    document.documentElement.classList.add(pref);
    document.documentElement.style.colorScheme = pref;
  }
} catch {
  // localStorage diblokir: biarkan tema sistem.
}
