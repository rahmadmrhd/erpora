import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { createAuthController } from "@erpora/platform-web";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { apiError, fakeApi, json, renderGate, sessionBody } from "../testing";

const LOGIN = "/api/core/auth/login";
const VERIFY = "/api/core/auth/mfa/verify";
const RECOVERY = "/api/core/auth/mfa/recovery";
const ENROLL = "/api/core/auth/mfa/enroll";
const CONFIRM = "/api/core/auth/mfa/confirm";
const FORGOT = "/api/core/auth/password/forgot";
const RESET = "/api/core/auth/password/reset";
const LOGOUT = "/api/core/auth/logout";

beforeEach(() => {
  localStorage.clear();
  // happy-dom melaporkan navigator.languages en-US; tes memakai bahasa Indonesia kecuali ditentukan lain.
  localStorage.setItem("erpora.locale", "id");
  window.history.replaceState(null, "", "/");
});
afterEach(() => {
  window.history.replaceState(null, "", "/");
});

async function fillLogin(
  user: ReturnType<typeof userEvent.setup>,
  email: string,
  password: string,
) {
  if (email) await user.type(screen.getByLabelText("Email"), email);
  if (password) await user.type(screen.getByLabelText("Sandi"), password);
  await user.click(screen.getByRole("button", { name: "Masuk" }));
}

describe("layar login", () => {
  test("tata letak: nama aplikasi, judul, email, sandi (dengan tombol tampilkan), Lupa sandi, tombol penuh", async () => {
    await renderGate();
    expect(screen.getByText("ERPora")).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1, name: "Masuk" })).toBeInTheDocument();
    expect(screen.getByLabelText("Email")).toHaveAttribute("type", "email");
    const password = screen.getByLabelText("Sandi");
    expect(password).toHaveAttribute("type", "password");
    await userEvent.setup().click(screen.getByRole("button", { name: "Tampilkan sandi" }));
    expect(password).toHaveAttribute("type", "text");
    expect(screen.getByRole("link", { name: "Lupa sandi?" })).toHaveAttribute(
      "href",
      "/forgot-password",
    );
    expect(screen.getByRole("button", { name: "Masuk" })).toBeInTheDocument();
  });

  test("tautan Lupa sandi memakai navigasi gerbang (tanpa memuat ulang halaman)", async () => {
    const { navigations } = await renderGate();
    await userEvent.setup().click(screen.getByRole("link", { name: "Lupa sandi?" }));
    expect(navigations).toEqual(["/forgot-password"]);
  });

  test("validasi sisi klien: wajib diisi dan format email, tanpa memanggil API", async () => {
    const { api } = await renderGate();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Masuk" }));
    expect(screen.getAllByText("Wajib diisi.")).toHaveLength(2);
    await user.type(screen.getByLabelText("Email"), "bukan-email");
    await user.type(screen.getByLabelText("Sandi"), "x");
    await user.click(screen.getByRole("button", { name: "Masuk" }));
    expect(screen.getByText("Masukkan alamat email yang valid.")).toBeInTheDocument();
    expect(api.calls).toHaveLength(0);
  });

  test("login sukses: mengirim email dan sandi, sesi diterima klien token", async () => {
    const { api, controller } = await renderGate({
      routes: { [LOGIN]: () => json(sessionBody("token-1")) },
    });
    await fillLogin(userEvent.setup(), "ani@example.test", "kata-sandi-panjang-2026");
    await waitFor(() => expect(controller.getState()).toEqual({ status: "authenticated" }));
    expect(controller.getAccessToken()).toBe("token-1");
    expect(api.calls[0]).toMatchObject({
      path: LOGIN,
      method: "POST",
      body: { email: "ani@example.test", password: "kata-sandi-panjang-2026" },
    });
  });

  test("galat login seragam (tidak membedakan email/sandi); sandi dikosongkan; teks server tidak tampil", async () => {
    await renderGate({ routes: { [LOGIN]: () => apiError(401, "invalid_credentials") } });
    const user = userEvent.setup();
    await fillLogin(user, "ani@example.test", "salah-salah-salah");
    expect(await screen.findByRole("alert")).toHaveTextContent("Email atau sandi salah.");
    expect(screen.queryByText("teks server")).toBeNull();
    expect(screen.getByLabelText("Sandi")).toHaveValue("");
    expect(screen.getByLabelText("Email")).toHaveValue("ani@example.test");
  });

  test("rate_limited memakai Retry-After; account_locked dan jaringan putus punya pesan sendiri", async () => {
    const cases: Array<[() => Response | Promise<Response>, string]> = [
      [
        () => apiError(429, "rate_limited", undefined, { "Retry-After": "42" }),
        "Terlalu banyak percobaan. Coba lagi dalam 42 detik.",
      ],
      [
        () => apiError(403, "account_locked"),
        "Akun terkunci. Atur ulang sandi atau hubungi administrator.",
      ],
      [
        () => {
          throw new TypeError("network");
        },
        "Tidak dapat terhubung ke server. Periksa koneksi Anda lalu coba lagi.",
      ],
    ];
    for (const [respond, text] of cases) {
      const { unmount } = await renderGate({ routes: { [LOGIN]: respond } });
      await fillLogin(userEvent.setup(), "ani@example.test", "kata-sandi-panjang-2026");
      expect(await screen.findByRole("alert")).toHaveTextContent(text);
      unmount();
    }
  });

  test("mfaState dari login menampilkan layar MFA yang sesuai", async () => {
    const { controller } = await renderGate({
      routes: { [LOGIN]: () => json(sessionBody("t", "verify_required")) },
    });
    await fillLogin(userEvent.setup(), "ani@example.test", "kata-sandi-panjang-2026");
    expect(
      await screen.findByRole("heading", { name: "Verifikasi dua langkah" }),
    ).toBeInTheDocument();
    expect(controller.getState()).toEqual({ status: "mfa", mfaState: "verify_required" });
  });
});

