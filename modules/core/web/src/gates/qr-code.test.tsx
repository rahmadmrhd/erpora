import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import { groupSecret } from "./mfa-enroll-screen";
import QrCode, { qrPath } from "./qr-code";

describe("qrPath", () => {
  test("satu persegi panjang per deretan modul gelap berurutan di tiap baris", () => {
    expect(
      qrPath([
        [true, true, false, true],
        [false, false, false, false],
        [true, false, true, true],
      ]),
    ).toBe("M0 0h2v1h-2zM3 0h1v1h-1zM0 2h1v1h-1zM2 2h2v1h-2z");
    expect(qrPath([])).toBe("");
  });
});

describe("QrCode", () => {
  test("dirender sebagai <svg> React berlabel, dengan zona tenang dan kontras tetap hitam di atas putih", () => {
    const { container } = render(
      <QrCode
        text="otpauth://totp/ERPora:ani@example.test?secret=JBSWY3DPEHPK3PXP&issuer=ERPora"
        label="QR"
      />,
    );
    const svg = screen.getByRole("img", { name: "QR" });
    expect(svg.tagName.toLowerCase()).toBe("svg");
    const [, , w, h] = (svg.getAttribute("viewBox") ?? "").split(" ").map(Number);
    expect(w).toBe(h);
    expect(w).toBeGreaterThan(8 + 20); // ukuran matriks + zona tenang 4 modul per sisi
    expect(container.querySelectorAll("rect")).toHaveLength(1);
    expect(container.querySelector("path")?.getAttribute("d")).toMatch(/^M\d+ \d+h\d+v1h-\d+z/);
    expect(container.innerHTML).not.toContain("<script");
  });
});

describe("groupSecret", () => {
  test("mengelompokkan per 4 karakter untuk input manual", () => {
    expect(groupSecret("JBSWY3DPEHPK3PXP")).toBe("JBSW Y3DP EHPK 3PXP");
    expect(groupSecret(" JBSWY3 DPEH ")).toBe("JBSW Y3DP EH");
    expect(groupSecret("")).toBe("");
  });
});
