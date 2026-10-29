// Entry sekunder `@erpora/ui/toaster` (M3a K-12): `Toaster` dan `toast` dari sonner (±35 KB gzip sendiri) dipisah dari barrel
// agar tidak masuk bundel awal. Shell memuat `Toaster` lewat `import()`; halaman yang menampilkan toast mengimpor `toast`
// dari sini (chunk yang sama).
export { toast } from "sonner";
export { Toaster } from "../components/ui/sonner";
