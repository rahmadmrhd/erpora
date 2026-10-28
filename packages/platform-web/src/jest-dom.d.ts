// Tipe matcher jest-dom untuk `bun:test` (`expect(el).toBeInTheDocument()` dst.), sama dengan web/test/setup.ts;
// diperlukan di sini karena tsconfig paket hanya memuat `src`. Matcher-nya didaftarkan oleh preload `web/test/setup.ts`.
import type { TestingLibraryMatchers } from "@testing-library/jest-dom/matchers";

declare module "bun:test" {
  interface Matchers<T> extends TestingLibraryMatchers<unknown, T> {}
}
