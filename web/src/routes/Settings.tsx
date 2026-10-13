import { useTranslation } from "react-i18next";
import { Config } from "@/components/Config";
import { History } from "@/components/History";

// Settings are the global ones: what applies everywhere unless a customer,
// project, environment or host says otherwise.
export function Settings() {
  const { t } = useTranslation();
  return (
    <div className="space-y-6">
      <h1 className="text-xl font-semibold">{t("settings.title")}</h1>
      <Config subject="global:1" />
      <History subject="global:1" />
    </div>
  );
}
