import React from 'react';
import { bindActionCreators } from 'redux';
import { connect } from 'react-redux';
import { push } from 'react-router-redux';
import PropTypes from 'prop-types';

export default function (ComposedComponent) {
  class Auth extends React.Component {
    componentDidMount() {
      const {
        isAuthenticated, isInstalled, redirectInstallation, redirectLogin,
      } = this.props;
      if (!isInstalled) {
        redirectInstallation();
      } else if (!isAuthenticated) {
        redirectLogin();
      }
    }

    render() {
      return (
        <div>
          { this.props.isAuthenticated ? <ComposedComponent {...this.props} /> : null }
        </div>
      );
    }
  }

  const mapStateToProps = (state) => ({
    isAuthenticated: state.auth.loggedIn,
    isInstalled: state.auth.installed,
  });

  const mapDispatchToProps = (dispatch) => bindActionCreators({
    redirectLogin: () => push('/login'),
    redirectInstallation: () => push('/install'),
  }, dispatch);

  Auth.propTypes = {
    isAuthenticated: PropTypes.bool.isRequired,
    isInstalled: PropTypes.bool.isRequired,
  };

  return connect(
    mapStateToProps,
    mapDispatchToProps,
  )(Auth);
}
