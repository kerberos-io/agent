import React from 'react';
// import { connect } from 'react-redux';
// import { withRouter } from 'react-router-dom';
/* import CRUDStorageProvider from '../../components/StorageProvider/CRUDStorageProvider';
import ListStorageProviders from '../../components/StorageProvider/ListStorageProviders';
import {
  getStorageProvider,
  addStorageProvider,
  editStorageProvider,
  removeStorageProvider } from '../../actions'; */
import styles from './Dashboard.module.scss';
import Header from '../../components/Header/Header';
import Warning from '../../components/Warning/Warning';

// eslint-disable-next-line react/prefer-stateless-function
class Dashboard extends React.Component {
  /* constructor() {
    super();
    this.getStorageProviders = this.getStorageProviders.bind(this);
    this.addStorageProvider = this.addStorageProvider.bind(this);
    this.editStorageProvider = this.editStorageProvider.bind(this);
    this.removeStorageProvider = this.removeStorageProvider.bind(this);
  }

  componentDidMount() {
    this.getStorageProviders();
  }

  getStorageProviders(){
    this.props.dispatchGetStorageProvider();
  }

  addStorageProvider(data) {
    this.props.dispatchAddStorageProvider(data, this.getStorageProviders);
  }

  editStorageProvider(data) {
    this.props.dispatchEditStorageProvider(data, this.getStorageProviders);
  }

  removeStorageProvider(data) {
    this.props.dispatchRemoveStorageProvider(data.id, this.getStorageProviders);
  } */

  render() {
    return (
      <div className={styles.dashboard}>
        <Header />
        <Warning />
      </div>
    );
  }
}

// const mapStateToProps = (state, ownProps) => ({
// providers: state.storage.providers,
// });

// const mapDispatchToProps = (dispatch, ownProps) => ({
// dispatchGetStorageProvider: () => dispatch(getStorageProvider()),
// dispatchAddStorageProvider: (data, success) => dispatch(addStorageProvider(data, success)),
// dispatchEditStorageProvider: (data, success) => dispatch(editStorageProvider(data, success)),
// dispatchRemoveStorageProvider: (id, success) => dispatch(removeStorageProvider(id, success))
// });

// export default withRouter(connect(mapStateToProps, mapDispatchToProps)(Dashboard));
export default Dashboard;
