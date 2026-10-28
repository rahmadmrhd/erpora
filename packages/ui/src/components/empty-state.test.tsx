import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import { EmptyState } from "./empty-state";

describe("EmptyState", () => {
  test("judul, deskripsi, dan aksi", () => {
    render(
      <EmptyState
        title="Belum ada notifikasi"
        description="Notifikasi baru muncul di sini."
        action={<button type="button">Muat ulang</button>}
      />,
    );
    expect(screen.getByText("Belum ada notifikasi")).toBeInTheDocument();
    expect(screen.getByText("Notifikasi baru muncul di sini.").className).toContain(
      "text-muted-foreground",
    );
    expect(screen.getByRole("button", { name: "Muat ulang" })).toBeInTheDocument();
  });

  test("ikon terdaftar tampil sebagai svg dekoratif", () => {
    const { container } = render(<EmptyState icon="bell" title="Kosong" />);
    const svg = container.querySelector("svg");
    expect(svg).not.toBeNull();
    expect(svg).toHaveAttribute("aria-hidden", "true");
  });

  test("tanpa ikon atau ikon asing tidak merender svg", () => {
    const { container, rerender } = render(<EmptyState title="Kosong" />);
    expect(container.querySelector("svg")).toBeNull();
    // @ts-expect-error nama ikon di luar IconName ditolak tipe; runtime tetap aman
    rerender(<EmptyState icon="tidak-ada" title="Kosong" />);
    expect(container.querySelector("svg")).toBeNull();
  });

  test("tanpa deskripsi dan aksi hanya judul", () => {
    const { container } = render(<EmptyState title="Kosong" />);
    expect(container.textContent).toBe("Kosong");
  });
});
