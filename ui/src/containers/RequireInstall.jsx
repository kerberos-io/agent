import React from 'react';
import PropTypes from 'prop-types';
import { useSelector } from 'react-redux';
import { Navigate } from 'react-router-dom';

export default function RequireInstall({ children }) {
  const isAuthenticated = useSelector((s) => s.authentication.loggedIn);
  const isInstalled = useSelector((s) => s.authentication.installed);
  if (isInstalled) {
    return <Navigate to={isAuthenticated ? '/' : '/login'} replace />;
  }
  return children;
}

RequireInstall.propTypes = {
  children: PropTypes.node.isRequired,
};
