import { useSyncExternalStore } from "react";
import { useUiLabel } from "../../labels";
import { Toaster as ShadcnToaster } from "../ui/sonner";

// Pembungkus `Toaster` shadcn (keputusan pemilik produk 31 Okt 2026): berkas registry `ui/sonner.tsx` memakai
// `next-themes` (tidak ada ThemeProvider di aplikasi ini, jadi selalu "system"). Prop `theme` ditimpa dari kelas
// `.dark` pada <html> (MutationObserver) agar Toaster mengikuti tema aplikasi; wilayah aria-live dan tombol tutup
// berlabel dari UiLabels. Pakai `toast` dari "sonner" (`toast.error(..., { action: { label, onClick } })`).
function subscribeTheme(callback: () => void): () => void {
  const observer = new MutationObserver(callback);
  observer.observe(document.documentElement, { attributes: true, attributeFilter: ["class"] });
  return () => observer.disconnect();
}

function documentTheme(): "light" | "dark" {
  return document.documentElement.classList.contains("dark") ? "dark" : "light";
}

export function Toaster(props: React.ComponentProps<typeof ShadcnToaster>) {
  const t = useUiLabel();
  const theme = useSyncExternalStore(subscribeTheme, documentTheme, () => "light" as const);
  return (
    <ShadcnToaster
      theme={theme}
      position="top-center"
      duration={5000}
      closeButton
      containerAriaLabel={t("toast.region")}
      toastOptions={{ closeButtonAriaLabel: t("toast.close") }}
      {...props}
    />
  );
}
