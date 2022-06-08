import React from 'react';
import { connect } from 'react-redux';
import classNames from 'classnames';
import { withRouter } from 'react-router-dom';
import PropTypes from 'prop-types';
import { login } from '../../../../actions';
import styles from './LoginForm.module.scss';
import LoginIcon from '../../../../assets/images/icons/login.svg';
import ShowPasswordIcon from '../../../../assets/images/icons/discover-password.svg';

class LoginForm extends React.Component {
  constructor() {
    super();
    this.state = {
      passwordVisible: false,
    };
    this.handleSubmit = this.handleSubmit.bind(this);
    this.togglePasswordVisible = this.togglePasswordVisible.bind(this);
  }

  handleSubmit(event) {
    event.preventDefault();
    const { dispatchLogin } = this.props;
    const { target } = event;
    const data = new FormData(target);
    dispatchLogin(data.get('username'), data.get('password'));
  }

  togglePasswordVisible() {
    const { passwordVisible } = this.state;
    this.setState({
      passwordVisible: !passwordVisible,
    });
  }

  render() {
    const { loginError, error } = this.props;
    const { passwordVisible } = this.state;
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
              <div className={classNames(styles.input, styles.username)}>
                <input
                  type="text"
                  id="username"
                  name="username"
                  placeholder="Your username"
                />
              </div>
              <label htmlFor="password">Password</label>
              <div className={classNames(styles.input, styles.password)}>
                <div className={styles.passwordWrapper}>
                  <input
                    type={passwordVisible ? 'text' : 'password'}
                    id="password"
                    name="password"
                    placeholder="Your password"
                  />
                  <div
                    role="button"
                    tabIndex={0}
                    className={styles.showPassword}
                    onClick={() => this.togglePasswordVisible()}
                    onKeyDown={() => this.togglePasswordVisible()}
                  >
                    <img src={ShowPasswordIcon} alt="Show password button" />
                  </div>
                </div>
              </div>
              <button type="submit">
                <img src={LoginIcon} alt="Login icon" />
                Login
              </button>
            </form>
          </section>
        </div>
      </div>
    );
  }
}

const mapStateToProps = state => ({
  loginError: state.auth.loginError,
  error: state.auth.error,
});

const mapDispatchToProps = dispatch => ({
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
