import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import { ErrorPage } from "./error-page";

describe("ErrorPage", () => {
  test("judul sebagai heading level 1, deskripsi, dan aksi", () => {
    render(
      <ErrorPage
        title="404: Halaman tidak ditemukan"
        description="Halaman yang dicari tidak ada."
        actions={<a href="/">Ke beranda</a>}
        status={404}
      />,
    );
    expect(
      screen.getByRole("heading", { level: 1, name: "404: Halaman tidak ditemukan" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Halaman yang dicari tidak ada.").className).toContain(
      "text-muted-foreground",
    );
    expect(screen.getByRole("link", { name: "Ke beranda" })).toBeInTheDocument();
  });

  test.each([403, 404, 500, 503] as const)("status %i dicatat pada data-status", (status) => {
    const { container } = render(<ErrorPage title="x" status={status} />);
    expect(container.firstElementChild).toHaveAttribute("data-status", String(status));
  });

  test("tanpa status, deskripsi, atau aksi tetap valid", () => {
    const { container } = render(<ErrorPage title="Galat" />);
    expect(container.firstElementChild).not.toHaveAttribute("data-status");
    expect(screen.getByRole("heading", { name: "Galat" })).toBeInTheDocument();
  });

  test("tidak menanam teks tampilan: semua teks dari props", () => {
    const { container } = render(<ErrorPage title="T" />);
    expect(container.textContent).toBe("T");
  });
});
