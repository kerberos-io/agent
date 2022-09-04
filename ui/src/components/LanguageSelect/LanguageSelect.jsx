import React from 'react';
import { useTranslation } from 'react-i18next';
import i18next from 'i18next';
import Popover from '@material-ui/core/Popover';
import List from '@material-ui/core/List';
import ListItem from '@material-ui/core/ListItem';
import ListSubheader from '@material-ui/core/ListSubheader';
import { Icon } from '@kerberos-io/ui';
import './LanguageSelect.scss';

const LanguageSelect = () => {
  const selected = localStorage.getItem('i18nextLng') || 'en=US';
  const { t } = useTranslation();

  const languageMap = {
    'en-US': {
      label: t('navigation.languages.english'),
      dir: 'ltr',
      active: true,
    },
    nl: { label: t('navigation.languages.dutch'), dir: 'ltr', active: false },
    fr: { label: t('navigation.languages.french'), dir: 'ltr', active: false },
  };

  const [menuAnchor, setMenuAnchor] = React.useState(null);
  React.useEffect(() => {
    document.body.dir = languageMap[selected].dir;
  }, [menuAnchor, selected]);

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
            <ListSubheader>{t('navigation.select_language')}</ListSubheader>
            {Object.keys(languageMap)?.map((item) => (
              <ListItem
                button
                key={item}
                onClick={() => {
                  i18next.changeLanguage(item);
                  setMenuAnchor(null);
                }}
              >
                {languageMap[item].label}
              </ListItem>
            ))}
          </List>
        </div>
      </Popover>
    </>
  );
};

export default LanguageSelect;
