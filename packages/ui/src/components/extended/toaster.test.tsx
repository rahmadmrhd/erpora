import { afterEach, describe, expect, test } from "bun:test";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { UiLabelsProvider } from "../../labels";
import { Toaster } from "./toaster";

const labels: Record<string, string> = {
  "toast.region": "Notifikasi",
  "toast.close": "Tutup notifikasi",
};

function setup() {
  return render(
    <UiLabelsProvider t={(k) => labels[k] ?? k}>
      <Toaster />
    </UiLabelsProvider>,
  );
}

afterEach(() => {
  act(() => {
    toast.dismiss();
  });
  document.documentElement.classList.remove("dark");
});

describe("Toaster (pembungkus Toaster shadcn)", () => {
  test("wilayah aria-live bernama dari UiLabels", async () => {
    setup();
    const region = await screen.findByRole("region", { name: /Notifikasi/ });
    expect(region).toHaveAttribute("aria-live", "polite");
  });

  test("toast.error menampilkan judul, deskripsi, dan aksi yang dapat diklik", async () => {
    const user = userEvent.setup();
    setup();
    let retried = 0;
    act(() => {
      toast.error("Gagal memuat data", {
        description: "Periksa koneksi.",
        action: { label: "Coba lagi", onClick: () => retried++ },
      });
    });
    expect(await screen.findByText("Gagal memuat data")).toBeInTheDocument();
    expect(screen.getByText("Periksa koneksi.")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Coba lagi" }));
    expect(retried).toBe(1);
  });

  test("tema mengikuti kelas .dark pada <html> (prop theme ditimpa, bukan next-themes)", async () => {
    document.documentElement.classList.add("dark");
    setup();
    act(() => {
      toast("Halo");
    });
    await screen.findByText("Halo");
    expect(document.querySelector("[data-sonner-toaster]")).toHaveAttribute(
      "data-sonner-theme",
      "dark",
    );
    act(() => {
      document.documentElement.classList.remove("dark");
    });
    await new Promise((r) => setTimeout(r, 20));
    expect(document.querySelector("[data-sonner-toaster]")).toHaveAttribute(
      "data-sonner-theme",
      "light",
    );
  });
});