describe("layar MFA: verifikasi", () => {
  function mfaController(routes: Parameters<typeof fakeApi>[0], mfa = "verify_required") {
    const api = fakeApi(routes);
    const controller = createAuthController({ fetch: api.fetch, initial: "anonymous" });
    controller.acceptSession({ accessToken: "pra-mfa", expiresIn: 600, mfaState: mfa as never });
    return { api, controller };
  }

  test("muncul saat sesi menunggu verifikasi, di path mana pun; kode TOTP terkirim otomatis saat 6 digit penuh", async () => {
    const { api, controller } = mfaController({ [VERIFY]: () => json(sessionBody("token-otp")) });
    await renderGate({ controller, path: "/core/change-password" });
    await screen.findByRole("heading", { name: "Verifikasi dua langkah" });
    await userEvent.setup().type(screen.getByLabelText("Kode verifikasi"), "123456");
    await waitFor(() => expect(controller.getState()).toEqual({ status: "authenticated" }));
    expect(api.calls.find((c) => c.path === VERIFY)).toMatchObject({
      method: "POST",
      body: { code: "123456" },
      authorization: "Bearer pra-mfa",
    });
    expect(controller.getAccessToken()).toBe("token-otp");
  });

  test("kode salah: pesan detail dari katalog dan isian dikosongkan", async () => {
    const { controller } = mfaController({
      [VERIFY]: () => apiError(422, "validation_failed", [{ field: "code", code: "invalid_code" }]),
    });
    await renderGate({ controller });
    await userEvent.setup().type(await screen.findByLabelText("Kode verifikasi"), "000000");
    expect(await screen.findByRole("alert")).toHaveTextContent("Kode tidak valid.");
    expect(controller.getState().status).toBe("mfa");
  });

  test("recovery code memakai endpoint recovery", async () => {
    const { api, controller } = mfaController({ [RECOVERY]: () => json(sessionBody("t-otp")) });
    await renderGate({ controller });
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Gunakan recovery code" }));
    await user.type(screen.getByLabelText("Recovery code"), "a1b2-c3d4-e5f6");
    await user.click(screen.getByRole("button", { name: "Verifikasi" }));
    await waitFor(() => expect(controller.getState()).toEqual({ status: "authenticated" }));
    expect(api.calls.find((c) => c.path === RECOVERY)?.body).toEqual({ code: "a1b2-c3d4-e5f6" });
    expect(api.count(VERIFY)).toBe(0);
  });

  test("Keluar memanggil logout dan kembali ke keadaan anonymous", async () => {
    const { api, controller } = mfaController({
      [LOGOUT]: () => new Response(null, { status: 204 }),
    });
    await renderGate({ controller });
    await userEvent.setup().click(await screen.findByRole("button", { name: "Keluar" }));
    await waitFor(() => expect(controller.getState()).toEqual({ status: "anonymous" }));
    expect(api.count(LOGOUT)).toBe(1);
  });
});

