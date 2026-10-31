export {
  createDesksFetcher,
  DESKS_PATH,
  type DesksClient,
  DesksError,
  parseDeskList,
  toDesksClient,
  useDesksQuery,
} from "./api";
export { deskContains, matchMenu } from "./match-menu";
export {
  type DeskContextValue,
  DeskProvider,
  type DeskProviderProps,
  isShowDesksState,
  SHOW_DESKS_STATE,
  useCurrentDesk,
  useDeskContext,
  useDesks,
  useShowDesks,
} from "./provider";
export {
  type ResolveInput,
  type ResolveResult,
  type ResolveStep,
  resolveDesk,
} from "./resolver";
export {
  DESK_STORAGE_KEYS,
  isValidDeskId,
  readRememberedDesk,
  readTabDesk,
  writeDeskChoice,
} from "./storage";
export {
  makeDashboardItem,
  makeDesk,
  makeDeskGroup,
  makeDeskItem,
} from "./testing";
export type {
  Desk,
  DeskList,
  DeskMenuGroup,
  DeskMenuItem,
  MessageRef,
} from "./types";
export { useDeskValidation } from "./validation";
