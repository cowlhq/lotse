import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";

const hour = 3600;
const day = 24 * hour;

// Time ranges, in seconds. Beyond two days the hub answers from averages
// of 5 minutes, beyond 30 days from hourly ones.
export const ranges = [hour, 6 * hour, day, 7 * day, 30 * day, 365 * day] as const;
export type RangeSeconds = (typeof ranges)[number];

export function RangePicker({ value, onChange }: { value: RangeSeconds; onChange: (v: RangeSeconds) => void }) {
  const { t } = useTranslation();
  return (
    <ToggleGroup
      value={[String(value)]}
      onValueChange={(v: string[]) => v[0] && onChange(Number(v[0]) as RangeSeconds)}
      variant="outline"
      size="sm"
    >
      {ranges.map((r) => (
        <ToggleGroupItem key={r} value={String(r)} aria-label={label(t, r, "")} title={label(t, r, "")}>
          {label(t, r, "short.")}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}

function label(t: TFunction, r: number, kind: "" | "short.") {
  if (r < 2 * day) return t(`range.${kind}hours`, { count: r / hour });
  if (r < 365 * day) return t(`range.${kind}days`, { count: r / day });
  return t(`range.${kind}years`, { count: r / (365 * day) });
}
