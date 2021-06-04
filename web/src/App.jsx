import React from 'react';
import PropTypes from 'prop-types';
import styles from './App.module.scss';

export default function App(props) {
  const { children } = props;
  return (
    <div className={styles.body}>
      <header>{children}</header>
    </div>
  );
}

App.propTypes = {
  children: PropTypes.node.isRequired,
};
