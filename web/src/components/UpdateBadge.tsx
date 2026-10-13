import { ArrowUpCircle } from "lucide-react";
import { useTranslation } from "react-i18next";

// UpdateBadge says that a newer release is waiting, in words.
export function UpdateBadge({ version }: { version: string }) {
  const { t } = useTranslation();
  return (
    <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
      <ArrowUpCircle className="size-3.5" />
      {t("update.available", { version })}
    </span>
  );
}
