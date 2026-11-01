import { describe, expect, test } from "bun:test";
import { formatDecimalString } from "./format";

describe("format desimal hanya lewat utilitas desimal dan Intl dengan string (Requirement 18.7)", () => {
  test("en: pemisah ribuan dan desimal", () => {
    expect(formatDecimalString("1234567.5", "en", { scale: 2 })).toBe("1,234,567.50");
    expect(formatDecimalString("0", "en")).toBe("0");
    expect(formatDecimalString("-1234.5", "en", { scale: 1 })).toBe("-1,234.5");
  });

  test("id: titik ribuan dan koma desimal", () => {
    expect(formatDecimalString("1234567.5", "id", { scale: 2 })).toBe("1.234.567,50");
    expect(formatDecimalString("-12.5", "id", { scale: 2 })).toBe("-12,50");
  });

  test("presisi di luar jangkauan float tetap eksak", () => {
    expect(formatDecimalString("123456789012345678901234567890.12", "en")).toBe(
      "123,456,789,012,345,678,901,234,567,890.12",
    );
    expect(formatDecimalString("0.1000000000000000055511151231257827", "en")).toBe(
      "0.1000000000000000055511151231257827",
    );
    expect(formatDecimalString("9007199254740993", "en")).toBe("9,007,199,254,740,993");
  });

  test("skala: pad ke skala field, tidak pernah memotong digit nilai", () => {
    expect(formatDecimalString("1.5", "en", { scale: 3 })).toBe("1.500");
    expect(formatDecimalString("1.23456", "en", { scale: 2 })).toBe("1.23456");
    expect(formatDecimalString("12", "en", { scale: 0 })).toBe("12");
  });

  test("nol negatif tidak bertanda minus", () => {
    expect(formatDecimalString("-0.00", "en", { scale: 2 })).toBe("0.00");
  });

  test("mata uang: simbol dari Intl, digit dari utilitas desimal", () => {
    const s = formatDecimalString("1234567.5", "en", { scale: 2, currency: "USD" });
    expect(s).toContain("1,234,567.50");
    expect(s).toContain("$");
    const idr = formatDecimalString("1234567", "id", { scale: 0, currency: "IDR" });
    expect(idr).toContain("1.234.567");
    expect(idr).toContain("Rp");
  });

  test("masukan bukan desimal kanonik: null (tidak ada konversi Number diam-diam)", () => {
    for (const bad of ["", "abc", "1e5", "+1", ".5", "1.", "01"])
      expect(formatDecimalString(bad, "en")).toBeNull();
    expect(formatDecimalString(1.5 as unknown as string, "en")).toBeNull();
  });

  test("bahasa tak dikenal tidak melempar", () => {
    expect(formatDecimalString("1234.5", "zz-INVALID-@@", { scale: 1 })).toBe("1,234.5");
  });
});
