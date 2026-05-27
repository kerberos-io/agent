import React from 'react';
import PropTypes from 'prop-types';
import { useSelector } from 'react-redux';
import { Redirect } from 'react-router-dom';

export default function RequireGuest({ children }) {
  const isAuthenticated = useSelector((s) => s.authentication.loggedIn);
  const isInstalled = useSelector((s) => s.authentication.installed);
  if (!isInstalled) {
    return <Redirect to="/install" />;
  }
  if (isAuthenticated) {
    return <Redirect to="/" />;
  }
  return children;
}

RequireGuest.propTypes = {
  children: PropTypes.node.isRequired,
};
