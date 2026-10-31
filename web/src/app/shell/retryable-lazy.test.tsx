import { describe, expect, spyOn, test } from "bun:test";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Suspense } from "react";
import { LocalBoundary } from "./local-boundary";
import { retryableLazy } from "./retryable-lazy";

describe("retryableLazy", () => {
  test("memuat komponen seperti React.lazy", async () => {
    const Lazy = retryableLazy(() => Promise.resolve({ default: () => <p>Termuat</p> }));
    render(
      <Suspense fallback={<p>Memuat</p>}>
        <Lazy />
      </Suspense>,
    );
    expect(await screen.findByText("Termuat")).toBeInTheDocument();
  });

  test("kegagalan chunk lalu coba lagi: factory dipanggil ulang dan komponen berhasil dimuat", async () => {
    const err = spyOn(console, "error").mockImplementation(() => {});
    try {
      let attempts = 0;
      const Lazy = retryableLazy(() => {
        attempts += 1;
        return attempts === 1
          ? Promise.reject(new Error("chunk gagal"))
          : Promise.resolve({ default: () => <p>Termuat pada percobaan {attempts}</p> });
      });
      const user = userEvent.setup();
      render(
        <LocalBoundary
          label="uji"
          resetKey="x"
          onRetry={() => Lazy.reset()}
          fallback={(retry) => (
            <button type="button" onClick={retry}>
              Coba lagi
            </button>
          )}
        >
          <Suspense fallback={<p>Memuat</p>}>
            <Lazy />
          </Suspense>
        </LocalBoundary>,
      );
      await user.click(await screen.findByRole("button", { name: "Coba lagi" }));
      expect(await screen.findByText("Termuat pada percobaan 2")).toBeInTheDocument();
      expect(attempts).toBe(2);
    } finally {
      err.mockRestore();
    }
  });

  test("berpindah halaman (resetKey berubah) juga mengulang pemuatan lewat onRetry", async () => {
    const err = spyOn(console, "error").mockImplementation(() => {});
    try {
      let attempts = 0;
      const Lazy = retryableLazy(() => {
        attempts += 1;
        return attempts === 1
          ? Promise.reject(new Error("chunk gagal"))
          : Promise.resolve({ default: () => <p>Pulih</p> });
      });
      const ui = (key: string) => (
        <LocalBoundary
          label="uji"
          resetKey={key}
          onRetry={() => Lazy.reset()}
          fallback={<p>Galat</p>}
        >
          <Suspense fallback={null}>
            <Lazy />
          </Suspense>
        </LocalBoundary>
      );
      const { rerender } = render(ui("a"));
      expect(await screen.findByText("Galat")).toBeInTheDocument();
      await act(async () => {
        rerender(ui("b"));
      });
      await waitFor(() => expect(screen.getByText("Pulih")).toBeInTheDocument());
    } finally {
      err.mockRestore();
    }
  });
});
