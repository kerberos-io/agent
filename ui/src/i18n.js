import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import Backend from 'i18next-xhr-backend';
import LanguageDetector from 'i18next-browser-languagedetector';
import translationEN from './locales/en/translation.json';
import translationNL from './locales/nl/translation.json';
import translationFR from './locales/fr/translation.json';

const resources = {
  en: {
    translation: translationEN,
  },
  nl: {
    translation: translationNL,
  },
  fr: {
    translation: translationFR,
  },
};

i18n
  .use(Backend)
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources,
    // lng: 'en',
    fallbackLng: 'en',
    debug: false,
    interpolation: {
      escapeValue: false,
    },
  });

export default i18n;
