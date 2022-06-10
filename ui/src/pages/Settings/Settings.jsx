import React from 'react';
import { Breadcrumb } from '@kerberos-io/ui';
// import { Link } from 'react-router-dom';
import styles from './Settings.scss';

// eslint-disable-next-line react/prefer-stateless-function
class Settings extends React.Component {
  render() {
    return (
      <div className={styles.dashboard}>
        <Breadcrumb title="Settings" level1="Onboard your camera" level1Link="">
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
export default Settings;
