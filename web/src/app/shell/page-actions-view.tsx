import {
  isPageActionNode,
  type PageAction,
  usePageChrome,
  useTranslation,
} from "@erpora/platform-web";
import {
  Button,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  resolveIcon,
  useIsMobile,
} from "@erpora/ui";

function ActionIcon({ name }: { name?: string }) {
  const C = resolveIcon(name);
  return C ? <C aria-hidden="true" /> : null;
}

function InlineButton({ action }: { action: PageAction }) {
  return (
    <Button
      type="button"
      variant={action.variant ?? "outline"}
      loading={action.loading}
      disabled={action.disabled}
      onClick={action.onSelect}
    >
      <ActionIcon name={action.icon} />
      {action.label}
    </Button>
  );
}

/**
 * Aksi halaman di kanan navbar. Layar >= md: semua aksi sebagai tombol (simpul kustom ikut tampil). Layar kecil:
 * aksi `pinned` (atau yang pertama bila tak ada) sebagai tombol ikon, sisanya di menu "⋯"; simpul kustom tidak tampil.
 */
export function PageActionsView() {
  const { actions } = usePageChrome();
  const compact = useIsMobile();
  const { t } = useTranslation("platform");
  if (actions.length === 0) return null;

  if (!compact) {
    return (
      <div className="ml-auto flex items-center gap-2">
        {actions.map((a) =>
          isPageActionNode(a) ? (
            <div key={a.id}>{a.node}</div>
          ) : (
            <InlineButton key={a.id} action={a} />
          ),
        )}
      </div>
    );
  }

  const plain = actions.filter((a): a is PageAction => !isPageActionNode(a));
  const pinned = plain.some((a) => a.pinned) ? plain.filter((a) => a.pinned) : plain.slice(0, 1);
  const rest = plain.filter((a) => !pinned.includes(a));
  return (
    <div className="ml-auto flex items-center gap-1">
      {pinned.map((a) => (
        <Button
          key={a.id}
          type="button"
          size="icon"
          variant={a.variant ?? "outline"}
          aria-label={a.label}
          loading={a.loading}
          disabled={a.disabled}
          onClick={a.onSelect}
        >
          <ActionIcon name={a.icon} />
        </Button>
      ))}
      {rest.length > 0 ? (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button type="button" size="icon" variant="ghost" aria-label={t("pageActions.more")}>
              <ActionIcon name="ellipsis" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            {rest.map((a) => (
              <DropdownMenuItem
                key={a.id}
                disabled={a.disabled}
                variant={a.variant === "destructive" ? "destructive" : "default"}
                onSelect={a.onSelect}
              >
                <ActionIcon name={a.icon} />
                {a.label}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      ) : null}
    </div>
  );
}
