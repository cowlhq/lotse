import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

// LiveBadge says that readings are arriving live: shown while the last one
// is at most 10 seconds old.
export function LiveBadge({ last }: { last: number }) {
  const { t } = useTranslation();
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 2000);
    return () => clearInterval(timer);
  }, []);
  if (now - last > 10_000) return null;
  return (
    <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
      <span className="size-1.5 animate-pulse rounded-full bg-current" />
      {t("live")}
    </span>
  );
}
