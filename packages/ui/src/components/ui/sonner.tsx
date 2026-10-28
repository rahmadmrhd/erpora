import {
  CircleCheckIcon,
  InfoIcon,
  LoaderCircleIcon,
  OctagonXIcon,
  TriangleAlertIcon,
} from "lucide-react";
import { useSyncExternalStore } from "react";
import { Toaster as Sonner, type ToasterProps } from "sonner";
import { useUiLabel } from "../../labels";

// Hasil `shadcn add sonner`, diadaptasi (M3a K-31): tanpa `next-themes` (tema mengikuti kelas `.dark` pada <html> lewat
// MutationObserver), bergaya token (`popover`, `border`), posisi tengah atas, 5 detik, tombol tutup, wilayah aria-live dan tombol tutup
// berlabel dari UiLabels (`toast.region`, `toast.close`). Pakai `toast` dari "sonner" (`toast.error(..., { action: { label, onClick } })`).
function subscribeTheme(callback: () => void): () => void {
  const observer = new MutationObserver(callback);
  observer.observe(document.documentElement, { attributes: true, attributeFilter: ["class"] });
  return () => observer.disconnect();
}

function documentTheme(): "light" | "dark" {
  return document.documentElement.classList.contains("dark") ? "dark" : "light";
}

const Toaster = ({ ...props }: ToasterProps) => {
  const t = useUiLabel();
  const theme = useSyncExternalStore(subscribeTheme, documentTheme, () => "light" as const);
  return (
    <Sonner
      theme={theme}
      className="toaster group"
      position="top-center"
      duration={5000}
      closeButton
      containerAriaLabel={t("toast.region")}
      toastOptions={{ closeButtonAriaLabel: t("toast.close") }}
      icons={{
        success: <CircleCheckIcon className="size-4" />,
        info: <InfoIcon className="size-4" />,
        warning: <TriangleAlertIcon className="size-4" />,
        error: <OctagonXIcon className="size-4" />,
        loading: <LoaderCircleIcon className="size-4 animate-spin motion-reduce:animate-none" />,
      }}
      style={
        {
          "--normal-bg": "var(--popover)",
          "--normal-text": "var(--popover-foreground)",
          "--normal-border": "var(--border)",
          "--border-radius": "var(--radius)",
        } as React.CSSProperties
      }
      {...props}
    />
  );
};

export { Toaster };
