import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { checkSetupToken, SetupDone } from "../api";

export function Setup({ token }: { token: string }) {
  const { t } = useTranslation();
  const check = useQuery({
    queryKey: ["setup", token],
    queryFn: () => checkSetupToken(token),
    retry: false,
  });

  let message: string;
  if (check.isPending) message = t("setup.checking");
  else if (check.error instanceof SetupDone) message = t("setup.done");
  else if (check.isError || !check.data.valid) message = t("setup.invalid");
  else message = t("setup.valid");

  return (
    <div className="space-y-2">
      <h2 className="text-lg font-medium">{t("setup.title")}</h2>
      <p>{message}</p>
    </div>
  );
}
