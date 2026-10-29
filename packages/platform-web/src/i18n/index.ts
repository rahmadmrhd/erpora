export type { Args, ArgValue, RichTags } from "./format";
export { Compiled, compile, format, formatRich, I18nFormatError } from "./format";
export { type CatalogEntry, CatalogLoadError, createCatalogLoader } from "./loader";
export {
  type ChooseLocaleInput,
  chooseLocale,
  getActiveLocale,
  matchLocale,
  setActiveLocale,
} from "./locale";
export { I18nProvider, useTranslation } from "./provider";
export { createTestI18n, type TestI18n } from "./testing";
export type {
  ArgsOf,
  I18nCatalogs,
  PlainArgs,
  RichArgsOf,
  RichTag,
  Translator,
} from "./types";
