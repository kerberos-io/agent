import React from 'react';
import { bindActionCreators } from 'redux';
import { connect } from 'react-redux';
import { push } from 'react-router-redux';
import PropTypes from 'prop-types';

export default function (ComposedComponent) {
  class Install extends React.Component {
    componentDidMount() {
      const { isAuthenticated, isInstalled, redirectDashboard, redirectLogin } = this.props;
      if (isInstalled) {
        if (isAuthenticated) {
          redirectDashboard();
        } else if (!isAuthenticated) {
          redirectLogin();
        }
      }
    }
    render() {
      return (
        <div>
          { !this.props.isInstalled ? <ComposedComponent {...this.props} /> : null }
        </div>
      );
    }
  }

  const mapStateToProps = (state) => {
    return {
      isAuthenticated: state.auth.loggedIn,
      isInstalled: state.auth.installed
    };
  };

  const mapDispatchToProps = dispatch => bindActionCreators({
    redirectDashboard: () => push('/'),
    redirectLogin: () => push('/login')
  }, dispatch)

  Install.propTypes = {
    isAuthenticated: PropTypes.bool.isRequired,
    isInstalled: PropTypes.bool.isRequired
  };

  return connect(
    mapStateToProps,
    mapDispatchToProps
  )(Install);
}
