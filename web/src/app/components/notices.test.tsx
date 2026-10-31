import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import { EmptyNotice, ErrorNotice } from "./notices";

describe("EmptyNotice (komposisi Empty shadcn)", () => {
  test("judul, deskripsi, dan aksi dirender; ikon dekoratif tersembunyi dari pembaca layar", () => {
    const { container } = render(
      <EmptyNotice
        icon="bell"
        title="Belum ada notifikasi"
        description="Semua sudah dibaca."
        action={<button type="button">Muat ulang</button>}
      />,
    );
    expect(screen.getByText("Belum ada notifikasi")).toBeInTheDocument();
    expect(screen.getByText("Semua sudah dibaca.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Muat ulang" })).toBeInTheDocument();
    expect(container.querySelector("svg")).toHaveAttribute("aria-hidden", "true");
  });

  test("tanpa ikon, deskripsi, dan aksi hanya judul", () => {
    const { container } = render(<EmptyNotice title="Kosong" />);
    expect(screen.getByText("Kosong")).toBeInTheDocument();
    expect(container.querySelector("svg")).toBeNull();
  });
});

describe("ErrorNotice (halaman galat dari Empty shadcn)", () => {
  test("judul berperan heading level 2 (satu h1 per halaman: breadcrumb), deskripsi, dan aksi; status menjadi data-status", () => {
    const { container } = render(
      <ErrorNotice
        status={404}
        title="Halaman tidak ditemukan"
        description="Periksa alamatnya."
        actions={<a href="/">Beranda</a>}
      />,
    );
    expect(
      screen.getByRole("heading", { level: 2, name: "Halaman tidak ditemukan" }),
    ).toBeVisible();
    expect(screen.getByText("Periksa alamatnya.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Beranda" })).toBeInTheDocument();
    expect(container.firstElementChild).toHaveAttribute("data-status", "404");
  });
});
