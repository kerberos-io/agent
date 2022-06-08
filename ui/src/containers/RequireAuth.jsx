import React from 'react';
import { bindActionCreators } from 'redux';
import { connect } from 'react-redux';
import { push } from 'react-router-redux';
import PropTypes from 'prop-types';

export default function RequireAuth(ComposedComponent) {
  class Auth extends React.Component {
    componentDidMount() {
      const { isAuthenticated, redirect } = this.props;
      console.log(isAuthenticated);
      if (!isAuthenticated) {
        redirect();
      }
    }

    render() {
      const { isAuthenticated } = this.props;
      return (
        <div>
          {/* eslint-disable-next-line react/jsx-props-no-spreading */}
          {isAuthenticated ? <ComposedComponent {...this.props} /> : null}
        </div>
      );
    }
  }

  const mapStateToProps = (state) => ({
    isAuthenticated: state.auth.loggedIn,
    isInstalled: state.auth.installed,
  });

  const mapDispatchToProps = (dispatch) =>
    bindActionCreators(
      {
        redirect: () => push('/login'),
      },
      dispatch
    );

  Auth.propTypes = {
    isAuthenticated: PropTypes.bool.isRequired,
    isInstalled: PropTypes.bool.isRequired,
    redirect: PropTypes.func.isRequired,
  };

  return connect(mapStateToProps, mapDispatchToProps)(Auth);
}
