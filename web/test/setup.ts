// Preload kedua untuk `bun test` (setelah dom.ts): matcher jest-dom, pembersihan, dan polyfill happy-dom.
import { afterEach, expect } from "bun:test";
import type { TestingLibraryMatchers } from "@testing-library/jest-dom/matchers";
import * as matchers from "@testing-library/jest-dom/matchers";
import { cleanup } from "@testing-library/react";

expect.extend(matchers);

// Tipe matcher jest-dom untuk `bun:test` (`expect(el).toBeInTheDocument()` dst.).
declare module "bun:test" {
  interface Matchers<T> extends TestingLibraryMatchers<unknown, T> {}
}

afterEach(() => {
  cleanup();
});

// Radix memakai API yang belum ada di happy-dom.
class ResizeObserverStub {
  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
}
if (typeof globalThis.ResizeObserver === "undefined") {
  globalThis.ResizeObserver = ResizeObserverStub;
}
const proto = Element.prototype;
if (!proto.hasPointerCapture) proto.hasPointerCapture = () => false;
if (!proto.setPointerCapture) proto.setPointerCapture = () => {};
if (!proto.releasePointerCapture) proto.releasePointerCapture = () => {};
if (!proto.scrollIntoView) proto.scrollIntoView = () => {};
