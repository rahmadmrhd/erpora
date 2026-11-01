// Entry sekunder `@erpora/platform-web/metadata`: evaluator metadata dipisah dari barrel utama agar tidak masuk
// bundel awal (anggaran 200 KB gzip). Hanya kode yang dimuat malas (`import()`) yang mengimpor entry ini.
export * from "../metadata";
