import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import {
  buildLock,
  checkLock,
  hashContent,
  listComponentFiles,
  normalizeCn,
  type ShadcnLock,
} from "../../packages/ui/scripts/shadcn-check";

// Test alat `shadcn-check` (M3a revisi, task 1.2; R1.1, R1.3, D-1): fungsi murni atas pohon sementara,
// tanpa jaringan. Kunci snapshot = hash isi per berkas setelah normalisasi impor `cn`.

const CN_UPSTREAM = 'import { cn } from "cn"\n\nexport const a = cn("x")\n';
const CN_LOCAL = 'import { cn } from "@erpora/ui/lib/utils"\n\nexport const a = cn("x")\n';

let root: string;
let ui: string;

const put = (rel: string, content: string): void => {
  const path = join(ui, rel);
  mkdirSync(join(path, ".."), { recursive: true });
  writeFileSync(path, content);
};

const synced = (lock: ShadcnLock): ShadcnLock => ({ ...lock, synced: true });

beforeEach(() => {
  root = mkdtempSync(join(tmpdir(), "shadcn-check-"));
  ui = join(root, "components", "ui");
  mkdirSync(ui, { recursive: true });
});

afterEach(() => {
  rmSync(root, { recursive: true, force: true });
});

describe("normalizeCn dan hashContent", () => {
  test("from cn dan from @erpora/ui/lib/utils menghasilkan hash yang sama (e)", () => {
    expect(normalizeCn(CN_UPSTREAM)).toBe(CN_LOCAL);
    expect(hashContent(CN_UPSTREAM)).toBe(hashContent(CN_LOCAL));
  });

  test("kutip tunggal ikut dinormalkan; modul lain bernama mirip tidak disentuh", () => {
    expect(normalizeCn("import { cn } from 'cn'")).toBe(
      'import { cn } from "@erpora/ui/lib/utils"',
    );
    expect(normalizeCn('import x from "cn-extra"')).toBe('import x from "cn-extra"');
    expect(normalizeCn('import x from "@scope/cn"')).toBe('import x from "@scope/cn"');
  });

  test("akhir baris CRLF dan LF menghasilkan hash yang sama (checkout Windows)", () => {
    expect(hashContent("a\r\nb\r\n")).toBe(hashContent("a\nb\n"));
  });

  test("hash berubah bila satu karakter berubah", () => {
    expect(hashContent(CN_LOCAL)).not.toBe(hashContent(CN_LOCAL.replace('"x"', '"y"')));
  });
});

describe("listComponentFiles", () => {
  test("hanya .tsx komponen; tes dan subfolder diabaikan (f)", () => {
    put("button.tsx", CN_LOCAL);
    put("button.test.tsx", "test");
    put("util.test.ts", "test");
    put("notes.md", "x");
    expect(listComponentFiles(ui)).toEqual(["button.tsx"]);
  });

  test("hasil terurut", () => {
    put("zeta.tsx", CN_LOCAL);
    put("alpha.tsx", CN_LOCAL);
    expect(listComponentFiles(ui)).toEqual(["alpha.tsx", "zeta.tsx"]);
  });
});

