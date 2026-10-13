import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import en from "./locales/en.json";

// Every text the UI shows comes from src/locales, so that languages can be
// added without touching components.
i18n.use(initReactI18next).init({
  resources: { en: { translation: en } },
  lng: "en",
  fallbackLng: "en",
  interpolation: { escapeValue: false },
  // Event kinds such as "container.health_status: unhealthy" are keys;
  // ":" must not split them into a namespace.
  nsSeparator: false,
});

export default i18n;
