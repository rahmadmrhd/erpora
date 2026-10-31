import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { UiLabelsProvider } from "../../labels";
import { LoadingButton } from "./loading-button";

describe("LoadingButton (pembungkus Button + Spinner)", () => {
  test("tanpa loading: tombol biasa tanpa aria-busy dan tanpa spinner", () => {
    render(<LoadingButton>Simpan</LoadingButton>);
    const b = screen.getByRole("button", { name: "Simpan" });
    expect(b).not.toHaveAttribute("aria-busy");
    expect(screen.queryByRole("status")).toBeNull();
    expect(b).toHaveAttribute("data-slot", "button");
  });

  test("loading: aria-busy, Spinner tampil, tombol nonaktif dan tidak dapat diklik", async () => {
    const user = userEvent.setup();
    let n = 0;
    render(
      <LoadingButton loading onClick={() => n++}>
        Menyimpan
      </LoadingButton>,
    );
    const b = screen.getByRole("button", { name: /Menyimpan/ });
    expect(b).toHaveAttribute("aria-busy", "true");
    expect(b).toBeDisabled();
    expect(screen.getByRole("status")).toBeInTheDocument();
    await user.click(b);
    expect(n).toBe(0);
  });

  test("label Spinner berasal dari UiLabels (status.loading), bukan teks Inggris tertanam", () => {
    render(
      <UiLabelsProvider t={(k) => (k === "status.loading" ? "Memuat…" : k)}>
        <LoadingButton loading>Simpan</LoadingButton>
      </UiLabelsProvider>,
    );
    expect(screen.getByRole("status", { name: "Memuat…" })).toBeInTheDocument();
  });

  test("meneruskan varian dan ukuran Button", () => {
    render(
      <LoadingButton variant="destructive" size="sm">
        Hapus
      </LoadingButton>,
    );
    const b = screen.getByRole("button", { name: "Hapus" });
    expect(b).toHaveAttribute("data-variant", "destructive");
    expect(b).toHaveAttribute("data-size", "sm");
  });
});