describe("checkLock", () => {
  test("(a) berkas sama dengan kunci lolos", () => {
    put("button.tsx", CN_LOCAL);
    put("badge.tsx", CN_LOCAL.replace('"x"', '"b"'));
    const lock = synced(buildLock(ui, "4.21.5"));
    expect(checkLock(ui, lock)).toEqual({ skipped: false, problems: [] });
  });

  test("berkas hasil CLI (from cn) lolos terhadap kunci yang dibuat dari berkas lokal", () => {
    put("button.tsx", CN_LOCAL);
    const lock = synced(buildLock(ui, "4.21.5"));
    put("button.tsx", CN_UPSTREAM);
    expect(checkLock(ui, lock).problems).toEqual([]);
  });

  test("(b) satu karakter berubah gagal; pesan menyebut komponen dan task shadcn:sync", () => {
    put("button.tsx", CN_LOCAL);
    put("badge.tsx", CN_LOCAL);
    const lock = synced(buildLock(ui, "4.21.5"));
    put("button.tsx", CN_LOCAL.replace('"x"', '"y"'));
    const res = checkLock(ui, lock);
    expect(res.skipped).toBe(false);
    expect(res.problems).toHaveLength(1);
    expect(res.problems[0]).toContain("button");
    expect(res.problems[0]).toContain("task shadcn:sync");
  });

  test("(c) berkas ada tanpa entri kunci gagal", () => {
    put("button.tsx", CN_LOCAL);
    const lock = synced(buildLock(ui, "4.21.5"));
    put("card.tsx", CN_LOCAL);
    const res = checkLock(ui, lock);
    expect(res.problems).toHaveLength(1);
    expect(res.problems[0]).toContain("card");
    expect(res.problems[0]).toContain("task shadcn:sync");
  });

  test("(d) entri kunci tanpa berkas gagal", () => {
    put("button.tsx", CN_LOCAL);
    put("card.tsx", CN_LOCAL);
    const lock = synced(buildLock(ui, "4.21.5"));
    rmSync(join(ui, "card.tsx"));
    const res = checkLock(ui, lock);
    expect(res.problems).toHaveLength(1);
    expect(res.problems[0]).toContain("card");
    expect(res.problems[0]).toContain("task shadcn:sync");
  });

  test("beberapa masalah dilaporkan sekaligus", () => {
    put("a.tsx", CN_LOCAL);
    put("b.tsx", CN_LOCAL);
    const lock = synced(buildLock(ui, "4.21.5"));
    put("a.tsx", "diubah");
    rmSync(join(ui, "b.tsx"));
    put("c.tsx", CN_LOCAL);
    expect(checkLock(ui, lock).problems).toHaveLength(3);
  });

  test("(f) berkas di luar components/ui (mis. extended/) diabaikan", () => {
    put("button.tsx", CN_LOCAL);
    const lock = synced(buildLock(ui, "4.21.5"));
    const extended = join(root, "components", "extended");
    mkdirSync(extended, { recursive: true });
    writeFileSync(join(extended, "loading-button.tsx"), "bebas diubah");
    put("button.test.tsx", "tes tidak terkunci");
    expect(checkLock(ui, lock).problems).toEqual([]);
  });

  test("kunci belum disinkronkan (synced: false) dilewati dengan peringatan, bukan gagal", () => {
    put("button.tsx", "menyimpang dari registry");
    const lock: ShadcnLock = { cli: "4.21.5", synced: false, components: {} };
    const res = checkLock(ui, lock);
    expect(res.skipped).toBe(true);
    expect(res.problems).toEqual([]);
  });
});

describe("buildLock", () => {
  test("memuat versi CLI, nama komponen, berkas, dan sha256 atas isi ternormalisasi", () => {
    put("button.tsx", CN_UPSTREAM);
    const lock = buildLock(ui, "4.21.5");
    expect(lock.cli).toBe("4.21.5");
    expect(lock.components.button).toEqual({
      file: "button.tsx",
      sha256: hashContent(CN_LOCAL),
    });
  });

  test("lock otomatis synced bila semua berkas komponen terkunci", () => {
    put("button.tsx", CN_LOCAL);
    expect(buildLock(ui, "4.21.5").synced).toBe(true);
  });

  test("subset: komponen yang belum disinkronkan membuat kunci tetap belum synced", () => {
    put("button.tsx", CN_LOCAL);
    put("sidebar.tsx", "lama, menyimpang");
    const lock = buildLock(ui, "4.21.5", { only: ["button"] });
    expect(Object.keys(lock.components)).toEqual(["button"]);
    expect(lock.synced).toBe(false);
  });

  test("subset digabung dengan kunci sebelumnya dan menjadi synced setelah semua terkunci", () => {
    put("button.tsx", CN_LOCAL);
    put("sidebar.tsx", "lama");
    const first = buildLock(ui, "4.21.5", { only: ["button"] });
    put("sidebar.tsx", CN_LOCAL.replace('"x"', '"s"'));
    const second = buildLock(ui, "4.21.5", { only: ["sidebar"], previous: first });
    expect(Object.keys(second.components)).toEqual(["button", "sidebar"]);
    expect(second.synced).toBe(true);
  });
});

// ---- Patch label deklaratif dan kunci hooks (keputusan pemilik produk 31 Okt 2026) ----

import {
  applyPatches,
  type LabelPatch,
  patchProblems,
} from "../../packages/ui/scripts/shadcn-check";

const PATCH: LabelPatch = {
  component: "dialog",
  find: '<span className="sr-only">Close</span>',
  replace: '<span className="sr-only"><UiLabel k="actions.close" /></span>',
  reason: "label via useUiLabel",
};
const RAW = `${CN_LOCAL}export const x = <span className="sr-only">Close</span>\n`;

