import React from 'react';
import { bindActionCreators } from 'redux';
import { connect } from 'react-redux';
import { push } from 'react-router-redux';
import PropTypes from 'prop-types';

export default function RequireAuth(ComposedComponent) {
  class Auth extends React.Component {
    componentDidMount() {
      const {
        isAuthenticated,
        isInstalled,
        redirectInstallation,
        redirectLogin,
      } = this.props;
      if (!isInstalled) {
        redirectInstallation();
      } else if (!isAuthenticated) {
        redirectLogin();
      }
    }

    render() {
      const { isAuthenticated } = this.props;
      return <div>{isAuthenticated ? <ComposedComponent /> : null}</div>;
    }
  }

  const mapStateToProps = (state) => ({
    isAuthenticated: state.auth.loggedIn,
    isInstalled: state.auth.installed,
  });

  const mapDispatchToProps = (dispatch) =>
    bindActionCreators(
      {
        redirectLogin: () => push('/login'),
        redirectInstallation: () => push('/install'),
      },
      dispatch
    );

  Auth.propTypes = {
    isAuthenticated: PropTypes.bool.isRequired,
    isInstalled: PropTypes.bool.isRequired,
    redirectInstallation: PropTypes.func.isRequired,
    redirectLogin: PropTypes.func.isRequired,
  };

  return connect(mapStateToProps, mapDispatchToProps)(Auth);
}
