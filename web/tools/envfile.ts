// Pengurai .env untuk skrip dev (dev-api.ts), sejalan dengan godotenv yang dipakai `erpora --env-file`:
// komentar baris, `export`, kutip ganda (escape \n \r \" \) dan tunggal (literal), komentar inline ` #...` di luar
// kutip, dan entri ganda (yang terakhir menang). Tidak mengekspansi ${VAR}. Nilai tidak pernah dicetak.

const LINE = /^\s*(?:export\s+)?([\w.]+)\s*[=:](.*)$/;

function parseValue(rest: string): string {
  const v = rest.trim();
  if (v.startsWith('"')) {
    let out = "";
    for (let i = 1; i < v.length; i++) {
      const c = v[i];
      if (c === "\\" && i + 1 < v.length) {
        const n = v[i + 1];
        out += n === "n" ? "\n" : n === "r" ? "\r" : n;
        i++;
      } else if (c === '"') {
        return out; // sisa setelah kutip penutup (mis. komentar) diabaikan
      } else {
        out += c;
      }
    }
    return v.slice(1); // kutip tak tertutup: ambil apa adanya
  }
  if (v.startsWith("'")) {
    const end = v.indexOf("'", 1);
    return end === -1 ? v.slice(1) : v.slice(1, end);
  }
  const comment = rest.search(/\s#/); // pada nilai mentah: `KEY= # catatan` berarti nilai kosong
  return (comment === -1 ? rest : rest.slice(0, comment)).trim();
}

/** Mengurai isi .env menjadi peta; entri ganda: yang terakhir menang. */
export function parseDotenv(text: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const line of text.replace(/^﻿/, "").split(/\r?\n/)) {
    if (line.trim() === "" || line.trimStart().startsWith("#")) continue;
    const m = LINE.exec(line);
    if (m?.[1] !== undefined) out[m[1]] = parseValue(m[2] ?? "");
  }
  return out;
}

/**
 * Menentukan variabel kunci rahasia yang perlu ditambahkan ke environment API dev dari .env. Environment proses menang
 * (seperti config Go): hanya variabel yang belum terisi di proses yang diteruskan. ERP_SECRET_KEY wajib ada (proses atau
 * .env), bila tidak galat tanpa nilai; ERP_SECRET_KEY_PREVIOUS opsional.
 */
export function resolveSecretEnv(
  processEnv: Record<string, string | undefined>,
  dotenvText: string | undefined,
): Record<string, string> {
  const fromFile = dotenvText === undefined ? {} : parseDotenv(dotenvText);
  const out: Record<string, string> = {};
  for (const name of ["ERP_SECRET_KEY", "ERP_SECRET_KEY_PREVIOUS"]) {
    if ((processEnv[name] ?? "").trim() !== "") continue;
    const value = (fromFile[name] ?? "").trim();
    if (value !== "") out[name] = value;
  }
  if ((processEnv.ERP_SECRET_KEY ?? "").trim() === "" && out.ERP_SECRET_KEY === undefined) {
    throw new Error(
      "ERP_SECRET_KEY belum diisi (wajib untuk serve, termasuk development). Buat sekali dengan `task erpora -- secret:generate`, lalu isi di .env.",
    );
  }
  return out;
}