describe("applyPatches", () => {
  test("mengganti string asal pada komponen yang cocok", () => {
    const out = applyPatches("dialog", RAW, [PATCH]);
    expect(out).toContain('<UiLabel k="actions.close" />');
    expect(out).not.toContain(">Close<");
  });

  test("idempoten: patch yang sudah terpasang tidak diterapkan dua kali", () => {
    const once = applyPatches("dialog", RAW, [PATCH]);
    expect(applyPatches("dialog", once, [PATCH])).toBe(once);
  });

  test("patch komponen lain diabaikan", () => {
    expect(applyPatches("sheet", RAW, [PATCH])).toBe(RAW);
  });

  test("string asal tidak ditemukan (registry berubah) melempar dengan nama komponen", () => {
    expect(() => applyPatches("dialog", CN_LOCAL, [PATCH])).toThrow(/dialog/);
  });

  test("jumlah kemunculan harus sama dengan count (default 1)", () => {
    const twice = `${RAW}${RAW}`;
    expect(() => applyPatches("dialog", twice, [PATCH])).toThrow(/2/);
    expect(applyPatches("dialog", twice, [{ ...PATCH, count: 2 }]).match(/UiLabel/g)).toHaveLength(
      2,
    );
  });

  test("patch yang string asalnya bagian dari pengganti (sisipan impor) idempoten", () => {
    const imp: LabelPatch = {
      component: "dialog",
      find: 'import * as React from "react"',
      replace: 'import * as React from "react"\nimport { UiLabel } from "../../labels"',
    };
    const src = 'import * as React from "react"\nconst a = 1\n';
    const once = applyPatches("dialog", src, [imp]);
    expect(once).toContain('import { UiLabel } from "../../labels"');
    expect(applyPatches("dialog", once, [imp])).toBe(once);
  });
});

describe("patchProblems", () => {
  test("patch terpasang: tidak ada masalah", () => {
    expect(patchProblems("dialog", applyPatches("dialog", RAW, [PATCH]), [PATCH])).toEqual([]);
  });

  test("patch tidak terpasang: masalah menyebut komponen dan task shadcn:sync", () => {
    const res = patchProblems("dialog", RAW, [PATCH]);
    expect(res).toHaveLength(1);
    expect(res[0]).toContain("dialog");
    expect(res[0]).toContain("task shadcn:sync");
  });
});

describe("kunci hooks dan patch pada checkLock/buildLock", () => {
  const putHook = (name: string, content: string): void => {
    mkdirSync(join(root, "hooks"), { recursive: true });
    writeFileSync(join(root, "hooks", name), content);
  };
  const hooksDir = (): string => join(root, "hooks");

  test("hook dikunci dengan kunci hooks/<nama>; berkas berubah gagal", () => {
    put("button.tsx", CN_LOCAL);
    putHook("use-mobile.ts", "export const a = 1\n");
    const lock = buildLock(ui, "4.21.5", { hooksDir: hooksDir() });
    expect(Object.keys(lock.components)).toEqual(["button", "hooks/use-mobile"]);
    expect(lock.components["hooks/use-mobile"]?.file).toBe("use-mobile.ts");
    expect(checkLock(ui, lock, { hooksDir: hooksDir() }).problems).toEqual([]);
    putHook("use-mobile.ts", "export const a = 2\n");
    const res = checkLock(ui, lock, { hooksDir: hooksDir() });
    expect(res.problems).toHaveLength(1);
    expect(res.problems[0]).toContain("hooks/use-mobile");
  });

  test("hook tanpa entri kunci gagal", () => {
    put("button.tsx", CN_LOCAL);
    const lock = buildLock(ui, "4.21.5", { hooksDir: hooksDir() });
    putHook("use-mobile.ts", "x");
    expect(checkLock(ui, lock, { hooksDir: hooksDir() }).problems[0]).toContain("hooks/use-mobile");
  });

  test("patch tidak terpasang pada berkas yang hash-nya cocok tetap gagal", () => {
    put("dialog.tsx", RAW);
    const lock = buildLock(ui, "4.21.5"); // kunci dibuat dari berkas tanpa patch
    const res = checkLock(ui, lock, { patches: [PATCH] });
    expect(res.problems).toHaveLength(1);
    expect(res.problems[0]).toContain("task shadcn:sync");
  });

  test("patch terpasang dan hash cocok lolos", () => {
    put("dialog.tsx", applyPatches("dialog", RAW, [PATCH]));
    const lock = buildLock(ui, "4.21.5");
    expect(checkLock(ui, lock, { patches: [PATCH] }).problems).toEqual([]);
  });

  test("patch menunjuk komponen tanpa berkas gagal", () => {
    put("button.tsx", CN_LOCAL);
    const lock = buildLock(ui, "4.21.5");
    const res = checkLock(ui, lock, { patches: [PATCH] });
    expect(res.problems.some((p) => p.includes("dialog"))).toBe(true);
  });
});
