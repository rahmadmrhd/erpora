import { describe, expect, test } from "bun:test";
import { badgeText } from "./notification-badge";

describe("badgeText", () => {
  test("angka apa adanya sampai 99, 99+ di atasnya", () => {
    expect(badgeText(1)).toBe("1");
    expect(badgeText(99)).toBe("99");
    expect(badgeText(100)).toBe("99+");
    expect(badgeText(12345)).toBe("99+");
  });
});
