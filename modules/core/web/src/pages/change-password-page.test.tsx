import { beforeEach, describe, expect, test } from "bun:test";
import { AuthProvider, createAuthController, StaticI18nProvider } from "@erpora/platform-web";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { GATE_CATALOGS } from "../gates/catalogs";
import { apiError, fakeApi, json } from "../testing";
import { ChangePasswordPage } from "./change-password-page";

const CHANGE = "/api/core/auth/password/change";

beforeEach(() => localStorage.clear());

function setup(routes: Parameters<typeof fakeApi>[0]) {
  const api = fakeApi(routes);
  const controller = createAuthController({ fetch: api.fetch, initial: "anonymous" });
  controller.acceptSession({ accessToken: "token-aktif", expiresIn: 600, mfaState: "none" });
  render(
    <QueryClientProvider client={new QueryClient()}>
      <AuthProvider controller={controller} boot="skip">
        <StaticI18nProvider locale="id" catalogsByLocale={GATE_CATALOGS}>
          <ChangePasswordPage />
        </StaticI18nProvider>
      </AuthProvider>
    </QueryClientProvider>,
  );
  return { api, controller };
}

async function fill(
  user: ReturnType<typeof userEvent.setup>,
  current: string,
  next: string,
  confirm: string,
) {
  await user.type(screen.getByLabelText("Sandi saat ini"), current);
  await user.type(screen.getByLabelText("Sandi baru"), next);
  await user.type(screen.getByLabelText("Ulangi sandi baru"), confirm);
  await user.click(screen.getByRole("button", { name: "Simpan sandi baru" }));
}

describe("halaman Ubah sandi", () => {
  test("sukses: mengirim sandi saat ini dan sandi baru dengan token aktif, lalu mengosongkan isian", async () => {
    const { api } = setup({ [CHANGE]: () => new Response(null, { status: 204 }) });
    const user = userEvent.setup();
    await fill(
      user,
      "sandi-lama-panjang-2026",
      "sandi-baru-panjang-2026",
      "sandi-baru-panjang-2026",
    );
    expect(await screen.findByRole("status")).toHaveTextContent(
      "Sandi berhasil diubah. Sesi di perangkat lain telah keluar.",
    );
    expect(api.calls[0]).toMatchObject({
      path: CHANGE,
      method: "POST",
      authorization: "Bearer token-aktif",
      body: { currentPassword: "sandi-lama-panjang-2026", newPassword: "sandi-baru-panjang-2026" },
    });
    expect(screen.getByLabelText("Sandi saat ini")).toHaveValue("");
    expect(screen.getByLabelText("Sandi baru")).toHaveValue("");
  });

  test("validasi klien: wajib diisi dan konfirmasi sama, tanpa memanggil API", async () => {
    const { api } = setup({});
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Simpan sandi baru" }));
    expect(screen.getAllByText("Wajib diisi.")).toHaveLength(2);
    await fill(user, "lama", "satu", "dua");
    expect(screen.getByText("Kedua sandi tidak sama.")).toBeInTheDocument();
    expect(api.calls).toHaveLength(0);
  });

  test("sandi saat ini salah dan pelanggaran kebijakan ditampilkan dari katalog", async () => {
    let n = 0;
    setup({
      [CHANGE]: () => {
        n++;
        return n === 1
          ? apiError(422, "validation_failed", [
              { field: "currentPassword", code: "wrong_current_password" },
            ])
          : apiError(422, "password_policy_violation", [
              { field: "newPassword", code: "too_short" },
            ]);
      },
    });
    const user = userEvent.setup();
    await fill(user, "salah", "sandi-baru-panjang-2026", "sandi-baru-panjang-2026");
    expect(await screen.findByRole("alert")).toHaveTextContent("Sandi saat ini salah.");
    await user.click(screen.getByRole("button", { name: "Simpan sandi baru" }));
    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent(
        "Sandi terlalu pendek (minimal 12 karakter).",
      ),
    );
  });

  test("batas laju memakai Retry-After", async () => {
    setup({ [CHANGE]: () => apiError(429, "rate_limited", undefined, { "Retry-After": "9" }) });
    await fill(userEvent.setup(), "a", "b-sandi-baru", "b-sandi-baru");
    expect(await screen.findByRole("alert")).toHaveTextContent("Coba lagi dalam 9 detik.");
  });

  test("respons sukses lain tidak dikenal tidak dianggap sukses", async () => {
    setup({ [CHANGE]: () => json({ error: { code: "internal_error", message: "x" } }, 500) });
    await fill(userEvent.setup(), "a", "b-sandi-baru", "b-sandi-baru");
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Terjadi kesalahan. Silakan coba lagi.",
    );
    expect(screen.queryByRole("status")).toBeNull();
  });
});
