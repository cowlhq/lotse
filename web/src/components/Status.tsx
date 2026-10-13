import { useTranslation } from "react-i18next";
import { status } from "@/lib/palette";

// Online is a status, so it never relies on color alone: dot and word.
export function Online({ online, revoked = false }: { online: boolean; revoked?: boolean }) {
  const { t } = useTranslation();
  const label = online ? t("status.online") : revoked ? t("status.revoked") : t("status.offline");
  return (
    <span className="inline-flex items-center gap-1.5 text-sm">
      <span className="size-2 rounded-full" style={{ background: online ? status.good : status.critical }} />
      {label}
    </span>
  );
}

// Meter shows a share filled; the fill turns to warning and critical with it.
export function Meter({ pct, color }: { pct: number | null | undefined; color: string }) {
  return (
    <div className="h-1.5 w-full rounded-full bg-muted">
      {pct != null && (
        <div className="h-1.5 rounded-full" style={{ width: `${Math.min(100, Math.max(0, pct))}%`, background: color }} />
      )}
    </div>
  );
}
