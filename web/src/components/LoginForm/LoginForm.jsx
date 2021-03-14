import React from 'react';
import { connect } from 'react-redux';
import { withRouter } from 'react-router-dom';
import PropTypes from 'prop-types';
import { login } from '../../actions';
import styles from './LoginForm.module.scss';

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
      <div className={styles.wrappper}>
        <div className={styles.loginform}>
          {loginError && <span className="error">{error}</span>}
          <header>
            <h1>Login</h1>
          </header>
          <section>
            <form onSubmit={this.handleSubmit} noValidate>
              <label htmlFor="username">Username</label>
              <div className={styles.input}>
                <input
                  type="text"
                  id="username"
                  name="username"
                  placeholder="Username"
                />
              </div>
              <label htmlFor="password">Password</label>
              <div className={styles.input}>
                <input
                  type="password"
                  id="password"
                  name="password"
                  placeholder="Password"
                />
              </div>
              <button type="submit">Login</button>
            </form>
          </section>
        </div>
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

export default withRouter(
  connect(mapStateToProps, mapDispatchToProps)(LoginForm)
);

/*
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
          */
