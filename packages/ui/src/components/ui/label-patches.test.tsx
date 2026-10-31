import { afterEach, describe, expect, test } from "bun:test";
import { render, screen, waitFor } from "@testing-library/react";
import { type UiKey, UiLabel, UiLabelsProvider } from "../../labels";
import { Breadcrumb, BreadcrumbEllipsis, BreadcrumbList } from "./breadcrumb";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogTitle } from "./dialog";
import { Sheet, SheetContent, SheetDescription, SheetTitle } from "./sheet";
import { Sidebar, SidebarProvider, SidebarRail, SidebarTrigger } from "./sidebar";

// Patch label deklaratif (shadcn.patches.json): teks Inggris tertanam di berkas registry diganti label
// UiLabels sehingga UI berbahasa Indonesia tidak menampilkan "Close", "More", "Toggle Sidebar", dst.
const labels: Partial<Record<UiKey, string>> = {
  "actions.close": "Tutup",
  "breadcrumb.label": "Jejak halaman",
  "breadcrumb.more": "Tampilkan jalur lengkap",
  "sidebar.toggle": "Ciutkan atau perluas sidebar",
  "sidebar.title": "Menu navigasi",
  "sidebar.description": "Menu navigasi utama aplikasi",
};
const t = (k: UiKey) => labels[k] ?? k;
const wrap = (ui: React.ReactNode) => <UiLabelsProvider t={t}>{ui}</UiLabelsProvider>;

describe("UiLabel", () => {
  test("merender teks label dari konteks (kunci bila tanpa provider)", () => {
    render(
      <>
        {wrap(
          <p data-testid="a">
            <UiLabel k="actions.close" />
          </p>,
        )}
        <p data-testid="b">
          <UiLabel k="actions.close" />
        </p>
      </>,
    );
    expect(screen.getByTestId("a")).toHaveTextContent("Tutup");
    expect(screen.getByTestId("b")).toHaveTextContent("actions.close");
  });
});

describe("label pada komponen shadcn hasil patch", () => {
  test("Dialog: tombol tutup (sr-only) dan tombol Tutup footer memakai UiLabels", () => {
    render(
      wrap(
        <Dialog open>
          <DialogContent>
            <DialogTitle>Judul</DialogTitle>
            <DialogDescription>Isi</DialogDescription>
            <DialogFooter showCloseButton />
          </DialogContent>
        </Dialog>,
      ),
    );
    const closers = screen.getAllByRole("button", { name: "Tutup" });
    expect(closers.length).toBe(2);
    expect(screen.queryByText("Close")).toBeNull();
  });

  test("Sheet: tombol tutup memakai UiLabels", () => {
    render(
      wrap(
        <Sheet open>
          <SheetContent>
            <SheetTitle>Judul</SheetTitle>
            <SheetDescription>Isi</SheetDescription>
          </SheetContent>
        </Sheet>,
      ),
    );
    expect(screen.getByRole("button", { name: "Tutup" })).toBeInTheDocument();
    expect(screen.queryByText("Close")).toBeNull();
  });

  test("Breadcrumb: nav bernama dan elipsis memakai UiLabels", () => {
    render(
      wrap(
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbEllipsis />
          </BreadcrumbList>
        </Breadcrumb>,
      ),
    );
    expect(screen.getByRole("navigation", { name: "Jejak halaman" })).toBeInTheDocument();
    expect(screen.getByText("Tampilkan jalur lengkap")).toBeInTheDocument();
    expect(screen.queryByText("More")).toBeNull();
  });

  test("Sidebar: SidebarTrigger dan SidebarRail memakai label sidebar.toggle", () => {
    render(
      wrap(
        <SidebarProvider>
          <Sidebar>
            <SidebarRail />
          </Sidebar>
          <SidebarTrigger />
        </SidebarProvider>,
      ),
    );
    expect(screen.getAllByText("Ciutkan atau perluas sidebar").length).toBeGreaterThan(0);
    expect(
      screen.getByLabelText("Ciutkan atau perluas sidebar", {
        selector: "button[data-sidebar=rail]",
      }),
    ).toBeInTheDocument();
    expect(screen.queryByText("Toggle Sidebar")).toBeNull();
  });
});

describe("Sidebar mobile (Sheet)", () => {
  const originalMatchMedia = window.matchMedia;
  const originalWidth = window.innerWidth;
  afterEach(() => {
    window.matchMedia = originalMatchMedia;
    window.innerWidth = originalWidth;
  });

  test("judul dan deskripsi Sheet memakai sidebar.title dan sidebar.description", async () => {
    window.innerWidth = 500;
    window.matchMedia = ((query: string) => ({
      matches: query.includes("767"),
      media: query,
      onchange: null,
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
      dispatchEvent: () => false,
    })) as typeof window.matchMedia;
    render(
      wrap(
        <SidebarProvider defaultOpen>
          <Sidebar>
            <p>isi</p>
          </Sidebar>
          <SidebarTrigger />
        </SidebarProvider>,
      ),
    );
    screen.getAllByRole("button", { name: "Ciutkan atau perluas sidebar" })[0]?.click();
    await waitFor(() => expect(screen.getByText("Menu navigasi")).toBeInTheDocument());
    expect(screen.getByText("Menu navigasi utama aplikasi")).toBeInTheDocument();
    expect(screen.queryByText("Displays the mobile sidebar.")).toBeNull();
  });
});
