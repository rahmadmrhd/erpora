import { formatRelativeTime, useTranslation } from "@erpora/platform-web";
import {
  Button,
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  EmptyState,
  Skeleton,
} from "@erpora/ui";
import { useState } from "react";
import { useModuleHistory, useOperation } from "./api";
import { type ModulesKey, useModulesT } from "./keys";
import type { EventView, ModuleView } from "./types";

const KNOWN_EVENTS = new Set([
  "installed",
  "enabled",
  "disabled",
  "upgraded",
  "uninstalled",
  "operation_failed",
  "interrupted",
  "retried",
  "reconciled",
]);

/** Langkah operasi: diminta hanya saat baris dibuka. `error` langkah dari server tidak ditampilkan (bisa memuat detail internal). */
function OperationSteps({ operationId, open }: { operationId: string; open: boolean }) {
  const t = useModulesT();
  const query = useOperation(operationId, open);
  if (query.isPending) return <Skeleton className="h-5 w-48" />;
  if (query.isError)
    return <p className="text-sm text-muted-foreground">{t("modules.history.error")}</p>;
  const steps = query.data.steps;
  if (steps.length === 0)
    return <p className="text-sm text-muted-foreground">{t("modules.history.stepsEmpty")}</p>;
  return (
    <ul className="text-sm text-muted-foreground">
      {steps.map((s) => (
        <li key={s.name}>
          {t("modules.history.step", { name: s.name, status: s.status, durationMs: s.durationMs })}
        </li>
      ))}
    </ul>
  );
}

function EventRow({ event, locale }: { event: EventView; locale: string }) {
  const t = useModulesT();
  const [stepsOpen, setStepsOpen] = useState(false);
  const [detailsOpen, setDetailsOpen] = useState(false);
  const hasDetails = Object.keys(event.details ?? {}).length > 0;
  const label = KNOWN_EVENTS.has(event.event)
    ? t(`modules.history.event.${event.event}` as ModulesKey as "modules.history.event.enabled")
    : t("modules.history.event.unknown", { event: event.event });

  return (
    <li className="flex flex-col gap-1 border-b py-3 last:border-b-0">
      <div className="flex flex-wrap items-baseline gap-x-3">
        <span className="font-medium">{label}</span>
        <span className="text-sm text-muted-foreground">
          {t("modules.history.actor", { actor: event.actor })}
        </span>
        <time className="text-sm text-muted-foreground" dateTime={event.occurredAt}>
          {formatRelativeTime(event.occurredAt, locale)}
        </time>
      </div>
      {event.operationId ? (
        <Collapsible open={stepsOpen} onOpenChange={setStepsOpen}>
          <CollapsibleTrigger asChild>
            <Button variant="ghost" size="sm" aria-expanded={stepsOpen}>
              {t("modules.history.operationSteps")}
            </Button>
          </CollapsibleTrigger>
          <CollapsibleContent>
            <OperationSteps operationId={event.operationId} open={stepsOpen} />
          </CollapsibleContent>
        </Collapsible>
      ) : null}
      {hasDetails ? (
        <Collapsible open={detailsOpen} onOpenChange={setDetailsOpen}>
          <CollapsibleTrigger asChild>
            <Button variant="ghost" size="sm">
              {t("modules.history.details")}
            </Button>
          </CollapsibleTrigger>
          <CollapsibleContent>
            {/* Teks apa adanya (bukan HTML): React meng-escape isinya. */}
            <pre className="max-h-60 overflow-auto rounded-md bg-muted p-2 text-xs">
              {JSON.stringify(event.details, null, 2)}
            </pre>
          </CollapsibleContent>
        </Collapsible>
      ) : null}
    </li>
  );
}

export function HistoryDialog({ module, onClose }: { module: ModuleView; onClose(): void }) {
  const t = useModulesT();
  const { locale } = useTranslation("platform");
  const { t: tui } = useTranslation("ui");
  const [page, setPage] = useState(1);
  const query = useModuleHistory(module.key, page);

  return (
    <Dialog open onOpenChange={(open) => (open ? undefined : onClose())}>
      <DialogContent size="wide">
        <DialogHeader>
          <DialogTitle>{t("modules.history.title", { name: module.name })}</DialogTitle>
          <DialogDescription>{t("modules.history.description")}</DialogDescription>
        </DialogHeader>
        {query.isPending ? (
          <div className="flex flex-col gap-2">
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-full" />
          </div>
        ) : query.isError ? (
          <p role="alert" className="text-sm">
            {t("modules.history.error")}
          </p>
        ) : query.data.events.length === 0 ? (
          <EmptyState icon="file-text" title={t("modules.history.empty")} />
        ) : (
          <ul className="max-h-[60vh] overflow-auto">
            {query.data.events.map((e) => (
              <EventRow key={e.id} event={e} locale={locale} />
            ))}
          </ul>
        )}
        <div className="flex items-center justify-between gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={page <= 1}
            onClick={() => setPage((p) => p - 1)}
          >
            {tui("actions.previous")}
          </Button>
          <span className="text-sm text-muted-foreground">
            {t("modules.history.page", { page })}
          </span>
          <Button
            variant="outline"
            size="sm"
            disabled={!query.data?.hasMore}
            onClick={() => setPage((p) => p + 1)}
          >
            {tui("actions.next")}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
