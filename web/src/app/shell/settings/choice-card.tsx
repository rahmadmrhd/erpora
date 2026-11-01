import { cn, Label, RadioGroupItem } from "@erpora/ui";
import { type ReactNode, useId } from "react";

/**
 * Kartu pilihan dengan pratinjau (Tema, Dock): `RadioGroupItem` shadcn di dalam `Label`, sehingga seluruh kartu dapat
 * diklik, operasi keyboard dan ARIA berasal dari Radix, dan nama aksesibel = `label` (pratinjau `aria-hidden`).
 * Harus berada di dalam `RadioGroup`.
 */
export function ChoiceCard({
  value,
  label,
  preview,
}: {
  value: string;
  label: string;
  preview: ReactNode;
}) {
  const id = useId();
  return (
    <Label
      htmlFor={id}
      className={cn(
        "cursor-pointer flex-col items-stretch gap-2 rounded-lg border bg-card p-2 transition-colors hover:bg-accent",
        "has-[[data-state=checked]]:border-primary has-[[data-state=checked]]:ring-2 has-[[data-state=checked]]:ring-ring/40",
        "has-[:focus-visible]:ring-[3px] has-[:focus-visible]:ring-ring/50",
      )}
    >
      <span aria-hidden="true" className="block">
        {preview}
      </span>
      <span className="flex items-center gap-2">
        <RadioGroupItem id={id} value={value} />
        <span>{label}</span>
      </span>
    </Label>
  );
}
