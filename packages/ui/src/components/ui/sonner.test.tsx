import { afterEach, describe, expect, test } from "bun:test";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { UiLabelsProvider } from "../../labels";
import { Toaster } from "./sonner";

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
});

describe("Toaster (sonner bergaya token)", () => {
  test("wilayah aria-live bernama dari UiLabels", async () => {
    setup();
    const region = await screen.findByRole("region", { name: /Notifikasi/ });
    expect(region).toHaveAttribute("aria-live", "polite");
  });

  test("toast.error menampilkan judul, deskripsi, dan aksi Coba lagi yang dapat diklik", async () => {
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

  test("tombol tutup berlabel dari UiLabels menutup toast", async () => {
    const user = userEvent.setup();
    setup();
    act(() => {
      toast("Tersimpan");
    });
    await screen.findByText("Tersimpan");
    const item = document.querySelector("[data-sonner-toast]") as HTMLElement;
    await user.click(screen.getByRole("button", { name: "Tutup notifikasi" }));
    await waitFor(() => expect(item).toHaveAttribute("data-removed", "true"));
  });

  test("posisi tengah atas", async () => {
    const { container } = setup();
    act(() => {
      toast("Halo");
    });
    await screen.findByText("Halo");
    expect(
      container.ownerDocument
        .querySelector("[data-sonner-toaster]")
        ?.getAttribute("data-y-position"),
    ).toBe("top");
    expect(
      container.ownerDocument
        .querySelector("[data-sonner-toaster]")
        ?.getAttribute("data-x-position"),
    ).toBe("center");
  });
});
