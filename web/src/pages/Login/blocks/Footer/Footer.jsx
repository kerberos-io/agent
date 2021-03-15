import React from 'react';
import styles from './Footer.module.scss';

export default function Footer() {
  return (
    <div className={styles.footer}>
      <ul>
        <li>Kerberos.io</li>
        <li>Github</li>
        <li>Twitter</li>
        <li>Reddit</li>
      </ul>
      <p>A product by verstraeten.io</p>
    </div>
  );
}
