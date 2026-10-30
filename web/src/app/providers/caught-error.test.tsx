import { afterEach, beforeEach, describe, expect, spyOn, test } from "bun:test";
import { screen } from "@testing-library/react";
import { ModuleInactiveError } from "../pages/pages";
import { renderApp } from "../test-utils";
import { handleCaughtError } from "./caught-error";

let error: ReturnType<typeof spyOn<Console, "error">>;
let debug: ReturnType<typeof spyOn<Console, "debug">>;
let warn: ReturnType<typeof spyOn<Console, "warn">>;

beforeEach(() => {
  localStorage.clear();
  Object.defineProperty(navigator, "languages", { value: [], configurable: true });
  error = spyOn(console, "error").mockImplementation(() => {});
  debug = spyOn(console, "debug").mockImplementation(() => {});
  warn = spyOn(console, "warn").mockImplementation(() => {});
});

afterEach(() => {
  error.mockRestore();
  debug.mockRestore();
  warn.mockRestore();
});

describe("handleCaughtError", () => {
  test("ModuleInactiveError (keadaan yang diharapkan) tidak dicatat sebagai error: hanya debug", () => {
    handleCaughtError(new ModuleInactiveError("acme.alpha", "disabled"), {});
    expect(error).not.toHaveBeenCalled();
    expect(debug).toHaveBeenCalledTimes(1);
    expect(String(debug.mock.calls[0]?.[0])).toContain("acme.alpha");
  });

  test("galat lain tetap dicatat sebagai error", () => {
    handleCaughtError(new Error("rusak"), { componentStack: "\n  at Halaman" });
    expect(error).toHaveBeenCalledTimes(1);
    expect(debug).not.toHaveBeenCalled();
  });
});

describe("guard modul nonaktif tidak membuat [error] di konsol", () => {
  test("reload di modul nonaktif: halaman 'Modul Tidak Aktif' tampil tanpa console.error", async () => {
    renderApp({ path: "/alpha", statuses: { alpha: "disabled" } });
    expect(await screen.findByText("Modul Tidak Aktif")).toBeInTheDocument();
    expect(error).not.toHaveBeenCalled();
  });

  test("galat render sungguhan di halaman tetap dicatat sebagai error", async () => {
    renderApp({ path: "/alpha/boom" });
    expect(await screen.findByText("Terjadi Kesalahan")).toBeInTheDocument();
    expect(error).toHaveBeenCalled();
  });
});
