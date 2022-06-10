import React from 'react';
import { Breadcrumb } from '@kerberos-io/ui';
// import { Link } from 'react-router-dom';
import styles from './Media.scss';

// eslint-disable-next-line react/prefer-stateless-function
class Media extends React.Component {
  render() {
    return (
      <div className={styles.dashboard}>
        <Breadcrumb
          title="Recordings"
          level1="All your recordings in a single place"
          level1Link=""
        >
          {/* <Link to="/deployments">
            <Button
              label="Add Kerberos Agent"
              icon="plus-circle"
              type="default"
            />
    </Link> */}
        </Breadcrumb>
      </div>
    );
  }
}
export default Media;
