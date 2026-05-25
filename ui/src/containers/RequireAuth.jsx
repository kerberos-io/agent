import React from 'react';
import PropTypes from 'prop-types';
import { useSelector } from 'react-redux';
import { Navigate } from 'react-router-dom';

export default function RequireAuth({ children }) {
  const isAuthenticated = useSelector((s) => s.authentication.loggedIn);
  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
  }
  return children;
}

RequireAuth.propTypes = {
  children: PropTypes.node.isRequired,
};