describe("layar MFA: pendaftaran (administrator)", () => {
  const enrollRoutes = (confirm: () => Response) => ({
    [ENROLL]: () =>
      json({
        data: {
          secret: "JBSWY3DPEHPK3PXP",
          otpauthUri:
            "otpauth://totp/ERPora:ani@example.test?secret=JBSWY3DPEHPK3PXP&issuer=ERPora",
        },
      }),
    [CONFIRM]: confirm,
  });
  const codes = Array.from({ length: 10 }, (_, i) => `kode-${i}-aaaa-bbbb`);

  function enrollController(routes: Parameters<typeof fakeApi>[0]) {
    const api = fakeApi(routes);
    const controller = createAuthController({ fetch: api.fetch, initial: "anonymous" });
    controller.acceptSession({
      accessToken: "pra-mfa",
      expiresIn: 600,
      mfaState: "enroll_required",
    });
    return { api, controller };
  }

  test("memulai pendaftaran SEKALI, menampilkan QR sebagai <svg> React dan secret terkelompok untuk input manual", async () => {
    const { api, controller } = enrollController(enrollRoutes(() => json({ data: { codes } })));
    const { container } = await renderGate({ controller });
    const qr = await screen.findByRole("img", { name: "Kode QR untuk aplikasi autentikator" });
    expect(qr.tagName.toLowerCase()).toBe("svg");
    expect(qr.querySelector("path")?.getAttribute("d")?.length).toBeGreaterThan(100);
    expect(screen.getByText("JBSW Y3DP EHPK 3PXP")).toBeInTheDocument();
    expect(api.count(ENROLL)).toBe(1);
    expect(container.innerHTML).not.toContain("<script");
  });

  test("konfirmasi: tampil 10 recovery code sekali dengan peringatan; lanjut hanya setelah dicentang; lalu sesi diterima", async () => {
    const { api, controller } = enrollController(
      enrollRoutes(() =>
        json({
          data: { codes, session: { accessToken: "t-otp", expiresIn: 600, mfaState: "none" } },
        }),
      ),
    );
    await renderGate({ controller });
    const user = userEvent.setup();
    await user.type(await screen.findByLabelText("Kode dari aplikasi autentikator"), "123456");

    expect(
      await screen.findByRole("heading", { name: "Simpan recovery code Anda" }),
    ).toBeInTheDocument();
    expect(screen.getByText(/hanya ditampilkan sekali/)).toBeInTheDocument();
    expect(screen.getAllByRole("listitem")).toHaveLength(10);
    for (const c of codes) expect(screen.getByText(c)).toBeInTheDocument();
    // Sesi baru belum dipakai: layar recovery tidak boleh tertutup sebelum pengguna melanjutkan.
    expect(controller.getState()).toEqual({ status: "mfa", mfaState: "enroll_required" });
    expect(api.calls.find((c) => c.path === CONFIRM)?.body).toEqual({ code: "123456" });

    const next = screen.getByRole("button", { name: "Lanjutkan" });
    expect(next).toBeDisabled();
    await user.click(
      screen.getByRole("checkbox", { name: "Saya sudah menyimpan recovery code ini" }),
    );
    await user.click(next);
    expect(controller.getState()).toEqual({ status: "authenticated" });
    expect(controller.getAccessToken()).toBe("t-otp");
  });

  test("kode konfirmasi salah: pesan galat dan tetap di layar pendaftaran", async () => {
    const { controller } = enrollController(
      enrollRoutes(() =>
        apiError(422, "validation_failed", [{ field: "code", code: "invalid_code" }]),
      ),
    );
    await renderGate({ controller });
    await userEvent
      .setup()
      .type(await screen.findByLabelText("Kode dari aplikasi autentikator"), "654321");
    expect(await screen.findByRole("alert")).toHaveTextContent("Kode tidak valid.");
    expect(screen.queryByRole("heading", { name: "Simpan recovery code Anda" })).toBeNull();
  });

  test("gagal memulai pendaftaran: pesan galat dan tombol coba lagi", async () => {
    const { api, controller } = enrollController({
      [ENROLL]: (_c, n) =>
        n === 1
          ? apiError(500, "internal_error")
          : json({ data: { secret: "AAAA", otpauthUri: "otpauth://totp/x?secret=AAAA" } }),
    });
    await renderGate({ controller });
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Terjadi kesalahan. Silakan coba lagi.",
    );
    await userEvent.setup().click(screen.getByRole("button", { name: "Coba lagi" }));
    await screen.findByRole("img", { name: "Kode QR untuk aplikasi autentikator" });
    expect(api.count(ENROLL)).toBe(2);
  });
});

