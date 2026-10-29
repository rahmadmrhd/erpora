import { moduleInfo as exampleModule } from "@erpora/module-example";
import { Link } from "@erpora/module-sdk";
import {
  describeApiError,
  Message,
  type MessageArgs,
  msg,
  scopedKey,
  useMessage,
  usePageActions,
  useTenantId,
} from "@erpora/platform-web";
import { Alert, AlertDescription, AlertTitle, Button, Skeleton } from "@erpora/ui";
import { useQuery } from "@tanstack/react-query";
import "../i18n/keys.gen";
import { fetchSummary, toApiError } from "../summary-api";

// Teks dari namespace bersama (`ui`, `platform`) dirujuk lewat MessageRef biasa (kunci galat dipilih saat runtime).
const retryLabel = { ns: "ui", key: "actions.retry" } as const;

export function SummaryPage() {
  const tenantId = useTenantId();
  const query = useQuery({
    queryKey: scopedKey(tenantId, "example-addon", "summary"),
    queryFn: fetchSummary,
  });
  const refresh = useMessage(msg("example-addon", "actions.refresh"));
  const noteCount = useMessage(msg("example-addon", "page.summary.note_count"), {
    count: query.data?.noteCount ?? 0,
  } satisfies MessageArgs);

  usePageActions([
    {
      id: "refresh",
      label: refresh,
      icon: "refresh-cw",
      variant: "outline",
      loading: query.isFetching,
      onSelect: () => {
        void query.refetch();
      },
    },
  ]);

  return (
    <div className="flex flex-col gap-4 p-6">
      <header className="flex flex-col gap-1">
        <h1 className="text-xl font-semibold">
          <Message message={msg("example-addon", "page.summary.title")} />
        </h1>
        <p className="text-sm text-muted-foreground">
          <Message message={msg("example-addon", "page.summary.description")} />
        </p>
      </header>

      {query.isPending ? (
        <Skeleton className="h-8 w-48" />
      ) : query.isError ? (
        <Alert variant="destructive">
          <AlertTitle>
            <Message message={msg("example-addon", "page.summary.error")} />
          </AlertTitle>
          <AlertDescription>
            <p>
              <Message
                message={{ ns: "platform", key: describeApiError(toApiError(query.error)).key }}
              />
            </p>
            <Button
              variant="outline"
              size="sm"
              loading={query.isFetching}
              onClick={() => {
                void query.refetch();
              }}
            >
              <Message message={retryLabel} />
            </Button>
          </AlertDescription>
        </Alert>
      ) : (
        <div className="flex items-center gap-4">
          <p className="text-2xl font-semibold">{noteCount}</p>
          <Button asChild variant="outline" size="sm">
            {/* Tautan antar-modul: slug dari entry paket @erpora/module-example (bukan deep import; aturan F2). */}
            <Link to={`/${exampleModule.slug}`}>
              <Message message={msg("example-addon", "page.summary.view_notes")} />
            </Link>
          </Button>
        </div>
      )}
    </div>
  );
}
