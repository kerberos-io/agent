import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import Backend from 'i18next-xhr-backend';
import LanguageDetector from 'i18next-browser-languagedetector';

i18n
  .use(Backend)
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    fallbackLng: 'en',
    debug: false,
    interpolation: {
      escapeValue: false,
    },
    load: 'languageOnly',
    whitelist: ['de', 'en', 'nl', 'fr', 'pl', 'es', 'pt', 'ja', 'ru', 'ro'],
  });

export default i18n;
