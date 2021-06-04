import React from 'react';
import classNames from 'classnames';
import styles from './Toggle.module.scss';

export default function Toggle() {
  return (
    <label className={styles.toggle}>
      <input type="checkbox" />
      <span className={classNames(styles.slider, styles.round)} />
    </label>
  );
}
