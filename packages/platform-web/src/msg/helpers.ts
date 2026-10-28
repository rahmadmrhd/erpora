import type { Crumb, MessageRef } from "@erpora/module-sdk";
import type { I18nCatalogs } from "./catalogs";

export interface MsgHelpers<C extends object> {
  /** Rujukan pesan terketik: namespace dan kunci harus ada di katalog (`tsc` menolak salah ketik). */
  msg<N extends keyof C & string, K extends keyof C[N] & string>(ns: N, key: K): MessageRef;
  /** Satu tingkat breadcrumb dari pesan terjemahan (untuk `staticData.crumb`). */
  crumb<N extends keyof C & string, K extends keyof C[N] & string>(ns: N, key: K): Crumb;
}

/**
 * Membuat `msg`/`crumb` terketik di atas bentuk katalog `C` (namespace -> kunci). Aplikasi memakai ekspor
 * `msg`/`crumb` (di atas `I18nCatalogs` global hasil `keys.gen.ts`); pembangun ini dipakai test dan fixture tipe.
 */
export function createMsgHelpers<C extends object>(): MsgHelpers<C> {
  return {
    msg: (ns, key) => Object.freeze({ ns, key }),
    crumb: (ns, key) => Object.freeze({ ref: Object.freeze({ ns, key }) }),
  };
}

const helpers = createMsgHelpers<I18nCatalogs>();
export const msg = helpers.msg;
export const crumb = helpers.crumb;
