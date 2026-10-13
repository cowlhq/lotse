import { useTranslation } from "react-i18next";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";

// Time ranges, in seconds. Raw readings are kept for 48 hours.
export const ranges = [3600, 6 * 3600, 24 * 3600, 48 * 3600] as const;
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
        <ToggleGroupItem key={r} value={String(r)}>
          {t("range.hours", { count: r / 3600 })}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}
