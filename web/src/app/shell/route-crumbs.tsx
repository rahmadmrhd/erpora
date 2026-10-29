import type { Crumb } from "@erpora/module-sdk";
import { Message, useMessage } from "@erpora/platform-web";
import { useMatches } from "@tanstack/react-router";
import { useMemo } from "react";
import type { ResolvedCrumb } from "./breadcrumbs";

const APP_NAME = { ns: "platform", key: "app.name" } as const;

/** Menormalkan `staticData.crumb` (satu tingkat atau beberapa) dari setiap route yang cocok menjadi satu jejak. */
export function useRouteCrumbs(): { crumbs: ResolvedCrumb[]; last: Crumb | undefined } {
  const matches = useMatches();
  return useMemo(() => {
    const flat: { crumb: Crumb; key: string; to: string | undefined }[] = [];
    for (const m of matches) {
      const raw = m.staticData?.crumb;
      if (raw === undefined) continue;
      const list = Array.isArray(raw) ? (raw as readonly Crumb[]) : [raw as Crumb];
      list.forEach((crumb, i) => {
        // Hanya tingkat terakhir sebuah route yang punya halaman sendiri; tingkat awalnya hanya teks.
        flat.push({
          crumb,
          key: `${m.id}:${i}`,
          to: i === list.length - 1 ? m.pathname : undefined,
        });
      });
    }
    const crumbs = flat.map(({ crumb, key, to }) => ({
      key,
      to,
      label: "ref" in crumb ? <Message message={crumb.ref} /> : crumb.text,
    }));
    return { crumbs, last: flat.at(-1)?.crumb };
  }, [matches]);
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
