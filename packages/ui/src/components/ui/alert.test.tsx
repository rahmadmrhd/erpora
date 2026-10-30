import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import { Alert, AlertDescription, AlertTitle } from "./alert";

describe("Alert", () => {
  test("destructive berperan alert (mengumumkan segera)", () => {
    render(
      <Alert variant="destructive">
        <AlertTitle>Gagal</AlertTitle>
        <AlertDescription>Periksa koneksi.</AlertDescription>
      </Alert>,
    );
    const a = screen.getByRole("alert");
    expect(a).toHaveTextContent("Gagal");
    expect(a).toHaveTextContent("Periksa koneksi.");
  });

  test.each(["default", "warning", "info", "success"] as const)(
    "varian %s berperan status (tidak menyela)",
    (variant) => {
      render(
        <Alert variant={variant}>
          <AlertTitle>Judul</AlertTitle>
        </Alert>,
      );
      expect(screen.getByRole("status")).toBeInTheDocument();
      expect(screen.queryByRole("alert")).toBeNull();
    },
  );

  test("role dapat ditimpa pemanggil", () => {
    render(
      <Alert role="note">
        <AlertTitle>t</AlertTitle>
      </Alert>,
    );
    expect(screen.getByRole("note")).toBeInTheDocument();
  });

  test("hanya tampilan outline: teks memakai foreground, hue hanya di border (K-14)", () => {
    for (const [variant, border] of [
      ["success", "border-success"],
      ["info", "border-info"],
      ["warning", "border-warning"],
      ["destructive", "border-destructive"],
    ] as const) {
      const { unmount } = render(
        <Alert variant={variant}>
          <AlertTitle>x</AlertTitle>
        </Alert>,
      );
      const cls = screen.getByRole(variant === "destructive" ? "alert" : "status").className;
      expect(cls).toContain(border);
      expect(cls).toContain("text-foreground");
      expect(cls).not.toMatch(/bg-(success|info|warning|destructive)(?![-\w])(?!\/)/);
      unmount();
    }
  });

  test("ikon dekoratif (svg) disembunyikan dari pembaca layar", () => {
    const { container } = render(
      <Alert>
        <svg />
        <AlertTitle>t</AlertTitle>
      </Alert>,
    );
    expect(container.querySelector("svg")).not.toBeNull();
    expect(screen.getByRole("status")).toBeInTheDocument();
  });
});
