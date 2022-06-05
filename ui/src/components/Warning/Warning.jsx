import React from 'react';
import styles from './Warning.module.scss';
import WarningIcon from '../../assets/images/icons/warning.svg';

export default function Warning() {
  return (
    <header className={styles.warning}>
      <img
        src={WarningIcon}
        alt="Warning icon, something is important to be reviewed."
      />
      <h1>Warning</h1>
      <p>Your disk is almost full. Please remove some images..</p>
    </header>
  );
}
