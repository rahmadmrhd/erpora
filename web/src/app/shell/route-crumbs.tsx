import type { Crumb, MessageRef } from "@erpora/module-sdk";
import { Message, useMessage, useModuleRegistry } from "@erpora/platform-web";
import { useMatches } from "@tanstack/react-router";
import { useMemo } from "react";
import type { ResolvedCrumb } from "./breadcrumbs";

const APP_NAME = { ns: "platform", key: "app.name" } as const;

/** Menormalkan `staticData.crumb` (satu tingkat atau beberapa) dari setiap route yang cocok menjadi satu jejak. */
export function useRouteCrumbs(): {
  crumbs: ResolvedCrumb[];
  /** Crumb berpesan beserta kuncinya (untuk probe teks dan de-duplikasi). */
  refs: { key: string; ref: MessageRef }[];
  last: Crumb | undefined;
} {
  const matches = useMatches();
  const registry = useModuleRegistry();
  return useMemo(() => {
    // Katalog namespace modul tidak tersaji saat modul tidak `enabled` (backend 404): crumb memakai slug modul sebagai teks
    // alih-alih memuat katalog yang tidak ada (M-1).
    const inactiveSlugs = new Map(
      registry.resolved
        .filter((r) => !registry.isEnabled(r.module.key))
        .map((r) => [r.module.slug, r.module.slug]),
    );
    const normalize = (crumb: Crumb): Crumb => {
      if ("ref" in crumb) {
        const slug = inactiveSlugs.get(crumb.ref.ns);
        if (slug !== undefined) return { text: slug };
      }
      return crumb;
    };
    const flat: { crumb: Crumb; key: string; to: string | undefined }[] = [];
    const refs: { key: string; ref: MessageRef }[] = [];
    for (const m of matches) {
      const raw = m.staticData?.crumb;
      if (raw === undefined) continue;
      const list = (Array.isArray(raw) ? (raw as readonly Crumb[]) : [raw as Crumb]).map(normalize);
      list.forEach((crumb, i) => {
        // Hanya tingkat terakhir sebuah route yang punya halaman sendiri; tingkat awalnya hanya teks.
        flat.push({
          crumb,
          key: `${m.id}:${i}`,
          to: i === list.length - 1 ? m.pathname : undefined,
        });
      });
    }
    const crumbs: ResolvedCrumb[] = flat.map(({ crumb, key, to }) => ({
      key,
      to,
      label: "ref" in crumb ? <Message message={crumb.ref} /> : crumb.text,
      ...("ref" in crumb ? {} : { text: crumb.text }),
    }));
    refs.push(
      ...flat.flatMap(({ crumb, key }) => ("ref" in crumb ? [{ key, ref: crumb.ref }] : [])),
    );
    return { crumbs, refs, last: flat.at(-1)?.crumb };
  }, [matches, registry]);
}

/** Judul halaman untuk `document.title` dan pengumuman rute: crumb terakhir, atau nama aplikasi. */
export function useRouteTitle(): string {
  const { last } = useRouteCrumbs();
  const appName = useMessage(APP_NAME);
  const lastRefText = useMessage(last && "ref" in last ? last.ref : APP_NAME);
  if (last === undefined) return appName;
  const title = "ref" in last ? lastRefText : last.text;
  return `${title} - ${appName}`;
}
