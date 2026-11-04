import { describe, expect, mock, test } from "bun:test";
import { createBootstrapUserSource } from "./bootstrap-user";

const user = { id: "u1", email: "ani@example.test", displayName: "Ani" };

describe("createBootstrapUserSource (C1.14)", () => {
  test("pengguna dari bootstrap menjadi CurrentUser; avatar belum ada (null)", async () => {
    const source = createBootstrapUserSource({ user, signOut: async () => {} });
    expect(await source.get()).toEqual({
      id: "u1",
      displayName: "Ani",
      email: "ani@example.test",
      avatarUrl: null,
    });
    expect(source.capabilities).toEqual({ manageAccount: true, signOut: true });
    expect(source.defaultDesk).toBeNull();
  });

  test("tanpa pengguna pada bootstrap: anonim", async () => {
    const source = createBootstrapUserSource({ user: undefined, signOut: async () => {} });
    expect(await source.get()).toBeNull();
  });

  test("manageAccountPath hanya path internal yang aman; selain itu null", () => {
    const make = (p?: string | null) =>
      createBootstrapUserSource({ user, signOut: async () => {}, manageAccountPath: p });
    expect(make("/core/change-password").manageAccountPath?.()).toBe("/core/change-password");
    expect(make("https://evil.test").manageAccountPath?.()).toBeNull();
    expect(make(null).manageAccountPath?.()).toBeNull();
    expect(make().manageAccountPath?.()).toBeNull();
  });

  test("signOut memanggil aksi keluar yang diberikan", async () => {
    const signOut = mock(async () => {});
    await createBootstrapUserSource({ user, signOut }).signOut?.();
    expect(signOut).toHaveBeenCalledTimes(1);
  });
});
