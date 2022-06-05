import React from 'react';
import styles from './Logo.module.scss';
import LogoSVG from '../../../../assets/images/icons/logo.svg';

export default function Logo() {
  return (
    <div className={styles.logo}>
      <img src={LogoSVG} alt="Kerberos.io logo" />
      <h1>
        Opensource Agent <span className={styles.version}>v3.0</span>
      </h1>
      <h2>Opensource surveillance all-in-one tool for a single camera.</h2>
      <h3>Kerberos.io</h3>
    </div>
  );
}
