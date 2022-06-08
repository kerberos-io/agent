import React from 'react';

import styles from './Dashboard.module.scss';
import Header from '../../components/Header/Header';
import Warning from '../../components/Warning/Warning';

// eslint-disable-next-line react/prefer-stateless-function
class Dashboard extends React.Component {
  render() {
    return (
      <div className={styles.dashboard}>
        <Header />
        <Warning />
      </div>
    );
  }
}
export default Dashboard;
