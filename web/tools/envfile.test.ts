import { describe, expect, test } from "bun:test";
import { parseDotenv, resolveSecretEnv } from "./envfile";

describe("parseDotenv (sejalan godotenv)", () => {
  test("komentar baris, baris kosong, export, dan CRLF", () => {
    const text = "# komentar\r\n\r\nexport A=1\r\nB = dua \r\n";
    expect(parseDotenv(text)).toEqual({ A: "1", B: "dua" });
  });

  test("komentar inline di luar kutip dibuang; # tanpa spasi di depannya tetap bagian nilai", () => {
    const text =
      "A=abc # catatan\nB=abc#tetap\nC=\"x # di dalam kutip\" # catatan\nD='y # literal' # catatan\n";
    expect(parseDotenv(text)).toEqual({
      A: "abc",
      B: "abc#tetap",
      C: "x # di dalam kutip",
      D: "y # literal",
    });
  });

  test("entri ganda: yang terakhir menang", () => {
    expect(parseDotenv("A=1\nA=2\nA=3\n")).toEqual({ A: "3" });
  });

  test("nilai kosong, kutip ganda dengan escape, kutip tunggal literal, BOM", () => {
    const text = '\uFEFFE=\nF="a\\nb\\"c"\nG=\'a\\nb\'\n';
    expect(parseDotenv(text)).toEqual({ E: "", F: 'a\nb"c', G: "a\\nb" });
  });
});

describe("resolveSecretEnv", () => {
  const key = `${"A".repeat(43)}=`;
  const prev = `${"B".repeat(43)}=`;

  test("environment proses menang atas .env, dan tidak ada override bila sudah terisi", () => {
    const out = resolveSecretEnv({ ERP_SECRET_KEY: key }, `ERP_SECRET_KEY=${prev}\n`);
    expect(out).toEqual({});
  });

  test("kunci dan kunci lama diteruskan dari .env (komentar inline dan duplikat ditangani)", () => {
    const text = `ERP_SECRET_KEY=lama # sementara\nERP_SECRET_KEY=${key} # dipakai\nERP_SECRET_KEY_PREVIOUS="${prev}"\n`;
    expect(resolveSecretEnv({}, text)).toEqual({
      ERP_SECRET_KEY: key,
      ERP_SECRET_KEY_PREVIOUS: prev,
    });
  });

  test("PREVIOUS dari .env diteruskan walau ERP_SECRET_KEY datang dari proses", () => {
    expect(resolveSecretEnv({ ERP_SECRET_KEY: key }, `ERP_SECRET_KEY_PREVIOUS=${prev}\n`)).toEqual({
      ERP_SECRET_KEY_PREVIOUS: prev,
    });
  });

  test("tanpa kunci di proses maupun .env: galat yang menunjuk secret:generate (tanpa nilai)", () => {
    expect(() => resolveSecretEnv({}, "ERP_SECRET_KEY= # kosong\n")).toThrow(/secret:generate/);
    expect(() => resolveSecretEnv({}, undefined)).toThrow(/secret:generate/);
  });
});
