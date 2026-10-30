import { describe, expect, test } from "bun:test";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "./dropdown-menu";

function Basic({ onSelect }: { onSelect?: (v: string) => void }) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger>Menu</DropdownMenuTrigger>
      <DropdownMenuContent>
        <DropdownMenuLabel>Akun</DropdownMenuLabel>
        <DropdownMenuItem onSelect={() => onSelect?.("kelola")}>Kelola Akun</DropdownMenuItem>
        <DropdownMenuItem onSelect={() => onSelect?.("bantuan")}>Bantuan</DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem variant="destructive" onSelect={() => onSelect?.("keluar")}>
          Keluar
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

describe("DropdownMenu", () => {
  test("dibuka dengan klik dan menutup dengan Esc; fokus kembali ke pemicu", async () => {
    const user = userEvent.setup();
    render(<Basic />);
    await user.click(screen.getByRole("button", { name: "Menu" }));
    expect(await screen.findByRole("menu")).toBeInTheDocument();
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
    expect(screen.getByRole("button", { name: "Menu" })).toHaveFocus();
  });

  test("dibuka dengan keyboard (Enter) dan navigasi panah mengubah fokus item", async () => {
    const user = userEvent.setup();
    render(<Basic />);
    screen.getByRole("button", { name: "Menu" }).focus();
    await user.keyboard("{Enter}");
    await screen.findByRole("menu");
    // Radix dapat sudah memfokuskan item pertama saat dibuka dengan keyboard; panah memastikan sebuah item terfokus.
    await user.keyboard("{ArrowDown}");
    const items = screen.getAllByRole("menuitem");
    const start = items.indexOf(document.activeElement as HTMLElement);
    expect(start).toBeGreaterThanOrEqual(0);
    await user.keyboard("{ArrowDown}");
    expect(items[start + 1]).toHaveFocus();
    await user.keyboard("{ArrowUp}");
    expect(items[start]).toHaveFocus();
  });

  test("Enter pada item memanggil onSelect lalu menutup menu", async () => {
    const user = userEvent.setup();
    const picked: string[] = [];
    render(<Basic onSelect={(v) => picked.push(v)} />);
    await user.click(screen.getByRole("button", { name: "Menu" }));
    await user.click(await screen.findByRole("menuitem", { name: "Bantuan" }));
    expect(picked).toEqual(["bantuan"]);
    await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
  });

  test("item destructive memakai penanda dan token destructive (Keluar)", async () => {
    const user = userEvent.setup();
    render(<Basic />);
    await user.click(screen.getByRole("button", { name: "Menu" }));
    const keluar = await screen.findByRole("menuitem", { name: "Keluar" });
    expect(keluar).toHaveAttribute("data-variant", "destructive");
    expect(keluar.className).toContain("text-logout-fg");
    expect(screen.getByRole("menuitem", { name: "Bantuan" })).toHaveAttribute(
      "data-variant",
      "default",
    );
  });

  test("label dan pemisah tampil", async () => {
    const user = userEvent.setup();
    render(<Basic />);
    await user.click(screen.getByRole("button", { name: "Menu" }));
    expect(await screen.findByText("Akun")).toBeInTheDocument();
    expect(screen.getByRole("separator")).toBeInTheDocument();
  });

  test("radio group: berperan menuitemradio dan aria-checked mengikuti pilihan", async () => {
    const user = userEvent.setup();
    function Theme() {
      const [v, setV] = useState("system");
      return (
        <DropdownMenu>
          <DropdownMenuTrigger>Tema</DropdownMenuTrigger>
          <DropdownMenuContent>
            <DropdownMenuRadioGroup value={v} onValueChange={setV}>
              <DropdownMenuRadioItem value="light">Terang</DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="dark">Gelap</DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="system">Sistem</DropdownMenuRadioItem>
            </DropdownMenuRadioGroup>
          </DropdownMenuContent>
        </DropdownMenu>
      );
    }
    render(<Theme />);
    await user.click(screen.getByRole("button", { name: "Tema" }));
    expect(await screen.findByRole("menuitemradio", { name: "Sistem" })).toHaveAttribute(
      "aria-checked",
      "true",
    );
    await user.click(screen.getByRole("menuitemradio", { name: "Gelap" }));
    await user.click(screen.getByRole("button", { name: "Tema" }));
    expect(await screen.findByRole("menuitemradio", { name: "Gelap" })).toHaveAttribute(
      "aria-checked",
      "true",
    );
    expect(screen.getByRole("menuitemradio", { name: "Sistem" })).toHaveAttribute(
      "aria-checked",
      "false",
    );
  });

  test("submenu terbuka dengan panah kanan", async () => {
    const user = userEvent.setup();
    render(
      <DropdownMenu>
        <DropdownMenuTrigger>Menu</DropdownMenuTrigger>
        <DropdownMenuContent>
          <DropdownMenuSub>
            <DropdownMenuSubTrigger>Bahasa</DropdownMenuSubTrigger>
            <DropdownMenuSubContent>
              <DropdownMenuItem>Indonesia</DropdownMenuItem>
              <DropdownMenuItem>English</DropdownMenuItem>
            </DropdownMenuSubContent>
          </DropdownMenuSub>
        </DropdownMenuContent>
      </DropdownMenu>,
    );
    await user.click(screen.getByRole("button", { name: "Menu" }));
    const trigger = await screen.findByRole("menuitem", { name: "Bahasa" });
    trigger.focus();
    await user.keyboard("{ArrowRight}");
    expect(await screen.findByRole("menuitem", { name: "Indonesia" })).toBeInTheDocument();
  });
});
