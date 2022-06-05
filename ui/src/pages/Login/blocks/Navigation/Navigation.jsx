import React from 'react';
import styles from './Navigation.module.scss';

export default function Navigation() {
  return (
    <ul className={styles.topnavigation}>
      <li>Camera Agent</li>
      <li>Dashboard</li>
      <li>Vault</li>
      <li>Enterprise Agent</li>
    </ul>
  );
}
