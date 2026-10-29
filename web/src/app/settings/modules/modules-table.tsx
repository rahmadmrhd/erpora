import {
  Badge,
  BadgeStatus,
  Button,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
  resolveIcon,
  type StatusTone,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@erpora/ui";
import { availableOps } from "./impact";
import { opKey, statusKey, useModulesT } from "./keys";
import type { ModuleOp, ModuleStatus, ModuleView } from "./types";

/** Peta status ke nada `BadgeStatus` (design §7). */
export const STATUS_TONE: Record<ModuleStatus, StatusTone> = {
  enabled: "success",
  disabled: "secondary",
  available: "secondary",
  uninstalled: "secondary",
  installed: "primary",
  upgrade_pending: "warning",
  failed: "error",
  missing: "error",
  incompatible: "error",
};

function hasUpgrade(m: ModuleView): boolean {
  return (
    m.status === "upgrade_pending" ||
    (m.installedVersion !== null && m.installedVersion !== m.binaryVersion)
  );
}

export interface ModulesTableProps {
  modules: readonly ModuleView[];
  /** `false` menyembunyikan kolom Aksi (mode hanya-baca); tombol Riwayat pindah ke sel nama. */
  showActions: boolean;
  onOperation(op: ModuleOp, module: ModuleView): void;
  onHistory(module: ModuleView): void;
}

export function ModulesTable({ modules, showActions, onOperation, onHistory }: ModulesTableProps) {
  const t = useModulesT();
  const MoreIcon = resolveIcon("ellipsis");

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t("modules.columns.name")}</TableHead>
          <TableHead>{t("modules.columns.version")}</TableHead>
          <TableHead>{t("modules.columns.status")}</TableHead>
          <TableHead>{t("modules.columns.dependencies")}</TableHead>
          <TableHead>{t("modules.columns.dependents")}</TableHead>
          {showActions ? <TableHead>{t("modules.columns.actions")}</TableHead> : null}
        </TableRow>
      </TableHeader>
      <TableBody>
        {modules.map((m) => {
          const ops = availableOps(m);
          return (
            <TableRow key={m.key}>
              <TableCell className="align-top">
                <div className="flex flex-col gap-1">
                  <span className="flex items-center gap-2 font-medium">
                    {m.name}
                    {m.required ? <Badge variant="outline">{t("modules.required")}</Badge> : null}
                  </span>
                  <code className="font-mono text-xs text-muted-foreground">{m.key}</code>
                  {showActions ? null : (
                    // Mode hanya-baca: kolom Aksi hilang, tetapi Riwayat adalah aksi baca (Q39 butir 2).
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-auto w-fit p-0"
                      aria-label={t("modules.history.title", { name: m.name })}
                      onClick={() => onHistory(m)}
                    >
                      {t("modules.op.history")}
                    </Button>
                  )}
                </div>
              </TableCell>
              <TableCell className="align-top">
                <div className="flex flex-col gap-0.5 text-sm">
                  <span>
                    {m.installedVersion === null
                      ? t("modules.version.notInstalled")
                      : t("modules.version.installed", { version: m.installedVersion })}
                  </span>
                  <span className="text-muted-foreground">
                    {t("modules.version.binary", { version: m.binaryVersion })}
                  </span>
                  {hasUpgrade(m) ? (
                    <Badge variant="secondary" className="w-fit">
                      {t("modules.version.upgradeAvailable")}
                    </Badge>
                  ) : null}
                </div>
              </TableCell>
              <TableCell className="align-top">
                <BadgeStatus tone={STATUS_TONE[m.status]}>{t(statusKey(m.status))}</BadgeStatus>
              </TableCell>
              <TableCell className="align-top">
                {m.dependencies.length === 0 ? (
                  <span className="text-muted-foreground">{t("modules.dependencies.none")}</span>
                ) : (
                  <ul className="flex flex-col gap-0.5 text-sm">
                    {m.dependencies.map((d) => (
                      <li key={d.module}>
                        {d.optional
                          ? t("modules.dependencies.optional", { name: d.module })
                          : d.module}
                      </li>
                    ))}
                  </ul>
                )}
              </TableCell>
              <TableCell className="align-top">
                {m.dependents.length === 0 ? (
                  <span className="text-muted-foreground">{t("modules.dependencies.none")}</span>
                ) : (
                  <ul className="flex flex-col gap-0.5 text-sm">
                    {m.dependents.map((d) => (
                      <li key={d}>{d}</li>
                    ))}
                  </ul>
                )}
              </TableCell>
              {showActions ? (
                <TableCell className="align-top">
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label={t("modules.op.menu", { name: m.name })}
                      >
                        {MoreIcon ? <MoreIcon /> : null}
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      {ops.map((op) => (
                        <DropdownMenuItem key={op} onSelect={() => onOperation(op, m)}>
                          {t(opKey(op))}
                        </DropdownMenuItem>
                      ))}
                      {ops.length > 0 ? <DropdownMenuSeparator /> : null}
                      <DropdownMenuItem onSelect={() => onHistory(m)}>
                        {t("modules.op.history")}
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </TableCell>
              ) : null}
            </TableRow>
          );
        })}
      </TableBody>
    </Table>
  );
}
