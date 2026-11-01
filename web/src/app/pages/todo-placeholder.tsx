import { useTranslation } from "@erpora/platform-web";
import { EmptyNotice } from "../components/notices";

/** Placeholder ToDo (`/platform/todo`): daftar tugas nyata hadir pada pembaruan berikutnya (revisi M3a R5.3). */
export function TodoPlaceholderPage() {
  const { t } = useTranslation("platform");
  return (
    <EmptyNotice
      className="min-h-[60vh] justify-center"
      icon="clipboard-list"
      title={t("todo.title")}
      description={t("todo.description")}
    />
  );
}
