import { describe, expect, test } from "bun:test";
import { formatDateTime, formatRelativeTime } from "./time";

const NOW = Date.parse("2026-10-29T12:00:00Z");
const at = (ms: number) => new Date(NOW + ms).toISOString();
const MIN = 60_000;
const HOUR = 60 * MIN;
const DAY = 24 * HOUR;

describe("formatRelativeTime", () => {
  test("en: menit, jam, hari lampau dan mendatang", () => {
    expect(formatRelativeTime(at(-5 * MIN), "en", NOW)).toBe("5 minutes ago");
    expect(formatRelativeTime(at(-3 * HOUR), "en", NOW)).toBe("3 hours ago");
    expect(formatRelativeTime(at(2 * HOUR), "en", NOW)).toBe("in 2 hours");
    expect(formatRelativeTime(at(-DAY), "en", NOW)).toBe("yesterday");
    expect(formatRelativeTime(at(-5 * DAY), "en", NOW)).toBe("5 days ago");
  });

  test("id: bahasa Indonesia", () => {
    expect(formatRelativeTime(at(-5 * MIN), "id", NOW)).toBe("5 menit yang lalu");
    expect(formatRelativeTime(at(-DAY), "id", NOW)).toBe("kemarin");
    expect(formatRelativeTime(at(3 * HOUR), "id", NOW)).toBe("dalam 3 jam");
  });

  test("selisih sangat kecil menjadi 'sekarang/now'", () => {
    expect(formatRelativeTime(at(-2_000), "en", NOW)).toBe("now");
    expect(formatRelativeTime(at(0), "id", NOW)).toBe("sekarang");
  });

  test("detik, bulan, dan tahun memilih satuan yang sesuai", () => {
    expect(formatRelativeTime(at(-30_000), "en", NOW)).toBe("30 seconds ago");
    expect(formatRelativeTime(at(-70 * DAY), "en", NOW)).toBe("2 months ago");
    expect(formatRelativeTime(at(-800 * DAY), "en", NOW)).toBe("2 years ago");
  });

  test("masukan bukan ISO yang valid tidak melempar dan dikembalikan apa adanya", () => {
    expect(formatRelativeTime("bukan tanggal", "en", NOW)).toBe("bukan tanggal");
    expect(formatRelativeTime("", "en", NOW)).toBe("");
  });

  test("locale tak valid tidak melempar", () => {
    expect(() => formatRelativeTime(at(-MIN), "!!", NOW)).not.toThrow();
  });
});

describe("formatDateTime", () => {
  const iso = "2026-10-15T12:00:00Z";

  test("memuat tahun dan mengikuti locale", () => {
    const en = formatDateTime(iso, "en");
    const id = formatDateTime(iso, "id");
    expect(en).toContain("2026");
    expect(id).toContain("2026");
    expect(en).toContain("Oct");
    expect(id).toContain("Okt");
  });

  test("masukan tidak valid dikembalikan apa adanya", () => {
    expect(formatDateTime("xx", "en")).toBe("xx");
  });
});
