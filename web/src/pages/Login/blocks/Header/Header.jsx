import React from 'react';
import styles from './Header.module.scss';
import Logo from '../Logo/Logo';
import Navigation from '../Navigation/Navigation';

export default function Header() {
  return (
    <header className={styles.header}>
      <Navigation />
      <Logo />
    </header>
  );
}
