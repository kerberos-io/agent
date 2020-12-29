import React from 'react';
import CssBaseline from '@material-ui/core/CssBaseline';
import Container from '@material-ui/core/Container';
import LoginForm from '../../components/LoginForm/LoginForm';
import './Login.css';

export default function Login() {
  return (
    <Container className="login-body" component="main" >
      <CssBaseline />
      <Container maxWidth="xs" className="login-container">
        <LoginForm className="login-form" />
      </Container>
    </Container>
  );
}
