import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { UiLabelsProvider } from "../../labels";
import {
  Breadcrumb,
  BreadcrumbEllipsis,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "./breadcrumb";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "./dropdown-menu";

const labels: Record<string, string> = {
  "breadcrumb.label": "Jejak halaman",
  "breadcrumb.more": "Tampilkan jalur lengkap",
};
const t = (k: string) => labels[k] ?? k;

function Sample() {
  return (
    <UiLabelsProvider t={t}>
      <Breadcrumb>
        <BreadcrumbList>
          <BreadcrumbItem>
            <BreadcrumbLink href="/settings">Pengaturan</BreadcrumbLink>
          </BreadcrumbItem>
          <BreadcrumbSeparator />
          <BreadcrumbItem>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <button type="button">
                  <BreadcrumbEllipsis />
                </button>
              </DropdownMenuTrigger>
              <DropdownMenuContent>
                <DropdownMenuItem>Modul</DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </BreadcrumbItem>
          <BreadcrumbSeparator />
          <BreadcrumbItem>
            <BreadcrumbPage>example</BreadcrumbPage>
          </BreadcrumbItem>
        </BreadcrumbList>
      </Breadcrumb>
    </UiLabelsProvider>
  );
}

describe("Breadcrumb", () => {
  test("<nav> bernama dari UiLabels dan berisi daftar berurut", () => {
    render(<Sample />);
    const nav = screen.getByRole("navigation", { name: "Jejak halaman" });
    expect(nav.querySelector("ol")).not.toBeNull();
  });

  test("item terakhir aria-current=page dan bukan tautan", () => {
    render(<Sample />);
    const page = screen.getByText("example");
    expect(page).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("link", { name: "Pengaturan" })).toHaveAttribute("href", "/settings");
  });

  test("pemisah dekoratif (tidak diumumkan)", () => {
    const { container } = render(<Sample />);
    for (const sep of container.querySelectorAll('[data-slot="breadcrumb-separator"]')) {
      expect(sep).toHaveAttribute("aria-hidden", "true");
      expect(sep).toHaveAttribute("role", "presentation");
    }
  });

  test("elipsis membuka menu berisi jalur yang dilipat; namanya dari UiLabels", async () => {
    const user = userEvent.setup();
    render(<Sample />);
    await user.click(screen.getByRole("button", { name: "Tampilkan jalur lengkap" }));
    expect(await screen.findByRole("menuitem", { name: "Modul" })).toBeInTheDocument();
  });

  test("tautan bergaya teks muted dengan hover foreground", () => {
    render(<Sample />);
    const link = screen.getByRole("link", { name: "Pengaturan" });
    expect(link.className).toContain("text-muted-foreground");
    expect(link.className).toContain("hover:text-foreground");
  });
});
