import { afterEach, describe, expect, test } from "bun:test";
import { writeTimezone } from "../prefs";
import { formatDateTime, formatRelativeTime } from "./time";

afterEach(() => {
  localStorage.clear();
});

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

  test("zona waktu eksplisit menggeser tanggal dan jam (UTC 23:30 menjadi hari berikutnya di Jakarta)", () => {
    const late = "2026-10-15T23:30:00Z";
    const jakarta = formatDateTime(late, "en", "Asia/Jakarta");
    const newYork = formatDateTime(late, "en", "America/New_York");
    expect(jakarta).toContain("Oct 16");
    expect(jakarta).toContain("6:30");
    expect(newYork).toContain("Oct 15");
    expect(newYork).toContain("7:30");
  });

  test("tanpa argumen zona memakai preferensi tersimpan; berubah seketika saat preferensi berubah", () => {
    const late = "2026-10-15T23:30:00Z";
    writeTimezone("Asia/Jakarta");
    expect(formatDateTime(late, "en")).toBe(formatDateTime(late, "en", "Asia/Jakarta"));
    writeTimezone("America/New_York");
    expect(formatDateTime(late, "en")).toBe(formatDateTime(late, "en", "America/New_York"));
    expect(formatDateTime(late, "en")).toContain("Oct 15");
  });

  test("zona tidak valid memakai zona bawaan (tidak melempar)", () => {
    const late = "2026-10-15T23:30:00Z";
    writeTimezone("Asia/Tokyo");
    expect(() => formatDateTime(late, "en", "Bukan/Zona")).not.toThrow();
    expect(formatDateTime(late, "en", "Bukan/Zona")).toBe(formatDateTime(late, "en", "Asia/Tokyo"));
  });

  test("locale tidak valid dengan zona tetap tidak melempar", () => {
    expect(() => formatDateTime("2026-10-15T12:00:00Z", "!!", "Asia/Jakarta")).not.toThrow();
  });
});
