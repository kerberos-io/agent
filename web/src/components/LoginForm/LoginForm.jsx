import React from 'react';
import { connect } from 'react-redux';
import { withRouter } from 'react-router-dom';
import PropTypes from 'prop-types';
import Button from '@material-ui/core/Button';
import TextField from '@material-ui/core/TextField';
// import Typography from '@material-ui/core/Typography';
import { login } from '../../actions';
import './LoginForm.css';

class LoginForm extends React.Component {
  constructor() {
    super();
    this.handleSubmit = this.handleSubmit.bind(this);
  }

  handleSubmit(event) {
    event.preventDefault();
    const { dispatchLogin } = this.props;
    const { target } = event;
    const data = new FormData(target);
    dispatchLogin(data.get('username'), data.get('password'));
  }

  render() {
    const { loginError, error } = this.props;
    return (
      <div className="paper-loginform">
        { loginError && <span className="error">{ error }</span> }
        <h1>Login</h1>
        <form className="form" onSubmit={this.handleSubmit} noValidate>
          <TextField
            variant="outlined"
            margin="normal"
            required
            fullWidth
            className="loginfield"
            id="username"
            label="Username"
            name="username"
          />
          <TextField
            variant="outlined"
            margin="normal"
            required
            fullWidth
            className="passwordfield"
            name="password"
            label="Password"
            type="password"
            id="password"
          />
          <Button
            type="submit"
            fullWidth
            variant="contained"
            color="primary"
            className="signin-button"
          >
            Let&apos;s Go
          </Button>

          <div className="shortcuts">
            Lost Password | Documentation
          </div>
        </form>
      </div>
    );
  }
}

const mapStateToProps = (state) => ({
  loginError: state.auth.loginError,
  error: state.auth.error,
});

const mapDispatchToProps = (dispatch) => ({
  dispatchLogin: (username, password) => {
    dispatch(login(username, password));
  },
});

LoginForm.propTypes = {
  loginError: PropTypes.bool.isRequired,
  error: PropTypes.string.isRequired,
  dispatchLogin: PropTypes.func.isRequired,
};

export default withRouter(connect(mapStateToProps, mapDispatchToProps)(LoginForm));
