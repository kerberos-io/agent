import React from 'react';
import { bindActionCreators } from 'redux';
import { connect } from 'react-redux';
import { push } from 'react-router-redux';
import PropTypes from 'prop-types';

export default function RequireGuest(ComposedComponent) {
  class Guest extends React.Component {
    componentDidMount() {
      const {
        isAuthenticated, isInstalled, redirectInstallation, redirectDashboard,
      } = this.props;
      if (!isInstalled) {
        redirectInstallation();
      } else if (isAuthenticated) {
        redirectDashboard();
      }
    }

    render() {
      const { isAuthenticated } = this.props;
      return (
        <div>
          { !isAuthenticated ? <ComposedComponent /> : null }
        </div>
      );
    }
  }

  const mapStateToProps = (state) => ({
    isAuthenticated: state.auth.loggedIn,
    isInstalled: state.auth.installed,
  });

  const mapDispatchToProps = (dispatch) => bindActionCreators({
    redirectDashboard: () => push('/'),
    redirectInstallation: () => push('/install'),
  }, dispatch);

  Guest.propTypes = {
    isAuthenticated: PropTypes.bool.isRequired,
    isInstalled: PropTypes.bool.isRequired,
    redirectInstallation: PropTypes.func.isRequired,
    redirectDashboard: PropTypes.func.isRequired,
  };

  return connect(
    mapStateToProps,
    mapDispatchToProps,
  )(Guest);
}
