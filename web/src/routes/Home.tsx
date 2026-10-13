import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { getHealth } from "../api";

export function Home() {
  const { t } = useTranslation();
  const health = useQuery({ queryKey: ["health"], queryFn: getHealth, refetchInterval: 10_000 });

  if (health.isPending) return <p className="text-slate-500">{t("app.loading")}</p>;
  if (health.isError) return <p className="text-red-600">{t("app.unreachable")}</p>;

  return (
    <div className="space-y-2">
      <p className="flex items-center gap-2">
        <span className="size-2.5 rounded-full bg-emerald-500" />
        {t("home.running")}
      </p>
      <p className="text-sm text-slate-500">
        {t("home.version", { version: health.data.version })} ·{" "}
        {t("home.database", { database: health.data.database })}
      </p>
      {health.data.setup && <p className="text-amber-700">{t("home.notSetUp")}</p>}
    </div>
  );
}
