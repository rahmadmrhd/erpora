import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { UiLabelsProvider } from "../../labels";
import {
  Command,
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
  CommandShortcut,
} from "./command";

function Inline({ onSelect = () => {} }: { onSelect?: (v: string) => void }) {
  return (
    <Command>
      <CommandInput placeholder="Cari atau ketik perintah" />
      <CommandList>
        <CommandEmpty>Tidak ada hasil</CommandEmpty>
        <CommandGroup heading="Halaman">
          <CommandItem value="Catatan" onSelect={onSelect}>
            Catatan
            <CommandShortcut>G N</CommandShortcut>
          </CommandItem>
          <CommandItem value="Laporan" onSelect={onSelect}>
            Laporan
          </CommandItem>
        </CommandGroup>
        <CommandSeparator />
        <CommandGroup heading="Desk">
          <CommandItem value="Platform" onSelect={onSelect}>
            Platform
          </CommandItem>
        </CommandGroup>
      </CommandList>
    </Command>
  );
}

describe("Command (shadcn + cmdk)", () => {
  test("memiliki combobox, listbox, opsi, dan penanda slot registry", () => {
    render(<Inline />);
    expect(screen.getByRole("combobox")).toBeInTheDocument();
    expect(screen.getByRole("listbox")).toBeInTheDocument();
    expect(screen.getAllByRole("option")).toHaveLength(3);
    expect(document.querySelector('[data-slot="command"]')).not.toBeNull();
    expect(document.querySelector('[data-slot="command-input-wrapper"]')).not.toBeNull();
  });

  test("mengetik menyaring daftar dan menampilkan keadaan kosong", async () => {
    const user = userEvent.setup();
    render(<Inline />);
    await user.type(screen.getByRole("combobox"), "lap");
    expect(screen.getAllByRole("option")).toHaveLength(1);
    expect(screen.getByRole("option", { name: "Laporan" })).toBeInTheDocument();
    await user.clear(screen.getByRole("combobox"));
    await user.type(screen.getByRole("combobox"), "zzzz");
    expect(screen.getByText("Tidak ada hasil")).toBeInTheDocument();
  });

  test("panah bawah dan Enter memilih item dengan nilai item", async () => {
    const user = userEvent.setup();
    const picked: string[] = [];
    render(<Inline onSelect={(v) => picked.push(v)} />);
    await user.click(screen.getByRole("combobox"));
    await user.keyboard("{ArrowDown}{Enter}");
    expect(picked).toEqual(["Laporan"]);
  });
});

describe("CommandDialog", () => {
  function InDialog() {
    const [open, setOpen] = useState(true);
    return (
      <UiLabelsProvider t={(k) => (k === "actions.close" ? "Tutup" : k)}>
        <CommandDialog
          open={open}
          onOpenChange={setOpen}
          title="Palet perintah"
          description="Cari halaman atau Desk"
        >
          <CommandInput placeholder="Cari" />
          <CommandList>
            <CommandItem value="a">A</CommandItem>
          </CommandList>
        </CommandDialog>
      </UiLabelsProvider>
    );
  }

  test("membuka Dialog bernama dari prop title dan Escape menutupnya", async () => {
    const user = userEvent.setup();
    render(<InDialog />);
    expect(await screen.findByRole("dialog", { name: "Palet perintah" })).toBeInTheDocument();
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
