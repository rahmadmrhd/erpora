import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Button, buttonVariants } from "./button";

// API shadcn terbaru (R1.2): varian default|destructive|outline|secondary|ghost|link, ukuran
// default|xs|sm|lg|icon|icon-xs|icon-sm|icon-lg. Tes memeriksa perilaku (peran ARIA, keyboard, atribut data-*),
// bukan kelas Tailwind internal.
describe("Button", () => {
  test("merender <button> dengan varian dan ukuran bawaan default", () => {
    render(<Button>Simpan</Button>);
    const b = screen.getByRole("button", { name: "Simpan" });
    expect(b).toHaveAttribute("data-slot", "button");
    expect(b).toHaveAttribute("data-variant", "default");
    expect(b).toHaveAttribute("data-size", "default");
  });

  test.each(["default", "destructive", "outline", "secondary", "ghost", "link"] as const)(
    "varian %s ditandai data-variant",
    (variant) => {
      render(<Button variant={variant}>x</Button>);
      expect(screen.getByRole("button")).toHaveAttribute("data-variant", variant);
    },
  );

  test.each(["default", "xs", "sm", "lg", "icon", "icon-xs", "icon-sm", "icon-lg"] as const)(
    "ukuran %s ditandai data-size",
    (size) => {
      render(
        <Button size={size} aria-label="x">
          i
        </Button>,
      );
      expect(screen.getByRole("button")).toHaveAttribute("data-size", size);
    },
  );

  test("buttonVariants menghasilkan kelas berbeda untuk varian dan ukuran berbeda", () => {
    expect(buttonVariants({ variant: "default" })).not.toBe(buttonVariants({ variant: "ghost" }));
    expect(buttonVariants({ size: "sm" })).not.toBe(buttonVariants({ size: "lg" }));
  });

  test("varian dan ukuran buatan lama (primary, md) bukan bagian API: ditolak tipe", () => {
    // @ts-expect-error `primary` bukan varian shadcn
    const bad = <Button variant="primary">p</Button>;
    // @ts-expect-error `md` bukan ukuran shadcn
    const bad2 = <Button size="md">m</Button>;
    expect(bad).toBeDefined();
    expect(bad2).toBeDefined();
  });

  test("klik dan keyboard (Enter, Spasi) memicu onClick", async () => {
    const user = userEvent.setup();
    let n = 0;
    render(<Button onClick={() => n++}>Aksi</Button>);
    await user.click(screen.getByRole("button"));
    screen.getByRole("button").focus();
    await user.keyboard("{Enter}");
    await user.keyboard(" ");
    expect(n).toBe(3);
  });

  test("disabled tidak memicu onClick", async () => {
    const user = userEvent.setup();
    let n = 0;
    render(
      <Button disabled onClick={() => n++}>
        x
      </Button>,
    );
    await user.click(screen.getByRole("button"));
    expect(n).toBe(0);
    expect(screen.getByRole("button")).toBeDisabled();
  });

  test("asChild merender elemen anak (tautan) dengan atribut data tombol", () => {
    render(
      <Button asChild variant="outline">
        <a href="/x">Tautan</a>
      </Button>,
    );
    const a = screen.getByRole("link", { name: "Tautan" });
    expect(a).toHaveAttribute("data-slot", "button");
    expect(a).toHaveAttribute("data-variant", "outline");
  });

  test("tanpa prop loading: aria-busy tidak diset (perilaku loading pindah ke LoadingButton)", () => {
    render(<Button>Biasa</Button>);
    expect(screen.getByRole("button")).not.toHaveAttribute("aria-busy");
  });
});
