import { createContext, type MouseEvent, type ReactNode, useContext } from "react";

/** Navigasi gerbang: gerbang dirender sebelum router ada, jadi navigasi disuntikkan oleh shell (history API). */
export type GateNavigate = (to: string) => void;

const NavContext = createContext<GateNavigate>(() => {});

export function GateNavProvider({
  navigate,
  children,
}: {
  navigate: GateNavigate;
  children: ReactNode;
}) {
  return <NavContext.Provider value={navigate}>{children}</NavContext.Provider>;
}

export function useGateNav(): GateNavigate {
  return useContext(NavContext);
}

/** Tautan internal gerbang: `href` nyata (buka di tab baru bekerja), klik biasa dialihkan ke navigasi gerbang. */
export function GateLink({
  to,
  className,
  children,
}: {
  to: string;
  className?: string;
  children: ReactNode;
}) {
  const navigate = useGateNav();
  const onClick = (e: MouseEvent<HTMLAnchorElement>) => {
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) {
      return;
    }
    e.preventDefault();
    navigate(to);
  };
  return (
    <a
      href={to}
      onClick={onClick}
      className={className ?? "text-sm underline underline-offset-4 hover:text-primary"}
    >
      {children}
    </a>
  );
}
