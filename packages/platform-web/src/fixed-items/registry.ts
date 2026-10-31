import { useSyncExternalStore } from "react";
import { safeInternalHref } from "../sources/href";
import { useNotifications } from "../sources/notifications";
import { useTodoCount } from "./todo-count";
import type { FixedItem, FixedItemInput } from "./types";

const ID = /^[a-z][a-z0-9-]*$/;

const useNoBadge = (): number => 0;
const useNotificationsBadge = (): number => useNotifications({ enabled: false }).unreadCount;

function defaults(): FixedItem[] {
  return [
    {
      id: "search",
      label: { ns: "platform", key: "fixedItems.search" },
      icon: "search",
      order: 10,
      action: { kind: "command", command: "search" },
      useBadge: useNoBadge,
      shortcut: ["Ctrl", "K"],
    },
    {
      id: "notifications",
      label: { ns: "platform", key: "notifications.title" },
      icon: "bell",
      order: 20,
      action: { kind: "command", command: "notifications" },
      useBadge: useNotificationsBadge,
    },
    {
      id: "todo",
      label: { ns: "platform", key: "fixedItems.todo" },
      icon: "clipboard-list",
      order: 30,
      action: { kind: "link", to: "/platform/todo" },
      useBadge: useTodoCount,
    },
  ];
}

function sorted(list: readonly FixedItem[]): readonly FixedItem[] {
  // Urut stabil: `order`, lalu urutan pendaftaran.
  return Object.freeze(
    list
      .map((item, i) => ({ item, i }))
      .sort((a, b) => a.item.order - b.item.order || a.i - b.i)
      .map((e) => e.item),
  );
}

let items: readonly FixedItem[] = sorted(defaults());
const listeners = new Set<() => void>();

function emit(next: readonly FixedItem[]): void {
  items = next;
  for (const l of listeners) l();
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

/**
 * Mendaftarkan item grup tetap panel menu; mengembalikan fungsi pelepas. Galat konfigurasi (id ganda atau tak valid,
 * tautan bukan path internal aman) dilempar saat pendaftaran sehingga tertangkap saat pengembangan.
 */
export function registerFixedItem(input: FixedItemInput): () => void {
  if (!ID.test(input.id)) throw new Error(`item tetap: id tidak valid "${input.id}"`);
  if (items.some((i) => i.id === input.id)) {
    throw new Error(`item tetap: id "${input.id}" sudah terdaftar`);
  }
  if (input.action.kind === "link" && safeInternalHref(input.action.to) === null) {
    throw new Error(`item tetap "${input.id}": tautan harus path internal (${input.action.to})`);
  }
  const item: FixedItem = { ...input, useBadge: input.useBadge ?? useNoBadge };
  emit(sorted([...items, item]));
  return () => emit(sorted(items.filter((i) => i !== item)));
}

/** Item tetap terurut. */
export function useFixedItems(): readonly FixedItem[] {
  return useSyncExternalStore(
    subscribe,
    () => items,
    () => items,
  );
}

/** Mengembalikan registri ke item bawaan (untuk test). */
export function resetFixedItems(): void {
  emit(sorted(defaults()));
}
