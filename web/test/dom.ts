// Preload pertama untuk `bun test`: mendaftarkan DOM happy-dom ke global (M3a K-2).
// HARUS berkas terpisah yang dimuat sebelum setup.ts: impor statis di-hoist, sehingga
// `@testing-library/*` akan membaca `document` sebelum terdaftar bila registrasi ada di berkas yang sama.
import { GlobalRegistrator } from "@happy-dom/global-registrator";

GlobalRegistrator.register({ url: "http://localhost/" });
