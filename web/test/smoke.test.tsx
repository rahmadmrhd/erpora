// Bukti stack uji komponen (M3a K-2): bun test + happy-dom + Testing Library + Radix.
import { describe, expect, test } from "bun:test";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Dialog, DropdownMenu } from "radix-ui";

describe("stack uji komponen", () => {
  test("matcher jest-dom dan render React bekerja", () => {
    render(<p>halo</p>);
    expect(screen.getByText("halo")).toBeInTheDocument();
  });

  test("cleanup berjalan antar test (tidak ada sisa render)", () => {
    expect(screen.queryByText("halo")).not.toBeInTheDocument();
  });

  test("DropdownMenu Radix terbuka lewat klik dan menutup lewat Esc", async () => {
    const user = userEvent.setup();
    render(
      <DropdownMenu.Root>
        <DropdownMenu.Trigger>Menu</DropdownMenu.Trigger>
        <DropdownMenu.Portal>
          <DropdownMenu.Content>
            <DropdownMenu.Item>Satu</DropdownMenu.Item>
          </DropdownMenu.Content>
        </DropdownMenu.Portal>
      </DropdownMenu.Root>,
    );
    expect(screen.queryByRole("menuitem")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Menu" }));
    expect(await screen.findByRole("menuitem", { name: "Satu" })).toBeInTheDocument();
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("menuitem")).not.toBeInTheDocument());
  });

  test("Dialog Radix berperan dialog, bernama, dan menutup lewat Esc", async () => {
    const user = userEvent.setup();
    render(
      <Dialog.Root defaultOpen>
        <Dialog.Portal>
          <Dialog.Overlay />
          <Dialog.Content>
            <Dialog.Title>Judul</Dialog.Title>
            <Dialog.Description>Deskripsi</Dialog.Description>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>,
    );
    expect(await screen.findByRole("dialog", { name: "Judul" })).toBeInTheDocument();
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  test("polyfill happy-dom tersedia untuk Radix", () => {
    expect(typeof ResizeObserver).toBe("function");
    expect(typeof Element.prototype.hasPointerCapture).toBe("function");
    expect(typeof Element.prototype.scrollIntoView).toBe("function");
  });
});
