import React from 'react';
import styles from './Header.module.scss';
import LogoSVG from '../../assets/images/icons/logo-w-border.svg';

export default function Header() {
  return (
    <header className={styles.header}>
      <div className={styles.left}>
        <img src={LogoSVG} alt="Kerberos.io logo" />
        <h1>
          Opensource Agent<span className={styles.version}>v3.0</span>
        </h1>
      </div>
      <div className={styles.center}>
        <ul className={styles.navigation}>
          <li className={styles.active}>Dashboard</li>
          <li>Activity Report</li>
          <li>Settings</li>
          <li>Allien</li>
        </ul>
      </div>
      <div className={styles.right}>recording</div>
    </header>
  );
}
