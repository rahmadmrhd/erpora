import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "./empty";

describe("Empty", () => {
  test("komposisi: judul, deskripsi, media, dan konten aksi dirender", () => {
    render(
      <Empty>
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <svg aria-hidden="true" />
          </EmptyMedia>
          <EmptyTitle>Belum ada notifikasi</EmptyTitle>
          <EmptyDescription>Semua sudah dibaca.</EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <button type="button">Muat ulang</button>
        </EmptyContent>
      </Empty>,
    );
    expect(screen.getByText("Belum ada notifikasi")).toHaveAttribute("data-slot", "empty-title");
    expect(screen.getByText("Semua sudah dibaca.")).toHaveAttribute(
      "data-slot",
      "empty-description",
    );
    expect(screen.getByRole("button", { name: "Muat ulang" })).toBeInTheDocument();
    expect(document.querySelector('[data-slot="empty-icon"]')).toHaveAttribute(
      "data-variant",
      "icon",
    );
  });

  test("className pemanggil digabung pada akar", () => {
    const { container } = render(<Empty className="min-h-40">x</Empty>);
    expect(container.firstElementChild?.className).toContain("min-h-40");
    expect(container.firstElementChild).toHaveAttribute("data-slot", "empty");
  });
});
