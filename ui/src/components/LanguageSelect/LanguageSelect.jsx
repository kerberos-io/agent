import React from 'react';
import i18next from 'i18next';
import Popover from '@material-ui/core/Popover';
import List from '@material-ui/core/List';
import ListItem from '@material-ui/core/ListItem';
import ListSubheader from '@material-ui/core/ListSubheader';
import { Icon } from '@kerberos-io/ui';
import { useTranslation } from 'react-i18next';
import './LanguageSelect.scss';

const LanguageSelect = () => {
  let selected = localStorage.getItem('i18nextLng') || i18next.language || 'en';
  const languageMap = {
    en: {
      label: 'English',
      dir: 'ltr',
      active: true,
    },
    nl: { label: 'Nederlands', dir: 'ltr', active: false },
    fr: { label: 'Francais', dir: 'ltr', active: false },
    pl: { label: 'Polski', dir: 'ltr', active: false },
    de: { label: 'Deutsch', dir: 'ltr', active: false },
    pt: { label: 'Português', dir: 'ltr', active: false },
    es: { label: 'Español', dir: 'ltr', active: false },
  };

  if (!languageMap[selected]) {
    selected = 'en';
  }

  const [menuAnchor, setMenuAnchor] = React.useState(null);
  React.useEffect(() => {
    document.body.dir = languageMap[selected].dir;
  }, [menuAnchor, selected]);

  const { t } = useTranslation();

  return (
    <>
      <li
        id="language-picker"
        onClick={({ currentTarget }) => setMenuAnchor(currentTarget)}
      >
        <a>
          <Icon label="world" />
          <span>{languageMap[selected].label}</span>
        </a>
      </li>

      <Popover
        open={!!menuAnchor}
        anchorEl={menuAnchor}
        onClose={() => setMenuAnchor(null)}
        anchorOrigin={{
          vertical: 'bottom',
          horizontal: 'left',
        }}
        transformOrigin={{
          vertical: 'top',
          horizontal: 'left',
        }}
      >
        <div>
          <List>
            <ListSubheader>{t('navigation.choose_language')}</ListSubheader>
            {Object.keys(languageMap)?.map((item) => (
              <ListItem
                button
                key={item}
                onClick={() => {
                  i18next.changeLanguage(item);
                  localStorage.setItem('i18nextLng', item);
                  setMenuAnchor(null);
                }}
              >
                {languageMap[item] ? languageMap[item].label : ''}
              </ListItem>
            ))}
            <hr />
            <a
              href="https://github.com/kerberos-io/agent/issues/47"
              rel="noreferrer"
              target="_blank"
            >
              <ListItem button key="contribute-language">
                Contribute language
              </ListItem>
            </a>
          </List>
        </div>
      </Popover>
    </>
  );
};

export default LanguageSelect;
