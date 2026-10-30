import {
  describeApiError,
  formatDateTime,
  getActiveLocale,
  Message,
  type MessageArgs,
  msg,
  useDb,
  useMessage,
  usePageActions,
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
import { useCallback, useState } from "react";
import { useNotesCollection } from "../db/notes-collection";
import "../i18n/keys.gen";
import { toApiError } from "../notes-api";

// Teks dari namespace bersama (`ui`, `platform`) dirujuk lewat MessageRef biasa: kunci `platform` untuk galat dipilih saat
// runtime (`describeApiError`), dan katalog `ui` belum punya keys.gen.ts.
const retryLabel = { ns: "ui", key: "actions.retry" } as const;

export function NotesPage() {
  // Menangguhkan (Suspense) sampai chunk TanStack DB termuat; DB tidak ada di JS awal (K-21).
  const collection = useNotesCollection();
  const { useLiveQuery } = useDb();
  const { data, isLoading, isError } = useLiveQuery({
    query: (q) => q.from({ note: collection }).orderBy(({ note }) => note.updatedAt, "desc"),
  });

  const [refreshing, setRefreshing] = useState(false);
  const refetch = useCallback(async () => {
    setRefreshing(true);
    try {
      await collection.utils.refetch();
    } catch {
      // Galat tampil lewat status koleksi (Alert); tidak perlu dilempar ulang ke penangan klik.
    } finally {
      setRefreshing(false);
    }
  }, [collection]);

  const refresh = useMessage(msg("example", "actions.refresh"));
  const emptyTitle = useMessage(msg("example", "page.notes.empty"));
  const notes = data;
  const count = useMessage(msg("example", "page.notes.count"), {
    count: notes.length,
  } satisfies MessageArgs);

  usePageActions([
    {
      id: "refresh",
      label: refresh,
      icon: "refresh-cw",
      variant: "outline",
      loading: refreshing,
      onSelect: () => {
        void refetch();
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

      {isLoading ? (
        <div className="flex flex-col gap-2">
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-full" />
        </div>
      ) : isError ? (
        <Alert variant="destructive">
          <AlertTitle>
            <Message message={msg("example", "page.notes.error")} />
          </AlertTitle>
          <AlertDescription>
            <p>
              <Message
                message={{
                  ns: "platform",
                  key: describeApiError(toApiError(collection.utils.lastError)).key,
                }}
              />
            </p>
            <Button
              variant="outline"
              size="sm"
              loading={refreshing}
              onClick={() => {
                void refetch();
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
