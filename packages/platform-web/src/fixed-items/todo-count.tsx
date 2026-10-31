import { useQuery } from "@tanstack/react-query";
import { createContext, type ReactNode, useContext } from "react";
import { useTenantId } from "../bootstrap";
import { scopedKey } from "../tenancy";

/**
 * Sumber jumlah ToDo untuk lencana item tetap. Sementara (modul tugas belum ada): sumber bawaan selalu 0 sehingga
 * lencana tidak tampil; modul tugas kelak memasang sumber sungguhan tanpa mengubah panel.
 */
export interface TodoCountSource {
  count(signal?: AbortSignal): Promise<number>;
}

export const emptyTodoCountSource: TodoCountSource = Object.freeze({
  count: () => Promise.resolve(0),
});

const TodoCountSourceContext = createContext<TodoCountSource>(emptyTodoCountSource);

export function TodoCountSourceProvider({
  source,
  children,
}: {
  source: TodoCountSource;
  children: ReactNode;
}) {
  return (
    <TodoCountSourceContext.Provider value={source}>{children}</TodoCountSourceContext.Provider>
  );
}

export function useTodoCountSource(): TodoCountSource {
  return useContext(TodoCountSourceContext);
}

/** Jumlah ToDo (0 saat memuat atau galat); data hanya di cache memori dengan key berawalan tenant. */
export function useTodoCount(): number {
  const source = useTodoCountSource();
  const tenantId = useTenantId();
  const query = useQuery({
    queryKey: scopedKey(tenantId, "todo", "count"),
    queryFn: ({ signal }) => source.count(signal),
    refetchInterval: source === emptyTodoCountSource ? false : 60_000,
  });
  return query.isError ? 0 : (query.data ?? 0);
}
