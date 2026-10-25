import { useTranslation } from "react-i18next";
import logo from "./logo.svg";
import logoDark from "./logo-dark.svg";

export function App() {
  const { t } = useTranslation();
  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-6 p-8 text-center">
      <picture>
        <source media="(prefers-color-scheme: dark)" srcSet={logoDark} />
        <img src={logo} alt="lotse" className="h-10" />
      </picture>
      <div>
        <h1 className="text-lg font-semibold text-fg">{t("empty.title")}</h1>
        <p className="text-muted">{t("empty.text")}</p>
      </div>
    </main>
  );
}
