import { describe, expect, mock, test } from "bun:test";
import { fireEvent, renderHook } from "@testing-library/react";
import { useCommandPaletteHotkey } from "./command-palette-hotkey";

function press(init: KeyboardEventInit, target: Window | Element = window): KeyboardEvent {
  const event = new KeyboardEvent("keydown", { bubbles: true, cancelable: true, ...init });
  fireEvent(target, event);
  return event;
}

describe("useCommandPaletteHotkey", () => {
  test("Ctrl+K membuka palet dan mencegah perilaku bawaan peramban", () => {
    const onOpen = mock(() => {});
    renderHook(() => useCommandPaletteHotkey(onOpen));
    const event = press({ key: "k", code: "KeyK", ctrlKey: true });
    expect(onOpen).toHaveBeenCalledTimes(1);
    expect(event.defaultPrevented).toBe(true);
  });

  test("Cmd+K (Mac) juga membuka; huruf besar tetap dikenali", () => {
    const onOpen = mock(() => {});
    renderHook(() => useCommandPaletteHotkey(onOpen));
    press({ key: "K", code: "KeyK", metaKey: true });
    expect(onOpen).toHaveBeenCalledTimes(1);
  });

  test("tetap berlaku saat fokus di kolom isian (memakai modifier)", () => {
    const onOpen = mock(() => {});
    renderHook(() => useCommandPaletteHotkey(onOpen));
    const input = document.createElement("input");
    document.body.append(input);
    press({ key: "k", code: "KeyK", ctrlKey: true }, input);
    input.remove();
    expect(onOpen).toHaveBeenCalledTimes(1);
  });

  test("tanpa modifier, dengan Shift atau Alt, atau tombol lain: diabaikan", () => {
    const onOpen = mock(() => {});
    renderHook(() => useCommandPaletteHotkey(onOpen));
    press({ key: "k", code: "KeyK" });
    press({ key: "K", code: "KeyK", ctrlKey: true, shiftKey: true });
    press({ key: "k", code: "KeyK", ctrlKey: true, altKey: true });
    press({ key: "j", code: "KeyJ", ctrlKey: true });
    expect(onOpen).not.toHaveBeenCalled();
  });

  test("penekanan berulang (tahan tombol) dan peristiwa yang sudah dibatalkan diabaikan", () => {
    const onOpen = mock(() => {});
    renderHook(() => useCommandPaletteHotkey(onOpen));
    press({ key: "k", code: "KeyK", ctrlKey: true, repeat: true });
    const handled = new KeyboardEvent("keydown", {
      key: "k",
      code: "KeyK",
      ctrlKey: true,
      cancelable: true,
    });
    handled.preventDefault();
    fireEvent(window, handled);
    expect(onOpen).not.toHaveBeenCalled();
  });

  test("pendengar dilepas saat unmount dan memakai callback terbaru", () => {
    const first = mock(() => {});
    const second = mock(() => {});
    const { rerender, unmount } = renderHook(({ cb }) => useCommandPaletteHotkey(cb), {
      initialProps: { cb: first },
    });
    rerender({ cb: second });
    press({ key: "k", code: "KeyK", ctrlKey: true });
    expect(first).not.toHaveBeenCalled();
    expect(second).toHaveBeenCalledTimes(1);
    unmount();
    press({ key: "k", code: "KeyK", ctrlKey: true });
    expect(second).toHaveBeenCalledTimes(1);
  });
});