describe("lupa sandi", () => {
  test("pesan netral setelah permintaan diterima (tanpa membedakan akun)", async () => {
    const { api } = await renderGate({
      path: "/forgot-password",
      routes: { [FORGOT]: () => new Response(null, { status: 204 }) },
    });
    const user = userEvent.setup();
    await user.type(screen.getByLabelText("Email"), "ani@example.test");
    await user.click(screen.getByRole("button", { name: "Kirim tautan" }));
    expect(await screen.findByRole("heading", { name: "Periksa email Anda" })).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("Bila email tersebut terdaftar");
    expect(api.calls[0]).toMatchObject({ path: FORGOT, body: { email: "ani@example.test" } });
    expect(screen.getByRole("link", { name: "Kembali ke halaman masuk" })).toHaveAttribute(
      "href",
      "/login",
    );
  });

  test("validasi email dan batas laju", async () => {
    await renderGate({
      path: "/forgot-password",
      routes: { [FORGOT]: () => apiError(429, "rate_limited", undefined, { "Retry-After": "30" }) },
    });
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Kirim tautan" }));
    expect(screen.getByText("Wajib diisi.")).toBeInTheDocument();
    await user.type(screen.getByLabelText("Email"), "ani@example.test");
    await user.click(screen.getByRole("button", { name: "Kirim tautan" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Coba lagi dalam 30 detik.");
  });
});

describe("reset sandi", () => {
  test("token dibaca dari fragmen lalu SEGERA dihapus dari URL; terkirim di body, bukan di URL", async () => {
    window.history.replaceState(null, "", "/reset-password?x=1#token=tok-rahasia-123");
    const { api } = await renderGate({
      path: "/reset-password",
      routes: { [RESET]: () => new Response(null, { status: 204 }) },
    });
    expect(window.location.hash).toBe("");
    expect(window.location.pathname + window.location.search).toBe("/reset-password?x=1");
    expect(window.location.href).not.toContain("tok-rahasia-123");

    const user = userEvent.setup();
    await user.type(screen.getByLabelText("Sandi baru"), "sandi-baru-yang-panjang-2026");
    await user.type(screen.getByLabelText("Ulangi sandi baru"), "sandi-baru-yang-panjang-2026");
    await user.click(screen.getByRole("button", { name: "Simpan sandi baru" }));
    expect(
      await screen.findByRole("heading", { name: "Sandi berhasil diubah" }),
    ).toBeInTheDocument();
    expect(api.calls[0]).toMatchObject({
      path: RESET,
      body: { token: "tok-rahasia-123", newPassword: "sandi-baru-yang-panjang-2026" },
    });
    expect(api.calls[0]?.path).not.toContain("tok-rahasia-123");
  });

  test("sandi tidak sama: galat sisi klien tanpa memanggil API", async () => {
    window.history.replaceState(null, "", "/reset-password#token=abc");
    const { api } = await renderGate({ path: "/reset-password" });
    const user = userEvent.setup();
    await user.type(screen.getByLabelText("Sandi baru"), "satu-sandi-panjang-2026");
    await user.type(screen.getByLabelText("Ulangi sandi baru"), "lain-sandi-panjang-2026");
    await user.click(screen.getByRole("button", { name: "Simpan sandi baru" }));
    expect(screen.getByText("Kedua sandi tidak sama.")).toBeInTheDocument();
    expect(api.calls).toHaveLength(0);
  });

  test("tanpa token di fragmen: pesan tautan tidak lengkap dan tautan minta ulang", async () => {
    window.history.replaceState(null, "", "/reset-password");
    await renderGate({ path: "/reset-password" });
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Tautan pengaturan ulang sandi tidak lengkap",
    );
    expect(screen.getByRole("link", { name: "Minta tautan baru" })).toHaveAttribute(
      "href",
      "/forgot-password",
    );
  });

  test("token ditolak server: pesan detail dan tautan minta ulang; pelanggaran kebijakan dirinci per aturan", async () => {
    window.history.replaceState(null, "", "/reset-password#token=kedaluwarsa");
    let call = 0;
    await renderGate({
      path: "/reset-password",
      routes: {
        [RESET]: () => {
          call++;
          return call === 1
            ? apiError(422, "password_policy_violation", [
                { field: "newPassword", code: "too_short" },
                { field: "newPassword", code: "common" },
              ])
            : apiError(422, "validation_failed", [{ field: "token", code: "invalid_token" }]);
        },
      },
    });
    const user = userEvent.setup();
    await user.type(screen.getByLabelText("Sandi baru"), "pendek");
    await user.type(screen.getByLabelText("Ulangi sandi baru"), "pendek");
    await user.click(screen.getByRole("button", { name: "Simpan sandi baru" }));
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("Sandi terlalu pendek (minimal 12 karakter).");
    expect(alert).toHaveTextContent("Sandi terlalu umum dan mudah ditebak.");

    await user.click(screen.getByRole("button", { name: "Simpan sandi baru" }));
    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent(
        "Tautan tidak valid atau sudah tidak berlaku.",
      ),
    );
    expect(screen.getByRole("link", { name: "Minta tautan baru" })).toBeInTheDocument();
  });
});

describe("bahasa gerbang", () => {
  test("pilihan tersimpan en menampilkan teks Inggris dari katalog yang dibundel", async () => {
    localStorage.setItem("erpora.locale", "en");
    await renderGate();
    expect(screen.getByRole("heading", { level: 1, name: "Sign in" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Forgot password?" })).toBeInTheDocument();
  });
});
