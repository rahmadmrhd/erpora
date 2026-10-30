import { afterEach, beforeEach, describe, expect, spyOn, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Button } from "./button";

describe("Button", () => {
  test("merender <button type=button> dengan label dan varian bawaan primary ukuran md", () => {
    render(<Button>Simpan</Button>);
    const b = screen.getByRole("button", { name: "Simpan" });
    expect(b).toHaveAttribute("type", "button");
    expect(b).toHaveAttribute("data-variant", "primary");
    expect(b).toHaveAttribute("data-size", "md");
    expect(b.className).toContain("bg-primary");
    expect(b.className).toContain("h-control-md");
  });

  test.each([
    ["primary", "bg-primary"],
    ["secondary", "bg-secondary"],
    ["outline", "border-input"],
    ["ghost", "hover:bg-accent"],
    ["destructive", "bg-destructive"],
  ] as const)("varian %s memakai token %s", (variant, token) => {
    render(<Button variant={variant}>x</Button>);
    const b = screen.getByRole("button");
    expect(b).toHaveAttribute("data-variant", variant);
    expect(b.className).toContain(token);
  });

  test.each([
    ["sm", "h-control-sm"],
    ["md", "h-control-md"],
    ["lg", "h-control-lg"],
  ] as const)("ukuran %s memakai tinggi token %s", (size, cls) => {
    render(<Button size={size}>x</Button>);
    expect(screen.getByRole("button").className).toContain(cls);
  });

  test("ukuran icon berbentuk persegi 34 px dan memakai aria-label", () => {
    render(
      <Button size="icon" aria-label="Tutup">
        i
      </Button>,
    );
    const b = screen.getByRole("button", { name: "Tutup" });
    expect(b.className).toContain("size-control-md");
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

  test("loading: aria-busy, LoadingIcon tampil, dan tombol tidak dapat diklik", async () => {
    const user = userEvent.setup();
    let n = 0;
    render(
      <Button loading onClick={() => n++}>
        Menyimpan
      </Button>,
    );
    const b = screen.getByRole("button", { name: /Menyimpan/ });
    expect(b).toHaveAttribute("aria-busy", "true");
    expect(b.querySelector("svg circle")).not.toBeNull();
    await user.click(b);
    expect(n).toBe(0);
  });

  test("asChild merender elemen anak (tautan) dengan gaya tombol", () => {
    render(
      <Button asChild variant="outline">
        <a href="/x">Tautan</a>
      </Button>,
    );
    const a = screen.getByRole("link", { name: "Tautan" });
    expect(a.className).toContain("border-input");
  });

  test("tanpa kelas palet langsung: gaya fokus memakai garis global 2 px (bukan ring 3 px shadcn)", () => {
    render(<Button>x</Button>);
    expect(screen.getByRole("button").className).not.toContain("ring-[3px]");
  });
});

describe("Button size=icon tanpa aria-label", () => {
  let warn: ReturnType<typeof spyOn>;
  beforeEach(() => {
    warn = spyOn(console, "warn").mockImplementation(() => {});
  });
  afterEach(() => warn.mockRestore());

  test("memicu peringatan dev", () => {
    render(<Button size="icon">i</Button>);
    expect(warn).toHaveBeenCalled();
    expect(String(warn.mock.calls[0]?.[0])).toContain("aria-label");
  });

  test("dengan aria-label atau aria-labelledby tidak memperingatkan", () => {
    render(
      <>
        <Button size="icon" aria-label="A">
          i
        </Button>
        <Button size="icon" aria-labelledby="x">
          i
        </Button>
      </>,
    );
    expect(warn).not.toHaveBeenCalled();
  });
});
