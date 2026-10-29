import {
  describeApiError,
  formatDateTime,
  getActiveLocale,
  Message,
  type MessageArgs,
  msg,
  scopedKey,
  useMessage,
  usePageActions,
  useTenantId,
} from "@erpora/platform-web";
import {
  Alert,
  AlertDescription,
  AlertTitle,
  Button,
  EmptyState,
  Skeleton,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@erpora/ui";
import { useQuery } from "@tanstack/react-query";
import "../i18n/keys.gen";
import { fetchAllNotes, toApiError } from "../notes-api";

// Teks dari namespace bersama (`ui`, `platform`) dirujuk lewat MessageRef biasa: kunci `platform` untuk galat dipilih saat
// runtime (`describeApiError`), dan katalog `ui` belum punya keys.gen.ts.
const retryLabel = { ns: "ui", key: "actions.retry" } as const;

export function NotesPage() {
  const tenantId = useTenantId();
  const query = useQuery({
    queryKey: scopedKey(tenantId, "example", "notes"),
    queryFn: fetchAllNotes,
  });
  const refresh = useMessage(msg("example", "actions.refresh"));
  const emptyTitle = useMessage(msg("example", "page.notes.empty"));
  const notes = query.data ?? [];
  const count = useMessage(msg("example", "page.notes.count"), {
    count: notes.length,
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

  const locale = getActiveLocale() ?? "id";

  return (
    <div className="flex flex-col gap-4 p-6">
      <header className="flex flex-col gap-1">
        <h1 className="text-xl font-semibold">
          <Message message={msg("example", "page.notes.title")} />
        </h1>
        <p className="text-sm text-muted-foreground">
          <Message message={msg("example", "page.notes.description")} />
        </p>
      </header>

      {query.isPending ? (
        <div className="flex flex-col gap-2">
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-full" />
        </div>
      ) : query.isError ? (
        <Alert variant="destructive">
          <AlertTitle>
            <Message message={msg("example", "page.notes.error")} />
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
      ) : notes.length === 0 ? (
        <EmptyState icon="file-text" title={emptyTitle} />
      ) : (
        <>
          <p className="text-sm">{count}</p>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>
                  <Message message={msg("example", "page.notes.columns.title")} />
                </TableHead>
                <TableHead>
                  <Message message={msg("example", "page.notes.columns.version")} />
                </TableHead>
                <TableHead>
                  <Message message={msg("example", "page.notes.columns.updated")} />
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {notes.map((note) => (
                <TableRow key={note.id}>
                  <TableCell>{note.title}</TableCell>
                  <TableCell>{note.version}</TableCell>
                  <TableCell>{formatDateTime(note.updatedAt, locale)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </>
      )}
    </div>
  );
}
